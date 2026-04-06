# Implementation Plan: Spec 007b — Orchestration (Triggers & API)

## Prerequisites

- spec-007a полностью реализована: таблицы `outbox_events`, `summaries`, `summary_posts` существуют (миграция `003`), Worker работает, модели и репозитории в `internal/app/summary/` готовы
- Пакеты `internal/app/post`, `internal/app/source` существуют с полными CRUD-сервисами
- Bootstrap (`internal/bootstrap/bootstrap.go`) собирает зависимости и запускает HTTP-сервер + worker

---

## Steps

### Step 1: Расширить OutboxEventRepository — метод создания событий

**Цель**: Добавить методы записи OutboxEvent, которых сейчас нет в интерфейсе (текущий интерфейс имеет только `FetchAndLockPending` и `UpdateStatus`).

**Действия**:
1. В `internal/app/summary/repository.go` добавить в интерфейс `OutboxEventRepository` два метода:
   - `Create(ctx, event *OutboxEvent) error` — создаёт одно событие, возвращает заполненный `ID`
   - `CreateScheduledForType(ctx, sourceType source.Type, scheduledAt time.Time) (int, error)` — multi-insert для всех sources указанного типа, возвращает количество созданных событий
2. В `internal/app/summary/adapters/postgres/outbox_event_repository.go` реализовать оба метода:
   - `Create`: обычный GORM `db.Create(&event)`
   - `CreateScheduledForType`: raw SQL multi-insert (как в спеке):
     ```sql
     INSERT INTO outbox_events (source_id, post_id, event_type, status, scheduled_at)
     SELECT id, NULL, 'SCHEDULED', 'PENDING', $1
     FROM sources WHERE type = $2
     ```
     Возвращает `result.RowsAffected`
3. Перегенерировать моки: `go generate ./internal/app/summary/...`

**Зависимости**: нет

**Результат**: Интерфейс `OutboxEventRepository` поддерживает создание единичных и batch-событий.

**Проверка**:
- `go build ./...` компилируется
- Unit-тест `outbox_event_repository` с go-mocket: `Create` записывает событие, `CreateScheduledForType` выполняет multi-insert

---

### Step 2: Post-save trigger — Transactional Outbox через `CreateWithOutbox`

**Цель**: При `post.Service.Create` и `post.Service.Update` для self-contained источника создавать OutboxEvent с `post_id` в той же транзакции (transactional outbox pattern).

**Действия**:
1. Расширить `post.Repository` интерфейс — добавить метод `CreateWithOutbox`:
   ```go
   type Repository interface {
       Create(context.Context, *Post) error
       CreateWithOutbox(ctx context.Context, post *Post, createEventFn func(postID uuid.UUID) (*OutboxEvent, error)) error
       // ... остальные методы
   }
   ```
   - `createEventFn` — callback, который получает `postID` (после INSERT) и возвращает `*OutboxEvent` или `nil` (если event не нужен)
   - Реализация в `postgres.Repository`: начать транзакцию, создать post, вызвать `createEventFn`, если вернул не-nil — создать event в той же транзакции, commit

2. Расширить `post.Service`:
   - Добавить поле `outboxBuilder func(post *Post) (*OutboxEvent, error)` (не зависит от summary-пакета)
   - Добавить метод `SetOutboxBuilder(fn func(post *Post) (*OutboxEvent, error))` для регистрации builder-функции
   - В `Create`: если `outboxBuilder != nil` — вызвать `repo.CreateWithOutbox`, иначе обычный `repo.Create`
   - В `Update`: аналогично

3. Создать `OutboxBuilder` в пакете `summary`:
   - В `internal/app/summary/post_outbox_builder.go` создать функцию:
     ```go
     func NewOutboxBuilder(sourceQueryRepo SourceQueryRepository) func(*post.Post) (*OutboxEvent, error) {
         return func(p *post.Post) (*OutboxEvent, error) {
             source, err := sourceQueryRepo.GetByID(context.Background(), p.SourceID)
             if err != nil { return nil, err }
             mode, err := ProcessingModeForSourceType(source.Type)
             if err != nil { return nil, err }
             if mode == ModeCumulative { return nil, nil } // no event
             return &OutboxEvent{
                 SourceID: p.SourceID,
                 PostID:   &p.ID,
                 EventType: EventTypeImmediate,
                 Status:    StatusPending,
             }, nil
         }
     }
     ```

**Зависимости**: Step 1 (метод `Create` в `OutboxEventRepository` для использования внутри транзакции)

**Результат**: `CreateWithOutbox` гарантирует атомарность: либо обе записи (post + event), либо ни одной. Self-contained пост создаёт IMMEDIATE event, cumulative — нет.

**Проверка**:
- Unit-тест `postgres.Repository.CreateWithOutbox`: транзакция commit при успехе, rollback при ошибке `createEventFn`
- Unit-тест `OutboxBuilder`: self-contained source → event, cumulative source → nil, неизвестный тип → ошибка
- Unit-тест `post.Service`: с зарегистрированным builder вызывается `CreateWithOutbox`
- Инвариант 3: один `Create`/`Update` создаёт максимум один OutboxEvent
- Инвариант 4: IMMEDIATE event имеет `post_id` NOT NULL

---

### Step 3: Cron scheduler для TELEGRAM_GROUP

**Цель**: Запускать по расписанию (00:00, 12:00 UTC) создание SCHEDULED OutboxEvent для всех TELEGRAM_GROUP sources.

**Действия**:
1. Создать `internal/app/summary/scheduler.go`:
   - Структура `Scheduler` с зависимостями: `OutboxEventRepository`, `*slog.Logger`
   - Метод `RunScheduled(ctx context.Context) error`:
     a. Вызвать `OutboxEventRepository.CreateScheduledForType(ctx, source.TypeTelegramGroup, time.Now())`
     b. Логировать количество созданных событий
     c. При ошибке — логировать и вернуть ошибку
   - Метод `Start(ctx context.Context)` — запускает горутину с тикером:
     a. Вычислить время до следующего 00:00 или 12:00 UTC
     b. `time.NewTimer` до следующего запуска
     c. После срабатывания — вызвать `RunScheduled`, затем вычислить следующий интервал
     d. При отмене `ctx` — остановиться
   - Вспомогательная функция `nextScheduleTime(now time.Time) time.Time`:
     a. Фиксированные точки: 00:00 UTC, 12:00 UTC
     b. Найти ближайшую будущую точку от `now`

**Зависимости**: Step 1 (метод `CreateScheduledForType`)

**Результат**: Scheduler создаёт batch-события для всех TELEGRAM_GROUP sources по расписанию.

**Проверка**:
- Unit-тест `nextScheduleTime`: проверить для разных `now` (01:00 → 12:00, 13:00 → 00:00 следующего дня, 00:00 ровно → 12:00, 23:59 → 00:00)
- Unit-тест `RunScheduled`: мокаем `OutboxEventRepository.CreateScheduledForType` — проверяем вызов с `source.TypeTelegramGroup`
- Инвариант 2: cron НЕ создаёт событий для non-TELEGRAM_GROUP sources (обеспечивается SQL WHERE type = 'telegram_group')

---

### Step 4: API endpoint — ProcessSource (Connect-RPC)

**Цель**: RPC endpoint для ручного запуска обработки source.

**Действия**:
1. Добавить метод в proto (например, в `source.proto` или новый `summary.proto`):
   ```protobuf
   service SourceService {
       // ... existing methods ...
       rpc ProcessSource(ProcessSourceRequest) returns (ProcessSourceResponse);
   }
   message ProcessSourceRequest {
       string id = 1;  // source UUID
   }
   message ProcessSourceResponse {
       string event_id = 1;
   }
   ```
2. Сгенерировать код: `./scripts/gen-proto.sh`
3. Создать `internal/app/summary/adapters/connectrpc/handler.go`:
   - Структура `Handler` с зависимостями: `OutboxEventRepository`, `SourceQueryRepository`, `*slog.Logger`
   - Метод `ProcessSource(ctx, req) (*connect.Response[ProcessSourceResponse], error)`:
     a. Валидировать UUID: `uuid.Parse(req.Msg.Id)` — при ошибке вернуть `connect.NewError(connect.CodeInvalidArgument, err)`
     b. Проверить существование source: `SourceQueryRepository.GetByID(ctx, id)` — при `ErrNotFound` вернуть `connect.NewError(connect.CodeNotFound, err)`
     c. Создать OutboxEvent: `OutboxEventRepository.Create(ctx, &OutboxEvent{SourceID: id, PostID: nil, EventType: MANUAL, Status: PENDING, ScheduledAt: nil})`
     d. Вернуть `&ProcessSourceResponse{EventId: event.ID.String()}`

**Зависимости**: Step 1 (метод `Create` в `OutboxEventRepository`)

**Результат**: RPC `ProcessSource` создаёт MANUAL OutboxEvent и возвращает 202 (Connect-RPC статус OK).

**Проверка**:
- Unit-тест: валидный UUID существующего source → success + event_id в ответе
- Unit-тест: невалидный UUID → CodeInvalidArgument
- Unit-тест: source не найден → CodeNotFound
- Инвариант 4: MANUAL event имеет `post_id` NULL
- Инвариант 5: событие создаётся независимо от наличия постов

---

### Step 5: Регистрация компонентов в Bootstrap

**Цель**: Связать все новые компоненты в `internal/bootstrap/bootstrap.go`.

**Действия**:
1. Создать `OutboxBuilder` и зарегистрировать в `post.Service`:
   ```
   outboxBuilder := summarysvc.NewOutboxBuilder(sourceQueryRepo)
   postService.SetOutboxBuilder(outboxBuilder)
   ```
2. Создать `Scheduler` и запустить его:
   ```
   scheduler := summarysvc.NewScheduler(outboxEventRepo, log)
   go scheduler.Start(ctx)
   ```
3. Зарегистрировать Connect-RPC handler:
   - Handler уже реализует generated интерфейс, подключить к существующему `SourceServiceHandler` или отдельному `SummaryServiceHandler`
   - Добавить в `connectrpc.NewSourceServiceHandler` (или создать отдельный `NewSummaryServiceHandler`)
4. Убедиться, что порядок инициализации корректен: репозитории → builder → сервис → scheduler → RPC handler

**Зависимости**: Steps 2, 3, 4

**Результат**: Приложение при запуске: регистрирует outbox builder для post-save, запускает cron scheduler, обслуживает RPC endpoint `ProcessSource`.

**Проверка**:
- `go build ./...` компилируется без ошибок
- `go vet ./...` без предупреждений
- Приложение стартует и логирует запуск scheduler
- RPC `ProcessSource` доступен через Connect

---

### Step 6: Тесты

**Цель**: Покрыть все компоненты unit-тестами (coverage > 80%).

**Действия**:
1. `internal/app/summary/post_outbox_builder_test.go`:
   - Self-contained source → возвращает OutboxEvent с `EventType=IMMEDIATE`, `PostID` NOT NULL
   - Cumulative source → возвращает `nil, nil` (no event)
   - Неизвестный source type → ошибка
   - Ошибка при получении source → ошибка пробрасывается
2. `internal/app/summary/scheduler_test.go`:
   - `nextScheduleTime` для разных временных точек
   - `RunScheduled` вызывает `CreateScheduledForType` с правильными аргументами
   - При ошибке `CreateScheduledForType` — возвращает ошибку
3. `internal/app/summary/adapters/connectrpc/handler_test.go`:
   - Success для валидного запроса
   - CodeInvalidArgument для невалидного UUID
   - CodeNotFound для несуществующего source
   - Проверка структуры ответа
4. `internal/app/post/service_test.go` — расширить существующие тесты:
   - `Create` с outboxBuilder → вызывает `repo.CreateWithOutbox`
   - `Create` без outboxBuilder → вызывает `repo.Create`
   - `Update` аналогично
   - Ошибка outboxBuilder приводит к ошибке `Create`/`Update`
5. `internal/app/post/adapters/postgres/repository_test.go` — расширить:
   - `CreateWithOutbox` commit при успехе обеих операций
   - `CreateWithOutbox` rollback при ошибке `createEventFn`
   - `CreateWithOutbox` не создаёт event если `createEventFn` вернул `nil, nil`
6. `internal/app/summary/adapters/postgres/outbox_event_repository_test.go` — расширить:
   - `Create` сохраняет событие
   - `CreateScheduledForType` выполняет multi-insert

**Зависимости**: Steps 1–5

**Результат**: Все новые компоненты покрыты тестами.

**Проверка**:
- `go test ./...` — все тесты проходят
- `go test -cover ./internal/app/summary/...` — coverage > 80%
- `go test -cover ./internal/app/post/...` — coverage > 80%

---

## Edge Cases

1. **Manual для source без постов**: RPC `ProcessSource` создаёт MANUAL event. Worker находит 0 постов за 24ч → Summary НЕ создаётся (обрабатывается в spec-007a worker logic), event → COMPLETED
2. **Manual + Scheduled overlap**: Оба события создаются как независимые PENDING записи. Worker обрабатывает их последовательно по `created_at ASC`. Каждое создаёт свой Summary
3. **Concurrent PROCESSING**: Новый manual/scheduled event создаётся с status=PENDING. Worker не возьмёт его, пока текущий PROCESSING не завершится (FIFO + SKIP LOCKED гарантирует порядок)
4. **Cron при 0 TELEGRAM_GROUP sources**: Multi-insert вставит 0 строк. Это не ошибка — `RowsAffected = 0`, просто логируем
5. **Post-save для Update**: `Update` тоже вызывает outbox builder. Для self-contained source создаётся новый IMMEDIATE event с тем же `post_id`. Worker обработает его независимо
6. **Source не найден при post-save**: `SourceQueryRepository.GetByID` вернёт ошибку → outbox builder вернёт ошибку → `post.Service.Create` провалится. Это корректно: пост не может существовать без source (FK constraint)
7. **Невалидный UUID в RPC**: `uuid.Parse` вернёт ошибку → Connect CodeInvalidArgument. Пустой id, числовой id, строка с пробелами — все отлавливаются парсером UUID
8. **Race: source удалён между проверкой и INSERT event**: FK constraint `outbox_events.source_id → sources.id` вызовет DB-ошибку → Connect CodeInternal. Допустимо для MVP (race window минимален)
9. **Cron multi-insert при конкурентном вызове**: Два cron-тика одновременно (теоретически невозможно при одном инстансе). Каждый создаст свои events. Допустимо для MVP — нет UNIQUE constraint на (source_id, event_type, scheduled_at)
10. ~~Post-save хук + DB transaction~~ **Resolved**: `CreateWithOutbox` обеспечивает атомарность — обе операции (post + event) в одной транзакции. Rollback при ошибке создания event.

---

## Verification

### Автоматическая
1. `go build ./...` — компиляция без ошибок
2. `go vet ./...` — без предупреждений
3. `golangci-lint run ./... -c .golangci.yml` — без ошибок линтера
4. `go test ./...` — все тесты проходят
5. `go test -cover ./internal/app/summary/... ./internal/app/post/...` — coverage > 80%

### Ручная (smoke test)
1. Запустить приложение: `go run ./cmd/feedium run`
2. Создать source типа `telegram_channel`
3. Создать post для этого source → в `outbox_events` появляется запись с `event_type=IMMEDIATE`, `post_id` NOT NULL
4. Создать source типа `telegram_group`
5. Создать post для group source → в `outbox_events` НЕТ новой записи
6. RPC `ProcessSource` для TELEGRAM_GROUP source → OK + event_id в ответе, в `outbox_events` появляется запись `event_type=MANUAL`
7. RPC `ProcessSource` с невалидным UUID → CodeInvalidArgument
8. RPC `ProcessSource` с несуществующим UUID → CodeNotFound
9. Дождаться cron (или подменить время) → в `outbox_events` появляются записи `event_type=SCHEDULED` для всех TELEGRAM_GROUP sources

---

## Open Questions — Resolved

| # | Question | Decision | Rationale |
|---|----------|----------|-----------|
| 1 | Transactional outbox для post-save | **Расширить `post.Repository` методом `CreateWithOutbox`** | Новый метод `CreateWithOutbox(ctx, post, createEventFn)` создаёт Post и OutboxEvent в одной DB-транзакции. `createEventFn` — функция, которая получает `postID` и возвращает `*OutboxEvent`, вызывается внутри транзакции после INSERT post. Это не нарушает layering: post-пакет не зависит от summary, а summary передаёт свою логику через callback. Если `createEventFn` вернёт `nil, nil` — event не создаётся (для cumulative sources). |
| 2 | HTTP handler vs Connect-RPC | **Connect-RPC метод в proto** | Добавить RPC `ProcessSource` в `source.proto` (или отдельный `summary.proto`). Handler реализует Connect-RPC интерфейс — консистентно с остальным API. |
| 3 | Scheduler graceful shutdown | **Только логирование** | Ошибки cron не являются fatal для работы приложения. Scheduler самостоятельно логирует ошибки, не пробрасывает в `errCh`. |
| 4 | Конкурентность при post Update | **A — каждое обновление = новый Summary** | Без дедупликации. `Update` для self-contained source создаёт новый IMMEDIATE event, worker создаёт новый Summary. Это корректное поведение: каждая версия поста может иметь свою summary. |
