---
doc_kind: feature
doc_function: spec
purpose: "Feature spec: API управления Post (CRUD + пагинация + дедуп) для FT-004. Читать перед implementation-plan и разработкой."
derived_from:
  - brief.md
  - ../../prd/PRD-001-mvp.md
  - ../../domain/architecture.md
  - ../../engineering/database.md
  - ../../engineering/api-contracts.md
  - ../FT-003-source-management/spec.md
status: active
delivery_status: done
---

# FT-004: Posts Management

## Цель

Предоставить персистентное хранилище постов и API (gRPC + HTTP) для их создания коллекторами, чтения потребителями конвейера и ручной правки/удаления разработчиком во время отладки — без рестарта процесса.

## Reference

- Brief: `memory-bank/features/FT-004-posts-management/brief.md`
- Upstream: `memory-bank/prd/PRD-001-mvp.md`
- Source spec (шаблон подхода): `memory-bank/features/FT-003-source-management/spec.md`
- Architecture: `memory-bank/domain/architecture.md`
- DB conventions: `memory-bank/engineering/database.md`
- API conventions: `memory-bank/engineering/api-contracts.md`
- Testing policy: `memory-bank/engineering/testing-policy.md`

## Scope

### Входит

- CRUD над сущностью `Post` через gRPC и HTTP REST (один proto-контракт).
- Идемпотентное создание по `(source_id, external_id)`.
- Персистентное хранение в PostgreSQL (таблица `posts`, FK на `sources.id`).
- Пагинация списка (cursor-based) с фильтром по `source_id` и выбором поля сортировки (`published_at` | `created_at`).
- Валидация обязательных полей (`source_id`, `external_id`, `published_at`, `text`).
- Unit-тесты `biz/` по TDD; тесты-после для `data/service` с testcontainers.

### НЕ входит

- AuthN/AuthZ.
- UI.
- Полнотекстовый поиск, фильтрация по произвольным полям.
- Сбор постов из источников (FT-003+).
- Валидация существования источника в рантайме за пределами FK-проверки PostgreSQL.
- Soft delete, audit, история изменений, optimistic locking.
- Батч-операции (bulk create/update/delete).
- Поиск по `metadata`.
- Каскадное удаление постов при удалении Source: FK создаётся с политикой `ON DELETE RESTRICT` — попытка удалить Source с существующими постами отклоняется БД. Изменение поведения `DeleteSource` в FT-003 (трансляция FK-ошибки в API-reason) — задача FT-003.

## Контекст

`Post` — доменная сущность, создаваемая коллекторами (FT-003 и далее) и потребляемая последующими модулями (скоринг, ранжирование, суммаризация). Фича разблокирует конвейер: без persist-слоя коллекторы некуда писать, а downstream-модули — неоткуда читать. FT-001/FT-002 предоставляют каркас и инфраструктуру БД. FT-003 создаёт таблицу `sources`, на которую ссылается FK.

## Функциональные требования

### FR-1. Создание поста

- Операция: `CreatePost(source_id, external_id, published_at, author?, text, metadata?) -> Post`.
- Обязательные: `source_id` (UUID существующего Source), `external_id` (непустая строка), `published_at` (timestamp), `text` (непустая строка после trim).
- Опциональные: `author` (строка), `metadata` (`map<string,string>`; рекомендуемые ключи `url`, `language`; набор ключей не валидируется).
- Сервер генерирует `id` (UUID v7, time-sortable), `created_at`, `updated_at`.
- Идемпотентность: при повторном вызове с тем же `(source_id, external_id)` новая запись не создаётся; возвращается существующий `Post` без модификаций и без ошибки.
- Ответ: полный объект `Post`.

### FR-2. Изменение поста

- Операция: `UpdatePost(id, external_id, published_at, author?, text, metadata?) -> Post` (семантика PUT — полная замена изменяемых полей).
- Неизменяемые поля: `id`, `source_id`, `created_at`.
- `text` после замены должен оставаться непустым; `author` может стать `null`.
- `(source_id, external_id)` после обновления должно оставаться уникальным; конфликт -> `ALREADY_EXISTS`.
- `updated_at` обновляется сервером.
- Last-write-wins: optimistic locking не применяется.
- Если `id` не найден -> `NOT_FOUND`.

### FR-3. Удаление поста

- Операция: `DeletePost(id) -> Empty`.
- Hard delete: строка физически удаляется из `posts`.
- Если `id` не найден -> `NOT_FOUND`.
- Операция не влияет на параллельно идущий `CreatePost` (row-level locking PostgreSQL).

### FR-4. Получение одного поста

- Операция: `GetPost(id) -> Post`.
- Если `id` не найден -> `NOT_FOUND`.

### FR-5. Получение списка постов

- Операция: `ListPosts(page_size, page_token, source_id?, order_by?, order_dir?) -> (items, next_page_token)`.
- `page_size`: дефолт 100, максимум 500; значения вне `[1, 500]` клэмпятся к границе.
- `page_token`: cursor-based pagination по ключу `(order_by_value, id)`. Формат — `base64(RFC3339Nano(order_by_value)|UUID)`, как в FT-003 (`internal/data/page_token.go`); helper выносится в общий пакет и переиспользуется.
- Фильтр `source_id` — опциональный; если задан — только посты этого источника.
- `order_by`: `published_at` (default) | `created_at`.
- `order_dir`: `DESC` (default) | `ASC`.
- Сортировка стабильна за счёт tie-breaker `id`.
- Ответ: `items`, `next_page_token` (пустой, если это последняя страница).

### FR-6. Валидация

- Пустой/отсутствующий `source_id`, `external_id`, `published_at`, `text` (после trim) -> `INVALID_ARGUMENT` с перечнем невалидных полей.
- `source_id` не является валидным UUID -> `INVALID_ARGUMENT`.
- `source_id` ссылается на несуществующий Source (FK violation от PostgreSQL) -> `NOT_FOUND` с причиной `POST_SOURCE_NOT_FOUND`.
- Состояние БД не меняется до прохождения валидации.

### FR-7. Представление `Post` в API

```text
Post {
  id: string (UUID)
  source: Source           // вложенный объект, всегда заполнен
  external_id: string
  published_at: timestamp
  author: string (nullable)
  text: string (non-empty)
  metadata: map<string, string> (рекомендуемые ключи: url, language; значения только строки)
  created_at: timestamp
  updated_at: timestamp
}

Source {
  id: string (UUID)
  type: SourceType (enum, см. FT-003: telegram_channel | telegram_group | rss | html)
}
```

- В API возвращается вложенный `Source` (минимальное представление: `id` + `type`), а не плоский `source_id`.
- Полное представление Source (с `config`, `processing_mode`, timestamps) — через FT-003 `GetSource`.
- При записи (`CreatePost`) клиент передаёт `source_id` (UUID) — отдельный input-поле в request-сообщении; в response возвращается развёрнутый `Source`.

## Нефункциональные требования

1. Хранение — PostgreSQL, Ent schema, goose-миграция. Таблица `posts`:
   - `id UUID PK`
   - `source_id UUID NOT NULL REFERENCES sources(id) ON DELETE RESTRICT`
   - `external_id TEXT NOT NULL`
   - `published_at TIMESTAMPTZ NOT NULL`
   - `author TEXT NULL`
   - `text TEXT NOT NULL` (в БД без ограничения длины)
   - `metadata JSONB NOT NULL DEFAULT '{}'` (хранение `map<string,string>` сериализуется как JSON-объект с string-значениями)
   - `created_at TIMESTAMPTZ NOT NULL`
   - `updated_at TIMESTAMPTZ NOT NULL`
   - `UNIQUE (source_id, external_id)`
   - Индексы: `(source_id, published_at DESC, id DESC)`, `(source_id, created_at DESC, id DESC)` — покрывают фильтр+сортировку+пагинацию `ListPosts`.
2. Контракт — proto в `api/feedium/post/v1/`, HTTP-аннотации `google.api.http` для REST.
3. Слои — `biz/` содержит чистую логику, валидацию и маппинг; `data/` — Ent-репозиторий с идемпотентным upsert по `(source_id, external_id)`; `service/` — тонкий адаптер `proto <-> biz`.
4. Ошибки — по `engineering/api-contracts.md`; reasons: `POST_NOT_FOUND`, `POST_INVALID_ARGUMENT`, `POST_SOURCE_NOT_FOUND`, `POST_ALREADY_EXISTS`.
5. Тесты — `biz/` по TDD (валидация, идемпотентность, маппинг); `data/service` — тесты-после с testcontainers. Goroutine leak detection согласно `testing-policy.md`.
6. Масштаб — верхняя граница 10 000 постов/день (PRD-001); ListPosts должен сохранять p95 стабильным на объёме ~10⁶ строк за счёт покрывающих индексов.
7. `biz/` не импортирует инфраструктуру; валидация и бизнес-правила не зависят от Ent.
8. Чтение Post всегда отдаёт вложенный `Source{id,type}` без N+1: `GetPost` использует один SQL-запрос с JOIN на `sources` (или Ent `WithSource()` eager-load в один запрос); `ListPosts` загружает все связанные источники одним дополнительным запросом (`SELECT ... FROM sources WHERE id = ANY($1)`) либо одним JOIN-ом — но не N отдельными запросами на страницу. Тест должен явно проверять количество SQL-запросов на страницу.

## Сценарии и edge cases

### Основной поток

- `CreatePost` c валидными полями -> `200`, возвращается `Post` с заполненными `id`, `created_at`, `updated_at`.
- `GetPost(id)` -> тот же объект.
- `ListPosts(source_id=X)` -> созданный пост присутствует на первой странице при сортировке `published_at DESC`.
- `UpdatePost` -> `text`/`metadata` обновляются, `updated_at` увеличивается.
- `DeletePost` -> `200`, последующий `Get/Delete` -> `NOT_FOUND`.

### Ошибки и edge cases

- `CreatePost` с пустым `text` (или только whitespace) -> `INVALID_ARGUMENT: POST_INVALID_ARGUMENT`.
- `CreatePost` без `author` -> допустимо, `author = null`.
- `CreatePost` с несуществующим `source_id` -> `NOT_FOUND: POST_SOURCE_NOT_FOUND`; состояние БД не изменено.
- `CreatePost` с невалидным UUID в `source_id` -> `INVALID_ARGUMENT`.
- Повторный `CreatePost` с теми же `(source_id, external_id)` -> возвращается существующий пост, новых строк нет, `updated_at` не меняется.
- `UpdatePost` несуществующего `id` -> `NOT_FOUND: POST_NOT_FOUND`.
- `UpdatePost` c `external_id`, уже занятым другой записью того же `source_id` -> `ALREADY_EXISTS: POST_ALREADY_EXISTS`.
- `UpdatePost` с пустым `text` -> `INVALID_ARGUMENT`.
- `DeletePost` несуществующего `id` -> `NOT_FOUND`.
- `ListPosts` на пустой таблице -> `items: []`, `next_page_token: ""`.
- `ListPosts` с `source_id` без постов -> пустой результат.
- `ListPosts` с `page_size=1` на `N>1` постах -> обход всех страниц по `next_page_token` возвращает каждый пост ровно один раз.
- `ListPosts` с `order_by=created_at ASC` -> результат отсортирован по возрастанию `created_at`, tie-break по `id ASC`.
- `ListPosts` с неизвестным `order_by`/`order_dir` -> `INVALID_ARGUMENT`.
- Невалидный `page_token` -> `INVALID_ARGUMENT`.
- Параллельный `CreatePost` и `UpdatePost`/`DeletePost` другого поста — не блокируют друг друга (row-level locking PostgreSQL).
- Очень длинный `text` (> 1 МБ) — принимается (БД не ограничивает); верхний предел транспорта определяется gRPC/HTTP-настройками сервера, явная бизнес-валидация длины не производится.

## Инварианты

- INV-1. `id` глобально уникален (UUID) и неизменяем.
- INV-2. `source_id` и `created_at` неизменяемы после создания.
- INV-3. Пара `(source_id, external_id)` уникальна на уровне БД.
- INV-4. `text` всегда непустой (после trim).
- INV-5. `source_id` всегда ссылается на существующий `Source` (обеспечивается FK).
- INV-6. `CreatePost` идемпотентен по `(source_id, external_id)`.
- INV-7. Состояние БД не меняется при проваленной валидации.
- INV-8. Любой возвращаемый `Post` содержит вложенный `Source{id,type}`; поле никогда не пустое.

## Acceptance Criteria

- [ ] `CreatePost` проходит через HTTP и gRPC, возвращает `Post` с заполненными `id`, `created_at`, `updated_at`.
- [ ] Повторный `CreatePost` с теми же `(source_id, external_id)` возвращает исходный `Post`, количество строк в `posts` не меняется.
- [ ] `CreatePost` с несуществующим `source_id` возвращает `POST_SOURCE_NOT_FOUND`; БД не изменена.
- [ ] `CreatePost` с пустым/whitespace `text` или отсутствующим обязательным полем возвращает `POST_INVALID_ARGUMENT` с перечнем полей.
- [ ] `CreatePost` без `author` сохраняется успешно, `author` = `null` в ответе.
- [ ] `GetPost` возвращает ранее созданный пост со всеми полями, включая `metadata`.
- [ ] `UpdatePost` заменяет все изменяемые поля, `updated_at` увеличивается; попытка сменить `source_id` невозможна (поле отсутствует в контракте Update).
- [ ] `UpdatePost` с `external_id`, конфликтующим по уникальности в пределах `source_id`, возвращает `POST_ALREADY_EXISTS`.
- [ ] `UpdatePost`/`GetPost`/`DeletePost` несуществующего `id` возвращают `POST_NOT_FOUND`.
- [ ] `DeletePost` физически удаляет строку; последующий `GetPost` -> `POST_NOT_FOUND`.
- [ ] `ListPosts` с пустой таблицей возвращает `items: []`, `next_page_token: ""`.
- [ ] `ListPosts` с `page_size=1` и `N>1` постах корректно пагинирует и возвращает каждый пост ровно один раз.
- [ ] `ListPosts` фильтрует по `source_id`, сортирует по `published_at DESC` (default), поддерживает `order_by=created_at` и `order_dir=ASC/DESC`.
- [ ] `ListPosts` с `page_size` вне `[1, 500]` клэмпится к границе.
- [ ] Goose-миграция создаёт таблицу `posts`, уникальный индекс `(source_id, external_id)` и индексы под пагинацию.
- [ ] Unit-тесты `biz/` покрывают: валидацию обязательных полей, идемпотентность Create, конфликт уникальности в Update, запрет пустого `text`.
- [ ] Тесты `data/service` покрывают CRUD + пагинацию через testcontainers.
- [ ] CRUD-операции работают на запущенном процессе без рестарта/редеплоя и не прерывают параллельно идущие Create.
- [ ] Любой ответ (`CreatePost`, `GetPost`, `UpdatePost`, `ListPosts`) содержит вложенный `Source{id,type}` для каждого `Post`.
- [ ] `GetPost` выполняет ровно один SQL-запрос (JOIN/eager-load); `ListPosts(page_size=N)` выполняет O(1) запросов независимо от `N` (≤ 2: посты + источники одним IN-запросом, либо один JOIN). Покрыто тестом подсчёта запросов на testcontainers.

## Ограничения

- Без auth — любой вызывающий имеет полный доступ (MVP-допущение).
- Без soft delete, audit, истории изменений, optimistic locking.
- Без батч-операций и поиска по `metadata`.
- Максимальная пропускная способность ориентирована на 10 000 постов/день; нагрузочное тестирование вне scope.
- Содержимое `metadata` не валидируется по схеме (свободный JSON-объект).

## Open Questions

Все вопросы закрыты:
- ~~OQ-1~~. FK `ON DELETE RESTRICT` (см. Scope «НЕ входит» и НФТ-1).
- ~~OQ-2~~. `page_token` = `base64(RFC3339Nano|UUID)` через переиспользование `internal/data/page_token.go` (FT-003).
- ~~OQ-3~~. `metadata` = `map<string,string>` (см. FR-7).
