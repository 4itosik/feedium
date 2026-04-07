# AI Summarization via OpenRouter

## Цель

Дать пользователю возможность понять ключевой смысл поста или групповой переписки без чтения полного текста, используя LLM через OpenRouter API.

## Reference
- Brief: `memory-bank/features/008/brief.md`
- Research: `memory-bank/features/008/research.md`

---

## Scope

### Входит
1. OpenRouter Processor — реализация `Processor` interface через OpenRouter Chat Completions API
2. Prompt templates — system, self_contained, cumulative (go:embed)
3. Retry с backoff для transient ошибок LLM — до 3 попыток, затем FAILED
4. Worker lifecycle component — перенос из `bootstrap.go` в `internal/components/summary/`
5. Read API — `GetSummaryByPost`, `ListSummariesBySource` (Connect/proto)
6. Расширение `Repository` и его postgres-адаптера read-методами
7. Bootstrap wiring — замена `stubProcessor` на OpenRouter processor

### НЕ входит
- Изменение бизнес-логики Worker (уже реализован)
- Изменение Scheduler (уже реализован)
- Изменение схемы БД (таблицы `summaries`, `summary_posts`, `outbox_events` уже созданы)
- Хранение использованной модели в БД
- Chunking / разбиение постов при превышении context window
- Аутентификация и авторизация API
- Integration и e2e тесты

---

## Контекст

### Текущее состояние кода

| Компонент | Файл | Статус |
|---|---|---|
| `Processor` interface | `internal/app/summary/processor.go` | ✅ существует |
| `Worker` (self-contained + cumulative) | `internal/app/summary/worker.go` | ✅ существует |
| `Scheduler` (00:00 и 12:00 UTC) | `internal/app/summary/scheduler.go` | ✅ существует |
| `OutboxEvent` model + statuses | `internal/app/summary/outbox_event.go` | ✅ существует |
| `OutboxEventRepository` (+ SELECT FOR UPDATE SKIP LOCKED) | `internal/app/summary/adapters/postgres/` | ✅ существует |
| `SummaryRepository.Create` | `internal/app/summary/adapters/postgres/summary_repository.go` | ✅ существует |
| DB таблицы `summaries`, `summary_posts`, `outbox_events` | `migrations/003_create_outbox_events.sql` | ✅ существует |
| Worker lifecycle loop | `internal/bootstrap/bootstrap.go:127-144` | ⚠️ не на месте |
| `stubProcessor` | `internal/bootstrap/bootstrap.go:147` | ⚠️ заглушка |
| OpenRouter Processor | — | ❌ нет |
| Connect API handler | — | ❌ нет |
| Read-методы Repository | — | ❌ нет |

### Пайплайн обработки (существующий)

```
Post.Create() → outbox_events (PENDING, IMMEDIATE)
Scheduler → outbox_events (PENDING, SCHEDULED) — в 00:00 и 12:00 UTC

WorkerRunner [polling 1s]
    → Worker.ProcessNext()
        → FetchAndLockPending() [SELECT FOR UPDATE SKIP LOCKED]
        → ProcessingMode по Source.Type
        → Processor.Process(posts) ← реализуем здесь
        → summaryRepo.Create()
        → outboxEventRepo.UpdateStatus()

Connect API → SummaryHandler → summaryRepo.Get*()
```

### Типы обработки

| Source Type | Mode | System Prompt | User Prompt | Trigger |
|---|---|---|---|---|
| TELEGRAM_CHANNEL | Self-Contained | `self_contained_system.txt` | `self_contained_user.txt` | IMMEDIATE |
| RSS | Self-Contained | `self_contained_system.txt` | `self_contained_user.txt` | IMMEDIATE |
| WEB_SCRAPING | Self-Contained | `self_contained_system.txt` | `self_contained_user.txt` | IMMEDIATE |
| TELEGRAM_GROUP | Cumulative | `cumulative_system.txt` | `cumulative_user.txt` | SCHEDULED |

---

## Функциональные требования

### FR-1: OpenRouter Processor

**FR-1.1** Processor реализует интерфейс `Processor` из `internal/app/summary/processor.go`.

**FR-1.2** Worker передаёт `ProcessingMode` в `Processor.Process(mode, posts)`:
- `mode == SelfContained` → используются `self_contained_system.txt` + `self_contained_user.txt`
- `mode == Cumulative` → используются `cumulative_system.txt` + `cumulative_user.txt`

Processor выбирает шаблоны на основе переданного mode, не по количеству постов.

**FR-1.3** Processor рендерит user-промпт через `text/template`:
- Self-contained: передаёт `struct{ Title, Content string }` из первого поста
- Cumulative: передаёт `struct{ Posts []struct{ Author, Content, PublishedAt string } }`; `PublishedAt` форматируется как `2006-01-02 15:04` UTC

**FR-1.4** Processor отправляет запрос на `https://openrouter.ai/api/v1/chat/completions` через `github.com/openai/openai-go`:
- messages: `[{role: system, content: <system.txt>}, {role: user, content: <rendered_template>}]`
- заголовки: `HTTP-Referer: https://feedium.app`, `X-Title: Feedium`

**FR-1.5** Processor возвращает `content` из первого choice ответа.

> **Empty content**: если `content == ""` (пустая строка), Processor возвращает ошибку (treat as transient) → Worker выполняет retry-логику по FR-3.

**FR-1.6** Если Content поста превышает `MaxContentLength` токенов/символов — Processor возвращает постоянную ошибку `ErrContentTooLarge` (retry не выполняется).

> Лимит: 32 000 символов (суммарно по всем постам). Обрезка не выполняется — возвращается ошибка.

**FR-1.7** Конфигурация через env vars:
- `OPENROUTER_API_KEY` — обязательный; если не задан, bootstrap падает с ошибкой при старте
- `OPENROUTER_MODEL` — опциональный; дефолт: `anthropic/claude-haiku-4-5`

### FR-2: Промпт-шаблоны

**FR-2.1** Шаблоны хранятся как `.txt` файлы, встроенные через `go:embed`:
```
internal/app/summary/adapters/openrouter/prompts/
    self_contained_system.txt
    self_contained_user.txt
    cumulative_system.txt
    cumulative_user.txt
```

**FR-2.2** Содержимое шаблонов:

`self_contained_system.txt`:
```
You are a precise content summarizer for a personal news aggregator.
Your task is to help the user quickly understand the key meaning of a single post without reading the full text.
Rules: preserve all factual information, numbers, and named entities; do not add opinions; be concise; 3-5 sentences max.
```

`self_contained_user.txt`:
```
Summarize this post in 3-5 sentences.

Title: {{ .Title }}

{{ .Content }}
```

`cumulative_system.txt`:
```
You are a precise conversation summarizer for a personal news aggregator.
Your task is to help the user quickly understand the key meaning of a group conversation without reading all messages.
Rules: preserve all factual information, numbers, and named entities; focus on main topics, key decisions, and important facts; attribute key statements to authors; do not add opinions; be concise; 5-7 sentences max.
```

`cumulative_user.txt`:
```
Summarize the following conversation in 5-7 sentences.
Focus on main topics, key decisions, and important facts.
Attribute key statements to their authors.

{{ range .Posts }}[{{ .PublishedAt }}] {{ .Author }}: {{ .Content }}
{{ end }}
```

### FR-3: Retry с backoff

**FR-3.1** Ошибки делятся на два класса:
- **Transient** — ошибки LLM (сетевые, rate limit, 5xx OpenRouter) → retry с backoff
- **Permanent** — `ErrPostNotFound`, `ErrSourceNotFound`, `ErrUnknownSourceType` (из `internal/app/summary/errors.go`), `ErrContentTooLarge` (из пакета openrouter-адаптера) → немедленный FAILED без retry

**FR-3.2** При transient-ошибке, если `event.RetryCount < MaxRetries`: Worker вызывает `outboxEventRepo.Requeue(ctx, event.ID, scheduledAt)` и возвращает `processResult{status: PENDING}`.

**FR-3.3** При transient-ошибке, если `event.RetryCount >= MaxRetries`: Worker возвращает `processResult{status: FAILED, incrementRetry: false}`.

**FR-3.4** `MaxRetries = 3`. При `retry_count = 0,1,2` → requeue; при `retry_count = 3` → FAILED.

**FR-3.5** Backoff при requeue: `scheduled_at = now() + 2^retry_count minutes` (1 мин, 2 мин, 4 мин).

**FR-3.6** К `OutboxEventRepository` добавляется метод `Requeue`:
```go
Requeue(ctx context.Context, id uuid.UUID, scheduledAt time.Time) error
```
- Устанавливает `status = PENDING`, инкрементирует `retry_count`, записывает `scheduled_at`
- `FetchAndLockPending` уже фильтрует по `scheduled_at <= now()` — изменений не требует
- `UpdateStatus` не изменяется

### FR-4: Worker lifecycle component

**FR-4.1** Создаётся `internal/components/summary/worker_runner.go` с типом `WorkerRunner`.

**FR-4.2** `WorkerRunner` содержит polling loop с интервалом 1s (вынесено из `bootstrap.go:127-144`).

**FR-4.3** `WorkerRunner.Start(ctx context.Context)` запускает одну горутину polling loop.

**FR-4.4** Polling loop завершается при отмене `ctx` (graceful shutdown): горутина дожидается завершения текущего `ProcessNext` (если он выполняется), затем выходит без ошибки.

**FR-4.5** При ошибке `Worker.ProcessNext` WorkerRunner логирует ошибку и продолжает polling (не завершает горутину).

> Отличие от текущего `startWorker`: не отправляет в `errCh` и не завершается при ошибке — это transient ошибки репозитория.

**FR-4.6** `bootstrap.go` удаляет `startWorker` и `stubProcessor`; использует `WorkerRunner` и OpenRouter Processor.

### FR-5: Read API

**FR-5.1** Proto-контракт `api/summary/v1/summary.proto`:

```protobuf
service SummaryService {
  rpc GetSummaryByPost(GetSummaryByPostRequest) returns (GetSummaryByPostResponse);
  rpc ListSummariesBySource(ListSummariesBySourceRequest) returns (ListSummariesBySourceResponse);
}

message GetSummaryByPostRequest {
  string post_id = 1; // UUID
}

message GetSummaryByPostResponse {
  string id = 1;
  string source_id = 2;
  string event_id = 3;
  string content = 4;
  google.protobuf.Timestamp created_at = 5;
  repeated string post_ids = 6;
}

message ListSummariesBySourceRequest {
  string source_id = 1; // UUID, если не указан — возвращает все summaries
  int32 limit = 2;      // > 0, максимум 100
}

message ListSummariesBySourceResponse {
  repeated SummaryItem summaries = 1;
}

message SummaryItem {
  string id = 1;
  string source_id = 2;
  string event_id = 3;
  string content = 4;
  google.protobuf.Timestamp created_at = 5;
  repeated string post_ids = 6;
}
```

**FR-5.2** `GetSummaryByPost`:
- Ищет summary по `post_id` через `summary_posts`
- Если не найдено → Connect error `codes.NotFound`
- Если найдено → Connect error `codes.OK`
- При внутренней ошибке репозитория → Connect error `codes.Internal`
- Возвращает поля Summary + `post_ids` из `summary_posts`

**FR-5.3** `ListSummariesBySource`:
- Если `source_id` указан — возвращает до `limit` summaries для конкретного source
- Если `source_id` не указан — возвращает до `limit` summaries из всех sources
- Сортировка по `created_at DESC` (от новых к старым)
- Если `limit <= 0` или `limit > 100` → Connect error `codes.InvalidArgument`
- Если ошибок нет → Connect error `codes.OK`
- При внутренней ошибке репозитория → Connect error `codes.Internal`
- Каждый SummaryItem содержит `post_ids` из `summary_posts`
- **N+1 проверка**: метод должен выполнять JOIN с `summary_posts` и возвращать post_ids одним запросом, не делая отдельных запросов на каждый summary

**FR-5.4** Расширение `Repository` interface:
```go
GetByPostID(ctx context.Context, postID uuid.UUID) (*Summary, []uuid.UUID, error)
// возвращает Summary и список postIDs из summary_posts

ListSummaries(ctx context.Context, sourceID *uuid.UUID, limit int) ([]SummaryWithPostIDs, error)

type SummaryWithPostIDs struct {
    Summary
    PostIDs []uuid.UUID
}
```

**FR-5.5** Postgres-адаптер реализует методы из FR-5.4:
- `GetByPostID` — JOIN через `summary_posts`; если не найдено → возвращает `nil, nil`
- `ListSummaries` — ORDER BY `created_at DESC`, LIMIT; если `sourceID == nil` — без фильтра по source; если нет записей — возвращает пустой слайз `[]SummaryWithPostIDs{}, nil`
- **N+1 защита**: `ListSummaries` должен получать post_ids через JOIN или агрегацию в одном запросе, без отдельных запросов на каждый summary

---

## Нефункциональные требования

**NFR-1** Processor не держит состояние между вызовами — потокобезопасен.

**NFR-2** `OPENROUTER_API_KEY` не логируется.

**NFR-3** Polling interval — 1s (константа в `WorkerRunner`, не в `Worker`).

**NFR-4** Суммарный лимит контента на запрос — 32 000 символов. Рассчитывается как сумма `len(post.Content)` для всех постов в вызове.

---

## Сценарии и edge cases

### Основной сценарий (self-contained)
1. Пост создаётся → `post_outbox_builder` создаёт `outbox_events` (PENDING, IMMEDIATE)
2. WorkerRunner polling → `Worker.ProcessNext()` → `FetchAndLockPending()` (status → PROCESSING)
3. Worker определяет Mode = SelfContained → `postQueryRepo.GetByID()`
4. `Processor.Process(mode, [post])` → рендерит `self_contained_user.txt` с system из `self_contained_system.txt` → POST OpenRouter → content
5. `summaryRepo.Create(summary, [postID])` → commit
6. `outboxEventRepo.UpdateStatus(COMPLETED, false, nil)`

### Transient ошибка LLM, retry_count = 1 (< 3)
- Processor возвращает ошибку (rate limit)
- Worker возвращает `{status: PENDING, incrementRetry: true}`
- `UpdateStatus(PENDING, true, scheduledAt=now()+2min)` — event уходит в ожидание
- Через 2 минуты FetchAndLockPending подберёт снова

### Transient ошибка LLM, retry_count = 3 (>= MaxRetries)
- Processor возвращает ошибку
- Worker возвращает `{status: FAILED, incrementRetry: false}`
- `UpdateStatus(FAILED, false, nil)`
- Событие больше не обрабатывается

### Постоянная ошибка (ErrContentTooLarge)
- Processor возвращает `ErrContentTooLarge`
- Worker возвращает `{status: FAILED, incrementRetry: false}` — без retry, без backoff
- `UpdateStatus(FAILED, false, nil)`

### OpenRouter вернул пустой content
- Processor получает `choices[0].message.content == ""`
- Processor возвращает `ErrEmptyLLMResponse` (treat as transient error)
- Worker по FR-3.2/3.3 выполняет requeue или FAILED в зависимости от retry_count

### GetSummaryByPost — summary ещё не сгенерирован
- `summaryRepo.GetByPostID()` → not found
- Connect handler возвращает `codes.NotFound`
- Клиент должен повторить запрос позже (polling на стороне клиента)

### Cumulative — нет постов за последние 24 часа
- `Worker.processCumulative` → `postQueryRepo.FindUnprocessedBySource()` → пустой список
- Worker возвращает `{status: COMPLETED, incrementRetry: false}` без создания summary
- Summary не создаётся — поведение корректное
- Если есть посты: Worker передаёт `mode = Cumulative` в `Processor.Process(mode, posts)`

### Cumulative IMMEDIATE event (пост из TELEGRAM_GROUP)
- Worker видит `mode == Cumulative && event.EventType == IMMEDIATE`
- Скипает с `{status: COMPLETED}` — не обрабатывает, ждёт SCHEDULED event

### ListSummariesBySource с limit = 0
- Connect handler возвращает `codes.InvalidArgument`

### Пустой OPENROUTER_API_KEY при старте
- Bootstrap возвращает ошибку при инициализации — приложение не запускается

---

## Инварианты

1. Один `outbox_event` порождает не более одного `summary` (связь через `event_id` в таблице `summaries`)
2. `summary_posts` содержит ≥ 1 запись для каждого summary
3. Self-contained summary всегда имеет ровно 1 запись в `summary_posts`
4. Event в статусе `PROCESSING` не берётся другим воркером (SELECT FOR UPDATE SKIP LOCKED)
5. При retry event возвращается в статус `PENDING` (не остаётся в `PROCESSING`)
6. `retry_count` только инкрементируется, никогда не сбрасывается
7. `scheduled_at` для retry-событий всегда `> now()` в момент записи

---

## Acceptance Criteria

### OpenRouter Processor
- [ ] Worker передаёт `ProcessingMode` в `Processor.Process(mode, posts)`
- [ ] При `mode == SelfContained` используются `self_contained_system.txt` + `self_contained_user.txt`
- [ ] При `mode == Cumulative` используются `cumulative_system.txt` + `cumulative_user.txt`
- [ ] Запрос к OpenRouter содержит system prompt и rendered user prompt
- [ ] При суммарном `len(content) > 32_000` символов возвращается `ErrContentTooLarge`
- [ ] При отсутствии `OPENROUTER_API_KEY` приложение не стартует

### Retry
- [ ] При transient-ошибке и `retry_count < 3`: event → PENDING, `retry_count` увеличивается, `scheduled_at = now() + backoff`
- [ ] При transient-ошибке и `retry_count >= 3`: event → FAILED
- [ ] При permanent-ошибке: event → FAILED, `retry_count` не увеличивается
- [ ] Backoff: retry 1 = 1 мин, retry 2 = 2 мин, retry 3 = 4 мин

### WorkerRunner
- [ ] `WorkerRunner` живёт в `internal/components/summary/`
- [ ] При ошибке `ProcessNext` WorkerRunner логирует и продолжает polling (не завершается)
- [ ] Горутина завершается при отмене контекста
- [ ] `bootstrap.go` не содержит `startWorker` и `stubProcessor`

### Read API
- [ ] `GetSummaryByPost` с несуществующим `post_id` → `codes.NotFound`
- [ ] `GetSummaryByPost` возвращает `id`, `source_id`, `event_id`, `content`, `created_at`, `post_ids`
- [ ] `ListSummariesBySource` с `limit <= 0` или `limit > 100` → `codes.InvalidArgument`
- [ ] `ListSummariesBySource` возвращает summaries, отсортированные по `created_at DESC`
- [ ] `ListSummariesBySource` без `source_id` возвращает summaries из всех sources
- [ ] Каждый `SummaryItem` содержит `post_ids`
- [ ] `ListSummariesBySource` не делает N+1 запросов (post_ids получаются JOIN/агрегацией)

---

## Ограничения

- Новая миграция БД не создаётся — существующая схема (migration 003) достаточна
- Один инстанс WorkerRunner запускает одну горутину
- `lib/openai-go` используется как HTTP-клиент к OpenRouter (OpenAI-совместимый API)
- Модель не хранится в БД и не возвращается в API

---

