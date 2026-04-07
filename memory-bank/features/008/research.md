# Research: Feature 008 — Summarization

## Context

Задача: дать пользователю возможность понять суть поста/переписки без чтения полного текста.
Целевые метрики: сокращение времени ознакомления ≥2x, точность достаточна для дайджестов, ≥10 постов за сессию.

---

## Текущее состояние кодовой базы

| Компонент | Файл | Статус |
|---|---|---|
| `Processor` interface | `internal/app/summary/processor.go` | ✅ |
| Worker (self-contained / cumulative) | `internal/app/summary/worker.go` | ✅ |
| Scheduler (00:00 и 12:00 UTC) | `internal/app/summary/scheduler.go` | ✅ |
| Outbox events model | `internal/app/summary/outbox_event.go` | ✅ |
| DB таблицы `summaries`, `summary_posts` | `migrations/003_create_outbox_events.sql` | ✅ |
| Postgres адаптеры | `internal/app/summary/adapters/postgres/` | ✅ |
| `stubProcessor` | `internal/bootstrap/bootstrap.go:147` | ⚠️ Заглушка |
| API для чтения summaries | — | ❌ Нет |
| Worker lifecycle в `internal/components/` | — | ❌ Нет (сейчас в bootstrap) |

---

## Архитектура по слоям

### Раскладка по слоям

| Слой | Назначение | Что входит в summary |
|---|---|---|
| `internal/app/summary/` | Бизнес-логика | Domain types, Processor interface, Worker logic, Repository interfaces |
| `internal/app/summary/adapters/postgres/` | Реализация БД | SummaryRepository, OutboxEventRepo, PostQueryRepo, SourceQueryRepo |
| `internal/app/summary/adapters/connect/` | Transport | Connect HTTP handler для чтения summaries |
| `internal/app/summary/adapters/openrouter/` (новый) | Внешняя интеграция | Реализация Processor через OpenRouter API |
| `internal/components/summary/` (новый) | Runtime lifecycle | Горутина воркера, polling loop, graceful shutdown |
| `internal/bootstrap/` | Wiring only | Создание зависимостей, регистрация обработчиков |

### Проблема текущего кода

`bootstrap.go:127-144` содержит `startWorker` с lifecycle горутины. Это должно жить в `internal/components/summary/`.

### Проверка разделения ответственности

| Что | Где живёт | Где НЕ должно быть |
|---|---|---|
| Бизнес-логика обработки | `internal/app/summary/worker.go` | bootstrap, components |
| LLM-вызов | `internal/app/summary/adapters/openrouter/processor.go` | bootstrap, worker |
| Polling loop / горутина | `internal/components/summary/worker_runner.go` | bootstrap, app |
| HTTP handler | `internal/app/summary/adapters/connect/handler.go` | app/summary (domain) |
| Wiring | `internal/bootstrap/bootstrap.go` | везде остальное |

---

## OpenRouter: интеграция

**OpenRouter API — OpenAI-совместимый**: те же Chat Completions endpoint, тот же формат запросов.

**Клиент**: `github.com/openai/openai-go` с кастомным base URL:
```go
client := openai.NewClient(
    option.WithAPIKey(cfg.OpenRouterAPIKey),
    option.WithBaseURL("https://openrouter.ai/api/v1"),
)
```

**Модели** — формат `{provider}/{model}`:
- `anthropic/claude-haiku-4-5` — быстро, дёшево, достаточно для summarization
- `openai/gpt-4o-mini` — альтернатива

**Опциональные заголовки** (рекомендованы OpenRouter для атрибуции):
```
HTTP-Referer: https://feedium.app
X-Title: Feedium
```

**Конфигурация** (env vars):
- `OPENROUTER_API_KEY` — обязательный
- `OPENROUTER_MODEL` — опциональный, дефолт: `anthropic/claude-haiku-4-5`

---

## Дизайн промптов

### Хранение промптов

**Выбор: `go:embed` + `text/template`**

Промпты хранятся как `.txt` файлы, встроенные в бинарник через `go:embed`:

```
internal/app/summary/adapters/openrouter/
    processor.go
    prompts/
        system.txt              — общий system prompt
        self_contained.txt      — user prompt для одиночного поста
        cumulative.txt          — user prompt для групповой переписки
```

```go
//go:embed prompts/*.txt
var promptFS embed.FS
```

### Выбор промпта

Processor выбирает шаблон по количеству постов:
- `len(posts) == 1` → `self_contained.txt`
- `len(posts) > 1` → `cumulative.txt`

### Переменные шаблонов

Processor рендерит шаблон через `text/template`.

**Self-Contained** (`self_contained.txt`) — данные: `struct { Title, Content string }`:
```
{{ .Title }}
{{ .Content }}
```

**Cumulative** (`cumulative.txt`) — данные: `struct { Posts []PostData }`:
```
{{ range .Posts }}[{{ .PublishedAt }}] {{ .Author }}: {{ .Content }}
{{ end }}
```

> Форматирование полей постов в строки (обрезка, очистка) — ответственность caller'а, не шаблона.

### Содержимое промптов (черновик)

**system.txt**:
```
You are a precise content summarizer for a personal news aggregator.
Your task is to help the user quickly understand the key meaning of content without reading the full text.
Rules: preserve all factual information, numbers, and named entities; do not add opinions; be concise.
```

**self_contained.txt**:
```
Summarize this post in 3-5 sentences.

Title: {{ .Title }}

{{ .Content }}
```

**cumulative.txt**:
```
Summarize the following conversation in 5-7 sentences.
Focus on main topics, key decisions, and important facts.
Attribute key statements to their authors.

{{ range .Posts }}[{{ .PublishedAt }}] {{ .Author }}: {{ .Content }}
{{ end }}
```

---

## Входные сущности

Processor получает `[]post.Post`. Поля, используемые при суммаризации:

| Поле | Используется | Примечание |
|---|---|---|
| `Title` | Да | Только self-contained |
| `Content` | Да | Оба режима |
| `Author` | Да | Cumulative — атрибуция |
| `PublishedAt` | Да | Cumulative — хронология |
| `SourceID` | Нет | Бизнес-контекст, не промпт |

---

## Типы summary для разных типов контента

| Source Type | Processing Mode | Промпт | Триггер |
|---|---|---|---|
| Telegram Channel | Self-Contained | `self_contained.txt` | IMMEDIATE при создании поста |
| RSS | Self-Contained | `self_contained.txt` | IMMEDIATE при создании поста |
| Web Scraping | Self-Contained | `self_contained.txt` | IMMEDIATE при создании поста |
| Telegram Group | Cumulative | `cumulative.txt` | SCHEDULED (00:00, 12:00 UTC) |

---

## Место summary в пайплайне обработки

```
Post.Create()
    → OutboxBuilder → outbox_events (PENDING, IMMEDIATE)
                          ↑
Scheduler               SCHEDULED events (Groups, 00:00/12:00)

WorkerRunner [components/summary]  ← polling 1s
    → Worker.ProcessNext() [app/summary]
        → ProcessingMode по Source.Type
        → PostQueryRepo.GetByID() / FindUnprocessedBySource()
        → Processor.Process(posts) [adapters/openrouter]
            → рендер шаблона → system + user prompt
            → POST openrouter.ai/api/v1/chat/completions
            → content string
        → summaryRepo.Create(summary, postIDs)
        → outbox_events.UpdateStatus(COMPLETED)

Connect API
    → SummaryHandler.GetSummaryByPost(post_id) [adapters/connect]
        → summaryRepo.GetByPostID()
```

---

## Связи сущностей

```
outbox_events ──── summaries ──── summary_posts ──── posts
    id ────────── event_id
    source_id ─── source_id
                      id ──────── summary_id
                                   post_id ─────── id
```

- `event_id` — трассировка: какое событие породило summary
- `source_id` — денормализация для быстрой выборки по источнику
- M2M `summary_posts` — cumulative покрывает N постов; self-contained — всегда 1

---

## API для чтения summaries

**Proto** `api/summary/v1/summary.proto`:
```protobuf
service SummaryService {
  rpc GetSummaryByPost(GetSummaryByPostRequest) returns (GetSummaryByPostResponse);
  rpc ListSummariesBySource(ListSummariesBySourceRequest) returns (ListSummariesBySourceResponse);
}
```

**Расширение Repository** (`internal/app/summary/repository.go`):
```go
GetByPostID(ctx context.Context, postID uuid.UUID) (*Summary, error)
ListBySourceID(ctx context.Context, sourceID uuid.UUID, limit int) ([]Summary, error)
```

---

## Миграции

Новая миграция **не нужна** — таблицы `summaries` и `summary_posts` уже в migration 003.

---

## Scope реализации

| # | Что | Файл |
|---|---|---|
| 1 | OpenRouter processor | `internal/app/summary/adapters/openrouter/processor.go` |
| 2 | Prompt templates | `internal/app/summary/adapters/openrouter/prompts/*.txt` |
| 3 | Worker lifecycle | `internal/components/summary/worker_runner.go` |
| 4 | Proto контракт | `api/summary/v1/summary.proto` + gen |
| 5 | Connect handler | `internal/app/summary/adapters/connect/handler.go` |
| 6 | Read-методы repo | `internal/app/summary/repository.go` + postgres adapter |
| 7 | Bootstrap wiring | `internal/bootstrap/bootstrap.go` |
