# Implementation Plan — Spec 007a: Core Processing (Outbox + Worker + Summary)

## Dependency
- Spec: `memory-bank/features/007/spec-007a.md`
- Brief: `memory-bank/features/007/brief.md`
- Существующие модули: `internal/app/source`, `internal/app/post`
- Существующие миграции: `migrations/001_create_sources.sql`, `migrations/002_create_posts.sql`

---

## Steps

### Step 1: Миграция — создание таблиц outbox_events, summaries, summary_posts

**Цель:** Создать схему данных для всего пайплайна обработки.

**Действия:**
1. Создать файл `migrations/003_create_outbox_events.sql` с goose-форматом (`-- +goose Up` / `-- +goose Down`)
2. В `Up`:
   - `CREATE TABLE outbox_events` (id, source_id FK→sources, post_id FK→posts NULL, event_type VARCHAR(20), status VARCHAR(20) DEFAULT 'PENDING', retry_count INTEGER DEFAULT 0, scheduled_at TIMESTAMPTZ NULL, created_at TIMESTAMPTZ DEFAULT NOW())
   - `CREATE INDEX idx_outbox_events_status_scheduled ON outbox_events(status, scheduled_at, created_at)`
   - `CREATE TABLE summaries` (id, source_id FK→sources, event_id FK→outbox_events, content TEXT NOT NULL, created_at TIMESTAMPTZ DEFAULT NOW())
   - `CREATE INDEX idx_summaries_source_id`, `CREATE INDEX idx_summaries_event_id`
   - `CREATE TABLE summary_posts` (id, summary_id FK→summaries ON DELETE CASCADE, post_id FK→posts ON DELETE CASCADE, UNIQUE(summary_id, post_id))
   - `CREATE INDEX idx_summary_posts_summary_id`, `CREATE INDEX idx_summary_posts_post_id`
3. В `Down`: DROP TABLE в обратном порядке (summary_posts → summaries → outbox_events)

**Зависимости:** Миграции 001, 002 (таблицы sources, posts должны существовать)

**Результат:** Три новые таблицы в БД с индексами и FK constraints.

**Проверка:** `go run ./cmd/feedium run migrate` проходит без ошибок. Таблицы видны в БД. Rollback (`goose down`) удаляет таблицы без ошибок.

---

### Step 2: Domain-модели — OutboxEvent, Summary, ProcessingMode

**Цель:** Определить доменные типы для outbox-событий, summary и маппинг SourceType → ProcessingMode.

**Действия:**
1. Создать пакет `internal/app/summary`
2. Создать файл `internal/app/summary/outbox_event.go`:
   - Тип `EventType string` с константами: `EventTypeImmediate`, `EventTypeScheduled`, `EventTypeManual`
   - Тип `EventStatus string` с константами: `EventStatusPending`, `EventStatusProcessing`, `EventStatusCompleted`, `EventStatusFailed`
   - Структура `OutboxEvent`: ID (uuid), SourceID (uuid), PostID (*uuid — nullable), EventType, Status, RetryCount (int), ScheduledAt (*time.Time — nullable), CreatedAt (time.Time)
3. Создать файл `internal/app/summary/summary.go`:
   - Структура `Summary`: ID (uuid), SourceID (uuid), EventID (uuid), Content (string), CreatedAt (time.Time)
4. Создать файл `internal/app/summary/processing_mode.go`:
   - Тип `ProcessingMode string` с константами: `ModeSelfContained`, `ModeCumulative`
   - Функция `ProcessingModeForSourceType(sourceType source.Type) (ProcessingMode, error)` — хардкод маппинг:
     - `TypeTelegramChannel` → `ModeSelfContained`
     - `TypeRSS` → `ModeSelfContained`
     - `TypeWebScraping` → `ModeSelfContained`
     - `TypeTelegramGroup` → `ModeCumulative`
     - Неизвестный тип → error (permanent error)

**Зависимости:** Пакет `internal/app/source` (для типа `source.Type`)

**Результат:** Доменные типы, маппинг ProcessingMode, валидация типов.

**Проверка:** `go build ./internal/app/summary/...` компилируется. Unit-тест на `ProcessingModeForSourceType` — все 4 типа возвращают корректный mode, неизвестный тип возвращает ошибку.

---

### Step 3: Processor Interface

**Цель:** Определить контракт процессора, который Worker будет вызывать.

**Действия:**
1. Создать файл `internal/app/summary/processor.go`:
   - Интерфейс `Processor`:
     ```
     Process(ctx context.Context, posts []post.Post) (content string, err error)
     ```
2. Добавить `go:generate mockgen` директиву для генерации мока

**Зависимости:** Пакет `internal/app/post` (для типа `post.Post`)

**Результат:** Интерфейс `Processor` + сгенерированный мок.

**Проверка:** `go generate ./internal/app/summary/...` генерирует мок без ошибок. `go build ./internal/app/summary/...` компилируется.

---

### Step 4: Repository Interface — OutboxEventRepository, SummaryRepository, PostQueryRepository, SourceQueryRepository

**Цель:** Определить интерфейсы репозиториев для outbox-событий, summary, постов и источников. Все интерфейсы определяются в пакете `summary` (где используются), а не в пакетах `post`/`source`.

**Действия:**
1. Создать файл `internal/app/summary/repository.go`:
   - Интерфейс `OutboxEventRepository`:
     - `FetchAndLockPending(ctx context.Context) (*OutboxEvent, time.Time, error)` — SELECT FOR UPDATE SKIP LOCKED, возвращает событие + lock_acquired_at timestamp. Фильтр: status=PENDING, scheduled_at IS NULL OR scheduled_at <= NOW(), ORDER BY created_at ASC, LIMIT 1. Возвращает nil, если нет событий.
     - `UpdateStatus(ctx context.Context, id uuid.UUID, status EventStatus, incrementRetry bool) error` — обновляет status и опционально retry_count+1
   - Интерфейс `SummaryRepository`:
     - `Create(ctx context.Context, summary *Summary, postIDs []uuid.UUID) error` — INSERT summary + INSERT summary_posts для всех postIDs в одной транзакции
   - Интерфейс `PostQueryRepository`:
     - `GetByID(ctx context.Context, id uuid.UUID) (*post.Post, error)` — получить пост по ID
     - `FindUnprocessedBySource(ctx context.Context, sourceID uuid.UUID, since time.Time) ([]post.Post, error)` — посты source за период, у которых нет записи в summary_posts
   - Интерфейс `SourceQueryRepository`:
     - `GetByID(ctx context.Context, id uuid.UUID) (*source.Source, error)` — получить source по ID (для определения ProcessingMode)
2. Добавить `go:generate mockgen` директивы для всех четырёх интерфейсов

**Зависимости:** Step 2 (доменные типы), пакеты `post` и `source` (для типов `post.Post`, `source.Source`)

**Результат:** Четыре интерфейса репозиториев в пакете `summary` + сгенерированные моки.

**Проверка:** `go generate` + `go build` проходят. Моки сгенерированы.

---

### Step 5: Postgres-адаптер — OutboxEventRepository

**Цель:** Реализовать DB-уровень для outbox-событий с SELECT FOR UPDATE SKIP LOCKED.

**Действия:**
1. Создать `internal/app/summary/adapters/postgres/outbox_event_repository.go`:
   - Структура с полем `*gorm.DB`
   - `FetchAndLockPending`: одна транзакция (TX1):
     ```sql
     SELECT id, source_id, post_id, event_type, status, retry_count, scheduled_at, created_at
     FROM outbox_events
     WHERE status = 'PENDING'
       AND (scheduled_at IS NULL OR scheduled_at <= NOW())
     ORDER BY created_at ASC
     LIMIT 1
     FOR UPDATE SKIP LOCKED
     ```
     Затем в той же транзакции: `UPDATE outbox_events SET status='PROCESSING' WHERE id = :id`
     COMMIT — лок отпускается. Возвращает lock_acquired_at = NOW() на момент SELECT.
     Если строк нет — возвращает nil, nil (без ошибки).
     Дальнейшая обработка (Processor, создание Summary) происходит вне этой транзакции.
   - `UpdateStatus`: UPDATE status, опционально retry_count = retry_count + 1
2. Unit-тест с go-mocket: проверить SQL-запросы, параметры, обработку пустого результата

**Зависимости:** Step 1 (миграция), Step 4 (интерфейс)

**Результат:** Реализация OutboxEventRepository с DB-level locking.

**Проверка:** Unit-тесты с go-mocket проходят. Тест на пустой результат (нет PENDING событий) возвращает nil без ошибки.

---

### Step 6: Postgres-адаптер — SummaryRepository

**Цель:** Реализовать создание Summary + связей summary_posts в одной транзакции.

**Действия:**
1. Создать `internal/app/summary/adapters/postgres/summary_repository.go`:
   - `Create`: в транзакции:
     1. INSERT INTO summaries → получить ID
     2. Для каждого postID: INSERT INTO summary_posts (summary_id, post_id)
     3. Commit
2. Unit-тест: проверить INSERT-ы, транзакционность, поведение при пустом postIDs (edge case — не должно падать, но и не должно вызываться в проде для self-contained)

**Зависимости:** Step 1 (миграция), Step 4 (интерфейс)

**Результат:** Реализация SummaryRepository.

**Проверка:** Unit-тесты проходят. Проверены SQL-запросы и транзакционное поведение.

---

### Step 7: Postgres-адаптер — PostQueryRepository

**Цель:** Реализовать получение постов для Worker (по ID и unprocessed за период).

**Действия:**
1. Создать `internal/app/summary/adapters/postgres/post_query_repository.go`:
   - `GetByID`: SELECT * FROM posts WHERE id = :id
   - `FindUnprocessedBySource`:
     ```sql
     SELECT p.* FROM posts p
     WHERE p.source_id = :source_id
       AND p.created_at >= :since
       AND NOT EXISTS (
         SELECT 1 FROM summary_posts sp WHERE sp.post_id = p.id
       )
     ORDER BY p.created_at ASC
     ```
2. Unit-тесты с go-mocket

**Зависимости:** Step 1 (миграция), Step 4 (интерфейс)

**Результат:** Реализация PostQueryRepository.

**Проверка:** Unit-тесты проходят. Проверено, что NOT EXISTS корректно фильтрует уже обработанные посты.

---

### Step 7b: Postgres-адаптер — SourceQueryRepository

**Цель:** Реализовать получение Source для Worker (определение ProcessingMode).

**Действия:**
1. Создать `internal/app/summary/adapters/postgres/source_query_repository.go`:
   - Структура с полем `*gorm.DB`
   - `GetByID`: SELECT * FROM sources WHERE id = :id
2. Unit-тест с go-mocket: source найден / не найден

**Зависимости:** Step 4 (интерфейс SourceQueryRepository)

**Результат:** Реализация SourceQueryRepository.

**Проверка:** Unit-тесты проходят.

---

### Step 8: Worker — основная логика обработки

**Цель:** Реализовать Worker, который забирает PENDING события и обрабатывает их.

**Действия:**
1. Создать файл `internal/app/summary/worker.go`:
   - Структура `Worker` с зависимостями: OutboxEventRepository, SummaryRepository, PostQueryRepository, SourceQueryRepository, Processor, slog.Logger. Все интерфейсы определены в пакете `summary` (где используются).
   - Метод `ProcessNext(ctx context.Context) (bool, error)`:
     1. Вызвать `OutboxEventRepository.FetchAndLockPending()` → event, lockTime
     2. Если event == nil → return false, nil (нет событий)
     3. Получить Source по event.SourceID через SourceQueryRepository.GetByID
     4. Определить ProcessingMode через `ProcessingModeForSourceType(source.Type)`
     5. Если ошибка маппинга → UpdateStatus(FAILED, incrementRetry=false), залогировать, return true, nil
     6. В зависимости от mode вызвать `processSelfContained` или `processCumulative`
     7. return true, nil
   - Приватный метод `processSelfContained(ctx, event, lockTime)`:
     1. Получить Post по event.PostID через PostQueryRepository.GetByID
     2. Если пост не найден → UpdateStatus(FAILED, incrementRetry=false), залогировать "Post not found, skipping summary"
     3. Вызвать Processor.Process([]Post{post})
     4. Если ошибка → UpdateStatus(FAILED, incrementRetry=true), залогировать
     5. Создать Summary через SummaryRepository.Create(summary, []uuid.UUID{post.ID})
     6. Если DB constraint violation → UpdateStatus(FAILED, incrementRetry=true), залогировать
     7. UpdateStatus(COMPLETED, incrementRetry=false)
   - Приватный метод `processCumulative(ctx, event, lockTime)`:
     1. Вызвать PostQueryRepository.FindUnprocessedBySource(event.SourceID, lockTime - 24h)
     2. Если постов 0 → UpdateStatus(COMPLETED, incrementRetry=false), return (Summary НЕ создаётся)
     3. Вызвать Processor.Process(posts)
     4. Если ошибка → UpdateStatus(FAILED, incrementRetry=true), залогировать
     5. Создать Summary через SummaryRepository.Create(summary, postIDs)
     6. Если DB constraint violation → UpdateStatus(FAILED, incrementRetry=true), залогировать
     7. UpdateStatus(COMPLETED, incrementRetry=false)

**Зависимости:** Steps 2–4 (модели, интерфейсы), Step 3 (Processor interface)

**Результат:** Worker с полной логикой обработки self-contained и cumulative событий.

**Проверка:** `go build ./internal/app/summary/...` компилируется.

---

### Step 9: Ошибки — доменные типы ошибок

**Цель:** Определить типы ошибок для различения permanent vs transient errors в Worker.

**Действия:**
1. Создать файл `internal/app/summary/errors.go`:
   - `ErrPostNotFound` — sentinel error для случая, когда пост удалён
   - `ErrUnknownSourceType` — sentinel error для неизвестного SourceType
   - Вспомогательная функция для определения, нужно ли инкрементировать retry_count (permanent error = no increment)

**Зависимости:** Нет

**Результат:** Типизированные ошибки для Worker.

**Проверка:** Используются в Worker (Step 8) для принятия решений о retry_count.

---

### Step 10: Unit-тесты Worker

**Цель:** Покрыть всю логику Worker unit-тестами с моками.

**Действия:**
1. Создать файл `internal/app/summary/worker_test.go` и/или `worker_internal_test.go`:
   - **Self-contained happy path**: PENDING event → Post найден → Processor.Process OK → Summary создан → status=COMPLETED
   - **Cumulative happy path**: PENDING event → 3 необработанных поста → Processor.Process OK → Summary + 3 summary_posts → status=COMPLETED
   - **Cumulative без постов**: PENDING event → 0 постов → Summary НЕ создан → status=COMPLETED
   - **Post not found (self-contained)**: PostQueryRepository.GetByID → error → status=FAILED, retry_count НЕ изменён
   - **Processor error (self-contained)**: Processor.Process → error → status=FAILED, retry_count+1, Summary НЕ создан
   - **Processor error (cumulative)**: Processor.Process → error → status=FAILED, retry_count+1, Summary НЕ создан
   - **DB constraint violation**: SummaryRepository.Create → error → status=FAILED, retry_count+1
   - **Unknown SourceType**: ProcessingModeForSourceType → error → status=FAILED, retry_count НЕ изменён
   - **Source not found**: SourceQueryRepository.GetByID → error → status=FAILED, retry_count НЕ изменён (permanent error)
   - **No pending events**: FetchAndLockPending → nil → ProcessNext returns false
   - **Scheduled event в будущем**: Не возвращается из FetchAndLockPending (проверяется на уровне repo, но тест Worker должен подтвердить, что FetchAndLockPending вызывается без параметров времени)
2. Использовать сгенерированные моки (go.uber.org/mock)

**Зависимости:** Steps 2–4, 8, 9

**Результат:** Полное покрытие Worker-логики. Coverage > 80%.

**Проверка:** `go test ./internal/app/summary/... -cover` — все тесты проходят, coverage > 80%.

---

### Step 11: Unit-тесты Postgres-адаптеров

**Цель:** Покрыть postgres-адаптеры unit-тестами с go-mocket.

**Действия:**
1. `internal/app/summary/adapters/postgres/outbox_event_repository_internal_test.go`:
   - FetchAndLockPending: есть PENDING событие → возвращает event + lockTime
   - FetchAndLockPending: нет событий → nil, nil
   - UpdateStatus: с incrementRetry=true → retry_count + 1
   - UpdateStatus: с incrementRetry=false → retry_count не меняется
2. `internal/app/summary/adapters/postgres/summary_repository_internal_test.go`:
   - Create: вставка summary + summary_posts
3. `internal/app/summary/adapters/postgres/post_query_repository_internal_test.go`:
   - GetByID: пост найден / не найден
   - FindUnprocessedBySource: фильтрация по source_id, since, NOT EXISTS summary_posts
4. `internal/app/summary/adapters/postgres/source_query_repository_internal_test.go`:
   - GetByID: source найден / не найден

**Зависимости:** Steps 5–7b

**Результат:** Unit-тесты всех postgres-адаптеров.

**Проверка:** `go test ./internal/app/summary/adapters/postgres/... -cover` — все тесты проходят.

---

### Step 12: Wiring — регистрация Worker в bootstrap

**Цель:** Подключить Worker к lifecycle приложения.

**Действия:**
1. В `internal/bootstrap/bootstrap.go`:
   - Создать экземпляры postgres-адаптеров (OutboxEventRepository, SummaryRepository, PostQueryRepository, SourceQueryRepository)
   - Создать stub-реализацию Processor (возвращает `"stub: processing not implemented"`)
   - Создать Worker с зависимостями
   - Запустить Worker в горутине: цикл, вызывающий `ProcessNext()` с интервалом 1 секунда между итерациями когда нет событий, без задержки если событие обработано
2. Graceful shutdown: Worker проверяет `ctx.Done()` перед новой итерацией. Если в процессе обработки — дожидается завершения (context с timeout 30 секунд)

**Зависимости:** Steps 5–8, существующий bootstrap

**Результат:** Worker запускается вместе с приложением и обрабатывает события.

**Проверка:** Приложение запускается без ошибок (`go run ./cmd/feedium`). В логах видно, что Worker работает.

---

## Edge Cases

1. **Нет необработанных постов за 24h (Cumulative):** FindUnprocessedBySource возвращает пустой slice → Summary НЕ создаётся, event.status → COMPLETED. Worker не падает, не логирует ошибку.

2. **Невалидный SourceType:** ProcessingModeForSourceType возвращает ошибку → event.status → FAILED, retry_count НЕ инкрементируется (permanent error). Логируется как ошибка.

3. **Пост удалён после создания OutboxEvent (Self-contained):** PostQueryRepository.GetByID возвращает "not found" → event.status → FAILED, retry_count НЕ инкрементируется. Лог: "Post not found, skipping summary".

4. **Worker crash во время PROCESSING:** Event остаётся в статусе PROCESSING навсегда. Это известное ограничение MVP (нет heartbeat/timeout recovery). Не реализуем обработку.

5. **Concurrent workers:** SELECT FOR UPDATE SKIP LOCKED гарантирует, что два воркера не захватят одно событие. Если один воркер держит lock, другой пропускает это событие и берёт следующее.

6. **Processor возвращает пустой content:** Спека не запрещает пустой content, но summaries.content имеет NOT NULL constraint. Пустая строка (`""`) допустима на уровне DB. Processor отвечает за content — Worker передаёт как есть.

7. **event.PostID = NULL для self-contained события:** Инвариант спеки: post_id NOT NULL для self-contained. Если нарушен — GetByID с nil UUID. Worker должен обработать как ошибку (FAILED, permanent).

8. **Все PENDING события имеют scheduled_at в будущем:** FetchAndLockPending возвращает nil → ProcessNext возвращает false. Worker засыпает до следующей итерации.

9. **DB constraint violation при создании summary_posts (duplicate):** UNIQUE(summary_id, post_id) может сработать при повторной обработке. Status → FAILED, retry_count + 1.

10. **Source удалён после создания OutboxEvent:** SourceRepository.GetByID вернёт ошибку → Worker должен обработать как FAILED, retry_count НЕ инкрементируется (permanent, retry не поможет).

---

## Verification

### Автоматическая проверка
1. `go build ./...` — проект компилируется без ошибок
2. `go vet ./...` — нет подозрительных конструкций
3. `golangci-lint run ./... -c .golangci.yml` — линтер проходит
4. `go test ./internal/app/summary/... -cover` — все тесты проходят, coverage > 80%
5. `go test ./internal/app/summary/adapters/postgres/... -cover` — все тесты адаптеров проходят
6. `go run ./cmd/feedium run migrate` — миграция проходит без ошибок

### Ручная проверка
1. Вставить вручную PENDING OutboxEvent в БД (self-contained, с существующим post_id)
2. Запустить приложение → Worker подхватывает событие → Summary создан → event.status = COMPLETED
3. Проверить tracability: summary.event_id указывает на обработанный event
4. Проверить summary_posts: запись связывает summary с post
5. Вставить PENDING Cumulative event → Worker подхватывает → проверить что все unprocessed посты source попали в summary_posts
6. Вставить PENDING event с несуществующим post_id → event.status = FAILED, retry_count = 0

### Проверка инвариантов
- Processing Mode определяется по Source.type — зашит в маппинг
- Worker использует SELECT FOR UPDATE SKIP LOCKED — видно в SQL логах или коде адаптера
- Каждый Summary имеет event_id NOT NULL — гарантировано DB constraint
- summary_posts имеет UNIQUE(summary_id, post_id) — гарантировано DB constraint

---

## Open Questions

Все вопросы решены. Ниже — принятые решения для справки.

### Решено: Stub Processor content
**Решение:** `"stub: processing not implemented"`. Временная заглушка до реализации реального Processor в отдельной спеке.

### Решено: Worker polling interval
**Решение:** 1 секунда между итерациями, когда нет событий. Если событие найдено — без задержки переходить к следующему.

### Решено: Транзакционность FetchAndLockPending + обработка
**Решение:** Две отдельные транзакции:
1. **TX1**: SELECT FOR UPDATE SKIP LOCKED + UPDATE status='PROCESSING' → COMMIT (лок отпускается)
2. **Вне TX**: вызов Processor.Process (может быть долгим, держать транзакцию открытой нельзя)
3. **TX2**: INSERT summary + summary_posts + UPDATE status='COMPLETED' → COMMIT

### Решено: Graceful shutdown
**Решение:** Worker проверяет `ctx.Done()` перед началом новой итерации. Если уже в процессе обработки — дожидается завершения (context с timeout 30 секунд). Если не успел — event останется PROCESSING (MVP ограничение).

### Решено: PostQueryRepository vs существующий post.Repository
**Решение:** Отдельный интерфейс `PostQueryRepository` определяется в пакете `summary` (где используется), а не в пакете `post`. Согласно конвенции проекта: интерфейс описывается там, где используется, а не где реализуется. Postgres-адаптер использует тот же `*gorm.DB`.

### Решено: Source не найден
**Решение:** status='FAILED', retry_count НЕ инкрементируется (permanent error, аналогично "Post not found").

### Решено: TIMESTAMPTZ vs TIMESTAMP
**Решение:** Использовать `TIMESTAMPTZ` в новой миграции для консистентности с существующими миграциями (001, 002). Спека DDL — концептуальная схема.
