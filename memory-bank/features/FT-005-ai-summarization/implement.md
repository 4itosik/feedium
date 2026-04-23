---
doc_kind: feature
doc_function: implementation-plan
purpose: "Пошаговый план реализации FT-005: AI-суммаризация постов и переписок — генерация саммари через LLM с дифференцированной обработкой по типу источника."
derived_from:
  - spec.md
status: active
delivery_status: done
---

# FT-005: Implementation Plan

## Steps

### 1. Расширение конфигурации: секция `summary` в `conf.proto`

- **Цель.** Описать все параметры из FR-10 в protobuf-конфигурации.
- **Действия.**
  1. В `internal/conf/conf.proto` добавить сообщение `Summary` с вложенными сообщениями:
     - `SummaryWorker` с полями: `google.protobuf.Duration poll_interval`, `int32 batch_size`.
     - `SummaryCron` с полями: `google.protobuf.Duration interval`.
     - `SummaryLLM` с полями: `google.protobuf.Duration timeout`, `int32 max_retries`, `string provider`; сообщение `SummaryLLMProvider` с полями `string api_key`, `string base_url`, `string model`; поле `map<string, SummaryLLMProvider> providers`.
     - `SummaryCumulative` с полями: `google.protobuf.Duration max_window`, `int32 max_input_chars`.
     - `SummaryOutbox` с полями: `google.protobuf.Duration event_ttl`.
  2. В `Bootstrap` добавить поле `Summary summary = N`.
- **Зависимости.** Нет.
- **Результат.** `conf.proto` описывает `bootstrap.summary.*` со всеми параметрами из FR-10.
- **Проверка.** Файл содержит сообщение `Summary` и все вложенные поля; имена и типы совпадают с FR-10.

### 2. Регенерация `conf.pb.go`

- **Цель.** Получить Go-типы для новой секции конфигурации.
- **Действия.** Запустить `make proto`; сгенерированный `internal/conf/conf.pb.go` закоммитить.
- **Зависимости.** Шаг 1.
- **Результат.** Типы `conf.Summary`, `conf.SummaryWorker`, `conf.SummaryLLM` и др. доступны в Go.
- **Проверка.** `go build ./internal/conf/...` — exit 0; повторная генерация не создаёт diff.

### 3. Дефолтные значения в `configs/config.yaml`

- **Цель.** Локальный конфиг с параметрами суммаризации для dev-окружения.
- **Действия.** Добавить в `configs/config.yaml` секцию `summary` со значениями:
  - `worker.poll_interval: 5s`, `worker.batch_size: 10`
  - `cron.interval: 1h`
  - `llm.timeout: 30s`, `llm.max_retries: 3`, `llm.provider: openrouter`
  - `llm.providers.openrouter.api_key`, `base_url`, `model` — с плейсхолдерами
  - `cumulative.max_window: 72h`, `cumulative.max_input_chars: 50000`
  - `outbox.event_ttl: 24h`
- **Зависимости.** Шаг 1.
- **Результат.** YAML содержит валидный блок `summary` со всеми параметрами.
- **Проверка.** `kratos config` парсит файл в `Bootstrap` без ошибок; `bc.Summary` заполнен.

### 4. Расширение `ListPostsFilter` временным диапазоном

- **Цель.** Обеспечить фильтрацию постов по `created_at BETWEEN start AND end` (для cumulative worker).
- **Действия.**
  1. В `internal/biz/post.go`: добавить в `ListPostsFilter` поля `CreatedAfter *time.Time`, `CreatedBefore *time.Time`. Обновить `ValidateListPostsFilter` — если заданы, проверить `CreatedAfter < CreatedBefore`.
  2. В `internal/data/post_repo.go`: в методе `List` добавить Ent-предикаты `post.CreatedAtGTE`/`post.CreatedAtLT` при заполненных полях.
  3. Обновить тесты: `internal/biz/post_test.go` (валидация), `internal/data/post_repo_test.go` (фильтрация).
- **Зависимости.** Нет.
- **Результат.** `PostRepo.List` поддерживает фильтр по временному диапазону.
- **Проверка.** `go build ./internal/biz/... ./internal/data/...` — exit 0; unit-тесты на валидацию проходят.

### 5. Расширение `ListSourcesFilter` полем `ProcessingMode`

- **Цель.** Обеспечить фильтрацию источников по `ProcessingMode` (для cron worker).
- **Действия.**
  1. В `internal/biz/source.go`: добавить в `ListSourcesFilter` поле `ProcessingMode *ProcessingMode`.
  2. В `internal/data/source_repo.go`: в методе `List` маппить `ProcessingMode` в предикат по `source.Type`. `ProcessingModeCumulative` → `source.TypeEQ(string(telegram_group))`. `ProcessingModeSelfContained` → `source.TypeIN(string(telegram_channel), string(rss), string(html))`. Это сохраняет SSoT: `ProcessingMode` вычисляется из типа (не хранится в БД), а репозиторий транслирует абстракцию в SQL-предикат.
  3. Обновить тесты: `internal/data/source_repo_test.go` (фильтрация по ProcessingMode).
- **Зависимости.** Нет.
- **Результат.** `SourceRepo.List` поддерживает фильтр по `ProcessingMode` без добавления колонки в БД.
- **Проверка.** `go build ./internal/biz/... ./internal/data/...` — exit 0; unit-тесты проходят.

### 6. Изменение сигнатуры `PostRepo.Save`: флаг `created`

- **Цель.** Дать `PostUsecase.Create` возможность отличить «пост создан» от «пост уже существовал» для условного создания `SummaryEvent`.
- **Действия.**
  1. Изменить сигнатуру `PostRepo.Save(ctx, Post) (Post, bool, error)` — второй возврат `created = true` при insert, `false` при upsert.
  2. Обновить реализацию в `internal/data/post_repo.go`: при успешном `Ent.Post.Create` → `created = true`; при ловле `23505` и возврате существующего → `created = false`.
  3. Обновить все вызовы `PostRepo.Save` в `PostUsecase` и тестах.
- **Зависимости.** Нет.
- **Результат.** `PostUsecase.Create` точно знает, был ли пост создан.
- **Проверка.** `go build ./...` — exit 0; существующие тесты адаптированы и проходят.

### 7. Доменные сущности: `Summary` и `SummaryEvent`

- **Цель.** Определить доменные типы в `biz/` согласно FR-01.1 и FR-01.2 (FR-01, INV-4..INV-9).
- **Действия.** Создать `internal/biz/summary.go`:
  1. Типы `SummaryEventType` (enum: `summarize_post`, `summarize_source`) и `SummaryEventStatus` (enum: `pending`, `processing`, `completed`, `failed`, `expired`) — строковые константы.
  2. Структура `Summary` с полями: `ID string`, `PostID *string`, `SourceID string`, `Text string`, `WordCount int`, `CreatedAt time.Time`. Инвариант: ровно одно из двух — `PostID != nil` (self-contained) или `PostID == nil` (cumulative).
  3. Структура `SummaryEvent` с полями: `ID string`, `PostID *string`, `SourceID string`, `EventType SummaryEventType`, `Status SummaryEventStatus`, `SummaryID *string`, `Error *string`, `CreatedAt time.Time`, `ProcessedAt *time.Time`.
  4. Функция-конструктор `NewSummaryEvent(eventType, sourceID, postID)` — генерирует UUID v7, устанавливает `status = pending`, `createdAt = now`.
  5. Функция валидации `ValidateSummary(text string) error` — проверяет: текст не пуст (после trim), `WordCount > 0`.
  6. Сентинельные ошибки: `ErrSummaryNotFound`, `ErrSummaryEventNotFound`, `ErrSummaryAlreadyProcessing`, `ErrSummarizeSelfContainedSource`.
- **Зависимости.** Нет.
- **Результат.** Доменные типы `Summary` и `SummaryEvent` доступны в `biz/`.
- **Проверка.** `go build ./internal/biz/...` — exit 0.

### 8. Интерфейсы в `biz/`: репозитории, `TxManager`, `LLMProvider`

- **Цель.** Определить контракты для дата-слоя и LLM согласно FR-02, FR-08.
- **Действия.** В `internal/biz/summary.go` (или отдельном `internal/biz/summary_interfaces.go`) добавить:
  1. `TxManager interface { InTx(ctx context.Context, fn func(ctx context.Context) error) error }` — FR-02.
  2. `SummaryRepo interface` с методами: `Save(ctx, Summary) (Summary, error)`, `Get(ctx, id string) (Summary, error)`, `ListByPost(ctx, postID string) ([]Summary, error)`, `ListBySource(ctx, sourceID string, pageSize int, pageToken string) (ListSummariesResult, error)`, `GetLastBySource(ctx, sourceID string) (*Summary, error)`.
  3. `SummaryOutboxRepo interface` с методами: `Save(ctx, SummaryEvent) (SummaryEvent, error)`, `Get(ctx, id string) (SummaryEvent, error)`, `ListPending(ctx, limit int) ([]SummaryEvent, error)`, `UpdateStatus(ctx, id string, status SummaryEventStatus, summaryID *string, errText *string) error`, `HasActiveEvent(ctx, sourceID string, eventType SummaryEventType) (bool, *SummaryEvent, error)`.
  4. `LLMProvider interface { Summarize(ctx context.Context, text string) (string, error) }` — FR-08.
  5. Тип `ListSummariesResult` с полями `Items []Summary`, `NextPageToken string`.
- **Зависимости.** Шаг 7.
- **Результат.** Все интерфейсы, которые нужно реализовать в `data/`, определены в `biz/`.
- **Проверка.** `go build ./internal/biz/...` — exit 0; интерфейсы не импортируют `data/` или `ent/`.

### 9. Генерация mocks для интерфейсов `biz/`

- **Цель.** Получить mockgen-моки для `SummaryRepo`, `SummaryOutboxRepo`, `LLMProvider`, `TxManager` — требуются в последующих шагах с тестами.
- **Действия.**
  1. Добавить `//go:generate mockgen` директивы в `internal/biz/summary.go` для `SummaryRepo`, `SummaryOutboxRepo`, `LLMProvider`, `TxManager`.
  2. Запустить `go generate ./internal/biz/...`.
  3. Сгенерированные моки закоммитить в `internal/biz/mock/`.
- **Зависимости.** Шаг 8.
- **Результат.** Все необходимые моки для biz/-тестов доступны.
- **Проверка.** `go generate ./internal/biz/...` — exit 0; `go build ./...` — exit 0.

### 10. Unit-тесты доменных сущностей и валидации (TDD biz/)

- **Цель.** Покрыть автотестами инварианты `Summary`, `SummaryEvent`, валидацию (testing-policy: TDD для biz/).
- **Действия.** Создать `internal/biz/summary_test.go`:
  1. `TestNewSummaryEvent` — проверяет: ID генерируется (UUID v7), `status = pending`, `createdAt` заполнен, `PostID` и `SourceID` корректны.
  2. `TestNewSummaryEvent_SelfContained` — `PostID != nil`, `EventType = summarize_post`.
  3. `TestNewSummaryEvent_Cumulative` — `PostID == nil`, `EventType = summarize_source`.
  4. `TestValidateSummary` — happy path: непустой текст → nil.
  5. `TestValidateSummary_EmptyText` — пустой текст (после trim) → ошибка.
  6. `TestValidateSummary_WhitespaceOnly` — только пробелы → ошибка.
  7. Тесты инварианта Summary: `PostID != nil` → self-contained; `PostID == nil` → cumulative; оба nil или оба not-nil — логическая ошибка в использовании, но не валидационная (инвариант обеспечивается вызывающим кодом).
- **Зависимости.** Шаги 7, 8.
- **Результат.** Все доменные инварианты покрыты автотестами.
- **Проверка.** `go test ./internal/biz/... -run TestNewSummaryEvent -run TestValidateSummary` — exit 0.

### 11. SQL-миграция: таблицы `summaries` и `summary_events`

- **Цель.** Создать таблицы в PostgreSQL для хранения саммари и событий outbox.
- **Действия.** Создать `migrations/20260415100000_create_summaries_and_events.sql`:
  1. Таблица `summaries`:
     - `id UUID PRIMARY KEY`
     - `post_id UUID NULL REFERENCES posts(id) ON DELETE CASCADE`
     - `source_id UUID NOT NULL REFERENCES sources(id) ON DELETE CASCADE`
     - `text TEXT NOT NULL`
     - `word_count INT NOT NULL`
     - `created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`
     - Индекс `idx_summaries_post_id` на `(post_id, created_at DESC)`
     - Индекс `idx_summaries_source_created` на `(source_id, created_at DESC, id DESC)`
  2. Таблица `summary_events`:
     - `id UUID PRIMARY KEY`
     - `post_id UUID NULL`
     - `source_id UUID NOT NULL REFERENCES sources(id) ON DELETE CASCADE`
     - `event_type TEXT NOT NULL`
     - `status TEXT NOT NULL DEFAULT 'pending'`
     - `summary_id UUID NULL REFERENCES summaries(id)`
     - `error TEXT NULL`
     - `created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`
     - `processed_at TIMESTAMPTZ NULL`
     - Частичный unique-индекс `idx_summary_events_unique_active_post` на `(post_id, event_type) WHERE status IN ('pending', 'processing')` — INV-2.
     - Частичный unique-индекс `idx_summary_events_unique_active_source` на `(source_id) WHERE event_type = 'summarize_source' AND status IN ('pending', 'processing')` — INV-3.
     - Индекс `idx_summary_events_pending` на `(status, created_at) WHERE status = 'pending'` — для polling воркера.
     - Индекс `idx_summary_events_source_type_status` на `(source_id, event_type, status)`.
  3. `-- +goose Down` — `DROP TABLE IF EXISTS summary_events; DROP TABLE IF EXISTS summaries;`.
- **Зависимости.** Нет (миграции независимы от Go-кода).
- **Результат.** Две новые таблицы в БД с констрейнтами и индексами.
- **Проверка.** `make migrate` на существующей БД — exit 0; `\d summaries` и `\d summary_events` показывают ожидаемую структуру.

### 12. Ent-схемы: `Summary` и `SummaryEvent`

- **Цель.** Создать Ent-схемы для codegen запросов к новым таблицам.
- **Действия.**
  1. Создать `internal/ent/schema/summary.go`:
     - Поля: `id` (UUID v7), `post_id` (UUID, optional, nillable), `source_id` (UUID), `text` (string, not empty), `word_count` (int), `created_at` (timestamptz).
     - Edges: `edge.From("post", Post.Type).Ref("summaries").Field("post_id").Unique()` — optional; `edge.From("source", Source.Type).Ref("source_summaries").Field("source_id").Unique().Required()`.
     - Indexes: `(post_id, created_at)`, `(source_id, created_at, id)`.
  2. Создать `internal/ent/schema/summary_event.go`:
     - Поля: `id` (UUID v7), `post_id` (UUID, optional, nillable), `source_id` (UUID), `event_type` (string, not empty), `status` (string, not empty, default "pending"), `summary_id` (UUID, optional, nillable), `error` (string, optional, nillable), `created_at` (timestamptz), `processed_at` (timestamptz, optional, nillable).
     - Edges: `edge.To("summary", Summary.Type).Field("summary_id").Unique()` — optional.
     - Indexes: `(status, created_at)`, `(source_id, event_type, status)`.
  3. Обновить существующие схемы: добавить `edge.To("summaries", Summary.Type)` в Post, `edge.To("source_summaries", Summary.Type)` и `edge.To("summary_events", SummaryEvent.Type)` в Source.
  4. Запустить `go generate ./internal/ent/` — регенерировать Ent-код.
- **Зависимости.** Шаг 11.
- **Результат.** Сгенерированные типы и builders для `Summary` и `SummaryEvent` в `internal/ent/`.
- **Проверка.** `go build ./internal/ent/...` — exit 0; повторная генерация не создаёт diff.

### 13. Реализация `TxManager` в `data/`

- **Цель.** Обеспечить выполнение операций в транзакции (FR-02, INV-1).
- **Действия.** Создать `internal/data/tx_manager.go`:
  1. Тип `txKey struct{}` — ключ для хранения `*entgo.Tx` в контексте.
  2. Функция-хелпер `clientFromContext(ctx, fallback *entgo.Client) *entgo.Client` — если в контексте есть `*entgo.Tx`, возвращает `tx.Client()`; иначе `fallback`.
  3. Тип `txManager` с полем `data *Data`.
  4. Конструктор `NewTxManager(data *Data) *txManager`.
  5. Метод `InTx(ctx, fn)`: вызывает `data.Ent.Tx(ctx)`, оборачивает `tx` в контекст через `context.WithValue(ctx, txKey{}, tx)`, выполняет `fn(txCtx)`, при ошибке — `tx.Rollback()`, при успехе — `tx.Commit()`.
  6. Все репозитории (`summaryRepo`, `summaryOutboxRepo`) должны использовать `clientFromContext(ctx, d.Ent)` вместо `d.Ent` напрямую — чтобы внутри транзакции работать с `tx.Client()`.
  7. Compile-time assertion: `var _ biz.TxManager = (*txManager)(nil)`.
- **Зависимости.** Шаги 8, 12.
- **Результат.** `TxManager` реализован; другие репозитории смогут работать в рамках одной транзакции.
- **Проверка.** `go build ./internal/data/...` — exit 0.

### 14. Реализация `SummaryRepo` в `data/`

- **Цель.** Персистентное хранение саммари (FR-07, INV-4, INV-5).
- **Действия.** Создать `internal/data/summary_repo.go`:
  1. Тип `summaryRepo` с полем `data *Data`.
  2. `Save` — создаёт Summary через Ent; маппит `biz.Summary` → ent-поля; возвращает сохранённую сущность. Использует `clientFromContext(ctx, d.Ent)` для поддержки транзакций.
  3. `Get` — получает по ID; `ErrSummaryNotFound` если не найден.
  4. `ListByPost` — список саммари для поста, `ORDER BY created_at DESC`.
  5. `ListBySource` — список саммари для источника с cursor-based пагинацией (аналогично `post_repo.go`), `ORDER BY created_at DESC, id DESC`.
  6. `GetLastBySource` — последний саммари для источника (для определения временного окна cumulative). Использует `ORDER BY created_at DESC LIMIT 1`.
  7. Приватные мапперы `mapEntSummaryToDomain`.
  8. Compile-time assertion: `var _ biz.SummaryRepo = (*summaryRepo)(nil)`.
- **Зависимости.** Шаги 8, 12.
- **Результат.** `SummaryRepo` реализует интерфейс из `biz/`.
- **Проверка.** `go build ./internal/data/...` — exit 0.

### 15. Реализация `SummaryOutboxRepo` в `data/`

- **Цель.** Outbox для событий суммаризации: сохранение, polling, обновление статуса (FR-02, FR-03, FR-09).
- **Действия.** Создать `internal/data/summary_outbox_repo.go`:
  1. Тип `summaryOutboxRepo` с полем `data *Data`.
  2. `Save` — создаёт SummaryEvent через Ent; маппит поля. При unique violation на частичных индексах (конфликт активного события) — возвращает `biz.ErrSummaryAlreadyProcessing` (определяется по PostgreSQL error code `23505`). Использует `clientFromContext(ctx, d.Ent)`.
  3. `Get` — по ID; `ErrSummaryEventNotFound` если не найден.
  4. `ListPending` — получает события `WHERE status = 'pending' ORDER BY created_at ASC LIMIT $1`. Использует `clientFromContext(ctx, d.Ent)` (поддержка транзакций).
  5. `UpdateStatus` — обновляет `status`, опционально `summary_id`, `error`, `processed_at`.
  6. `HasActiveEvent` — проверяет наличие активного события `(source_id, event_type)` со `status IN (pending, processing)`. Возвращает `(found bool, event *SummaryEvent, err error)`.
  7. Compile-time assertion: `var _ biz.SummaryOutboxRepo = (*summaryOutboxRepo)(nil)`.
- **Зависимости.** Шаги 8, 12.
- **Результат.** `SummaryOutboxRepo` реализует интерфейс из `biz/`.
- **Проверка.** `go build ./internal/data/...` — exit 0.

### 16. Интеграционные тесты репозиториев `data/`

- **Цель.** Покрыть корректность Ent-запросов, маппинга и транзакций (testing-policy: testcontainers).
- **Действия.** Создать `internal/data/summary_repo_test.go` и `internal/data/summary_outbox_repo_test.go`:
  1. **summary_repo_test.go:**
     - `TestIntegration_SummaryRepo_SaveAndGet` — сохраняет и читает по ID.
     - `TestIntegration_SummaryRepo_Save_SelfContained` — `PostID != nil`.
     - `TestIntegration_SummaryRepo_Save_Cumulative` — `PostID == nil`.
     - `TestIntegration_SummaryRepo_ListByPost` — несколько саммари для одного поста, проверяет сортировку DESC.
     - `TestIntegration_SummaryRepo_ListBySource_Pagination` — cursor-based пагинация.
     - `TestIntegration_SummaryRepo_GetLastBySource` — возвращает последний саммари.
     - `TestIntegration_SummaryRepo_GetLastBySource_NoSummary` — возвращает nil.
     - `TestIntegration_SummaryRepo_Get_NotFound` — `ErrSummaryNotFound`.
  2. **summary_outbox_repo_test.go:**
     - `TestIntegration_OutboxRepo_SaveAndGet` — сохраняет и читает.
     - `TestIntegration_OutboxRepo_ListPending` — только `pending`, сортировка по `created_at`.
     - `TestIntegration_OutboxRepo_UpdateStatus_Completed` — устанавливает `completed`, `summary_id`, `processed_at`.
     - `TestIntegration_OutboxRepo_UpdateStatus_Failed` — устанавливает `failed`, `error`.
     - `TestIntegration_OutboxRepo_UpdateStatus_Expired` — устанавливает `expired`.
     - `TestIntegration_OutboxRepo_HasActiveEvent_True` — есть pending-событие.
     - `TestIntegration_OutboxRepo_HasActiveEvent_False` — нет активных.
     - `TestIntegration_OutboxRepo_HasActiveEvent_Failed_NotBlocked` — failed-событие не считается активным.
     - `TestIntegration_OutboxRepo_UniqueActivePost` — две попытки создать событие для одного поста → `ErrSummaryAlreadyProcessing`.
     - `TestIntegration_OutboxRepo_UniqueActiveSource` — две попытки для одного источника → `ErrSummaryAlreadyProcessing`.
  3. **tx_manager_test.go:**
     - `TestIntegration_TxManager_Commit` — обе операции видны после коммита.
     - `TestIntegration_TxManager_Rollback` — ни одна не видна после ошибки.
  Все тесты используют testcontainers PostgreSQL, `goleak.VerifyNone`, AAA-паттерн.
- **Зависимости.** Шаги 13, 14, 15.
- **Результат.** Репозитории покрыты интеграционными тестами.
- **Проверка.** `go test ./internal/data/... -run Integration` — exit 0 при запущенном Docker.

### 17. Рефакторинг `PostUsecase.Create`: атомарное создание поста + SummaryEvent

- **Цель.** FR-02, INV-1: при создании self-contained поста атомарно создавать `SummaryEvent` в одной транзакции.
- **Действия.** Изменить `internal/biz/post.go`:
  1. Расширить `PostUsecase`: добавить поля `txManager TxManager`, `outboxRepo SummaryOutboxRepo`, `sourceRepo SourceRepo`.
  2. Обновить `NewPostUsecase` — принять все три новых зависимости: `NewPostUsecase(repo PostRepo, txManager TxManager, outboxRepo SummaryOutboxRepo, sourceRepo SourceRepo)`.
  3. Изменить метод `Create`:
     - После `ValidateCreatePost` — загрузить источник через `sourceRepo.Get(ctx, sourceID)`.
     - Определить `ProcessingMode` через `ProcessingModeForType(source.Type)`.
     - Обернуть операцию в `txManager.InTx`:
       - Внутри транзакции: `repo.Save(txCtx, post)` — сохраняет пост, получает `created` флаг.
       - Если `ProcessingMode == self_contained` и `created == true` (пост новый, не upsert): создать `SummaryEvent` с `event_type = summarize_post`, `post_id = post.ID`, `source_id = sourceID`; вызвать `outboxRepo.Save(txCtx, event)`.
       - Если `ProcessingMode == cumulative` — не создавать `SummaryEvent`.
     - Если `created == false` (upsert, пост уже существовал) — не создавать `SummaryEvent` (FR-09: идемпотентность upsert).
  4. Обновить `biz/wire.go`: `ProviderSet` — `NewPostUsecase` изменил сигнатуру, Wire автоматически подставит новые зависимости при наличии их в графе.
- **Зависимости.** Шаги 6, 7, 8, 13, 15.
- **Результат.** `PostUsecase.Create` атомарно создаёт Post + SummaryEvent для self-contained источников.
- **Проверка.** `go build ./internal/biz/...` — exit 0.

### 18. Unit-тесты `PostUsecase.Create` с суммаризацией (TDD)

- **Цель.** Покрыть TDD новые сценарии `PostUsecase.Create` (testing-policy: TDD для biz/).
- **Действия.** Обновить `internal/biz/post_usecase_test.go`:
  1. `TestPostUsecase_Create_SelfContained_CreatesSummaryEvent` — моки `PostRepo.Save` (возвращает `(post, true, nil)`), `SourceRepo.Get` (возвращает telegram_channel), `SummaryOutboxRepo.Save` (ожидается вызов). `TxManager.InTx` — вызывает fn с ctx напрямую (mock-реализация). Проверяет: оба repo вызваны, `SummaryEvent` корректен.
  2. `TestPostUsecase_Create_Cumulative_NoSummaryEvent` — `SourceRepo.Get` возвращает telegram_group. `SummaryOutboxRepo.Save` — НЕ ожидается.
  3. `TestPostUsecase_Create_SelfContained_TxRollback` — `PostRepo.Save` возвращает ошибку. `SummaryOutboxRepo.Save` — НЕ вызывается. Возвращается ошибка.
  4. `TestPostUsecase_Create_Upsert_NoSummaryEvent` — `PostRepo.Save` возвращает `(post, false, nil)` (пост уже был). `SummaryOutboxRepo.Save` — НЕ вызывается.
  5. `TestPostUsecase_Create_SourceNotFound` — `SourceRepo.Get` возвращает `ErrSourceNotFound`.
  Все тесты: AAA, `goleak.VerifyNone`, mockgen-моки из `internal/biz/mock/` (шаг 9).
- **Зависимости.** Шаги 9, 17.
- **Результат.** Все сценарии FR-02 покрыты автотестами на уровне biz/.
- **Проверка.** `go test ./internal/biz/... -run TestPostUsecase_Create` — exit 0.

### 19. `SummaryUsecase`: бизнес-логика чтения саммари и ручного запуска

- **Цель.** Реализовать FR-05, FR-06, FR-07: ручной запуск суммаризации, чтение саммари, polling статуса.
- **Действия.** Создать `internal/biz/summary_usecase.go`:
  1. Тип `SummaryUsecase` с полями: `summaryRepo SummaryRepo`, `outboxRepo SummaryOutboxRepo`, `sourceRepo SourceRepo`.
  2. Конструктор `NewSummaryUsecase`.
  3. `TriggerSourceSummarize(ctx, sourceID) (eventID string, isExisting bool, err error)`:
     - Загрузить источник через `sourceRepo.Get`.
     - Проверить `ProcessingMode == cumulative`; если self-contained — вернуть `ErrSummarizeSelfContainedSource`.
     - Проверить идемпотентность: `outboxRepo.HasActiveEvent(sourceID, summarize_source)`.
     - Если есть активное событие (`pending`/`processing`) — вернуть его ID, `isExisting = true` (вызывающий код вернёт `200 OK`).
     - Если есть `failed` — не блокирует (создаётся новое событие).
     - Создать `NewSummaryEvent(summarize_source, sourceID, nil)`, сохранить через `outboxRepo.Save`.
     - Вернуть `eventID, false, nil` (вызывающий код вернёт `202 Accepted`).
  4. `GetSummaryEvent(ctx, id) (*SummaryEvent, error)` — делегирует `outboxRepo.Get`.
  5. `GetSummary(ctx, id) (Summary, error)` — делегирует `summaryRepo.Get`.
  6. `ListPostSummaries(ctx, postID) ([]Summary, error)` — делегирует `summaryRepo.ListByPost`.
  7. `ListSourceSummaries(ctx, sourceID, pageSize int, pageToken string) (ListSummariesResult, error)` — делегирует `summaryRepo.ListBySource`.
- **Зависимости.** Шаги 7, 8.
- **Результат.** Бизнес-логика для API чтения саммари и ручного триггера.
- **Проверка.** `go build ./internal/biz/...` — exit 0.

### 20. Unit-тесты `SummaryUsecase` (TDD)

- **Цель.** Покрыть TDD бизнес-логику `SummaryUsecase`.
- **Действия.** Создать `internal/biz/summary_usecase_test.go`:
  1. `TestSummaryUsecase_TriggerSourceSummarize_Cumulative_CreatesEvent` — happy path, возвращает eventID, `isExisting = false`.
  2. `TestSummaryUsecase_TriggerSourceSummarize_SelfContained_ReturnsError` — `ErrSummarizeSelfContainedSource`.
  3. `TestSummaryUsecase_TriggerSourceSummarize_ActiveEventExists_ReturnsExisting` — `HasActiveEvent` возвращает pending-событие; `isExisting = true`.
  4. `TestSummaryUsecase_TriggerSourceSummarize_FailedEventExists_CreatesNew` — failed-событие не блокирует, создаётся новое.
  5. `TestSummaryUsecase_TriggerSourceSummarize_SourceNotFound` — `ErrSourceNotFound`.
  6. `TestSummaryUsecase_GetSummaryEvent_Found` — возвращает событие.
  7. `TestSummaryUsecase_GetSummaryEvent_NotFound` — `ErrSummaryEventNotFound`.
  8. `TestSummaryUsecase_GetSummary_Found` / `NotFound`.
  9. `TestSummaryUsecase_ListPostSummaries` — делегирует repo.
  10. `TestSummaryUsecase_ListSourceSummaries_WithPagination` — делегирует repo.
- **Зависимости.** Шаги 9, 19.
- **Результат.** Все сценарии FR-05, FR-07 покрыты автотестами.
- **Проверка.** `go test ./internal/biz/... -run TestSummaryUsecase` — exit 0.

### 21. Proto-контракт: Summary API

- **Цель.** Описать API-контракт для FR-05, FR-06, FR-07 в proto.
- **Действия.** Создать `api/feedium/summary.proto`:
  1. `enum SummaryEventType { SUMMARY_EVENT_TYPE_UNSPECIFIED = 0; SUMMARY_EVENT_TYPE_SUMMARIZE_POST = 1; SUMMARY_EVENT_TYPE_SUMMARIZE_SOURCE = 2; }`
  2. `enum SummaryEventStatus { ... }` — pending, processing, completed, failed, expired.
  3. `message Summary` — поля: id, post_id (optional), source_id, text, word_count, created_at.
  4. `message SummaryEvent` — поля: id, source_id, post_id (optional), event_type, status, summary_id (optional), error (optional), created_at, processed_at (optional).
  5. `message V1SummarizeSourceRequest { string source_id = 1; }`
  6. `message V1SummarizeSourceResponse { string task_id = 1; bool existing = 2; }` — `existing` отличает `200 OK` от `202 Accepted`.
  7. `message V1GetSummaryEventRequest { string id = 1; }`
  8. `message V1GetSummaryEventResponse { SummaryEvent event = 1; }`
  9. `message V1ListPostSummariesRequest { string post_id = 1; }`
  10. `message V1ListPostSummariesResponse { repeated Summary summaries = 1; }`
  11. `message V1ListSourceSummariesRequest { string source_id = 1; int32 page_size = 2; string page_token = 3; }`
  12. `message V1ListSourceSummariesResponse { repeated Summary summaries = 1; string next_page_token = 2; }`
  13. `message V1GetSummaryRequest { string id = 1; }`
  14. `message V1GetSummaryResponse { Summary summary = 1; }`
  15. `service SummaryService`:
      - `rpc V1SummarizeSource(V1SummarizeSourceRequest) returns (V1SummarizeSourceResponse) { option (google.api.http) = { post: "/v1/sources/{source_id}/summarize" body: "*" }; }`
      - `rpc V1GetSummaryEvent(V1GetSummaryEventRequest) returns (V1GetSummaryEventResponse) { option (google.api.http) = { get: "/v1/summary-events/{id}" }; }`
      - `rpc V1ListPostSummaries(V1ListPostSummariesRequest) returns (V1ListPostSummariesResponse) { option (google.api.http) = { get: "/v1/posts/{post_id}/summaries" }; }`
      - `rpc V1ListSourceSummaries(V1ListSourceSummariesRequest) returns (V1ListSourceSummariesResponse) { option (google.api.http) = { get: "/v1/sources/{source_id}/summaries" }; }`
      - `rpc V1GetSummary(V1GetSummaryRequest) returns (V1GetSummaryResponse) { option (google.api.http) = { get: "/v1/summaries/{id}" }; }`
  16. Обновить `api/feedium/error_reason.proto` — добавить `ERROR_REASON_SUMMARIZE_SELF_CONTAINED_SOURCE`, `ERROR_REASON_SUMMARY_NOT_FOUND`, `ERROR_REASON_SUMMARY_EVENT_NOT_FOUND`.
- **Зависимости.** Нет.
- **Результат.** Proto-файл с пятью RPC-методами и всеми сообщениями.
- **Проверка.** `protoc --proto_path=. --proto_path=third_party --go_out=. --go_opt=paths=source_relative api/feedium/summary.proto` — exit 0.

### 22. Генерация proto-кода для Summary

- **Цель.** Получить Go-stubs для gRPC и HTTP.
- **Действия.** Запустить `make proto`; закоммитить сгенерированные файлы: `summary.pb.go`, `summary_grpc.pb.go`, `summary_http.pb.go`, обновлённый `error_reason.pb.go`.
- **Зависимости.** Шаг 21.
- **Результат.** Типы и stubs доступны в `api/feedium/`.
- **Проверка.** `go build ./api/feedium/...` — exit 0; повторный `make proto` не создаёт diff.

### 23. Service-слой: `SummaryService`

- **Цель.** Тонкий адаптер proto ↔ domain (FR-05, FR-06, FR-07).
- **Действия.** Создать `internal/service/summary/`:
  1. `summary.go`:
     - Тип `SummaryService` с `feedium.UnimplementedSummaryServiceServer` и полем `uc Usecase` (локальный интерфейс, аналогично `service/post/`).
     - `Usecase` interface: `TriggerSourceSummarize`, `GetSummaryEvent`, `GetSummary`, `ListPostSummaries`, `ListSourceSummaries` — с domain-типами.
     - Конструктор `NewSummaryService(uc Usecase) *SummaryService`.
     - `V1SummarizeSource` — вызывает `uc.TriggerSourceSummarize`; если `isExisting == true` → `200 OK` (в proto: `existing = true`); иначе → `202 Accepted` (в proto: `existing = false`). При `ErrSummarizeSelfContainedSource` → `400` с reason. При `ErrSourceNotFound` → `404`.
     - `V1GetSummaryEvent` — вызывает `uc.GetSummaryEvent`; `404` если не найден.
     - `V1ListPostSummaries` — вызывает `uc.ListPostSummaries`; `404` если пост не найден (дополнительно проверить через uc или вернуть пустой список).
     - `V1ListSourceSummaries` — вызывает `uc.ListSourceSummaries` с пагинацией; `404` если источник не найден.
     - `V1GetSummary` — вызывает `uc.GetSummary`; `404` если не найден.
     - Приватные мапперы domain → proto.
     - `mapDomainErrorToStatus` — по аналогии с `service/post/`.
  2. `wire.go` — `ProviderSet = wire.NewSet(NewSummaryService)`.
- **Зависимости.** Шаги 19, 22.
- **Результат.** Service-слой для Summary API.
- **Проверка.** `go build ./internal/service/summary/...` — exit 0.

### 24. Генерация mocks для `service/summary/`

- **Цель.** Получить mockgen-мок для `Usecase` интерфейса `service/summary/` — требуется для тестов SummaryService.
- **Действия.**
  1. Добавить `//go:generate mockgen` директиву в `internal/service/summary/summary.go` для интерфейса `Usecase`.
  2. Запустить `go generate ./internal/service/summary/...`.
  3. Сгенерированный мок закоммитить в `internal/service/summary/mock/`.
- **Зависимости.** Шаг 23.
- **Результат.** Mock для `Usecase` интерфейса доступен для тестов SummaryService.
- **Проверка.** `go generate ./internal/service/summary/...` — exit 0; `go build ./...` — exit 0.

### 25. Тесты `SummaryService`

- **Цель.** Покрыть маппинг proto ↔ domain (testing-policy: минимально для service/).
- **Действия.** Создать `internal/service/summary/summary_test.go`:
  1. `TestSummaryService_V1SummarizeSource_CreatesEvent` — мок uc возвращает `eventID, false, nil`; response содержит `task_id`, `existing = false`.
  2. `TestSummaryService_V1SummarizeSource_ExistingEvent` — `isExisting = true`; response содержит `existing = true`.
  3. `TestSummaryService_V1SummarizeSource_SelfContainedSource` — `ErrSummarizeSelfContainedSource` → gRPC status `InvalidArgument`.
  4. `TestSummaryService_V1SummarizeSource_SourceNotFound` → gRPC status `NotFound`.
  5. `TestSummaryService_V1GetSummaryEvent_Found` / `NotFound`.
  6. `TestSummaryService_V1GetSummary_Found` / `NotFound`.
  7. `TestSummaryService_V1ListPostSummaries` — маппинг нескольких саммари.
  8. `TestSummaryService_V1ListSourceSummaries_WithPagination` — проверяет `next_page_token`.
- **Зависимости.** Шаги 23, 24.
- **Результат.** Service-слой покрыт тестами.
- **Проверка.** `go test ./internal/service/summary/...` — exit 0.

### 26. LLM Provider: реализация OpenRouter

- **Цель.** Реализовать `LLMProvider` интерфейс для OpenRouter (FR-08, Ограничения: MVP использует OpenRouter).
- **Действия.** Создать `internal/data/llm_provider.go`:
  1. Тип `openRouterProvider` с полями: `apiKey string`, `baseURL string`, `model string`, `httpClient *http.Client`, `timeout time.Duration`, `log *slog.Logger`.
  2. Конструктор `NewOpenRouterProvider(cfg *conf.Summary_LLM, logger *slog.Logger) *openRouterProvider` — читает конфигурацию провайдера из `cfg.Providers[cfg.Provider]`.
  3. Метод `Summarize(ctx, text) (string, error)`:
     - Формирует HTTP-запрос к OpenRouter Chat Completions API.
     - Промпт: фиксированный системный промпт как string-константа в этом же файле. Требования к промпту: саммари на том же языке что оригинал, сохранить ключевые факты/имена/числа, concise summary. Конкретная формулировка пишется на этом шаге.
     - Таймаут — из конфигурации `cfg.Timeout`.
     - Парсит ответ, извлекает `choices[0].message.content`.
     - Возвращает текст саммари или ошибку.
  4. Compile-time assertion: `var _ biz.LLMProvider = (*openRouterProvider)(nil)`.
  5. Провайдер выбирается по конфигурации `summary.llm.provider`. Для MVP — только OpenRouter; фабрика провайдеров создаётся на случай расширения.
- **Зависимости.** Шаги 2, 8.
- **Результат.** `LLMProvider` реализован и может вызывать OpenRouter API.
- **Проверка.** `go build ./internal/data/...` — exit 0.

### 27. Тесты LLM Provider

- **Цель.** Покрыть LLM-провайдер тестами без реальных API-вызовов.
- **Действия.** Создать `internal/data/llm_provider_test.go`:
  1. `TestOpenRouterProvider_Summarize_Success` — `httptest.Server` возвращает успешный ответ; проверяет текст саммари.
  2. `TestOpenRouterProvider_Summarize_APIError` — сервер возвращает 500; проверяет ошибку.
  3. `TestOpenRouterProvider_Summarize_Timeout` — сервер задерживает ответ; проверяет `context.DeadlineExceeded`.
  4. `TestOpenRouterProvider_Summarize_EmptyResponse` — `choices` пустой; ошибка.
  5. `TestOpenRouterProvider_Summarize_InvalidJSON` — невалидный JSON; ошибка.
  6. `TestNewOpenRouterProvider_MissingProviderConfig` — провайдер не найден в конфигурации; ошибка.
- **Зависимости.** Шаг 26.
- **Результат.** LLM-провайдер покрыт unit-тестами.
- **Проверка.** `go test ./internal/data/... -run TestOpenRouterProvider` — exit 0.

### 28. Summary Worker (self-contained + cumulative) в `task/`

- **Цель.** Реализовать worker, реализующий `transport.Server` kratos lifecycle, поллящий outbox и обрабатывающий `summarize_post` и `summarize_source` события (FR-03, NFR-03).
- **Действия.** Создать `internal/task/summary_worker.go`:
  1. Тип `SummaryWorker` с полями: `outboxRepo biz.SummaryOutboxRepo`, `postRepo biz.PostRepo`, `summaryRepo biz.SummaryRepo`, `llmProvider biz.LLMProvider`, `cfg *conf.Summary`, `log *slog.Logger`, `done chan struct{}`, `wg sync.WaitGroup`.
  2. Конструктор `NewSummaryWorker(...)`.
  3. Метод `Start(ctx)` — интерфейс `transport.Server`:
     - Запускает горутину с polling-циклом: `time.Sleep(cfg.Worker.PollInterval)` → `processBatch(ctx)`.
     - Цикл завершается при отмене контекста или закрытии `done`.
  4. Метод `Stop(ctx)` — сигнализирует `done`, ждёт `wg.Wait()`.
  5. Метод `processBatch(ctx)`:
     - Вызывает `outboxRepo.ListPending(ctx, cfg.Worker.BatchSize)`.
     - Для каждого события — `processEvent(ctx, event)`.
  6. Метод `processEvent(ctx, event)`:
     - Лог: `summary_event_id`, `event_type`, `source_id`, `post_id`.
     - **Проверяет TTL первым:** если `created_at + event_ttl < now` → `status = expired`, лог с `age`, skip. Это предотвращает лишний переход `pending → processing → expired` и снижает риск зависания события в `processing` при краше.
     - Если TTL OK — устанавливает `status = processing`.
     - Если `event_type == summarize_post`:
       a. Загружает пост `postRepo.Get(ctx, event.PostID)`. Не найден → `status = failed`, `error = "post not found"`.
       b. Вызывает `llmProvider.Summarize(ctx, post.Text)` с retry (exponential backoff, до `cfg.LLM.MaxRetries` попыток). При каждой ошибке — лог `attempt`, `err`.
       c. Валидирует: `strings.TrimSpace(summaryText) != ""`. Если пустой — ошибка LLM, retry.
       d. После исчерпания retry → `status = failed`, `error = <last error>`.
       e. При успехе: создаёт `Summary` через `summaryRepo.Save`; `status = completed`, `summary_id`, `processed_at`.
     - Если `event_type == summarize_source`:
       a. Определяет временное окно: `GetLastBySource(ctx, event.SourceID)` → от `lastSummary.CreatedAt` до `now`. Если нет предыдущего → последние `cfg.Cumulative.MaxWindow` (72h). Окно обрезается до `maxWindow`.
       b. Загружает посты источника за окно: `postRepo.List(ctx, filter)` с `SourceID`, `OrderBy = SortByCreatedAt`, `OrderDir = SortAsc`, `CreatedAfter = windowStart`, `CreatedBefore = &now` (шаг 4).
       c. Если постов 0 → `status = completed` без создания Summary.
       d. Конкатенирует тексты в хронологическом порядке. Проверяет: `len(concat) > cfg.Cumulative.MaxInputChars` → `status = failed`, `error = "input text exceeds max_input_chars limit"`.
       e. Вызывает `llmProvider.Summarize(ctx, concat)` с retry.
       f. При успехе: создаёт `Summary(postID = nil, sourceID)`.
       g. `status = completed`.
     - Лог: `summary_event_id`, `status`, `duration`.
- **Зависимости.** Шаги 4, 8, 14, 15, 26.
- **Результат.** Summary Worker реализует полный цикл обработки событий обоих типов.
- **Проверка.** `go build ./internal/task/...` — exit 0.

### 29. Cron Worker (cumulative) в `task/`

- **Цель.** Реализовать cron-процесс для автоматической суммаризации cumulative источников (FR-04, BR-02).
- **Действия.** Создать `internal/task/cron_worker.go`:
  1. Тип `CronWorker` с полями: `outboxRepo biz.SummaryOutboxRepo`, `sourceRepo biz.SourceRepo`, `summaryRepo biz.SummaryRepo`, `postRepo biz.PostRepo`, `cfg *conf.Summary`, `log *slog.Logger`, `done chan struct{}`, `wg sync.WaitGroup`.
  2. Конструктор `NewCronWorker(...)`.
  3. Метод `Start(ctx)`:
     - Запускает горутину: `time.Sleep(cfg.Cron.Interval)` → `tick(ctx)`; цикл завершается при отмене контекста или `done`.
  4. Метод `Stop(ctx)` — сигнализирует `done`, ждёт `wg.Wait()`.
  5. Метод `tick(ctx)`:
     - Получает все cumulative источники: `sourceRepo.List(ctx, ListSourcesFilter{ProcessingMode: &ProcessingModeCumulative, PageSize: maxPageSize})` с итерацией по страницам (фильтр по `ProcessingMode` → маппится в `source.TypeEQ("telegram_group")` в репозитории; шаг 5).
     - Для каждого cumulative источника:
       a. `outboxRepo.HasActiveEvent(sourceID, summarize_source)`. Если есть — skip.
       b. Проверяет: есть ли новые посты после последнего саммари. `summaryRepo.GetLastBySource(sourceID)` → если `lastSummary.CreatedAt < now` и есть посты после `lastSummary.CreatedAt` (запрос через `postRepo.List` с временным фильтром из шага 4). Если нет новых постов — skip.
       c. Создаёт `SummaryEvent(summarize_source, sourceID, nil)`, сохраняет.
- **Зависимости.** Шаги 4, 5, 8, 14, 15.
- **Результат.** Cron Worker автоматически создаёт события суммаризации для cumulative источников.
- **Проверка.** `go build ./internal/task/...` — exit 0.

### 30. Тесты Summary Worker и Cron Worker

- **Цель.** Покрыть lifecycle, обработку ошибок и retry (testing-policy: task/ — тесты с моками).
- **Действия.**
  1. `internal/task/summary_worker_test.go`:
     - `TestSummaryWorker_ProcessSummarizePost_Success` — моки: outbox возвращает pending-событие, postRepo возвращает пост, llmProvider возвращает саммари. Проверяет: `summaryRepo.Save` вызван с корректными данными, `outboxRepo.UpdateStatus` вызван с `completed`.
     - `TestSummaryWorker_ProcessSummarizePost_PostNotFound` → `status = failed`.
     - `TestSummaryWorker_ProcessSummarizePost_LLMError_RetryThenFail` — llmProvider возвращает ошибку 3 раза → `status = failed`.
     - `TestSummaryWorker_ProcessSummarizePost_LLMError_RetryThenSuccess` — 2 ошибки, 3-й успех → `completed`.
     - `TestSummaryWorker_ProcessSummarizePost_EmptySummary` — llmProvider возвращает пустую строку → retry → fail.
     - `TestSummaryWorker_ProcessSummarizePost_ExpiredTTL` → `status = expired` (без промежуточного `processing`).
     - `TestSummaryWorker_ProcessSummarizeSource_Success` — моки: загружает посты, конкатенирует, вызывает LLM.
     - `TestSummaryWorker_ProcessSummarizeSource_NoPosts` → `completed` без Summary.
     - `TestSummaryWorker_ProcessSummarizeSource_ExceedsMaxInputChars` → `failed`.
     - `TestSummaryWorker_ProcessSummarizeSource_FirstRun_72hWindow` — нет предыдущего саммари → окно = 72h.
     - `TestSummaryWorker_StartStop` — горутина стартует и останавливается без утечек (`goleak`).
  2. `internal/task/cron_worker_test.go`:
     - `TestCronWorker_Tick_CreatesEventsForCumulativeSources` — sourceRepo возвращает cumulative-источник, нет активного события, есть новые посты → событие создано.
     - `TestCronWorker_Tick_SkipsActiveEvent` — есть pending-событие → skip.
     - `TestCronWorker_Tick_SkipsNoNewPosts` — нет новых постов после последнего саммари → skip.
     - `TestCronWorker_Tick_IgnoresSelfContainedSources` — self-contained источники пропускаются.
     - `TestCronWorker_StartStop` — lifecycle без утечек.
- **Зависимости.** Шаги 9, 28, 29.
- **Результат.** Worker и Cron покрыты unit-тестами с моками.
- **Проверка.** `go test ./internal/task/...` — exit 0.

### 31. Wire graph: подключение всех новых компонентов

- **Цель.** Собрать DI-граф: новая конфигурация, репозитории, провайдеры, usecases, workers, сервисы.
- **Действия.**
  1. Обновить `internal/data/wire.go`: добавить `NewSummaryRepo`, `NewSummaryOutboxRepo`, `NewTxManager`, `NewOpenRouterProvider` + `wire.Bind` для `biz.SummaryRepo`, `biz.SummaryOutboxRepo`, `biz.TxManager`, `biz.LLMProvider`.
  2. Обновить `internal/biz/wire.go`: добавить `NewSummaryUsecase`.
  3. Создать `internal/task/wire.go`: `ProviderSet = wire.NewSet(NewSummaryWorker, NewCronWorker)`.
  4. Обновить `cmd/feedium/wire.go`:
     - Добавить `task.ProviderSet`, `summaryservice.ProviderSet` в `wire.Build`.
     - Добавить `wire.Bind(new(summaryservice.Usecase), new(*biz.SummaryUsecase))` — привязка интерфейса service-слоя к конкретному usecase.
     - Обновить `newApp` — добавить workers как параметры: `newApp(logger *slog.Logger, hs *http.Server, gs *grpc.Server, sw *task.SummaryWorker, cw *task.CronWorker) *kratos.App`.
     - В теле `newApp`: `kratos.Server(hs, gs, sw, cw)` — workers реализуют `transport.Server`, kratos управляет их lifecycle.
  5. Обновить `internal/server/http.go` и `internal/server/grpc.go`: зарегистрировать `SummaryService` (добавить параметр `ss *summary.SummaryService` и вызвать `feedium.RegisterSummaryServiceHTTPServer` / `feedium.RegisterSummaryServiceServer`).
  6. Запустить `make wire`; закоммитить `wire_gen.go`.
- **Зависимости.** Шаги 17, 19, 23, 26, 28, 29.
- **Результат.** Wire-граф включает все новые компоненты; cleanup закрывает соединения.
- **Проверка.** `go build ./...` — exit 0; повторный `make wire` не создаёт diff.

### 32. Финальная верификация

- **Цель.** Закрыть все Acceptance Criteria из `spec.md`.
- **Действия.**
  1. Прогнать `make lint && make build && make test && make migrate` против локальной БД; запустить `make run`; вручную проверить сценарии из Verification ниже.
  2. Обновить документацию memory-bank:
     - В `memory-bank/features/FT-005-ai-summarization/implement.md`: `delivery_status: done`.
     - В `memory-bank/features/FT-005-ai-summarization/brief.md`: `delivery_status: done`.
     - В `memory-bank/index.md`: обновить статус FT-005 — `Delivery: done`.
- **Зависимости.** Все предыдущие шаги.
- **Результат.** Все чекбоксы Acceptance Criteria закрыты; документация актуальна.
- **Проверка.** См. раздел Verification.

## Edge Cases

- **Пост удалён до обработки воркером.** Worker загружает пост по `PostID` → `ErrPostNotFound` → `status = failed`, `error = "post not found"`.
- **Пустой текст от LLM.** `strings.TrimSpace(summaryText) == ""` → считается ошибкой LLM → retry. После исчерпания retry → `status = failed`.
- **Cumulative источник без новых постов в окне.** Worker загружает 0 постов → `status = completed` без создания Summary.
- **Первый запуск cumulative (нет предыдущего саммари).** Окно = последние `max_window` (72h) от текущего момента.
- **Устаревшее событие (TTL истёк).** `created_at + event_ttl < now` → `status = expired` (напрямую из `pending`, без перехода через `processing`), событие пропускается. В лог — `age`.
- **Повторное создание поста (upsert).** `PostRepo.Save` возвращает существующий пост → `SummaryEvent` не создаётся (идемпотентность).
- **LLM-таймаут.** `context.DeadlineExceeded` от LLM-провайдера → retry с exponential backoff.
- **LLM rate limit (429).** Обрабатывается как ошибка LLM → retry.
- **Конкатенированный текст превышает `max_input_chars`.** → `status = failed`, `error = "input text exceeds max_input_chars limit"`.
- **Два конкурентных запроса на ручную суммаризацию одного источника.** Частичный unique-индекс гарантирует: вторая попытка создания pending-события получит `23505`; `HasActiveEvent` вернёт существующее → `200 OK` с `task_id` первого события.
- **Cron и ручной запрос одновременно.** Оба проверяют `HasActiveEvent`; кто первый создал — тот «владелец»; второй получит `HasActiveEvent = true` (cron skip) или `23505` (ручной — вернёт существующий `task_id`).
- **Failed-событие и повторный ручной запрос.** `HasActiveEvent` проверяет только `pending/processing`; `failed` не блокирует → создаётся новый `SummaryEvent`.
- **Cumulative окно > max_window (72h).** Обрезается до `max_window` от текущего момента; `windowStart = max(lastSummary.CreatedAt, now - maxWindow)`.
- **Событие в статусе `processing` при падении воркера.** Воркер не завершил `UpdateStatus` → событие остаётся `processing`. Polling `ListPending` не вернёт его (только `pending`). Recovery вынесен в отдельную задачу (не блокирует MVP).
- **Пустой список саммари для поста/источника.** API возвращает `200 OK` с `summaries: []`.
- **Невалидный UUID в path-параметре.** Service-слой возвращает `400 InvalidArgument`.
- **Конфигурация LLM-провайдера отсутствует.** `NewOpenRouterProvider` возвращает ошибку → fail-fast при старте (wire phase).

## Verification

1. `make lint` — exit 0.
2. `make build` — exit 0; бинарь `bin/feedium` создан.
3. `make test` — exit 0 (все unit и integration тесты).
4. `make migrate` на БД с существующими таблицами — exit 0; таблицы `summaries` и `summary_events` созданы.
5. `make run` — стартует без ошибок; в логе видны сообщения о запуске Summary Worker и Cron Worker.
6. Создать self-contained источник (telegram_channel) → создать пост через `POST /v1/posts` → в БД появляется `Post` и `SummaryEvent(status=pending)`.
7. Дождаться обработки (worker poll) → `SummaryEvent.status = completed`, `Summary` создан с текстом.
8. `GET /v1/posts/{post_id}/summaries` — возвращает список с одним саммари.
9. `GET /v1/summaries/{summary_id}` — возвращает саммари.
10. Создать cumulative источник (telegram_group) → создать несколько постов → `GET /v1/sources/{source_id}/summaries` — пустой список.
11. `POST /v1/sources/{source_id}/summarize` — возвращает `202 Accepted` с `task_id`.
12. `GET /v1/summary-events/{task_id}` — показывает `pending` → `processing` → `completed`.
13. Повторный `POST /v1/sources/{source_id}/summarize` при активном событии — возвращает `200 OK` с существующим `task_id`.
14. `POST /v1/sources/{self_contained_source_id}/summarize` — возвращает `400` с reason `SUMMARIZE_SELF_CONTAINED_SOURCE`.
15. Удалить пост до обработки → `SummaryEvent.status = failed`, `error = "post not found"`.
16. Остановить LLM-провайдер (или вернуть ошибку) → после 3 retry → `SummaryEvent.status = failed`.
17. Cumulative источник без новых постов → `SummaryEvent.status = completed` без Summary.
18. После `failed`-события → повторный `POST .../summarize` создаёт новый SummaryEvent.
19. Чек-лист Acceptance Criteria `spec.md` — все пункты закрыты.

## Open Questions

Нет. Все вопросы разрешены (см. Resolved Questions ниже).

## Resolved Questions

1. ~~**Фильтр по временному диапазону в `ListPostsFilter`**~~ → Решение: расширить `ListPostsFilter` полями `CreatedAfter`/`CreatedBefore` (шаг 4).

2. ~~**Фильтр по `ProcessingMode` в `ListSourcesFilter`**~~ → Решение: добавить поле `ProcessingMode` в `ListSourcesFilter`; в `source_repo.List` маппить `ProcessingMode` в предикат по `source.Type` (без добавления колонки в БД). `ProcessingMode` остаётся вычисляемым значением (SSoT) (шаг 5).

3. ~~**Отличие нового поста от upsert в `PostRepo.Save`**~~ → Решение: изменить сигнатуру на `(Post, bool, error)` с флагом `created` (шаг 6).

4. ~~**Recovery зависших `processing`-событий**~~ → Решение: вынести в отдельную задачу, не блокирующую MVP. Оставить на будущее. Риск задокументирован в Edge Cases.

5. ~~**Retry-стратегия в worker**~~ → Решение: in-memory retry внутри `processEvent` (exponential backoff, до `max_retries` попыток). При краше retry начнётся заново.

6. ~~**Формат ответа `POST /v1/sources/{source_id}/summarize`**~~ → Решение: proto field `existing` для различения `200` (существующее событие) от `202` (новое).

7. ~~**LLM-промпт**~~ → Решение: фиксированный промпт как string-константа в `data/llm_provider.go`. Требования к промпту: (1) саммари на том же языке что оригинал, (2) сохранить ключевые факты, имена, числа, (3) concise summary. Конкретная формулировка — на шаге 26 при написании кода.

8. ~~**Критерий качества саммари (NFR-02)**~~ → Решение: вынести в отдельную задачу после MVP. Не блокирует реализацию. Минимальный automated gate: текст не пустой (валидация в worker), `word_count > 0`.

9. ~~**Фильтрация по `ProcessingMode` без колонки в БД**~~ → Решение: `source_repo.List` маппит `ProcessingModeCumulative` → `source.TypeEQ("telegram_group")`, `ProcessingModeSelfContained` → `source.TypeIN("telegram_channel", "rss", "html")`. SSoT сохраняется: `ProcessingMode` вычисляется из типа, БД не дублирует логику.

10. ~~**Порядок TTL-проверки в worker**~~ → Решение: TTL проверяется до установки `processing`. Устаревшие события переходят `pending → expired` напрямую, минуя `processing`. Это снижает риск зависания событий в `processing` при краше воркера (шаг 28).
