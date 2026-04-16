---
doc_kind: feature
doc_function: spec
purpose: "Feature spec: AI-суммаризация постов и переписок — генерация саммари через LLM с дифференцированной обработкой по типу источника."
derived_from:
  - brief.md
  - ../../prd/PRD-001-mvp.md
  - ../../domain/architecture.md
  - ../../domain/glossary.md
status: active
delivery_status: planned
---

# FT-005: AI Summarization

## Цель

Автоматическая генерация саммари для постов и переписок, позволяющая понять ключевой смысл без чтения полного текста. Саммари используется для быстрого потребления контента и дальнейшей обработки (дайджесты OpenClaw).

## Reference

- Brief: `memory-bank/features/FT-005-ai-summarization/brief.md`
- PRD: `memory-bank/prd/PRD-001-mvp.md` (G-02, BR-01..BR-03, RISK-02)
- Architecture: `memory-bank/domain/architecture.md`
- Glossary: `memory-bank/domain/glossary.md`

## Scope

### Входит

- Генерация саммари для self-contained постов при создании поста (атомарно через outbox).
- Генерация саммари для cumulative источников по крону (инкрементальный дайджест).
- Ручной запуск суммаризации cumulative источника через API (асинхронный с polling).
- GET API для чтения саммари (по посту, по источнику, по ID).
- Polling API для отслеживания статуса задачи суммаризации.
- Summary Worker — обработка событий из outbox.
- Персистентное хранение саммари и событий суммаризации.
- Идемпотентность обработки событий.
- Автотесты согласно testing-policy.

### НЕ входит

- Аутентификация и авторизация.
- Integration и e2e тесты.
- AI-скоринг и ранжирование (FT-009).
- UI для отображения саммари (FT-010).
- Сбор постов из источников (FT-008).
- Выбор конкретного LLM-провайдера или модели (задача Design).
- Конфигурация промптов (задача Design).
- Стратегия обработки длинных текстов, превышающих контекстное окно LLM (задача Design).

## Контекст

Feedium — персональный агрегатор контента. FT-001..FT-004 обеспечивают каркас, хранилище, источники и посты. Система различает два режима обработки по типу источника (BR-01):

| Тип источника      | ProcessingMode   | Триггер суммаризации            |
|---------------------|------------------|---------------------------------|
| telegram_channel    | self_contained   | Создание поста                  |
| rss                 | self_contained   | Создание поста                  |
| html                | self_contained   | Создание поста                  |
| telegram_group      | cumulative       | Cron / ручной запрос через API  |

Доменные сущности `ProcessingMode`, `SourceType` определены в `internal/biz/source.go`. Функция `ProcessingModeForType()` возвращает режим обработки по типу источника.

## Функциональные требования

### FR-01: Доменные сущности

#### FR-01.1: Summary

Новая доменная сущность в `biz/`:

| Поле       | Тип       | Обязательность | Описание                                                    |
|------------|-----------|----------------|-------------------------------------------------------------|
| ID         | UUID v7   | required       | Уникальный идентификатор                                    |
| PostID     | UUID      | nullable       | Ссылка на пост. Заполняется для self-contained              |
| SourceID   | UUID      | required       | Ссылка на источник. Заполняется всегда                      |
| Text       | string    | required       | Текст саммари                                               |
| WordCount  | int       | required       | Количество слов в саммари                                   |
| CreatedAt  | time.Time | required       | Время создания                                              |

Инвариант: ровно одно из двух условий истинно:
- `PostID != nil` — саммари для self-contained поста.
- `PostID == nil` — саммари для cumulative источника (привязано к SourceID).

#### FR-01.2: SummaryEvent (outbox)

Новая доменная сущность в `biz/`:

| Поле        | Тип               | Обязательность | Описание                                          |
|-------------|--------------------|----------------|---------------------------------------------------|
| ID          | UUID v7            | required       | Уникальный идентификатор (используется как task_id)|
| PostID      | UUID               | nullable       | Ссылка на пост (для self-contained)               |
| SourceID    | UUID               | required       | Ссылка на источник                                |
| EventType   | SummaryEventType   | required       | Тип события (см. ниже)                            |
| Status      | SummaryEventStatus | required       | Статус обработки (см. ниже)                       |
| SummaryID   | UUID               | nullable       | Ссылка на созданный Summary (после завершения)    |
| Error       | string             | nullable       | Текст ошибки (при status=failed)                  |
| CreatedAt   | time.Time          | required       | Время создания события                            |
| ProcessedAt | time.Time          | nullable       | Время завершения обработки                        |

**SummaryEventType** (enum):
- `summarize_post` — суммаризация одного поста (self-contained).
- `summarize_source` — суммаризация накопленных постов источника (cumulative).

**SummaryEventStatus** (enum):
- `pending` — создано, ожидает обработки.
- `processing` — взято в обработку воркером.
- `completed` — обработано, Summary создан (SummaryID заполнен).
- `failed` — ошибка после исчерпания retry. Остаётся в outbox для ручного retry.
- `expired` — событие устарело (TTL истёк). Не обрабатывается.

### FR-02: Атомарное создание поста и события суммаризации

При создании поста для self-contained источника `PostUsecase.Create` создаёт `Post` и `SummaryEvent` в одной транзакции.

Поток:

```
PostUsecase.Create (biz/)
  → TxManager.InTx
    → PostRepo.Save         ← пишет post (в той же tx)
    → OutboxRepo.Save       ← пишет SummaryEvent (в той же tx)
  → коммит или роллбэк
```

Требования:
- `PostUsecase.Create` получает `SourceRepo` (или `SourceInfo`) для определения `ProcessingMode` текущего источника.
- Если `ProcessingMode == self_contained` → в рамках транзакции создаётся `SummaryEvent` с `event_type = summarize_post`, `status = pending`, `post_id = <новый post ID>`.
- Если `ProcessingMode == cumulative` → `SummaryEvent` не создаётся при создании поста.
- Если транзакция откатывается — ни Post, ни SummaryEvent не сохраняются.

Новые интерфейсы в `biz/`:

```
TxManager interface {
    InTx(ctx context.Context, fn func(ctx context.Context) error) error
}

SummaryOutboxRepo interface {
    Save(ctx context.Context, event SummaryEvent) (SummaryEvent, error)
    Get(ctx context.Context, id string) (SummaryEvent, error)
    ListPending(ctx context.Context, limit int) ([]SummaryEvent, error)
    UpdateStatus(ctx context.Context, id string, status SummaryEventStatus, summaryID *string, errText *string) error
}
```

### FR-03: Summary Worker (self-contained)

Summary Worker — реализует `transport.Server` в `task/`. Поллит outbox, обрабатывает события.

Поток обработки `summarize_post`:
1. Worker вызывает `SummaryOutboxRepo.ListPending(ctx, batchSize)` — получает события со статусом `pending`.
2. Для каждого события:
   a. Устанавливает `status = processing`.
   b. Проверяет TTL события: если `created_at + summary.outbox.event_ttl < now` — устанавливает `status = expired`, пропускает обработку.
   c. Загружает пост по `PostID` через `PostRepo.Get`. Если пост не найден — устанавливает `status = failed`, `error = "post not found"`, пропускает обработку.
   d. Вызывает `LLMProvider.Summarize(ctx, text)` — получает текст саммари.
   e. Валидирует: текст саммари не пустой (после trim). Если пустой — считается ошибкой LLM, переход к retry.
   f. Создаёт `Summary` через `SummaryRepo.Save`.
   g. Обновляет `SummaryEvent`: `status = completed`, `summary_id = <новый ID>`, `processed_at = now`.
3. При ошибке LLM: retry с exponential backoff (3 попытки, настраивается в config). После исчерпания: `status = failed`, `error = <текст ошибки>`.

### FR-04: Cron Worker (cumulative)

Отдельный процесс/горутина в `task/`, запускается по cron-интервалу из конфигурации (BR-02).

Поток:
1. Получает список cumulative источников: все источники с `ProcessingMode == cumulative`.
2. Для каждого источника:
   a. Проверяет: нет ли уже `SummaryEvent` со `status IN (pending, processing)` для данного `source_id` и `event_type = summarize_source`. Если есть — пропускает (идемпотентность).
   b. Проверяет: есть ли новые посты с момента последнего саммари для этого источника. Если нет — пропускает.
   c. Создаёт `SummaryEvent` с `event_type = summarize_source`, `status = pending`.
3. Summary Worker подхватывает событие (аналогично FR-03, но с другой логикой загрузки данных).

Поток обработки `summarize_source` в Summary Worker:
1. Определяет временное окно: от `created_at` последнего саммари этого источника до текущего момента. Максимум — 72 часа (3 суток). Если предыдущего саммари нет — берёт последние 72 часа.
2. Загружает все посты источника в этом окне через `PostRepo.List` (с фильтром по source_id и временному диапазону).
3. Если постов 0 — устанавливает `status = completed` без создания Summary.
4. Конкатенирует тексты постов (в хронологическом порядке). Если суммарная длина превышает `summary.cumulative.max_input_chars` — устанавливает `status = failed`, `error = "input text exceeds max_input_chars limit"`, пропускает обработку.
5. Передаёт конкатенированный текст в `LLMProvider.Summarize`.
6. Создаёт `Summary` с `post_id = nil`, `source_id = <ID источника>`.
7. Обновляет `SummaryEvent`: `status = completed`.

### FR-05: Ручной запуск суммаризации (BR-03)

API endpoint для ручного запуска суммаризации cumulative источника.

**Request:** `POST /v1/sources/{source_id}/summarize`

**Response (202 Accepted):**
```json
{
  "task_id": "<summary_event_id>"
}
```

Логика:
1. Валидация: `source_id` существует и `ProcessingMode == cumulative`. Если источник self-contained — возвращает ошибку `400 Bad Request` с reason `SUMMARIZE_SELF_CONTAINED_SOURCE`.
2. Проверка идемпотентности: если уже есть `SummaryEvent` со `status IN (pending, processing)` для этого `source_id` и `event_type = summarize_source` — возвращает существующий `task_id` (тот же `SummaryEvent.ID`), статус `200 OK`. Если существует событие со `status = failed` — оно не блокирует создание нового: создаётся новый `SummaryEvent`.
3. Создаёт `SummaryEvent` с `event_type = summarize_source`, `status = pending`.
4. Возвращает `202 Accepted` с `task_id = SummaryEvent.ID`.

### FR-06: Polling статуса задачи

**Request:** `GET /v1/summary-events/{id}`

**Response (200 OK):**
```json
{
  "id": "<event_id>",
  "source_id": "<source_id>",
  "post_id": "<post_id or null>",
  "event_type": "summarize_source",
  "status": "completed",
  "summary_id": "<summary_id or null>",
  "error": "<error text or null>",
  "created_at": "2026-04-16T12:00:00Z",
  "processed_at": "2026-04-16T12:00:05Z"
}
```

Если `id` не найден — `404 Not Found`.

### FR-07: GET API для чтения саммари

#### FR-07.1: Саммари для поста (self-contained)

**Request:** `GET /v1/posts/{post_id}/summaries`

**Response (200 OK):**
```json
{
  "summaries": [
    {
      "id": "<summary_id>",
      "post_id": "<post_id>",
      "source_id": "<source_id>",
      "text": "...",
      "word_count": 150,
      "created_at": "2026-04-16T12:00:05Z"
    }
  ]
}
```

Возвращает список саммари для поста, отсортированный по `created_at DESC`. Для self-contained поста обычно одна запись; может быть несколько при повторной обработке.

Если пост не найден — `404 Not Found`. Если саммари ещё нет — `200 OK` с пустым списком.

#### FR-07.2: Саммари для источника (cumulative)

**Request:** `GET /v1/sources/{source_id}/summaries?page_size=N&page_token=TOKEN`

**Response (200 OK):**
```json
{
  "summaries": [...],
  "next_page_token": "<token or empty>"
}
```

Возвращает список саммари для источника, отсортированный по `created_at DESC`. Поддерживает cursor-based пагинацию (аналогично существующему паттерну `ListPostsResult`).

Если источник не найден — `404 Not Found`. Если саммари нет — `200 OK` с пустым списком.

#### FR-07.3: Саммари по ID

**Request:** `GET /v1/summaries/{id}`

**Response (200 OK):** один объект Summary.

Если не найден — `404 Not Found`.

### FR-08: LLM Provider Interface

Интерфейс в `biz/`:

```
LLMProvider interface {
    Summarize(ctx context.Context, text string) (string, error)
}
```

- Принимает текст, возвращает текст саммари.
- Реализация в `data/` — конкретный LLM-провайдер (задача Design).
- Retry-логика (exponential backoff, 3 попытки) реализуется в `task/` (вызывающий код), не в интерфейсе.
- Таймаут на вызов LLM — из конфигурации.

### FR-09: Идемпотентность

#### Self-contained:
- Уникальный constraint на `SummaryEvent`: `(post_id, event_type)` WHERE `status IN (pending, processing)`. Гарантирует: для одного поста не может быть двух активных событий суммаризации одного типа.
- Если пост создаётся повторно (upsert по `source_id, external_id`) и уже существует — `SummaryEvent` не создаётся повторно.

#### Cumulative:
- Перед созданием `SummaryEvent` проверяется: нет ли уже активного события `(source_id, event_type = summarize_source)` WHERE `status IN (pending, processing)`.
- Если есть — новое событие не создаётся (cron пропускает; ручной запрос возвращает существующий `task_id`).

### FR-10: Конфигурация

Параметры в `configs/*.yaml`:

| Параметр                          | Тип      | Описание                                              |
|-----------------------------------|----------|-------------------------------------------------------|
| summary.worker.poll_interval      | duration | Интервал polling outbox воркером                      |
| summary.worker.batch_size         | int      | Количество событий за один poll                       |
| summary.cron.interval             | duration | Интервал cron для cumulative источников (BR-02)       |
| summary.llm.timeout               | duration | Таймаут одного вызова LLM                             |
| summary.llm.max_retries           | int      | Количество retry при ошибке LLM (default: 3)         |
| summary.cumulative.max_window     | duration | Макс. окно для cumulative дайджеста (default: 72h)    |
| summary.cumulative.max_input_chars| int      | Макс. длина конкатенированного текста для LLM-вызова  |
| summary.outbox.event_ttl          | duration | TTL события в outbox. Устаревшие помечаются `expired` |
| summary.llm.provider              | string   | Активный LLM-провайдер (default: openrouter)          |
| summary.llm.providers             | map      | Конфигурация провайдеров: API key, base URL, model    |

## Нефункциональные требования

### NFR-01: Экономика LLM

При потоке до 10 000 постов/день (RISK-02 PRD-001) Design обязан учитывать стоимость LLM-вызовов. Spec не определяет конкретные лимиты, но фиксирует:
- Один LLM-вызов на один SummaryEvent.
- Для cumulative: один вызов покрывает все посты за окно (не по одному вызову на пост).
- Batch processing событий (FR-10: `batch_size`).

### NFR-02: Качество саммари

Design обязан определить:
- Измеримый критерий качества саммари (например: сохранение всех именованных сущностей, ключевых фактов, числовых данных из оригинала).
- Способ автоматической или полуавтоматической проверки (например: сравнение NER-сущностей оригинала и саммари).

Spec фиксирует минимальное требование: саммари должно быть на том же языке, что и оригинальный текст.

### NFR-03: Производительность

- Summary Worker не блокирует создание постов — обработка асинхронная.
- Worker выполняет polling outbox с интервалом `summary.worker.poll_interval`. Внутри одного batch события обрабатываются последовательно.
- Таймаут LLM-вызова ограничен конфигурацией (`summary.llm.timeout`).

### NFR-04: Наблюдаемость

Structured logging (slog) в `task/` слое:
- При взятии события в обработку: `summary_event_id`, `event_type`, `source_id`, `post_id`.
- При завершении: `summary_event_id`, `status`, `duration`.
- При ошибке: `summary_event_id`, `err`, `attempt`.

## Сценарии и edge cases

### Основной сценарий (self-contained)

1. Коллектор вызывает `PostUsecase.Create` с постом из telegram_channel.
2. `PostUsecase` определяет `ProcessingMode == self_contained`.
3. В одной транзакции создаются `Post` и `SummaryEvent(event_type=summarize_post, status=pending)`.
4. Summary Worker подхватывает событие, загружает текст поста, вызывает LLM.
5. LLM возвращает саммари. Worker валидирует: текст не пустой.
6. Worker создаёт `Summary(post_id=post.ID, source_id=post.SourceID)`.
7. Worker обновляет событие: `status=completed, summary_id=summary.ID`.

### Основной сценарий (cumulative)

1. Cron Worker срабатывает по интервалу.
2. Находит cumulative источник (telegram_group) с новыми постами после последнего саммари.
3. Создаёт `SummaryEvent(event_type=summarize_source, status=pending, source_id=...)`.
4. Summary Worker подхватывает событие.
5. Загружает посты источника за окно (от последнего саммари, макс. 72ч).
6. Конкатенирует тексты в хронологическом порядке, вызывает LLM.
7. Создаёт `Summary(post_id=nil, source_id=source.ID)`.
8. Обновляет событие: `status=completed`.

### Ручной запуск

1. Пользователь вызывает `POST /sources/{source_id}/summarize`.
2. Система проверяет: источник существует, `ProcessingMode == cumulative`.
3. Проверяет: нет активного события. Если есть — возвращает `200` с существующим `task_id`.
4. Создаёт `SummaryEvent`, возвращает `202` с `task_id`.
5. Пользователь поллит `GET /summary-events/{task_id}`.
6. Когда `status == completed` — получает `summary_id`, запрашивает `GET /summaries/{id}`.

### Ошибка LLM

1. Worker берёт событие, вызывает LLM.
2. LLM возвращает ошибку (таймаут, 5xx, rate limit).
3. Worker делает retry с exponential backoff (до `max_retries` попыток).
4. После исчерпания retry: `status = failed`, `error = <описание ошибки>`.
5. Событие остаётся в outbox. Ручной retry: пользователь вызывает `POST /sources/{source_id}/summarize` повторно — создаётся новое `SummaryEvent` (failed-событие не блокирует создание).

### Пост без текста

- Если `post.Text` пуст (после trim) — `SummaryEvent` не создаётся. Валидация в `PostUsecase.Create` уже отклоняет посты с пустым `text` (существующее поведение в `ValidateCreatePost`).

### Повторное создание поста (upsert)

- `PostRepo.Save` при конфликте `(source_id, external_id)` возвращает существующий пост.
- Если пост уже существует — `SummaryEvent` не создаётся (пост не новый).

### Cumulative источник без постов в окне

- Worker загружает посты за окно, получает 0 записей.
- Устанавливает `status = completed` без создания Summary.

### Первый запуск cumulative (нет предыдущего саммари)

- Окно: последние 72 часа от текущего момента.

### Устаревшее событие (expired TTL)

1. Worker берёт событие из outbox.
2. Проверяет: `created_at + summary.outbox.event_ttl < now`.
3. Если истёк — устанавливает `status = expired`, пропускает обработку.
4. В лог пишется info с `summary_event_id`, `age`.

## Инварианты

1. Если `Post` сохранён для self-contained источника, `SummaryEvent` гарантированно создан в той же транзакции.
2. Для одного поста не может существовать двух `SummaryEvent` со `status IN (pending, processing)` и одинаковым `event_type`.
3. Для одного источника не может существовать двух `SummaryEvent` со `status IN (pending, processing)` и `event_type = summarize_source`.
4. `Summary.SourceID` всегда заполнен.
5. `Summary.PostID` заполнен тогда и только тогда, когда саммари создано для self-contained поста.
6. `SummaryEvent` со `status = completed` всегда имеет заполненный `SummaryID`.
7. `SummaryEvent` со `status = failed` всегда имеет заполненный `Error`.
8. Для cumulative: окно выборки постов не превышает 72 часа.
9. `SummaryEvent` с `created_at + event_ttl < now` при обработке получает `status = expired`.

## Acceptance Criteria

- [ ] Для self-contained поста: при вызове `PostUsecase.Create` создаются `Post` и `SummaryEvent` в одной транзакции.
- [ ] Для cumulative источника: Cron Worker создаёт `SummaryEvent` по интервалу из конфигурации.
- [ ] Summary Worker обрабатывает `summarize_post` события: загружает пост, вызывает LLM, создаёт Summary.
- [ ] Summary Worker обрабатывает `summarize_source` события: загружает посты за окно (от последнего саммари, макс. 72ч), вызывает LLM, создаёт Summary привязанный к Source.
- [ ] `POST /sources/{source_id}/summarize` создаёт SummaryEvent и возвращает `task_id`. Для self-contained источника — `400`.
- [ ] `GET /summary-events/{id}` возвращает текущий статус события и `summary_id` при завершении.
- [ ] `GET /posts/{post_id}/summaries` возвращает список саммари для поста.
- [ ] `GET /sources/{source_id}/summaries` возвращает список саммари для источника с пагинацией.
- [ ] `GET /summaries/{id}` возвращает саммари по ID.
- [ ] Текст саммари не пустой (валидация после LLM-вызова).
- [ ] Повторная обработка одного поста не создаёт дубликатов SummaryEvent (идемпотентность).
- [ ] Повторный cron-запуск не создаёт дубликатов SummaryEvent при наличии активного события (идемпотентность).
- [ ] При ошибке LLM после retry: SummaryEvent переходит в `failed`, сохраняет текст ошибки.
- [ ] Cumulative без новых постов в окне — SummaryEvent завершается без создания Summary.
- [ ] Устаревшие события (TTL) помечаются `expired` и не обрабатываются.
- [ ] Если пост удалён до обработки воркером — SummaryEvent переходит в `failed` с `error = "post not found"`.
- [ ] Если конкатенированный текст cumulative-источника превышает `summary.cumulative.max_input_chars` — SummaryEvent переходит в `failed`.
- [ ] `POST /sources/{source_id}/summarize` при наличии failed-события создаёт новый SummaryEvent (failed не блокирует).
- [ ] Конфигурация LLM-провайдера (`summary.llm.provider`, `summary.llm.providers`) загружается из config.
- [ ] Операции суммаризации покрыты автотестами: biz/ (TDD, mocked interfaces), data/ (testcontainers), task/ (worker lifecycle).

## Ограничения

- MVP обрабатывает только текстовые посты (BR-05). Посты с другими типами контента игнорируются.
- MVP использует OpenRouter как единственный LLM-провайдер. Конфигурация поддерживает несколько провайдеров для будущего расширения (приоритеты провайдеров — задача отдельной фичи).
- Формат промпта и стратегия обработки длинных текстов определяются на Design-фазе.
- Design обязан определить измеримый критерий качества саммари и способ его проверки (NFR-02).

## Open Questions

Нет открытых вопросов.
