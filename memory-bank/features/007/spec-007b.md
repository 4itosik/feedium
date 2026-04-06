# Spec 007b: Orchestration (Triggers & API)

## Dependency
- Brief: `memory-bank/features/007/brief.md`
- Depends on: `memory-bank/features/007/spec-007a.md` (core processing)

## Scope

### Входит
1. Cron scheduler для TELEGRAM_GROUP sources (00:00/12:00 UTC)
2. API для ручного запуска: `POST /sources/{id}/process`
3. Создание OutboxEvent по триггерам (cron, manual, post-save для self-contained)
4. Публичный endpoint (без авторизации)

### НЕ входит
1. Логика обработки событий (worker) — см. spec-007a
2. Модель данных — см. spec-007a
3. Формат результата обработки
4. UI отображения
5. Error handling retries для failed events
6. Конфигурируемый интервал (хардкод 12 часов)
7. Обработка зависших PROCESSING событий

## Triggers

### Cron Schedule (TELEGRAM_GROUP only)
- Fixed schedule: 00:00 UTC, 12:00 UTC
- Хардкод для MVP, не конфигурируется
- Проходим по ВСЕМ источникам типа TELEGRAM_GROUP, создаём события даже для sources с нулём постов
- Один multi-insert запрос на все sources:
  ```sql
  INSERT INTO outbox_events (source_id, post_id, event_type, status, scheduled_at)
  SELECT id, NULL, 'SCHEDULED', 'PENDING', NOW()
  FROM sources
  WHERE type = 'TELEGRAM_GROUP'
  ```
- При ошибке INSERT — ни одно событие не создаётся (атомарный запрос)

### Post-save Trigger (Self-contained only)
- Срабатывает при вызове `post.Service.Create` и `post.Service.Update` для Post с self-contained источником
- OutboxEvent содержит `post_id` конкретного поста
- SQL:
  ```sql
  INSERT INTO outbox_events (source_id, post_id, event_type, status, scheduled_at)
  VALUES (source_id, post_id, 'IMMEDIATE', 'PENDING', NULL)
  ```

## API

### POST /sources/{id}/process

Запускает ручную обработку source.

**Authorization**: None (публичный endpoint)

**Request**: Path parameter `id` (UUID source)

**Behavior**:
```sql
INSERT INTO outbox_events (source_id, post_id, event_type, status, scheduled_at)
VALUES ({id}, NULL, 'MANUAL', 'PENDING', NULL)
```

**Responses**:

| Code | Condition | Body |
|------|-----------|------|
| `202 Accepted` | Событие создано | `{ "event_id": "<uuid>" }` |
| `404 Not Found` | Source с данным id не существует | `{ "error": "source not found" }` |
| `400 Bad Request` | id не является валидным UUID | `{ "error": "invalid source id" }` |

**Processing**: Worker выбирает посты по правилам выборки (см. ниже)

## Правила выборки постов

Зависит от наличия `post_id` в OutboxEvent:

| `post_id` | Условие | Выборка |
|-----------|---------|---------|
| NOT NULL | IMMEDIATE (self-contained) | Один конкретный пост по `post_id` |
| NULL | SCHEDULED / MANUAL (cumulative) | Все посты source за последние 24 часа, у которых нет записей в `summary_posts` |

SQL для cumulative:
```sql
SELECT p.* FROM posts p
WHERE p.source_id = :source_id
  AND p.created_at >= NOW() - INTERVAL '24 hours'
  AND NOT EXISTS (
    SELECT 1 FROM summary_posts sp WHERE sp.post_id = p.id
  )
```

## Сценарии

### Сценарий 1: Сохранение статьи из Telegram-канала
```
User saves post from Telegram Channel
→ System detects Source.type = TELEGRAM_CHANNEL
→ Determines ProcessingMode = SELF_CONTAINED
→ Creates Post (in transaction)
→ Creates OutboxEvent (post_id=post.id, IMMEDIATE, PENDING, scheduled_at=NULL)
→ Worker processes event
→ Creates Summary with event_id
→ Creates summary_posts (1 entry)
```

### Сценарий 2: Сохранение сообщения из Telegram-группы
```
User saves post from Telegram Group
→ System detects Source.type = TELEGRAM_GROUP
→ Determines ProcessingMode = CUMULATIVE
→ Creates Post (NO outbox event created)
→ Cron 00:00/12:00 runs
→ Creates OutboxEvent for ALL TELEGRAM_GROUP sources (даже с нулём постов)
→ Worker processes event at scheduled time
→ Worker выбирает посты за 24ч без записей в summary_posts
→ Creates Summary with event_id
→ Creates summary_posts (N entries for selected posts)
```

### Сценарий 3: Ручной запуск обработки
```
User wants summary for Telegram Group posts earlier
→ Calls POST /sources/{id}/process
→ System creates OutboxEvent (MANUAL, PENDING, scheduled_at=NULL)
→ Response: 202 { "event_id": "<uuid>" }
→ Worker processes event
→ Worker выбирает посты за 24ч без записей в summary_posts
→ Creates Summary with event_id
→ Creates summary_posts for selected posts
```

## Edge Cases

### Edge case 1: Manual для source без подходящих постов
**Condition**: POST /sources/{id}/process, у source нет постов за 24ч или все уже обработаны  
**Behavior**: OutboxEvent создаётся (post_id=NULL), worker находит 0 постов по правилам выборки, создаёт пустой Summary (см. spec-007a)

### Edge case 2: Manual + Scheduled overlap
**Condition**: Manual запуск создан, пока scheduled ещё не выполнен  
**Behavior**: Независимые события, оба обрабатываются, два Summary создаются

### Edge case 3: Обработка во время обработки
**Condition**: Manual запуск для source с активным PROCESSING событием  
**Behavior**: Новое событие создаётся с status = PENDING. Worker выбирает следующее PENDING событие для данного source_id после перевода текущего в COMPLETED/FAILED. Порядок — по `created_at ASC`

### Edge case 4: Публичный API без авторизации
**Condition**: Любой пользователь может вызвать endpoint  
**Behavior**: 202 Accepted, event создаётся (rate limiting out of scope)

## Acceptance Criteria

- [ ] Cron 00:00/12:00 UTC создаёт OutboxEvent для ВСЕХ TELEGRAM_GROUP sources одним multi-insert запросом
- [ ] POST /sources/{id}/process создаёт manual OutboxEvent и возвращает 202 с event_id
- [ ] POST /sources/{id}/process возвращает 404 для несуществующего source и 400 для невалидного UUID
- [ ] Endpoint публичный (без авторизации)
- [ ] Self-contained post save создаёт immediate OutboxEvent
- [ ] Cumulative post save НЕ создаёт OutboxEvent
- [ ] Manual + Scheduled создают независимые события
- [ ] При concurrent processing новое событие ставится в PENDING queue

## Инварианты

1. `outbox_events.source_id` ссылается на существующий Source (FK constraint)
2. Cron НЕ создаёт события для non-TELEGRAM_GROUP sources
3. Один post save (Create/Update) создаёт максимум один OutboxEvent с `post_id` этого поста
4. IMMEDIATE events всегда имеют `post_id` NOT NULL; SCHEDULED/MANUAL — всегда `post_id` NULL
5. Manual trigger всегда создаёт событие, независимо от наличия постов у source
6. Worker обрабатывает события для одного source_id последовательно, по `created_at ASC`
7. Cumulative выборка: только посты за 24ч без записей в `summary_posts`

## Ограничения

1. Расписание фиксировано: 00:00 UTC, 12:00 UTC (хардкод)
2. Для cumulative материалов обработка только по времени/ручному запуску, без проверки "достаточности контекста"
3. API публичный (без авторизации)
4. Нет rate limiting на API
5. Нет приоритезации между immediate/scheduled/manual

## Open Questions
None.
