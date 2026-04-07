# Implement Plan: Source Management API (feature 003)

## Reference

- Spec: `memory-bank/features/003/spec.md`
- Brief: `memory-bank/features/003/brief.md`

## Цель

Реализовать полный CRUD + List для `Source` через Connect-go с хранением в PostgreSQL (GORM),
строгой валидацией по типу источника, корректным маппингом ошибок в Connect-коды,
unit-тестами сервисного слоя на `go.uber.org/mock` и обязательными quality checks.

## Preflight и зависимости

1. Проверить, что требования зафиксированы в `memory-bank/features/003/spec.md` и не конфликтуют с архитектурными инвариантами.
2. Проверить наличие/порядок миграций в `migrations/`:
- если миграций ещё нет: создать `001_create_sources.sql`;
- если уже есть миграции: создать следующий номер (`NNN_create_sources.sql`), не редактируя существующие.
3. Новые библиотеки не добавлять без согласования; в рамках фичи использовать текущий стек (`Connect`, `GORM`, `Postgres driver`, `uuid`, `goose`, `mockgen`).

## Порядок реализации

### 1. API-контракт и генерация кода

Изменения:
- добавить `api/source/v1/source.proto` с `SourceType`, `SourceConfig oneof`, `Source`, `SourceService` и всеми request/response из spec;
- сгенерировать:
  - `api/source/v1/source.pb.go`
  - `api/source/v1/sourcev1connect/source.connect.go`;
- зафиксировать воспроизводимую генерацию (либо `go:generate`, либо явная команда в README/комментарии рядом с proto).

Проверка шага:
- генерация воспроизводится одной командой без ручного редактирования generated-файлов;
- `go test ./...` проходит (как минимум компиляция всех пакетов);
- generated-файлы не редактируются вручную.

Зависимости:
- этот шаг должен быть завершён до реализации Connect-хендлера и bootstrap wiring.

### 2. Доменный слой `internal/app/source`

Изменения:
- добавить доменную модель `Source`, enum-like `Type`, `ListFilter`, `ErrNotFound`, интерфейс `Repository`;
- реализовать `Service` (`Create`, `Get`, `Update`, `Delete`, `List`) с логикой:
  - `Create`: validate -> repo.Create -> return source;
  - `Get`: repo.GetByID;
  - `Update`: validate -> repo.GetByID (проверка существования) -> repo.Update -> return source;
  - `Delete`: repo.Delete;
  - `List`: нормализация пагинации (`page_size`: default 50, max 100; `page`: default 1).

Правила валидации:
- общая (для `Create` и `Update`):
  - `name`: обязательный, `TrimSpace`, длина 1..255;
  - `url`: обязательный, валидный `http|https`;
  - `type`: не `UNSPECIFIED`;
  - `config`: обязателен и соответствует `type`;
- типоспецифичная:
  - `telegram_channel.channel_id` — непустой после `TrimSpace`;
  - `telegram_group.group_id` — непустой после `TrimSpace`;
  - `rss.feed_url` — валидный `http|https`;
  - `web_scraping.selector` — непустой после `TrimSpace`.

Правила для `List`:
- `type_filter=UNSPECIFIED` трактуется как "без фильтра", ошибкой не считается;
- любое иное неизвестное значение enum на transport-уровне -> `CodeInvalidArgument`.

Проверка шага:
- unit-тесты сервиса (см. шаг 5) покрывают позитивные и негативные кейсы валидации;
- сервис не маппит transport-коды, а возвращает доменные/репозиторные ошибки наверх.

Зависимости:
- используется адаптерами Postgres и Connect, поэтому должен быть завершён до них.

### 3. Postgres-слой: `platform` + `repository`

Изменения:
- добавить `internal/platform/postgres/postgres.go` с `Open(dsn)`;
- добавить `internal/app/source/adapters/postgres/repository.go`:
  - локальная GORM-модель только внутри адаптера;
  - `config` хранится как JSONB;
  - `GetByID` и `Delete` возвращают `source.ErrNotFound`, когда запись отсутствует;
  - `Update` возвращает `source.ErrNotFound`, если обновлять нечего (`RowsAffected == 0`);
  - `List` реализует фильтр по `type`, `LIMIT/OFFSET`, отдельный `COUNT(*)`.
- добавить SQL-миграцию `migrations/NNN_create_sources.sql` в goose-формате из spec.

Проверка шага:
- `go test ./...` проходит;
- миграция соответствует spec: таблица `sources`, индекс `idx_sources_type`, `Down` удаляет таблицу;
- отсутствуют изменения старых миграций.

Зависимости:
- зависит от шага 2 (доменный интерфейс `Repository`).

### 4. Connect-хендлер и bootstrap wiring

Изменения:
- добавить `internal/app/source/adapters/connect/handler.go`, реализующий `sourcev1connect.SourceServiceHandler`;
- реализовать двусторонний mapping:
  - proto enum/oneof <-> domain `Type` + `Config map[string]any`;
  - UUID parse/format;
  - timestamp conversion;
- реализовать mapping ошибок:
  - validation/invalid UUID -> `CodeInvalidArgument`;
  - `ErrNotFound` -> `CodeNotFound`;
  - прочее -> `CodeInternal` (детали только в логах);
- обновить `internal/bootstrap`:
  - читать `DATABASE_URL` и возвращать ошибку при пустом значении;
  - инициализировать `DB -> source repo -> source service -> source handler`;
  - зарегистрировать source routes на текущем mux;
  - `GET /healthz` и сигнатуру `Run(ctx, log)` не менять.

Проверка шага:
- целевые handler-тесты на mapping ошибок проходят;
- `go test -run TestHealthHandler ./internal/bootstrap` проходит;
- `go test ./...` проходит.

Зависимости:
- зависит от шагов 1, 2 и 3.

### 5. Тесты и quality gates

Обязательные тесты:
- генерация mock-репозитория через `mockgen` (например, `internal/app/source/mocks/repository_mock.go`), без handwritten mocks;
- сервис (`internal/app/source/service_test.go`) с моками из `go.uber.org/mock` (`mockgen`), без handwritten mocks:
  - `Create`: success для 4 типов; invalid name/url/type/config mismatch/per-type config;
  - `Get`: success; not found; internal repo error passthrough;
  - `Update`: success; not found на existence-check; validation failure; not found при `repo.Update` (гонка после existence-check);
  - `Delete`: success; not found;
  - `List`: нормализация default pagination; `page_size` cap 100; type filter passthrough; `UNSPECIFIED` = без фильтра;
- handler (минимально table-driven):
  - invalid UUID -> `CodeInvalidArgument`;
  - invalid enum значение (кроме `UNSPECIFIED` для `List`) -> `CodeInvalidArgument`;
  - not found -> `CodeNotFound`;
  - internal -> `CodeInternal`.

Обязательные команды проверки:
- `go test ./...`
- `go test -run TestHealthHandler ./internal/bootstrap`
- `go vet ./...`
- `golangci-lint run ./... -c .golangci.yml`
- `go test -cover ./...` (контроль покрытия как quality signal)

Критерий завершения:
- все acceptance-критерии из `spec.md` закрыты;
- покрытие новой функциональности (`internal/app/source` + transport mapping) >= 80%.

## Edge Cases (должны быть явно покрыты реализацией или тестами)

- `name` из пробелов должен отвергаться.
- `channel_id/group_id/selector` из пробелов должны отвергаться.
- `url` и `rss.feed_url` с не-`http|https` схемой должны отвергаться.
- `type=UNSPECIFIED` должен отвергаться.
- `config` отсутствует или не соответствует `type` (oneof mismatch) -> ошибка валидации.
- `List`: `page<=0` и `page_size<=0` должны нормализоваться к дефолтам.
- `List`: `page_size>100` должен ограничиваться до 100.
- `List`: `type_filter=UNSPECIFIED` означает отсутствие фильтра.
- `Delete/Get/Update` с невалидным UUID на transport-уровне -> `CodeInvalidArgument`.
- `Delete/Get/Update` c валидным UUID, но без записи -> `CodeNotFound`.
- `Update`: запись удалена между `GetByID` и `Update` -> `CodeNotFound`.
- внутренние ошибки репозитория не утекут клиенту как детали, но логируются на сервере.

## Непротиворечивые правила по уровням

- Валидация UUID относится к transport-уровню (handler), не к сервису.
- Валидация enum-значений proto относится к transport-уровню (handler).
- Сервис не знает про Connect-коды и не выполняет transport mapping.
- Репозиторий не знает про transport и возвращает доменные ошибки (`ErrNotFound`/internal error).
- `UpdateSource` использует full-replace semantics (partial update не поддерживается).
