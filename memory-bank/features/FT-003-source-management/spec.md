---
doc_kind: feature
doc_function: spec
purpose: "Feature spec: API управления Source (CRUD + валидация + пагинация) для FT-003. Читать перед implementation-plan и разработкой."
derived_from:
  - brief.md
  - ../../prd/PRD-001-mvp.md
  - ../../domain/architecture.md
  - ../../engineering/database.md
  - ../../engineering/api-contracts.md
status: active
delivery_status: planned
---

# FT-003: Source Management

## Цель

Предоставить API для управления конфигурацией источников сбора данных Feedium в рантайме (создание, изменение, удаление, чтение) без правки кода и повторного деплоя.

## Reference

- Brief: `memory-bank/features/FT-003-source-management/brief.md`
- Upstream: `memory-bank/prd/PRD-001-mvp.md` (BR-01)
- Architecture: `memory-bank/domain/architecture.md`
- DB conventions: `memory-bank/engineering/database.md`
- API conventions: `memory-bank/engineering/api-contracts.md`

## Scope

### Входит

- CRUD над сущностью `Source` через HTTP REST и gRPC (оба транспорта из одного proto-контракта).
- Поддержка 4 фиксированных типов: `telegram_channel`, `telegram_group`, `rss`, `html`.
- Валидация типа и обязательных полей до изменения состояния.
- Персистентное хранение в PostgreSQL (одна таблица `sources` с `config JSONB`).
- Пагинация списка и фильтрация по типу.
- Unit-тесты `biz/` по TDD; тесты-после для `data/service`.

### НЕ входит

- AuthN/AuthZ.
- Сбор данных из источников (FT-008).
- Проверка фактической доступности (ping RSS, существование канала).
- UI.
- Integration/e2e тесты.
- Добавление новых типов источников.
- Soft delete, history/audit, optimistic locking.

## Контекст

`Source` — верхнеуровневая доменная сущность; от неё зависят коллекторы (FT-008), пайплайн (FT-005/006), суммаризация (FT-007). FT-001/FT-002 дают каркас и доступную БД. Режим обработки (`self-contained/cumulative`) определяется типом по BR-01 и задаётся системой.

## Функциональные требования

### FR-1. Создание источника

- Операция: `CreateSource(type, config) -> Source`.
- `type` — один из: `telegram_channel`, `telegram_group`, `rss`, `html`.
- `config` — type-specific объект; обязательные поля:
- `telegram_channel`: `tg_id` (`int64`, обязательно). Опционально: `username` (`string`).
- `telegram_group`: `tg_id` (`int64`, обязательно). Опционально: `username` (`string`).
- `rss`: `feed_url` (`string`, обязательно, валидный URL).
- `html`: `url` (`string`, обязательно, валидный URL).
- Сервер генерирует `id` (UUID v7, time-sortable для стабильной пагинации по `created_at`), `created_at`, `updated_at`.
- `processing_mode` вычисляется по `type` (BR-01) и возвращается в ответе, но в БД не дублируется.
- Ответ: полный объект `Source`.

### FR-2. Изменение источника

- Операция: `UpdateSource(id, type, config) -> Source` (семантика PUT — полная замена параметров).
- `id` неизменяем.
- `type` неизменяем: при попытке изменить `type` запрос отклоняется с ошибкой валидации; состояние не меняется.
- `config` валидируется по тем же правилам, что в FR-1.
- Last-write-wins: optimistic locking не применяется.
- Ответ: обновлённый объект `Source`; поле `updated_at` обновляется.

### FR-3. Удаление источника

- Операция: `DeleteSource(id) -> Empty`.
- Hard delete: строка удаляется из `sources` физически.
- Если `id` не найден — возвращается `NotFound`.

### FR-4. Получение списка источников

- Операция: `ListSources(page_size, page_token, type?) -> (items, next_page_token)`.
- Пагинация: `page_size` с дефолтом 100 и максимумом 500; `page_token` — opaque cursor (рекомендуется `created_at + id`).
- Фильтр `type` — опциональный; если задан — только источники этого типа.
- Порядок: по `created_at ASC`, `id ASC` (стабильный).
- Ответ включает `next_page_token` (пустой, если страница последняя).

### FR-5. Получение одного источника

- Операция: `GetSource(id) -> Source` (вспомогательная для симметрии CRUD и консистентных ответов после Create/Update).
- Если `id` не найден — `NotFound`.

### FR-6. Валидация

- Некорректный `type`, отсутствующие/неверные обязательные поля, невалидный URL, `tg_id == 0` -> запрос отклоняется с кодом `INVALID_ARGUMENT` и диагностикой (перечень невалидных полей с причинами).
- Состояние БД не меняется до прохождения валидации.

### FR-7. Представление `Source` в API

```text
Source {
  id: string (UUID)
  type: enum
  processing_mode: enum (self_contained | cumulative, вычислено)
  config: oneof по type (telegram_channel|telegram_group|rss|html)
  created_at: timestamp
  updated_at: timestamp
}
```

## Нефункциональные требования

1. Хранение — PostgreSQL, Ent schema, goose миграция. Таблица `sources`: `id UUID PK`, `type TEXT NOT NULL`, `config JSONB NOT NULL`, `created_at TIMESTAMPTZ NOT NULL`, `updated_at TIMESTAMPTZ NOT NULL`. Индекс `(type, created_at, id)` для фильтра+пагинации.
2. Контракт — proto в `api/feedium/source/v1/`, HTTP-аннотации `google.api.http` для REST.
3. Слои — `biz/` содержит чистую логику и валидацию; `data/` — Ent-репозиторий; `service/` — тонкий адаптер `proto <-> biz`; маппинг `type -> processing_mode` — в `biz/`.
4. Ошибки — по `engineering/api-contracts.md` (error reasons: `SOURCE_NOT_FOUND`, `SOURCE_INVALID_TYPE`, `SOURCE_INVALID_CONFIG`, `SOURCE_TYPE_IMMUTABLE`).
5. Тесты — `biz/` по TDD (валидация, маппинг mode, бизнес-правила); `data/service` — тесты-после с testcontainers согласно `engineering/testing-policy.md`.
6. Масштаб — 10..1000 источников, низкая частота изменений; одиночный запрос `List` без фильтра помещается в 10 страниц по 100.

## Сценарии и edge cases

### Основной поток

- `Create` RSS `{feed_url: "https://example.com/feed"}` -> `200`, возвращается `Source` с `processing_mode=cumulative`.
- `List` -> созданный источник присутствует.
- `Update` -> меняется `feed_url`, `updated_at` обновляется.
- `Delete` -> `200` (тело — пустой JSON `{}`), последующий `List` не содержит его.

### Ошибки и edge cases

- Неизвестный `type` -> `INVALID_ARGUMENT: SOURCE_INVALID_TYPE`.
- `Create` RSS без `feed_url` -> `INVALID_ARGUMENT: SOURCE_INVALID_CONFIG` с указанием поля.
- `Create telegram_channel` без `tg_id` (только `username`) -> отклонение.
- `Create` с `feed_url = "not-a-url"` -> отклонение.
- `Update` с `type`, отличным от текущего -> `INVALID_ARGUMENT: SOURCE_TYPE_IMMUTABLE`; БД не меняется.
- `Update/Delete/Get` несуществующего `id` -> `NOT_FOUND: SOURCE_NOT_FOUND`.
- Невалидный `page_token` -> `INVALID_ARGUMENT`.
- Два одновременных `Update` одного `id` -> побеждает последний записавший (last-write-wins), потеря промежуточного состояния допустима.
- Пустые состояния: `List` при пустой таблице -> `items: []`, `next_page_token: ""`.
- Фильтр `type` с неизвестным значением -> `INVALID_ARGUMENT`.
- `page_size` за пределами `[1, 500]` -> клэмпится к границе (или отклонение — см. Open Questions).

## Инварианты

- INV-1. `type` источника неизменяем после создания.
- INV-2. `processing_mode` однозначно определяется `type` по BR-01 и никогда не хранится отдельно от него.
- INV-3. `config` всегда соответствует схеме своего `type` (проверено biz-валидацией до записи).
- INV-4. Состояние БД не меняется при проваленной валидации.
- INV-5. `id` глобально уникален (UUID) и неизменяем.
- INV-6. Дублирование `feed_url/tg_id` на уровне БД не запрещено (`uniqueness` не требуется).

## Acceptance Criteria

- [ ] Создание источника каждого из 4 типов проходит через HTTP и gRPC, возвращает `Source` с заполненными `id`, `processing_mode`, `created_at`, `updated_at`.
- [ ] Созданный источник возвращается в `List` (с учётом фильтра по типу и пагинации).
- [ ] `Update` PUT заменяет `config`, `updated_at` увеличивается; попытка сменить `type` отклоняется с `SOURCE_TYPE_IMMUTABLE`.
- [ ] `Delete` hard-удаляет источник; последующий `Get/Delete` того же `id` возвращает `SOURCE_NOT_FOUND`.
- [ ] `List` с пустой таблицей возвращает пустой массив и пустой `next_page_token`.
- [ ] `List` с `page_size=1` на `N>1` источниках возвращает корректный `next_page_token`, обход всех страниц даёт каждый источник ровно один раз.
- [ ] Фильтр `type` в `List` возвращает только источники заданного типа.
- [ ] Создание с неизвестным `type` / отсутствующим обязательным полем / невалидным URL / `tg_id == 0` возвращает `INVALID_ARGUMENT` с перечнем невалидных полей; БД не изменена.
- [ ] `processing_mode` для `telegram_channel/rss/html = self_contained`, для `telegram_group = cumulative` (по BR-01).
- [ ] Миграция goose создаёт таблицу `sources` и индекс `(type, created_at, id)`.
- [ ] Unit-тесты `biz/` покрывают валидацию всех 4 типов, маппинг `type -> mode`, отказ смены `type`. Тесты `data/service` покрывают CRUD через testcontainers.
- [ ] CRUD работает на запущенном процессе без рестарта/редеплоя.

## Ограничения

- Ровно 4 типа источников; добавление нового типа = отдельная фича.
- Без auth — любой вызывающий имеет полный доступ (MVP-допущение).
- Без soft delete, без audit log, без истории изменений.
- Без optimistic locking.
- Без интеграции с коллекторами в рамках FT-003.

## Open Questions

- OQ-1. `processing_mode` для `telegram_group` в BR-01 требует явной фиксации при реализации (значение должно быть вычитано из PRD-001, а не выбрано заново).
- OQ-2. Поведение при `page_size` вне `[1, 500]` — клэмп к границе или `INVALID_ARGUMENT`? Предлагается клэмп (мягче для клиентов), финальное решение — на этапе plan.
- OQ-3. Формат `page_token` — `base64(created_at|id)` или opaque через ent-пагинатор? Решается в plan.
