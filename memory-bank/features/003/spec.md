# Source Management API

## Цель

Предоставить CRUD API для управления источниками данных, чтобы добавление, изменение и удаление источников не требовало изменения кода и деплоя.

---

## Reference

Brief: memory-bank/features/003/brief.md

---

## Scope

### Входит:
- Proto-файл с определением `SourceService` и сообщений
- Кодогенерация Connect-go из proto
- Модуль `source` в `internal/app/source/`
- Интерфейс репозитория в `internal/app/source/`
- Сервис (use-case) в `internal/app/source/`
- GORM-реализация репозитория в `internal/app/source/adapters/postgres/`
- Connect-go хендлер в `internal/app/source/adapters/connect/`
- Регистрация хендлера и зависимостей в `internal/bootstrap/`
- Подключение к PostgreSQL через GORM в `internal/platform/postgres/`
- SQL-миграция через goose для таблицы `sources`
- Валидация входных данных по типу источника
- Unit-тесты сервисного слоя с мокированием репозитория

### НЕ входит:
- Аутентификация и авторизация
- Пользовательский интерфейс
- Выполнение сбора данных из источников
- Проверка доступности источников
- Integration и e2e тесты
- Soft delete

---

## Требования

### 1. Proto-файл

Файл: `api/source/v1/source.proto`

Пакет: `source.v1`, go_package: `feedium/api/source/v1;sourcev1`

**Enum SourceType:**
```
SOURCE_TYPE_UNSPECIFIED = 0
SOURCE_TYPE_TELEGRAM_CHANNEL = 1
SOURCE_TYPE_TELEGRAM_GROUP = 2
SOURCE_TYPE_RSS = 3
SOURCE_TYPE_WEB_SCRAPING = 4
```

**Config через oneof:**
```protobuf
message TelegramChannelConfig {
  string channel_id = 1;  // числовой ID (-100...) или @username
}

message TelegramGroupConfig {
  string group_id = 1;    // числовой ID (-...) или @username
}

message RssConfig {
  string feed_url = 1;    // URL фида, может отличаться от основного url
}

message WebScrapingConfig {
  string selector = 1;    // CSS-селектор для извлечения контента
}

message SourceConfig {
  oneof config {
    TelegramChannelConfig telegram_channel = 1;
    TelegramGroupConfig telegram_group = 2;
    RssConfig rss = 3;
    WebScrapingConfig web_scraping = 4;
  }
}
```

**Message Source:**
- `string id` — UUID
- `SourceType type`
- `string name`
- `string url`
- `SourceConfig config`
- `google.protobuf.Timestamp created_at`
- `google.protobuf.Timestamp updated_at`

**RPCs (service SourceService):**
- `CreateSource(CreateSourceRequest) returns (CreateSourceResponse)`
- `GetSource(GetSourceRequest) returns (GetSourceResponse)`
- `UpdateSource(UpdateSourceRequest) returns (UpdateSourceResponse)`
- `DeleteSource(DeleteSourceRequest) returns (DeleteSourceResponse)`
- `ListSources(ListSourcesRequest) returns (ListSourcesResponse)`

**Request/Response messages:**

| RPC | Request fields | Response fields |
|---|---|---|
| CreateSource | `SourceType type`, `string name`, `string url`, `SourceConfig config` | `Source source` |
| GetSource | `string id` | `Source source` |
| UpdateSource | `string id`, `SourceType type`, `string name`, `string url`, `SourceConfig config` | `Source source` |
| DeleteSource | `string id` | (пустой) |
| ListSources | `int32 page_size`, `int32 page`, `SourceType type_filter` | `repeated Source sources`, `int32 total_count` |

UpdateSource — full replace: клиент передаёт все поля, сервер перезаписывает запись целиком.

### 2. Кодогенерация

Зависимости: `protoc-gen-go`, `protoc-gen-connect-go`

Генерируемые файлы:
- `api/source/v1/source.pb.go`
- `api/source/v1/sourcev1connect/source.connect.go`

### 3. Доменная модель Source

Файл: `internal/app/source/source.go`

```go
type Type string

const (
    TypeTelegramChannel Type = "telegram_channel"
    TypeTelegramGroup   Type = "telegram_group"
    TypeRSS             Type = "rss"
    TypeWebScraping     Type = "web_scraping"
)
```

Доменная структура `Source` с полями: `ID` (uuid), `Type`, `Name`, `URL`, `Config` (map[string]any), `CreatedAt`, `UpdatedAt`. Не содержит GORM-тегов — это чистая доменная модель.

### 4. SQL-миграция

Файл: `migrations/001_create_sources.sql` (goose format)

```sql
-- +goose Up
CREATE TABLE sources (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type       VARCHAR(32) NOT NULL,
    name       VARCHAR(255) NOT NULL,
    url        TEXT NOT NULL,
    config     JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_sources_type ON sources (type);

-- +goose Down
DROP TABLE IF EXISTS sources;
```

### 5. Интерфейс репозитория

Файл: `internal/app/source/repository.go`

Интерфейс `Repository` с методами:
- `Create(ctx, *Source) error`
- `GetByID(ctx, uuid.UUID) (*Source, error)`
- `Update(ctx, *Source) error`
- `Delete(ctx, uuid.UUID) error`
- `List(ctx, ListFilter) ([]Source, int, error)` — возвращает список, total_count, error

`ListFilter`: `Type` (пустая = без фильтра), `PageSize`, `Page`

### 6. Реализация репозитория

Файл: `internal/app/source/adapters/postgres/repository.go`

GORM-реализация интерфейса. GORM-модель (с тегами) определяется здесь же, в адаптере — не в доменном слое. Sentinel error `ErrNotFound` в `internal/app/source/errors.go`.

- `GetByID` — если не найдено, возвращает `ErrNotFound`
- `Delete` — если RowsAffected == 0, возвращает `ErrNotFound`

### 7. Сервис

Файл: `internal/app/source/service.go`

```go
type Service struct {
    repo Repository
    log  *slog.Logger
}

func NewService(repo Repository, log *slog.Logger) *Service
```

Методы:
- `Create(ctx context.Context, s *Source) (*Source, error)` — валидация → repo.Create → возврат созданного Source
- `Get(ctx context.Context, id uuid.UUID) (*Source, error)` — repo.GetByID
- `Update(ctx context.Context, s *Source) (*Source, error)` — валидация → repo.GetByID (проверка существования) → repo.Update → возврат обновлённого Source
- `Delete(ctx context.Context, id uuid.UUID) error` — repo.Delete
- `List(ctx context.Context, filter ListFilter) ([]Source, int, error)` — нормализация filter → repo.List

При ошибке от репозитория, отличной от `ErrNotFound`, сервис возвращает её без обёртки — хендлер маппит в `CodeInternal`.

### 8. Connect-go хендлер

Файл: `internal/app/source/adapters/connect/handler.go`

Реализует `sourcev1connect.SourceServiceHandler`. Маппит proto ↔ domain, вызывает сервис, маппит ошибки в Connect-коды.

### 9. Правила валидации

**Общие:**
- `name` — обязательное, 1–255 символов, непустое после `strings.TrimSpace`
- `url` — обязательное, `net/url.Parse` без ошибки, scheme строго `http` или `https`
- `type` — обязательное, не `UNSPECIFIED`
- `config` — обязательное, oneof-вариант должен соответствовать `type` (например, type=rss → config.rss). Несоответствие type и config → `CodeInvalidArgument`

**По типу:**

| Тип | Поле config | Правило |
|---|---|---|
| telegram_channel | channel_id | Обязательное, непустое |
| telegram_group | group_id | Обязательное, непустое |
| rss | feed_url | Обязательное, `net/url.Parse` без ошибки, scheme `http`/`https` |
| web_scraping | selector | Обязательное, непустое |

**Пагинация ListSources:**
- `page_size`: если ≤ 0 → 50, если > 100 → 100
- `page`: если ≤ 0 → 1

### 10. Подключение к БД

Файл: `internal/platform/postgres/postgres.go`

```go
func Open(dsn string) (*gorm.DB, error)
```

DSN читается из `DATABASE_URL`. Если не задан — `bootstrap.Run` возвращает ошибку.

### 11. Изменения в bootstrap

`bootstrap.Run` расширяется:
1. Читает `DATABASE_URL`, если пуст — ошибка
2. Открывает GORM-соединение
3. Создаёт repo → service → handler
4. Регистрирует Connect-go хендлер на mux
5. Health check остаётся без изменений

Сигнатура `Run` не меняется.

### 12. Маппинг ошибок

| Ситуация | Connect Code |
|---|---|
| Ошибка валидации | `CodeInvalidArgument` |
| Невалидный UUID | `CodeInvalidArgument` |
| Source not found | `CodeNotFound` |
| Внутренняя ошибка БД | `CodeInternal` (детали логируются, не возвращаются клиенту) |

### 13. Unit-тесты

Файл: `internal/app/source/service_test.go`

Мок-репозиторий, реализующий интерфейс `Repository`.

Сценарии:
- **Create**: успех для каждого из 4 типов; ошибки: пустое name, невалидный URL, type UNSPECIFIED, отсутствие обязательного поля config, несоответствие type и config oneof-варианта
- **Get**: успех; ошибки: невалидный UUID, not found
- **Update**: успех; ошибки: not found, невалидные данные
- **Delete**: успех; ошибки: not found, невалидный UUID
- **List**: без фильтра, с фильтром по типу, нормализация page/page_size, пустой список

### 14. Зависимости go.mod

Добавить: `gorm.io/gorm`, `gorm.io/driver/postgres`, `github.com/google/uuid`, `github.com/pressly/goose/v3`

---

## Инварианты

- Зависимости направлены строго по слоям:
  - `cmd` импортирует `bootstrap`
  - `bootstrap` импортирует `platform`, `app`, адаптеры
  - Адаптеры (`adapters/postgres/`, `adapters/connect/`) импортируют `app` и `platform`
  - `app` (доменный слой) не импортирует адаптеры
- Интерфейс `Repository` объявлен в `internal/app/source/`
- Пакет `internal/platform/postgres` не содержит бизнес-логики
- SQL-миграции — единственный способ изменения схемы БД
- Health check работает и не изменён

---

## Acceptance Criteria

- [ ] Существует `api/source/v1/source.proto` с `SourceService` и 5 RPCs
- [ ] Существуют сгенерированные файлы `source.pb.go` и `source.connect.go`
- [ ] `CreateSource` создаёт источник и возвращает его с id, created_at, updated_at
- [ ] `GetSource` возвращает источник по UUID
- [ ] `GetSource` возвращает `CodeNotFound` для несуществующего ID
- [ ] `UpdateSource` обновляет все поля источника
- [ ] `DeleteSource` удаляет источник
- [ ] `ListSources` возвращает список с total_count и поддерживает фильтр по типу
- [ ] Валидация: пустое name → `CodeInvalidArgument`
- [ ] Валидация: невалидный URL → `CodeInvalidArgument`
- [ ] Валидация: type UNSPECIFIED → `CodeInvalidArgument`
- [ ] Валидация: отсутствие обязательного поля config → `CodeInvalidArgument`
- [ ] Валидация: несоответствие type и config (type=rss, config=telegram_channel) → `CodeInvalidArgument`
- [ ] Невалидный UUID → `CodeInvalidArgument`
- [ ] `DATABASE_URL` не задан → `bootstrap.Run` возвращает ошибку
- [ ] `GET /healthz` по-прежнему возвращает 200
- [ ] `go build ./...` проходит без ошибок
- [ ] `go test ./...` проходит без ошибок
- [ ] Unit-тесты покрывают все сценарии из раздела 13
- [ ] Все инварианты не нарушены

---

## Ограничения

- Не изменять `internal/platform/logger/logger.go`
- Не изменять `internal/bootstrap/health.go`
- Не изменять `cmd/feedium/main.go`
- Не добавлять аутентификацию
- Не использовать GORM AutoMigrate для production
- Не трогать существующие миграции
- Сгенерированный код (`*.pb.go`, `*.connect.go`) не редактируется вручную
