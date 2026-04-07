# Spec 007a: Core Processing (Outbox + Worker + Summary)

## Dependency
- This spec depends on Brief: `memory-bank/features/007/brief.md`

## Scope

### Входит
1. Модель данных: таблицы `outbox_events`, `summaries`, `summary_posts` (с retry_count)
2. Worker locking через DB-level locking паттерн
3. Worker processing: логика обработки событий (OutboxEvent → Summary)
4. Tracability через `event_id`
5. Processing Mode определение по SourceType (для Worker)
6. Базовый Error Handling: FAILED status, retry_count инкремент

### НЕ входит
1. Создание OutboxEvent (cron, API, post-save hooks, transactional outbox creation) — см. spec-007b
2. Формат `summaries.content` и реализация Processor — определяется в отдельной спеке
3. UI отображения
4. Recovery: автоматический retry для FAILED, heartbeat для stuck PROCESSING
5. Версионирование

## Модель данных

### SQL DDL

```sql
-- События для обработки (transactional outbox)
CREATE TABLE outbox_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_id UUID NOT NULL REFERENCES sources(id),
    post_id UUID NULL REFERENCES posts(id),  -- NOT NULL для self-contained, NULL для cumulative
    event_type VARCHAR(20) NOT NULL,  -- validated in code: IMMEDIATE|SCHEDULED|MANUAL
    status VARCHAR(20) NOT NULL DEFAULT 'PENDING',  -- validated in code: PENDING|PROCESSING|COMPLETED|FAILED
    retry_count INTEGER NOT NULL DEFAULT 0,  -- инкрементируется при каждой ошибке
    scheduled_at TIMESTAMP NULL,  -- NULL для immediate/manual, NOT NULL для scheduled
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_outbox_events_status_scheduled ON outbox_events(status, scheduled_at, created_at);

-- Результаты обработки
CREATE TABLE summaries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_id UUID NOT NULL REFERENCES sources(id),
    event_id UUID NOT NULL REFERENCES outbox_events(id),
    content TEXT NOT NULL,  -- формат определяется отдельно
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_summaries_source_id ON summaries(source_id);
CREATE INDEX idx_summaries_event_id ON summaries(event_id);

-- Связь summary с posts (many-to-many)
CREATE TABLE summary_posts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    summary_id UUID NOT NULL REFERENCES summaries(id) ON DELETE CASCADE,
    post_id UUID NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    UNIQUE(summary_id, post_id)
);

CREATE INDEX idx_summary_posts_summary_id ON summary_posts(summary_id);
CREATE INDEX idx_summary_posts_post_id ON summary_posts(post_id);
```

### Processing Modes

| SourceType | Mode | Event Scope | summary_posts |
|------------|------|-------------|---------------|
| TELEGRAM_CHANNEL | Self-contained | Single post | 1 entry |
| RSS | Self-contained | Single post | 1 entry |
| WEB_SCRAPING | Self-contained | Single post | 1 entry |
| TELEGRAM_GROUP | Cumulative | Batch all posts | N entries |

## Функциональные требования

### FR-001: Worker Event Processing

*Примечание: Создание OutboxEvent описано в spec-007b. В данной спеке предполагается, что события уже созданы и находятся в статусе PENDING.*

#### Behavior

1. Worker выбирает **только** PENDING события в порядке FIFO (created_at ASC)
   - События со статусом FAILED игнорируются
2. Worker пропускает события с `scheduled_at > NOW()`
3. Worker гарантирует mutual exclusion: только один инстанс обрабатывает событие
4. Worker использует `SELECT FOR UPDATE SKIP LOCKED` для DB-level locking
5. Событие захватывается путём атомарного перехода статуса `PENDING → PROCESSING`

#### Processor Interface (used by Worker)
```go
type Processor interface {
    // Process возвращает content для summaries.content
    // Self-contained: вызывается с []Post{single}
    // Cumulative: вызывается с []Post{all unprocessed posts for source}
    Process(ctx context.Context, posts []Post) (content string, err error)
}
```

### FR-003: Создание Summary

#### Self-contained Processing (TELEGRAM_CHANNEL, RSS, WEB_SCRAPING)
1. Найти Post по `outbox_events.post_id` (NOT NULL для self-contained событий)
2. Вызвать `Processor.Process([]Post{post})` → получить content
3. Создать Summary:
   ```sql
   INSERT INTO summaries (source_id, event_id, content)
   VALUES (:source_id, :event_id, :content)
   RETURNING id;
   ```
4. Связать Summary с Post:
   ```sql
   INSERT INTO summary_posts (summary_id, post_id)
   VALUES (:summary_id, :post_id);
   ```

#### Cumulative Processing (TELEGRAM_GROUP)
1. Найти все Posts для source, где:
   - `created_at >= lock_acquired_at - INTERVAL '24 hours'` (где `lock_acquired_at` — timestamp, зафиксированный в момент `SELECT FOR UPDATE SKIP LOCKED`)
   - Нет связанной записи в summary_posts (не обработан ранее)
2. Если таких постов нет: Summary НЕ создаётся, status → COMPLETED
3. Иначе: вызвать `Processor.Process(posts)` → получить content
4. Создать Summary (как в п.3 Self-contained)
5. Связать Summary со всеми обработанными Posts (как в п.4 Self-contained)

#### Завершение обработки
- Успешно: `UPDATE outbox_events SET status='COMPLETED' WHERE id = :event_id`
- Ошибка: см. Error Handling section

#### Post Not Found (Self-contained only)
- **Condition**: Post был удалён после создания OutboxEvent
- **Behavior**: 
  1. `UPDATE outbox_events SET status='FAILED' WHERE id = :event_id`
  2. `retry_count` НЕ инкрементируется (permanent error, retry не поможет)
  3. Error logged: "Post not found, skipping summary"
  4. Worker продолжает со следующим событием

### FR-004: Без событий при удалении
При удалении Post: OutboxEvent НЕ создаётся.

### FR-005: Tracability
Каждый Summary содержит `event_id` → можно найти какое событие породило данный результат.

## Нефункциональные требования

### NFR-001: Расширяемость
Добавление нового SourceType не должно изменять логику существующих:
- Новый тип должен быть явно mapped на ProcessingMode
- Отсутствие mapping = ошибка при сохранении Post

### NFR-002: Надёжность
OutboxEvent для self-contained должен создаваться в той же транзакции, что и Post (transactional outbox pattern).

### NFR-003: Тестируемость
Каждый ProcessingMode должен быть тестируем независимо.

### NFR-004: Производительность
Индексы на `outbox_events(status, scheduled_at, created_at)` и `summary_posts(post_id)` для оптимизации запросов.

## Error Handling

### Обработка ошибок при создании Summary

#### DB Constraint Violation
- **Condition**: Ошибка UNIQUE/NOT NULL/FK при INSERT в summaries или summary_posts
- **Behavior**: 
  1. `UPDATE outbox_events SET status='FAILED', retry_count = retry_count + 1 WHERE id = :event_id`
  2. Error logged
  3. Worker продолжает со следующим событием

### Post Not Found (Self-contained)
- **Condition**: Post был удалён после создания OutboxEvent (не найден по source_id)
- **Behavior**: 
  1. `UPDATE outbox_events SET status='FAILED' WHERE id = :event_id` (retry_count НЕ меняется)
  2. Error logged
  3. Worker продолжает со следующим событием

#### Processor Error
- **Condition**: `Processor.Process()` возвращает error
- **Behavior**: 
  1. Summary НЕ создаётся, записи в summary_posts НЕ создаются
  2. `UPDATE outbox_events SET status='FAILED', retry_count = retry_count + 1 WHERE id = :event_id`
  3. Error logged to stderr
  4. Worker продолжает со следующим событием
  5. **Cumulative mode**: посты остаются необработанными (без связи в summary_posts) и попадут в следующий батч

### Recovery (Out of Scope для MVP)
- Автоматический retry FAILED событий — не входит
- Heartbeat/timeout для stuck PROCESSING событий — не входит
- Dead letter queue — не входит

## Инварианты

1. **Processing Mode Immutability**: Тип обработки определяется при создании Post и не меняется
2. **Transactional Outbox (Self-contained)**: OutboxEvent создаётся атомарно с Post в одной транзакции, post_id NOT NULL
3. **Post ID Constraint**: outbox_events.post_id NOT NULL для self-contained, NULL для cumulative
4. **Worker Safety**: `SELECT FOR UPDATE SKIP LOCKED` предотвращает concurrent processing
5. **At-least-once Processing**: Каждый PENDING OutboxEvent будет обработан минимум один раз
6. **Source Isolation**: Обработка одного SourceType не влияет на другие SourceType
7. **Tracability**: Каждый Summary имеет NOT NULL event_id
8. **Junction Integrity**: summary_posts.id UUID PRIMARY KEY, UNIQUE(summary_id, post_id)

## Ограничения

1. Processing Mode определяется только по Source.type на момент сохранения
2. Хардкод mapping SourceType → ProcessingMode в коде (MVP)
3. Нет приоритезации между immediate/scheduled/manual в пределах одного типа
4. Нет recovery для stuck PROCESSING событий (no heartbeat/timeout) — MVP ограничение
5. Нет автоматического retry для FAILED событий — MVP ограничение
6. DB CHECK constraints отсутствуют, валидация в коде

## Edge Cases

### Edge case 1: Нет необработанных постов за 24h
**Condition**: Worker получает Cumulative event для TELEGRAM_GROUP source, но нет постов за последние 24 часа (от `lock_acquired_at`) без связи в summary_posts  
**Behavior**: Summary НЕ создаётся, status → COMPLETED

### Edge case 2: Невалидный SourceType в событии
**Condition**: OutboxEvent ссылается на Source с SourceType, для которого нет ProcessingMode mapping  
**Behavior**: `status='FAILED'`, `retry_count` НЕ инкрементируется (permanent error)

### Edge case 3: Пост удалён во время обработки
**Condition**: Post удалён после начала обработки OutboxEvent  
**Behavior**: Worker делает SELECT posts в начале обработки (до вызова Processor) и работает с этим результатом, даже если посты удалятся во время обработки (consistency snapshot)

### Edge case 4: Worker падает во время PROCESSING
**Condition**: Worker упал после `UPDATE status='PROCESSING'`  
**Behavior**: Event остаётся PROCESSING (out of scope для MVP, требуется heartbeat/timeout)

## Acceptance Criteria

*Примечание: AC помеченные [spec-007b] тестируются в рамках другой спеки, здесь — предположение/контекст*

### Worker Processing
- [ ] Worker обрабатывает только PENDING события (FAILED игнорируются)
- [ ] Worker обрабатывает события в FIFO порядке (created_at ASC)
- [ ] Worker пропускает события с scheduled_at в будущем
- [ ] Worker гарантирует mutual exclusion: только один инстанс обрабатывает событие
- [ ] Worker делает SELECT FOR UPDATE SKIP LOCKED (или аналог)
- [ ] Worker атомарно меняет статус PENDING → PROCESSING

### Self-contained Processing
- [ ] Один Post → один Summary → один summary_posts entry
- [ ] При ошибке Processor: status='FAILED', retry_count увеличивается на 1
- [ ] При DB constraint violation: status='FAILED', retry_count увеличивается на 1
- [ ] При отсутствии Post (deleted): status='FAILED', retry_count НЕ меняется (permanent error)

### Cumulative Processing
- [ ] Все Posts source (24h от момента начала обработки, без summary_posts связи) → один Summary → N summary_posts entries
- [ ] Cumulative без постов: Summary НЕ создаётся, status → COMPLETED

### Data Integrity
- [ ] Summary.event_id NOT NULL (tracability)
- [ ] При успехе: status='COMPLETED'
- [ ] retry_count можно прочитать из БД и проверить его значение

## Open Questions
None.
