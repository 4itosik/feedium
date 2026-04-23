---
doc_kind: feature
doc_function: implementation-plan
purpose: "Пошаговый план реализации FT-003: CRUD API управления источниками (Source) через HTTP REST и gRPC с валидацией, пагинацией и персистентным хранением в PostgreSQL."
derived_from:
  - spec.md
status: active
delivery_status: done
---

# FT-003: Implementation Plan

## Решения по Open Questions spec

Приняты до начала шагов, чтобы план был однозначным.

| OQ | Вопрос | Решение | Обоснование |
|---|---|---|---|
| OQ-1 | processing_mode: spec AC противоречит BR-01 | **По BR-01 (canonical):** telegram_channel/rss/html → `self-contained`; telegram_group → `cumulative`. Spec AC — ошибка составления. | BR-01 — upstream для brief и spec; spec «Контекст» прямо ссылается на BR-01. |
| OQ-2 spec | `page_size` вне `[1, 500]` | **Клэмп** к границе. Значение < 1 → 1; > 500 → 500. | Мягче для клиентов; соответствует предложению в spec. |
| OQ-3 spec | Формат `page_token` | `base64(rfc3339nano_created_at + "\|" + id)` | Прозрачный, отладочный; не привязан к Ent-пагинатору; UUID v7 time-sortable подтверждает стабильность сортировки. |
| Proto location | spec: `api/feedium/source/v1/`; convention: `api/feedium/` | **Flat `api/feedium/`** — один файл `source.proto` рядом с `health.proto` | api-contracts.md — канонический upstream; версионирование через префикс метода, не через директорию. |
| Ent integration | FT-002 отложил Ent до первой таблицы | Ent client поверх существующего `*sql.DB` в `Data` через `entsql.OpenDB` | Зафиксировано в FT-002 plan шаг 6; `Data.DB` — единственный пул. |
| UUID vs BIGSERIAL | database.md: BIGSERIAL; spec: UUID v7 | **UUID v7** — осознанное отклонение от database.md | UUID v7 обоснован потребностью в стабильной курсорной пагинации. Зафиксировать как отклонение в комментарии к миграции. |
| Config десериализация | JSONB → Go: type-specific struct vs map | **Type-specific struct** + switch по type | Типобезопасность, раннее обнаружение ошибок. |
| optional username | Proto3: отличить «не задан» от «пустая строка» | **Без optional** — `string username = 2;` пустая строка = «не задан» | Проще; для MVP семантика «пустой строки = отсутствие username» достаточна. |
| UUID библиотека | Какую библиотеку для генерации UUID v7 | **google/uuid v1.6+** (уже в go.mod транзитивно) | `uuid.Must(uuid.NewV7())` для генерации. |

## Steps

### Шаг 1. Добавить зависимость Ent

- **Цель.** Подключить Ent ORM для работы с таблицей `sources`.
- **Действия.**
  1. Выполнить `go get entgo.io/ent@latest`.
  2. Выполнить `go get entgo.io/ent/dialect/sql`. Требуется для `entsql.OpenDB`.
  3. `go mod tidy`.
- **Зависимости.** Нет.
- **Результат.** `go.mod` содержит `entgo.io/ent` и `entgo.io/ent/dialect/sql`.
- **Как проверить.** `go build ./...` exit 0; `go mod tidy` не удаляет добавленные зависимости.

### Шаг 2. Инициализация ent/ и создание схемы Source

- **Цель.** Создать Ent-схему сущности Source, описывающую таблицу `sources`.
- **Действия.**
  1. Создать директорию `ent/`.
  2. Создать `ent/schema/source.go` со схемой `Source`:
     - Поля: `id` (тип UUID, генерируется на уровне Go, не БД), `type` (string, not null), `config` (JSON, not null — хранит type-specific конфиг как JSONB), `created_at` (timestamp, not null, default now), `updated_at` (timestamp, not null, default now, update now).
     - Индекс: составной `(type, created_at, id)`.
     - Имя таблицы явно не переназначается — Ent использует `sources` по умолчанию.
   3. UUID v7 генерируется в `Default` hook схемы: `source.ID = uuidgen()` при `Create`.
   4. Создать `ent/generate.go` с директивой генерации:
      ```go
      package ent

      //go:generate go run -mod=mod entgo.io/ent/cmd/ent generate ./schema
      ```
- **Зависимости.** Шаг 1.
- **Результат.** `ent/schema/source.go` описывает 5 полей и 1 индекс; `ent/generate.go` обеспечивает работу `go generate ./ent`.
- **Как проверить.** Файл существует; поля и индекс соответствуют spec (FR-1, NFR-1).

### Шаг 3. Генерация Ent-кода

- **Цель.** Получить Go-типы и builder для работы с таблицей `sources`.
- **Действия.**
  1. Запустить `go generate ./ent`.
  2. Сгенерированные файлы в `ent/` закоммитить.
- **Зависимости.** Шаг 2.
- **Результат.** Типы `ent.Source`, `ent.SourceClient`, `ent.Create` и др. доступны в проекте.
- **Как проверить.** `go build ./ent/...` exit 0; повторная генерация не создаёт diff.

### Шаг 4. Goose-миграция для таблицы `sources`

- **Цель.** Создать SQL-миграцию, создающую таблицу `sources` и индекс (NFR-1).
- **Действия.**
  1. Создать файл `migrations/YYYYMMDDHHMMSS_create_sources_table.sql` (время генерируется `goose create`).
  2. `-- +goose Up`:
     - `CREATE TABLE sources (id UUID PRIMARY KEY, type TEXT NOT NULL, config JSONB NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`.
     - `CREATE INDEX idx_sources_type_created_at_id ON sources(type, created_at, id)`.
  3. `-- +goose Down`:
     - `DROP TABLE IF EXISTS sources`.
  4. Запустить `make migrate` на локальной БД.
- **Зависимости.** Шаг 2 (схема должна быть согласована с миграцией).
- **Результат.** Миграция применена; таблица `sources` и индекс существуют в БД.
- **Как проверить.** `\d sources` в psql показывает все 5 колонок и индекс; повторный `make migrate` — exit 0 (goose пропускает).

### Шаг 5. Интеграция Ent client в Data

- **Цель.** Создать Ent client поверх существующего `*sql.DB` пула; обновить Wire-провайдеры.
- **Действия.**
  1. В `internal/data/data.go` расширить `Data`: добавить поле `Ent *ent.Client` рядом с `DB *sql.DB`.
  2. В `NewData`: после успешного `sql.Open` + `PingContext` создать Ent driver через `entsql.OpenDB(dialect.Postgres, db)` и `ent.NewClient(ent.Driver(drv))`.
  3. Обновить cleanup: добавить `d.Ent.Close()` перед `d.DB.Close()`.
- **Зависимости.** Шаги 1, 3.
- **Результат.** `Data.Ent` доступен всем repo в `data/`; пул соединений — единственный (`*sql.DB`).
- **Как проверить.** `go build ./internal/data/...` exit 0.

### Шаг 6. Доменные типы в biz/

- **Цель.** Определить типы, используемые валидацией, usecase и интерфейсом репозитория.
- **Действия.** Создать `internal/biz/source.go`:
  1. `type SourceType string` с 4 константами: `SourceTypeTelegramChannel`, `SourceTypeTelegramGroup`, `SourceTypeRSS`, `SourceTypeHTML`.
  2. `type ProcessingMode string` с константами: `ProcessingModeSelfContained`, `ProcessingModeCumulative`.
  3. Config-структуры для каждого типа: `TelegramChannelConfig { TgID int64; Username string }`, `TelegramGroupConfig { TgID int64; Username string }`, `RSSConfig { FeedURL string }`, `HTMLConfig { URL string }`. Каждая реализует интерфейс `SourceConfig` (маркерный, без методов).
  4. `type Source struct` со всеми полями из FR-7: `ID string`, `Type SourceType`, `ProcessingMode ProcessingMode`, `Config SourceConfig`, `CreatedAt time.Time`, `UpdatedAt time.Time`.
  5. Sentinel errors: `ErrSourceNotFound`, `ErrInvalidSourceType`, `ErrInvalidConfig`, `ErrTypeImmutable`.
  6. `type ListSourcesFilter struct { Type SourceType; PageSize int; PageToken string }`.
  7. `type ListSourcesResult struct { Items []Source; NextPageToken string }`.
- **Зависимости.** Нет.
- **Результат.** Все доменные типы определены; типы доступны для тестов (шаг 7).
- **Как проверить.** `go build ./internal/biz/...` exit 0.

### Шаг 7. TDD Red: тесты валидации config

- **Цель.** Написать падающие тесты на валидацию config для всех 4 типов (FR-6).
- **Действия.** Создать `internal/biz/source_test.go` (table-driven, AAA, goleak):
  - **Valid cases** (по одному на тип):
    - telegram_channel: `{tg_id: 123, username: "channel"}` → nil error.
    - telegram_group: `{tg_id: 456}` → nil error (username опционально).
    - rss: `{feed_url: "https://example.com/feed"}` → nil error.
    - html: `{url: "https://example.com"}` → nil error.
  - **Invalid cases:**
    - telegram_channel без tg_id (нулевое значение) → error, поле `tg_id` в диагностике.
    - telegram_channel с `tg_id == 0` → error.
    - telegram_group без tg_id → error.
    - rss без feed_url (пустая строка) → error, поле `feed_url`.
    - rss с `feed_url = "not-a-url"` → error, причина "invalid URL".
    - html без url → error.
    - html с `url = "not-a-url"` → error.
    - Пустой/nil config → error.
    - Неизвестный type → `ErrInvalidSourceType`.
  - Каждый тест вызывает `ValidateSourceConfig(sourceType, config)` — функция ещё не существует.
  - Все тесты оборачиваются в `defer goleak.VerifyNone(t, goleak.IgnoreCurrent())`.
- **Зависимости.** Шаг 6 (типы должны быть определены).
- **Результат.** Все тесты падают (Red) — функция `ValidateSourceConfig` не существует.
- **Как проверить.** `go test ./internal/biz/...` — все новые тесты FAIL; старые тесты (если есть) PASS.

### Шаг 8. TDD Green: реализация валидации config

- **Цель.** Реализовать `ValidateSourceConfig` так, чтобы все тесты из шага 7 прошли.
- **Действия.** В `internal/biz/source.go` добавить:
  1. Функция `ValidateSourceType(t SourceType) error` — проверяет, что type входит в множество из 4 значений; иначе `ErrInvalidSourceType`.
  2. Функция `ValidateSourceConfig(t SourceType, cfg SourceConfig) error` — switch по type:
     - telegram_channel / telegram_group: утверждает, что cfg — соответствующий тип; проверяет `TgID != 0`.
     - rss: проверяет `FeedURL` непустой и валидный URL (`net/url.ParseRequestURI`).
     - html: проверяет `URL` непустой и валидный URL.
     - Если type неизвестен — `ErrInvalidSourceType`.
  3. При ошибке возвращать `ErrInvalidConfig` с деталями (имя поля + причина) — можно через агрегацию в `fmt.Errorf("invalid config: %w", ...)`.
- **Зависимости.** Шаг 7.
- **Результат.** Все тесты из шага 7 проходят (Green).
- **Как проверить.** `go test ./internal/biz/... -run TestValidateSourceConfig` exit 0.

### Шаг 9. TDD Red: тесты маппинга type → processing_mode

- **Цель.** Написать падающие тесты на функцию `ProcessingModeForType`.
- **Действия.** В `internal/biz/source_test.go` добавить table-driven тесты:
  - `SourceTypeTelegramChannel` → значение по BR-01 (см. Open Questions OQ-1).
  - `SourceTypeTelegramGroup` → значение по BR-01.
  - `SourceTypeRSS` → значение по BR-01.
  - `SourceTypeHTML` → значение по BR-01.
  - Неизвестный type → error или паника (недостижимо при валидации).
- **Зависимости.** Шаг 6.
- **Результат.** Тесты падают — функция не существует.
- **Как проверить.** `go test ./internal/biz/... -run TestProcessingModeForType` — FAIL.

### Шаг 10. TDD Green: реализация маппинга type → processing_mode

- **Цель.** Реализовать `ProcessingModeForType` так, чтобы все тесты из шага 9 прошли.
- **Действия.** В `internal/biz/source.go` добавить:
  1. Чистая функция `ProcessingModeForType(t SourceType) ProcessingMode` — switch по type, возвращает соответствующий `ProcessingMode`.
  2. Значения — строго по BR-01 (PRD-001): telegram_channel/rss/html → `ProcessingModeSelfContained`; telegram_group → `ProcessingModeCumulative`. Spec AC содержит ошибку составления — решение OQ-1 зафиксировано в таблице решений выше.
- **Зависимости.** Шаг 9.
- **Результат.** Все тесты из шага 9 проходят.
- **Как проверить.** `go test ./internal/biz/... -run TestProcessingModeForType` exit 0.

### Шаг 11. Интерфейс SourceRepo в biz/

- **Цель.** Определить интерфейс репозитория, через который usecase работает с хранилищем (NFR-3, architecture.md).
- **Действия.** В `internal/biz/source.go` добавить:
  1. `type SourceRepo interface` с методами:
     - `Save(ctx context.Context, source Source) (Source, error)` — insert.
     - `Update(ctx context.Context, source Source) (Source, error)` — update по ID; если не найден → `ErrSourceNotFound`.
     - `Delete(ctx context.Context, id string) error` — hard delete; если не найден → `ErrSourceNotFound`.
     - `Get(ctx context.Context, id string) (Source, error)` — если не найден → `ErrSourceNotFound`.
     - `List(ctx context.Context, filter ListSourcesFilter) (ListSourcesResult, error)` — с пагинацией и фильтром по type.
- **Зависимости.** Шаг 6.
- **Результат.** Интерфейс `SourceRepo` определён в `biz/`; готов для мока и usecase.
- **Как проверить.** `go build ./internal/biz/...` exit 0.

### Шаг 12. TDD Red: тесты SourceUsecase

- **Цель.** Написать падающие тесты на все 5 операций usecase (FR-1..FR-5, INV-1, INV-4).
- **Действия.** Создать `internal/biz/source_usecase_test.go`. Использовать `mockgen` для генерации мока `SourceRepo`. Table-driven, AAA, goleak. Тест-кейсы:
  - **Create:**
    - Валидный RSS → repo.Save вызван, Source возвращён с `processing_mode`, `id`, `created_at`, `updated_at` заполнены.
    - Невалидный type → `ErrInvalidSourceType`; repo.Save НЕ вызван (INV-4).
    - Невалидный config (нет feed_url) → `ErrInvalidConfig`; repo.Save НЕ вызван.
  - **Update:**
    - Валидный update (тот же type, новый config) → repo.Update вызван, `updated_at` обновлён.
    - Попытка сменить type → `ErrTypeImmutable`; repo.Update НЕ вызван (INV-1).
    - Несуществующий id → repo.Get возвращает `ErrSourceNotFound` → usecase возвращает `ErrSourceNotFound`.
    - Невалидный config → `ErrInvalidConfig`; repo.Update НЕ вызван.
  - **Delete:**
    - Существующий id → repo.Delete вызван, nil error.
    - Несуществующий id → `ErrSourceNotFound`.
  - **Get:**
    - Существующий id → Source возвращён.
    - Несуществующий id → `ErrSourceNotFound`.
  - **List:**
    - Happy path с пагинацией: `page_size=2`, 3 источника → первая страница 2 элемента + `next_page_token`, вторая страница 1 элемент + пустой `next_page_token`.
    - Пустая таблица → `items: []`, `next_page_token: ""`.
    - Фильтр по type → возвращаются только источники этого типа.
    - `page_size` < 1 → клэмпится до 1.
    - `page_size` > 500 → клэмпится до 500.
    - Невалидный `page_token` → `ErrInvalidConfig` или специализированная ошибка парсинга.
  - Создать `//go:generate mockgen -source=source.go -destination=mock/source_mock.go -package=mock` в `biz/`.
  - Запустить `go generate ./internal/biz/...` для генерации мока.
- **Зависимости.** Шаги 8, 10, 11.
- **Результат.** Все тесты падают — `SourceUsecase` не существует.
- **Как проверить.** `go test ./internal/biz/...` — новые тесты FAIL.

### Шаг 13. TDD Green: реализация SourceUsecase

- **Цель.** Реализовать `SourceUsecase` так, чтобы все тесты из шага 12 прошли.
- **Действия.** В `internal/biz/source.go` добавить:
  1. `type SourceUsecase struct { repo SourceRepo }`.
  2. Конструктор `NewSourceUsecase(repo SourceRepo) *SourceUsecase`.
  3. `Create(ctx, sourceType, config) (Source, error)`:
     - Валидация type через `ValidateSourceType`.
     - Валидация config через `ValidateSourceConfig`.
     - Генерация UUID v7 → `source.ID`.
     - Вычисление `ProcessingMode` через `ProcessingModeForType`.
     - Установка `created_at`, `updated_at` в `time.Now()`.
     - Вызов `repo.Save`.
  4. `Update(ctx, id, sourceType, config) (Source, error)`:
     - Получить текущий source через `repo.Get(id)`.
     - Проверить, что `sourceType == existing.Type` (INV-1); иначе → `ErrTypeImmutable`.
     - Валидация config через `ValidateSourceConfig`.
     - Обновить `config`, `updated_at = time.Now()`.
     - Вызов `repo.Update`.
  5. `Delete(ctx, id) error`:
     - Вызов `repo.Delete(id)`.
  6. `Get(ctx, id) (Source, error)`:
     - Вызов `repo.Get(id)`.
   7. `List(ctx, filter) (ListSourcesResult, error)`:
      - Клэмп `filter.PageSize` в [1, 500].
      - Если `filter.Type` задан — валидация через `ValidateSourceType`.
      - Парсинг `filter.PageToken` (base64 → created_at|id).
      - Вызов `repo.List(ctx, filter)`.
   8. Создать `internal/biz/wire.go`:
      ```go
      package biz

      import "github.com/google/wire"

      var ProviderSet = wire.NewSet(NewSourceUsecase)
      ```
- **Зависимости.** Шаг 12.
- **Результат.** Все тесты из шага 12 проходят (Green). Бизнес-логика полностью покрыта unit-тестами. Wire-провайдер `biz.ProviderSet` доступен для сборки графа (шаг 22).
- **Как проверить.** `go test ./internal/biz/...` exit 0.

### Шаг 14. Рефакторинг biz/ (Red-Green-Refactor)

- **Цель.** Очистить код после Green-фазы: убрать дублирование, улучшить имена, убедиться в отсутствии мутаций входных аргументов.
- **Действия.**
  1. Проверить, что все функции в `biz/` чистые (не мутируют входные аргументы).
  2. Убедиться, что `internal/biz/` не импортирует `ent`, `http`, `sql`, proto.
  3. Запустить тесты — должны остаться зелёными.
- **Зависимости.** Шаг 13.
- **Результат.** Код `biz/` следует coding-style.md; тесты зелёные.
- **Как проверить.** `go test ./internal/biz/...` exit 0; `golangci-lint run ./internal/biz/...` exit 0.

### Шаг 15. Реализация SourceRepo в data/

- **Цель.** Реализовать интерфейс `biz.SourceRepo` через Ent.
- **Действия.** Создать `internal/data/source_repo.go`:
  1. `type sourceRepo struct { data *Data }`.
  2. Конструктор `NewSourceRepo(data *Data) *sourceRepo`.
  3. Compile-time assertion: `var _ biz.SourceRepo = (*sourceRepo)(nil)`.
  4. `Save`: `d.data.Ent.Source.Create().SetID(source.ID).SetType(string(source.Type)).SetConfig(source.Config).SetCreatedAt(source.CreatedAt).SetUpdatedAt(source.UpdatedAt).Save(ctx)`. Маппинг Ent → доменный `Source`.
  5. `Update`: `d.data.Ent.Source.UpdateOneID(id).SetConfig(source.Config).SetUpdatedAt(source.UpdatedAt).Save(ctx)`. Если `UpdateOneID` возвращает `NotFound` → `biz.ErrSourceNotFound`.
  6. `Delete`: `d.data.Ent.Source.DeleteOneID(id).Exec(ctx)`. NotFound → `biz.ErrSourceNotFound`.
  7. `Get`: `d.data.Ent.Source.Get(ctx, id)`. NotFound → `biz.ErrSourceNotFound`.
  8. `List`: курсорная пагинация через Ent query:
     - Декодировать `page_token` (base64 → created_at|id).
     - Query: `d.data.Ent.Source.Query().Where(source.TypeEQ(filter.Type))` (если фильтр задан) `.Where(source.CreatedAtGT(decodedCreatedAt))` или `CreatedAtEQ AND IDGT` для стабильности. `.Order(ent.Asc(source.FieldCreatedAt), ent.Asc(source.FieldID)).Limit(filter.PageSize + 1)`.
     - Если результатов > PageSize — сформировать `next_page_token` из последнего элемента (base64(created_at|id)).
     - Маппинг каждой Ent-записи в доменный `Source` (config десериализуется из JSONB в соответствующий тип по `source.Type`).
  9. Config сериализация/десериализация: в `Save`/`Update` — `json.Marshal(config)`; в `Get`/`List` — switch по type, `json.Unmarshal` в нужную структуру.
  10. Обновить `internal/data/wire.go`: добавить `NewSourceRepo` и `wire.Bind(new(biz.SourceRepo), new(*sourceRepo))` в `ProviderSet`.
- **Зависимости.** Шаги 5, 11.
- **Результат.** `sourceRepo` реализует `biz.SourceRepo`; Wire-провайдеры обновлены.
- **Как проверить.** `go build ./internal/data/...` exit 0; compile-time assertion компилируется.

### Шаг 16. Тесты data/ с testcontainers

- **Цель.** Интеграционные тесты репозитория — CRUD через реальную PostgreSQL (NFR-5, testing-policy.md).
- **Действия.** Создать `internal/data/source_repo_test.go`:
  1. `setupTestDB(t)` — testcontainers PostgreSQL 18.3-alpine, goose миграции, Ent client. Возвращает `*ent.Client` и cleanup.
  2. Тесты (AAA, table-driven):
     - **Save** — создаёт source каждого из 4 типов; проверяет, что все поля возвращены корректно, config маппится туда-обратно.
     - **Get** — после Save вызывает Get; проверяет совпадение всех полей.
     - **Get not found** — случайный UUID → `biz.ErrSourceNotFound`.
     - **Update** — создаёт RSS source, обновляет feed_url; проверяет новый config и updated_at > created_at.
     - **Update not found** — случайный UUID → `biz.ErrSourceNotFound`.
     - **Delete** — создаёт, удаляет, повторный Get → `ErrSourceNotFound`.
     - **Delete not found** — случайный UUID → `ErrSourceNotFound`.
     - **List empty** → `items: []`, `next_page_token: ""`.
     - **List pagination** — создаёт 3 source с page_size=2; первая страница 2 элемента + token; вторая 1 элемент + пустой token; обход всех страниц даёт 3 уникальных source.
     - **List filter by type** — создаёт mix типов; фильтр по RSS → только RSS.
     - **List filter unknown type** — пустой результат (type валидируется выше, но на уровне repo просто не найдёт строк).
     - **Config roundtrip** — для каждого типа: создать с config, прочитать, проверить побайтовое совпадение (с учётом опциональных полей).
  3. Все тесты оборачиваются в `defer goleak.VerifyNone(t, goleak.IgnoreCurrent())`.
  4. Тесты запускаются как `TestIntegration_*`.
- **Зависимости.** Шаг 15.
- **Результат.** `go test ./internal/data/... -run Integration` exit 0 при запущенном Docker.
- **Как проверить.** Запустить `go test ./internal/data/... -run Integration -v` — все тесты PASS.

### Шаг 17. Proto-контракт: error_reason.proto

- **Цель.** Определить error reasons для Source Management как plain enum (NFR-4).
- **Действия.** Создать `api/feedium/error_reason.proto`:
   1. `syntax = "proto3"`, `package feedium`, `option go_package = "feedium/api/feedium;feedium"`.
   2. `enum ErrorReason { ERROR_REASON_UNSPECIFIED = 0; ERROR_REASON_SOURCE_NOT_FOUND = 1; ERROR_REASON_SOURCE_INVALID_TYPE = 2; ERROR_REASON_SOURCE_INVALID_CONFIG = 3; ERROR_REASON_SOURCE_TYPE_IMMUTABLE = 4; }`.
   3. Kratos error extensions (`errors/errors.proto`, `(errors.default_code)`) **не используются** — это plain enum. Service-слой (шаг 20) маппит доменные ошибки в `status.Error(codes.XXX, ...)` с текстом из enum-значений. Не нужен ни `third_party/errors/errors.proto`, ни `protoc-gen-go-errors`.
- **Зависимости.** Нет (можно параллельно с шагами 6-14).
- **Результат.** Error reasons определены в proto как plain enum; не требуют сторонних proto-зависимостей.
- **Как проверить.** Файл существует; все 4 error reasons из spec присутствуют (SOURCE_NOT_FOUND, SOURCE_INVALID_TYPE, SOURCE_INVALID_CONFIG, SOURCE_TYPE_IMMUTABLE).

### Шаг 18. Proto-контракт: source.proto

- **Цель.** Зафиксировать HTTP + gRPC контракт Source Management (FR-1..FR-5, FR-7).
- **Действия.** Создать `api/feedium/source.proto`:
  1. `syntax = "proto3"`, `package feedium`, `option go_package = "feedium/api/feedium;feedium"`.
  2. `import "google/api/annotations.proto"`.
  3. Enums:
     - `SourceType`: `SOURCE_TYPE_TELEGRAM_CHANNEL = 0; SOURCE_TYPE_TELEGRAM_GROUP = 1; SOURCE_TYPE_RSS = 2; SOURCE_TYPE_HTML = 3;` (proto3: первое значение = 0 — default; UPPER_SNAKE_CASE по api-contracts.md).
     - `ProcessingMode`: `PROCESSING_MODE_SELF_CONTAINED = 0; PROCESSING_MODE_CUMULATIVE = 1;`.
  4. Config messages (без V-префикса — shared):
     - `TelegramChannelConfig { int64 tg_id = 1; string username = 2; }`.
     - `TelegramGroupConfig { int64 tg_id = 1; string username = 2; }`.
     - `RSSConfig { string feed_url = 1; }`.
     - `HTMLConfig { string url = 1; }`.
     - `SourceConfig { oneof config { TelegramChannelConfig telegram_channel = 1; TelegramGroupConfig telegram_group = 2; RSSConfig rss = 3; HTMLConfig html = 4; } }`.
  5. Shared message `Source`: `string id = 1; SourceType type = 2; ProcessingMode processing_mode = 3; SourceConfig config = 4; google.protobuf.Timestamp created_at = 5; google.protobuf.Timestamp updated_at = 6;`.
  6. Service `SourceService`:
     - `V1CreateSource(V1CreateSourceRequest) returns (V1CreateSourceResponse)` — HTTP `POST /v1/sources`.
     - `V1UpdateSource(V1UpdateSourceRequest) returns (V1UpdateSourceResponse)` — HTTP `PUT /v1/sources/{id}`.
     - `V1DeleteSource(V1DeleteSourceRequest) returns (V1DeleteSourceResponse)` — HTTP `DELETE /v1/sources/{id}`.
     - `V1ListSources(V1ListSourcesRequest) returns (V1ListSourcesResponse)` — HTTP `GET /v1/sources`.
     - `V1GetSource(V1GetSourceRequest) returns (V1GetSourceResponse)` — HTTP `GET /v1/sources/{id}`.
  7. Request/Response messages:
     - `V1CreateSourceRequest { SourceType type = 1; SourceConfig config = 2; }` → `V1CreateSourceResponse { Source source = 1; }`.
     - `V1UpdateSourceRequest { string id = 1; SourceType type = 2; SourceConfig config = 3; }` → `V1UpdateSourceResponse { Source source = 1; }`.
     - `V1DeleteSourceRequest { string id = 1; }` → `V1DeleteSourceResponse {}` (пустой message, не `google.protobuf.Empty`).
     - `V1ListSourcesRequest { int32 page_size = 1; string page_token = 2; optional SourceType type = 3; }` → `V1ListSourcesResponse { repeated Source items = 1; string next_page_token = 2; }`.
     - `V1GetSourceRequest { string id = 1; }` → `V1GetSourceResponse { Source source = 1; }`.
- **Зависимости.** Нет (можно параллельно с шагами 6-14).
- **Результат.** Полный proto-контракт CRUD с HTTP-аннотациями.
- **Как проверить.** Файл существует; 5 RPC методов; HTTP-пути соответствуют REST-конвенции; `optional` на `type` в ListSources; пустой ответ Delete не использует `google.protobuf.Empty`.

### Шаг 19. Генерация proto-кода

- **Цель.** Получить Go-стабы для HTTP и gRPC.
- **Действия.**
   1. Запустить `make proto`.
   2. Сгенерированные файлы в `api/feedium/` закоммитить.
- **Зависимости.** Шаги 17, 18.
- **Результат.** Типы `SourceServiceServer`, `SourceServiceHTTPServer`, все messages доступны в `api/feedium`.
- **Как проверить.** `go build ./api/feedium/...` exit 0; повторный `make proto` не создаёт diff.

### Шаг 20. Service-слой: SourceService

- **Цель.** Тонкий адаптер proto DTO ↔ доменные объекты ↔ biz (NFR-3).
- **Действия.** Создать `internal/service/source/`:
  1. `source.go`:
     - `type SourceService struct { feediumv1.UnimplementedSourceServiceServer; uc *biz.SourceUsecase }`.
     - Конструктор `NewSourceService(uc *biz.SourceUsecase) *SourceService`.
     - Каждый RPC-метод:
       - Маппинг proto → domain (type enum → `biz.SourceType`; config oneof → соответствующий `biz.*Config` struct).
       - Вызов `biz.SourceUsecase`.
       - Маппинг domain → proto (включая `ProcessingMode`).
        - Конвертация доменных ошибок в gRPC status errors с текстом из enum-значений `ErrorReason`:
          - `biz.ErrSourceNotFound` → `status.Error(codes.NotFound, feediumv1.ErrorReason_NAME.String())` где `ErrorReason_NAME = ERROR_REASON_SOURCE_NOT_FOUND`.
          - `biz.ErrInvalidSourceType` → `status.Error(codes.InvalidArgument, feediumv1.ErrorReason_ERROR_REASON_SOURCE_INVALID_TYPE.String())`.
          - `biz.ErrInvalidConfig` → `status.Error(codes.InvalidArgument, feediumv1.ErrorReason_ERROR_REASON_SOURCE_INVALID_CONFIG.String())`.
          - `biz.ErrTypeImmutable` → `status.Error(codes.InvalidArgument, feediumv1.ErrorReason_ERROR_REASON_SOURCE_TYPE_IMMUTABLE.String())`.
     - В `V1UpdateSource` — проверка, что `request.Type` соответствует `oneof` variant в `request.Config`. Несоответствие → `INVALID_ARGUMENT`.
  2. `wire.go`: `var ProviderSet = wire.NewSet(NewSourceService)`.
  3. `doc.go` — ссылка на coding-style.md §service/.
- **Зависимости.** Шаги 13, 19.
- **Результат.** `SourceService` реализует `feediumv1.SourceServiceServer`; маппинг proto ↔ domain; ошибки конвертированы в status errors.
- **Как проверить.** `go build ./internal/service/source/...` exit 0.

### Шаг 21. Тесты service/

- **Цель.** Покрыть маппинг proto ↔ domain и конверсию ошибок (testing-policy.md: tests-after, mock biz usecase).
- **Действия.** Создать `internal/service/source/source_test.go`:
  1. Сгенерировать мок для `biz.SourceUsecase` (через mockgen — `//go:generate` директива в `internal/biz/source.go` или отдельный файл).
  2. Тесты (AAA, table-driven):
     - **V1CreateSource happy path** — мок uc.Create возвращает валидный Source; проверка: response содержит source с заполненными полями.
     - **V1CreateSource invalid type** — uc.Create возвращает `biz.ErrInvalidSourceType`; response — `INVALID_ARGUMENT` с `SOURCE_INVALID_TYPE`.
     - **V1CreateSource invalid config** — uc.Create возвращает `biz.ErrInvalidConfig`; response — `INVALID_ARGUMENT` с `SOURCE_INVALID_CONFIG`.
     - **V1UpdateSource happy path** — проверка `updated_at` в response.
     - **V1UpdateSource type immutable** — uc.Update возвращает `biz.ErrTypeImmutable`; response — `INVALID_ARGUMENT` с `SOURCE_TYPE_IMMUTABLE`.
     - **V1UpdateSource not found** — `NOT_FOUND` с `SOURCE_NOT_FOUND`.
     - **V1DeleteSource happy path** — пустой response.
     - **V1DeleteSource not found** — `NOT_FOUND`.
     - **V1GetSource happy path** — все поля Source маппятся корректно.
     - **V1GetSource not found** — `NOT_FOUND`.
     - **V1ListSources happy path** — items, next_page_token маппятся корректно.
     - **V1ListSources empty** — пустой items, пустой next_page_token.
     - **Config oneof маппинг** — по одному кейсу на каждый тип (telegram_channel с username и без; telegram_group; rss; html). Проверить, что proto oneof variant соответствует domain типу.
  3. `goleak.VerifyNone` для каждого теста.
- **Зависимости.** Шаг 20.
- **Результат.** `go test ./internal/service/source/...` exit 0.
- **Как проверить.** Тесты проходят; маппинг каждого из 4 типов покрыт.

### Шаг 22. Wire-граф и регистрация в серверах

- **Цель.** Подключить SourceService к HTTP и gRPC серверам; обновить Wire-граф.
- **Действия.**
  1. В `internal/server/grpc.go`: добавить параметр `srcSvc *sourceservice.SourceService`; зарегистрировать `feediumv1.RegisterSourceServiceServer(srv, srcSvc)`.
  2. В `internal/server/http.go`: добавить параметр `srcSvc *sourceservice.SourceService`; зарегистрировать `feediumv1.RegisterSourceServiceHTTPServer(srv, srcSvc)`.
   3. В `cmd/feedium/wire.go`: добавить `sourcebiz.ProviderSet` и `sourceservice.ProviderSet` в `wire.Build`:
      ```go
      wire.Build(
          newServerFromBootstrap,
          newDataFromBootstrap,
          server.ProviderSet,
          data.ProviderSet,
          healthservice.ProviderSet,
          sourcebiz.ProviderSet,      // NewSourceUsecase + SourceRepo binding
          sourceservice.ProviderSet,  // NewSourceService
          newApp,
      )
      ```
  4. Запустить `make wire`; закоммитить `wire_gen.go`.
- **Зависимости.** Шаги 15, 20, 19.
- **Результат.** SourceService доступен через HTTP и gRPC; Wire-граф собирается.
- **Как проверить.** `go build ./...` exit 0; `make wire` не создаёт diff при повторном запуске.

### Шаг 23. Lint-правила на архитектурные границы

- **Цель.** Статическая гарантия: `service/` не зависит от `data/`; `biz/` не зависит от `ent`, `http`, `sql`, `proto`.
- **Действия.**
  1. В `.golangci.yml` добавить/расширить `depguard`-правила:
     - `internal/biz/` → запретить `entgo.io/ent`, `net/http`, `database/sql`, `google.golang.org/protobuf`, `google.golang.org/grpc`.
     - `internal/service/` → запретить `entgo.io/ent`, `database/sql`, `github.com/4itosik/feedium/internal/data`.
  2. `make lint` — exit 0.
- **Зависимости.** Шаги 14, 20.
- **Результат.** Нарушение границ ловится линтером.
- **Как проверить.** `make lint` exit 0; добавление `import "github.com/4itosik/feedium/internal/data"` в service/ → ошибка линтера.

### Шаг 24. Финальная верификация

- **Цель.** Закрыть все Acceptance Criteria spec.md.
- **Действия.** См. раздел Verification ниже.
- **Зависимости.** Все предыдущие шаги.
- **Результат.** Все AC закрыты; `make lint && make build && make test` зелёные.
- **Как проверить.** См. раздел Verification.

## Edge Cases

### Валидация

1. **Неизвестный `type`** (строка не из 4 допустимых) → `INVALID_ARGUMENT: SOURCE_INVALID_TYPE`. БД не затронута.
2. **Create telegram_channel без `tg_id`** (только `username`) → `INVALID_ARGUMENT: SOURCE_INVALID_CONFIG`, диагностика указывает `tg_id`.
3. **Create telegram_channel с `tg_id == 0`** → то же, что п.2.
4. **Create RSS с пустым `feed_url`** → `INVALID_ARGUMENT: SOURCE_INVALID_CONFIG`.
5. **Create RSS с `feed_url = "not-a-url"`** → `INVALID_ARGUMENT: SOURCE_INVALID_CONFIG`, причина "invalid URL".
6. **Create HTML с невалидным `url`** → аналогично п.5.
7. **Create с пустым config (oneof не установлен)** → `INVALID_ARGUMENT: SOURCE_INVALID_CONFIG`.

### Неизменяемость type

8. **Update с другим `type`** → `INVALID_ARGUMENT: SOURCE_TYPE_IMMUTABLE`. Текущая запись в БД не изменена (INV-1).

### NotFound

9. **Update несуществующего `id`** → `NOT_FOUND: SOURCE_NOT_FOUND`.
10. **Delete несуществующего `id`** → `NOT_FOUND: SOURCE_NOT_FOUND`.
11. **Get несуществующего `id`** → `NOT_FOUND: SOURCE_NOT_FOUND`.

### Пагинация

12. **Пустая таблица** → `items: []`, `next_page_token: ""`.
13. **`page_size = 0`** → клэмпится до 1.
14. **`page_size = -1`** → клэмпится до 1.
15. **`page_size = 1000`** → клэмпится до 500.
16. **Невалидный `page_token`** (не base64, неверный формат) → `INVALID_ARGUMENT`.
17. **`page_size = 1`, N>1 источников** — обход всех страниц через `next_page_token` даёт каждый источник ровно один раз; последняя страница имеет `next_page_token = ""`.

### Фильтрация

18. **Фильтр по неизвестному `type`** → `INVALID_ARGUMENT: SOURCE_INVALID_TYPE` (валидация типа до запроса к БД).
19. **Фильтр по типу, для которого нет источников** → `items: []`, `next_page_token: ""`.

### Конкурентность

20. **Два одновременных Update одного `id`** → last-write-wins; промежуточное состояние теряется — допустимо по spec (INV не требует optimistic locking).

### Config roundtrip

21. **Telegram config с `username`** — создаётся и читается с сохранением `username`.
22. **Telegram config без `username`** — создаётся и читается; `username` — пустая строка (означает «не задан»).
23. **RSS config с URL, содержащим query params** — roundtrip без искажения.

### Proto oneof consistency

24. **CreateRequest с `type=RSS`, но oneof variant = `telegram_channel`** → `INVALID_ARGUMENT` (type не соответствует oneof variant).

### Граничные значения

25. **`feed_url` с допустимым URL не-RSS-формата** (например `https://example.com`) — проходит валидацию URL (spec не требует проверки RSS-формата).
26. **Очень длинный `username`** — валидация не ограничивает длину (spec не задаёт ограничений).
27. **`tg_id` = максимальное int64 значение** — проходит валидацию (только `!= 0`).

## Verification

1. **Lint:** `make lint` exit 0 (включая depguard-правила из шага 23).
2. **Build:** `make build` exit 0.
3. **Unit-тесты:** `go test ./internal/biz/... -v` exit 0. Все тесты валидации, маппинга и usecase проходят.
4. **Service-тесты:** `go test ./internal/service/source/... -v` exit 0. Маппинг proto ↔ domain покрыт.
5. **Integration-тесты:** `go test ./internal/data/... -run Integration -v` exit 0 при запущенном Docker.
6. **Migration:** `make migrate` на чистой БД exit 0. `\d sources` в psql показывает 5 колонок и индекс.
7. **HTTP Create:** Запустить `make run`. `curl -X POST http://<addr>/v1/sources -H 'Content-Type: application/json' -d '{"type":"SOURCE_TYPE_RSS","config":{"rss":{"feed_url":"https://example.com/feed"}}}'` → `200`, ответ содержит `Source` с `id`, `processing_mode`, `created_at`, `updated_at`.
8. **HTTP List:** `curl http://<addr>/v1/sources` → созданный source присутствует.
9. **HTTP Update:** `curl -X PUT http://<addr>/v1/sources/<id> -H 'Content-Type: application/json' -d '{"type":"SOURCE_TYPE_RSS","config":{"rss":{"feed_url":"https://new.example.com/feed"}}}'` → `200`, `updated_at` обновлён, `feed_url` изменён.
10. **HTTP Update с другим type:** `curl -X PUT http://<addr>/v1/sources/<id> -d '{"type":"SOURCE_TYPE_HTML",...}'` → `INVALID_ARGUMENT: SOURCE_TYPE_IMMUTABLE`.
11. **HTTP Delete:** `curl -X DELETE http://<addr>/v1/sources/<id>` → `200`, пустое тело. Повторный `GET` → `NOT_FOUND`.
12. **HTTP List empty:** После удаления всех → `{"items":[],"next_page_token":""}`.
13. **HTTP List pagination:** Создать 3 источника, `GET /v1/sources?page_size=2` → 2 элемента + `next_page_token`. `GET /v1/sources?page_size=2&page_token=<token>` → 1 элемент + пустой `next_page_token`.
14. **HTTP List filter:** `GET /v1/sources?type=SOURCE_TYPE_RSS` → только RSS.
15. **HTTP validation errors:**
    - Неизвестный type → `INVALID_ARGUMENT: SOURCE_INVALID_TYPE`.
    - RSS без feed_url → `INVALID_ARGUMENT: SOURCE_INVALID_CONFIG`.
    - telegram_channel без tg_id → `INVALID_ARGUMENT: SOURCE_INVALID_CONFIG`.
    - RSS с невалидным URL → `INVALID_ARGUMENT: SOURCE_INVALID_CONFIG`.
16. **gRPC:** `grpcurl -plaintext <grpc_addr> feedium.SourceService/V1ListSources` → `items` с созданными источниками.
17. **Processing mode:** Создать source каждого типа; проверить `processing_mode` в ответе — соответствует BR-01.
18. **CRUD без рестарта:** Все операции (п.7–15) выполнены на одном запущенном процессе без рестарта/редеплоя.

## Open Questions

Нет. Все ранее открытые вопросы разрешены — см. таблицу «Решения по Open Questions spec» в начале документа.

Специфичные注意事项 для реализации:

1. **Spec AC опечатка** — AC пункт 9 говорит «processing_mode для telegram_channel/rss/html = cumulative». Это ошибка. Реализация следует BR-01: telegram_channel/rss/html = `self-contained`, telegram_group = `cumulative`. При приёмке — сверяться с BR-01, а не с этим пунктом AC.

2. **UUID v7 — отклонение от database.md** — convention говорит «UUID: не используем как PK», но spec требует UUID v7 для стабильной пагинации. В комментарии к миграции `create_sources_table.sql` зафиксировать: «UUID v7 PK — осознанное отклонение от database.md convention; обосновано стабильной курсорной пагинацией». При появлении ADR-процесса — оформить формально.

3. **username = пустая строка** — без `optional` в proto невозможно отличить «не задан» от «пустая строка». На уровне валидации `biz/` — `username` не валидируется (опциональное поле); на уровне storage — пустая строка пишется как есть. Потребители config должны интерпретировать `""` как «username не задан».
