---
doc_kind: feature
doc_function: plan
purpose: "Implementation plan для FT-004: Posts Management. Пошаговый, проверяемый, без кода."
derived_from:
  - spec.md
  - ../../engineering/coding-style.md
  - ../../engineering/testing-policy.md
  - ../../engineering/database.md
  - ../../engineering/api-contracts.md
status: active
delivery_status: done
---

# Implementation Plan — FT-004: Posts Management

## Steps

### Step 1. Обобщить page_token.go

**Цель:** Обобщить `internal/data/page_token.go` для поддержки любого timestamp-поля
сортировки, не только `created_at`. Оба репозитория (source, post) остаются в пакете
`internal/data/` — отдельный подпакет не нужен.

**Действия:**

1. В `pageTokenData` переименовать поле `CreatedAt` → `SortValue`.
2. Обновить `encodePageToken` — переименовать параметр `createdAt` → `sortValue`.
3. Обновить `decodePageToken` — возвращать `SortValue` вместо `CreatedAt`.
4. Обновить `source_repo.go` — заменить обращения к `decodedToken.CreatedAt` на
   `decodedToken.SortValue`.
5. Обновить все call sites и тесты, где используется поле/параметр `CreatedAt`:
   `source_repo_test.go` и любые другие файлы, ссылающиеся на `decodedToken.CreatedAt`
   или `encodePageToken(createdAt, ...)`.

**Зависимости:** нет.

**Результат:** `encodePageToken(sortValue, id)` / `decodePageToken(token)` — универсальные
функции для курсорной пагинации по любому `TIMESTAMPTZ` полю.

**Как проверить:** `go test ./internal/data/...` — существующие тесты FT-003 зелёные.

---

### Step 2. Proto-контракт

**Цель:** Определить proto-сообщения и сервис для Post CRUD, добавить error reasons.

**Действия:**

**Pre-condition:** Разрешить OQ-1 (metadata: `map<string,string>` vs `message PostMetadata`) — решение зафиксировано ниже.

1. Создать `api/feedium/post.proto`:
   - Enum `PostSortField`: `POST_SORT_FIELD_UNSPECIFIED = 0`,
     `POST_SORT_FIELD_PUBLISHED_AT = 1`, `POST_SORT_FIELD_CREATED_AT = 2`.
   - Enum `SortDirection`: `SORT_DIRECTION_UNSPECIFIED = 0`,
     `SORT_DIRECTION_DESC = 1`, `SORT_DIRECTION_ASC = 2`.
   - Сообщение `PostSourceRef` — минимальное представление Source внутри Post:
     `string id`, `SourceType type`.
   - Сообщение `Post`: `id`, `source` (PostSourceRef), `external_id`,
     `published_at` (Timestamp), `author` (string, optional),
     `text` (string), `metadata` (map<string,string>),
     `created_at`, `updated_at`.
   - Request/Response для каждого RPC: `V1CreatePost`,
     `V1UpdatePost`, `V1DeletePost`, `V1GetPost`, `V1ListPosts`.
   - `V1CreatePostRequest`: `source_id` (string), `external_id`, `published_at`,
     `author` (optional string), `text`, `metadata` (optional map).
   - `V1UpdatePostRequest`: `id`, `external_id`, `published_at`, `author`,
     `text`, `metadata`. Поле `source_id` отсутствует — неизменяемо.
   - `V1ListPostsRequest`: `page_size` (int32), `page_token` (string),
     `source_id` (optional string), `order_by` (PostSortField),
     `order_dir` (SortDirection).
   - Маппинг UNSPECIFIED → default: в service/ при `POST_SORT_FIELD_UNSPECIFIED`
     передавать biz-дефолт `SortByPublishedAt`; при `SORT_DIRECTION_UNSPECIFIED`
     → `SortDesc`. Proto3 default=0 неотличим от "не задан".
   - `V1ListPostsResponse`: `repeated Post items`, `next_page_token`.
   - Service `PostService` с пятью RPC + `google.api.http` аннотации:
     - POST `/v1/posts` (Create)
     - PUT `/v1/posts/{id}` (Update)
     - DELETE `/v1/posts/{id}` (Delete)
     - GET `/v1/posts/{id}` (Get)
     - GET `/v1/posts` (List)
2. Обновить `api/feedium/error_reason.proto` — добавить значения:
    `ERROR_REASON_POST_NOT_FOUND`, `ERROR_REASON_POST_INVALID_ARGUMENT`,
    `ERROR_REASON_POST_SOURCE_NOT_FOUND`, `ERROR_REASON_POST_ALREADY_EXISTS`.
3. Запустить `make proto`.
4. Закоммитить сгенерированные файлы.

**Зависимости:** нет.

**Результат:** Сгенерированный Go-код для Post-контракта в `api/feedium/`.

**Как проверить:** `make proto` без ошибок; `go build ./api/...` компилируется.

---

### Step 3. Goose-миграция

**Цель:** Создать таблицу `posts` с FK, уникальным ограничением и индексами пагинации.

**Действия:**

1. Создать файл `migrations/YYYYMMDDHHMMSS_create_posts_table.sql`:
   - `CREATE TABLE posts`:
     - `id UUID PRIMARY KEY`
     - `source_id UUID NOT NULL REFERENCES sources(id) ON DELETE RESTRICT`
     - `external_id TEXT NOT NULL`
     - `published_at TIMESTAMPTZ NOT NULL`
     - `author TEXT NULL`
     - `text TEXT NOT NULL`
     - `metadata JSONB NOT NULL DEFAULT '{}'`
     - `created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`
     - `updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`
   - `UNIQUE (source_id, external_id)`
   - Индекс `idx_posts_source_published_id`:
     `(source_id, published_at DESC, id DESC)`
   - Индекс `idx_posts_source_created_id`:
     `(source_id, created_at DESC, id DESC)`
   - Down: `DROP TABLE IF EXISTS posts`.

**Зависимости:** таблица `sources` существует (FT-003).

**Результат:** Миграция, создающая таблицу `posts`.

**Как проверить:** `goose -dir migrations <dsn> up` → `\d posts` в psql показывает
таблицу, FK, UNIQUE constraint, оба индекса.

---

### Step 4. Ent-схема Post

**Цель:** Описать сущность Post в Ent для генерации Go-кода + edge на Source.

**Действия:**

1. Создать `internal/ent/schema/post.go`:
   - Поля:
     - `id` — UUID, Default `uuid.NewV7()`, Immutable.
     - `source_id` — UUID, NotEmpty.
     - `external_id` — String, NotEmpty.
     - `published_at` — Time, timestamptz, NotEmpty.
     - `author` — String, Optional/Nillable.
     - `text` — String, NotEmpty.
     - `metadata` — JSON `map[string]string`, jsonb, Default `map[string]string{}`.
     - `created_at` — Time, timestamptz, Default `time.Now`, Immutable.
     - `updated_at` — Time, timestamptz, Default `time.Now`, UpdateDefault `time.Now`.
   - Edge: `source` — M2O на `Source` (для Ent eager-load / `WithSource()`).
   - Индексы: `(source_id, published_at, id)`, `(source_id, created_at, id)`.
2. Запустить `go generate ./internal/ent/...` (или `go generate ./internal/ent/generate.go`).
3. Закоммитить сгенерированный код.

**Зависимости:** Ent-схема Source (FT-003).

**Результат:** Сгенерированный Go-код `internal/ent/post*.go`, `internal/ent/schema/post.go`.

**Как проверить:** `go generate ./internal/ent/...` без ошибок; `go build ./internal/ent/...` компилируется.

---

### Step 5. biz/ — Доменные типы, ошибки, PostRepo интерфейс (TDD: Red)

**Цель:** Определить доменную модель, sentinel errors, интерфейс репозитория и чистые
функции валидации. Написать падающие тесты.

**Действия:**

1. Создать `internal/biz/post.go`:
   - Тип `SourceInfo` struct: `ID string`, `Type SourceType`.
   - Тип `Post` struct: `ID`, `SourceID`, `Source SourceInfo`, `ExternalID`,
     `PublishedAt time.Time`, `Author *string`, `Text string`,
     `Metadata map[string]string`, `CreatedAt time.Time`, `UpdatedAt time.Time`.
   - Тип `SortField`: константы `SortByPublishedAt`, `SortByCreatedAt`.
   - Тип `SortDirection`: константы `SortDesc`, `SortAsc`.
   - Тип `ListPostsFilter`: `SourceID string`, `PageSize int`, `PageToken string`,
     `OrderBy SortField`, `OrderDir SortDirection`.
   - Тип `ListPostsResult`: `Items []Post`, `NextPageToken string`.
    - Sentinel errors (имена доменные, без префикса; префикс `ERROR_REASON_` — только
      в proto): `ErrPostNotFound`, `ErrPostInvalidArgument`,
      `ErrPostSourceNotFound`, `ErrPostAlreadyExists`.
   - Функция `ValidateCreatePost(sourceID, externalID, text string, publishedAt time.Time) error`:
     - `sourceID` — непустой, валидный UUID.
     - `externalID` — непустая строка.
     - `text` — непустая после `strings.TrimSpace`.
     - `publishedAt` — не Zero (non-zero time).
     - Возвращает `ErrPostInvalidArgument` с перечнем невалидных полей.
   - Функция `ValidateUpdatePost(externalID, text string, publishedAt time.Time) error`:
     - Те же правила для `externalID`, `text`, `publishedAt`.
   - Функция `ValidateListPostsFilter(filter ListPostsFilter) error`:
     - `OrderBy` — один из допустимых значений.
     - `OrderDir` — один из допустимых значений.
     - `SourceID` — если непустой, валидный UUID.
    - Интерфейс `PostRepo`:
      - `Save(ctx, Post) (Post, error)` — идемпотентный upsert: при конфликте
        `(source_id, external_id)` возвращает существующий Post без модификации.
      - `Update(ctx, Post) (Post, error)`.
      - `Delete(ctx, id string) error`.
      - `Get(ctx, id string) (Post, error)`.
      - `List(ctx, ListPostsFilter) (ListPostsResult, error)`.
2. Создать `internal/biz/post_test.go` — падающие тесты:
   - `TestValidateCreatePost`: пустой sourceID / невалидный UUID / пустой externalID /
     пустой text / whitespace-only text / zero publishedAt / валидный набор.
   - `TestValidateUpdatePost`: пустой externalID / пустой text / валидный.
   - `TestValidateListPostsFilter`: невалидный OrderBy / невалидный OrderDir /
     невалидный SourceID / валидный.

**Зависимости:** нет.

**Результат:** Падающие тесты валидации + компилируемый интерфейс PostRepo.

**Как проверить:** `go test ./internal/biz/... -run TestValidate` — падают (Red).

---

### Step 6. biz/ — PostUsecase (TDD: Red → Green → Refactor)

**Цель:** Реализовать PostUsecase с бизнес-правилами: идемпотентность Create, PUT-семантика
Update, валидация, клэмпинг page_size.

**Действия:**

1. Написать падающие тесты (`internal/biz/post_usecase_test.go`):

   **Create:**
    - Валидные поля → `repo.Save` вызван, результат содержит ID, SourceInfo, timestamps.
    - Невалидные поля → ошибка до обращения к repo (mock не вызывается).
    - Идемпотентность: повторный `repo.Save` с теми же (source_id, external_id)
      возвращает тот же Post (upsert-семантика Save). Mock настроен: первый вызов
      создаёт Post, второй вызов возвращает тот же Post без модификации.
    - Отсутствующий author → допустим, `Author = nil`.

   **Update:**
   - Валидный → `repo.Get(id)` найден → `repo.Update` вызван с обновлёнными полями,
     `UpdatedAt` увеличен.
   - Not found → `repo.Get(id)` → `ErrPostNotFound`.
   - Конфликт external_id → `repo.Update` возвращает `ErrPostAlreadyExists`.
   - Пустой text → ошибка валидации до обращения к repo.

   **Delete:**
   - Валидный → `repo.Delete` вызван.
   - Not found → `repo.Delete` возвращает `ErrPostNotFound`.

   **Get:**
   - Найден → возвращён Post с SourceInfo.
   - Not found → `ErrPostNotFound`.

   **List:**
   - page_size < 1 → клэмпится к 1, repo.List вызван с PageSize=1.
   - page_size > 500 → клэмпится к 500.
   - Невалидный OrderBy → `ErrPostInvalidArgument`.
   - Невалидный OrderDir → `ErrPostInvalidArgument`.
   - Валидный фильтр → repo.List вызван, результат возвращён.

2. Реализовать `internal/biz/post.go` — `PostUsecase`:

   - `NewPostUsecase(repo PostRepo) *PostUsecase`.
    - `Create(ctx, sourceID, externalID, text string, publishedAt time.Time, author *string, metadata map[string]string) (Post, error)`:
      1. `ValidateCreatePost(...)` — при ошибке → возврат.
      2. Сформировать `Post`: сгенерировать ID (UUID v7), заполнить timestamps,
         `metadata` default `{}` если nil.
      3. `repo.Save(ctx, post)` — идемпотентный upsert. При конфликте
         `(source_id, external_id)` Save возвращает существующий Post без
         модификации (INV-6). FK violation (source не существует) транслируется
         из data/ как `ErrPostSourceNotFound`.
   - `Update(ctx, id, externalID, text string, publishedAt time.Time, author *string, metadata map[string]string) (Post, error)`:
     1. `ValidateUpdatePost(...)` — при ошибке → возврат.
     2. `repo.Get(ctx, id)` — если not found → `ErrPostNotFound`.
     3. Заменить изменяемые поля (external_id, published_at, author, text, metadata),
        обновить `updated_at`. `source_id` и `created_at` не меняются.
     4. `repo.Update(ctx, post)` — unique violation → `ErrPostAlreadyExists`.
   - `Delete(ctx, id) error` — делегирует `repo.Delete`.
   - `Get(ctx, id) (Post, error)` — делегирует `repo.Get`.
   - `List(ctx, filter) (ListPostsResult, error)`:
     1. Клэмп `PageSize` к `[1, 500]`.
     2. `ValidateListPostsFilter(filter)` — при ошибке → возврат.
     3. `repo.List(ctx, filter)`.

3. Refactor: убрать дублирование, проверить naming, убедиться что biz/ не импортирует
   инфраструктуру.

**Зависимости:** Step 5 (PostRepo интерфейс, типы).

**Результат:** Зелёные тесты PostUsecase.

**Как проверить:** `go test ./internal/biz/... -run TestPostUsecase` — все зелёные.

---

### Step 7. data/ — PostRepo реализация

**Цель:** Реализовать `biz.PostRepo` через Ent: идемпотентный Save, JOIN/eager-load
SourceInfo, cursor-пагинация с O(1) SQL-запросов.

**Действия:**

1. Создать `internal/data/post_repo.go`:

   - `postRepo struct { data *Data }`.
   - Compile-time: `var _ biz.PostRepo = (*postRepo)(nil)`.
   - `NewPostRepo(data *Data) *postRepo`.

    **Save(ctx, post) — идемпотентный upsert:**
    1. Ent `Post.Create().Set*(...).OnConflictColumns(post.FieldSourceID, post.FieldExternalID).Ignore().Return*(...)`.
    2. Если создана новая строка → вернуть маппинг с SourceInfo (eager-load Source).
    3. Если ON CONFLICT DO NOTHING (0 affected rows) → SELECT по
       `(source_id, external_id)` с `WithSource()`.
    4. При FK violation (source_id не существует) → перевести Ent/PG ошибку в
       `biz.ErrPostSourceNotFound`.

    **Update(ctx, post):**
   1. `UpdateOneID(id).Set*(...)` с новыми значениями полей.
   2. Not found → `biz.ErrPostNotFound`.
   3. Unique violation `(source_id, external_id)` → `biz.ErrPostAlreadyExists`.
   4. Вернуть обновлённый Post с SourceInfo.

   **Delete(ctx, id):**
   1. `DeleteOneID(id).Exec(ctx)`.
   2. Not found → `biz.ErrPostNotFound`.

   **Get(ctx, id):**
   1. `Post.Get(ctx, id)` с `WithSource()` — ровно один SQL-запрос (JOIN/eager-load).
   2. Not found → `biz.ErrPostNotFound`.
   3. Маппинг с SourceInfo.

   **List(ctx, filter):**
   1. `Post.Query()`:
      - Если `filter.SourceID != ""` → WHERE source_id = $1.
      - Order: `(order_by field, id)` по `filter.OrderBy` и `filter.OrderDir`.
      - Cursor: если `filter.PageToken != ""` → decode через `decodePageToken`,
        применить comparison (`<` для DESC, `>` для ASC) по `(sort_value, id)`.
      - Limit: `filter.PageSize + 1` (для обнаружения next page).
   2. После получения N+1 строк: первые N — результат, (N+1)-й — признак next page.
   3. Сбор source_id из результата → batch-load sources:
      `sr.data.Ent.Source.Query().Where(source.IDIn(sourceIDs...)).All(ctx)`.
      Один дополнительный запрос независимо от N.
   4. Assembly: каждый Post получает SourceInfo из загруженных sources.
   5. `NextPageToken` = `encodePageToken(lastItem.sortValue, lastItem.ID)`.
   6. Декодированный из токена `sortValue` — это значение поля `order_by`
      (published_at или created_at), не фиксированное created_at.

2. Создать helper `mapEntPostToDomain(entPost, *ent.Source) biz.Post` — маппинг
   Ent-структуры в доменную, включая SourceInfo.

**Зависимости:** Steps 1, 3, 4, 5 (pagetoken, миграция, Ent-схема, PostRepo интерфейс).

**Результат:** `postRepo` реализует `biz.PostRepo`.

**Как проверить:** `go build ./internal/data/...` — компилируется.

---

### Step 8. service/ — PostService адаптер

**Цель:** Тонкий адаптер proto ↔ biz для Post CRUD.

**Действия:**

1. Создать `internal/service/post/post.go`:

   - Интерфейс `Usecase` (определяется здесь, как в FT-003):
     `Create`, `Update`, `Delete`, `Get`, `List` с доменными сигнатурами.
   - `PostService struct` — embed `feedium.UnimplementedPostServiceServer`.
   - `NewPostService(uc Usecase) *PostService`.
   - Методы-адаптеры:
     - `V1CreatePost`: маппинг proto → домен (source_id из req, metadata default `{}`),
       вызов `uc.Create`, маппинг результата → proto (Post + SourceRef).
     - `V1UpdatePost`: маппинг, вызов `uc.Update`, маппинг результата.
     - `V1DeletePost`: вызов `uc.Delete`.
     - `V1GetPost`: вызов `uc.Get`, маппинг.
     - `V1ListPosts`: сборка `ListPostsFilter` из req (OrderBy/OrderDir из proto enum),
       вызов `uc.List`, маппинг.
    - Маппинг ошибок: доменные → gRPC status errors:
      - `ErrPostNotFound` → `codes.NotFound` + `ERROR_REASON_POST_NOT_FOUND`
      - `ErrPostInvalidArgument` → `codes.InvalidArgument` + `ERROR_REASON_POST_INVALID_ARGUMENT`
      - `ErrPostSourceNotFound` → `codes.NotFound` + `ERROR_REASON_POST_SOURCE_NOT_FOUND`
      - `ErrPostAlreadyExists` → `codes.AlreadyExists` + `ERROR_REASON_POST_ALREADY_EXISTS`
   - Маппинг `biz.SourceInfo` → proto `PostSourceRef`.
   - Маппинг `biz.Post.Metadata` (map[string]string) ↔ proto map<string,string>.

2. Создать `internal/service/post/wire.go`: `ProviderSet = wire.NewSet(NewPostService)`.

3. Создать `internal/service/post/mock/` — `//go:generate mockgen` директива для Usecase.

**Зависимости:** Steps 2 (proto), 6 (PostUsecase интерфейс).

**Результат:** PostService реализует proto `PostServiceServer`.

**Как проверить:** `go build ./internal/service/post/...` — компилируется.

---

### Step 9. Wire интеграция

**Цель:** Связать все слои: data → biz → service → server.

**Действия:**

1. Обновить `internal/data/wire.go`:
   - Добавить `NewPostRepo`.
   - Добавить `wire.Bind(new(biz.PostRepo), new(*postRepo))`.
2. Обновить `internal/biz/wire.go`:
   - Добавить `NewPostUsecase`.
3. Обновить `internal/server/http.go`:
   - Добавить import `"github.com/4itosik/feedium/internal/service/post"`.
   - Добавить параметр `ps *postservice.PostService` в `NewHTTPServer`.
   - Добавить `feediumv1.RegisterPostServiceHTTPServer(srv, ps)`.
4. Обновить `internal/server/grpc.go`:
   - Добавить import `"github.com/4itosik/feedium/internal/service/post"`.
   - Добавить параметр `ps *postservice.PostService` в `NewGRPCServer`.
   - Добавить `feediumv1.RegisterPostServiceServer(srv, ps)`.
5. Запустить `go generate ./cmd/feedium/...` (Wire).
6. Запустить `go generate ./internal/service/post/...` (mockgen).
7. Закоммитить `wire_gen.go` и моки.

**Зависимости:** Steps 7, 8.

**Результат:** PostService доступен через HTTP и gRPC при старте приложения.

**Как проверить:** `go generate ./... && go build ./cmd/feedium/...` — компилируется
без ошибок. Ручной smoke-test (опционально, требует запущенной PostgreSQL и применённых
миграций `goose -dir migrations <dsn> up`, а также запущенного `cmd/feedium`):
`POST /v1/posts` → 200.

---

### Step 10. Тесты data/ — интеграционные (testcontainers)

**Цель:** Проверить PostRepo CRUD + идемпотентность + пагинация + N+1 protection
против реальной PostgreSQL.

**Действия:**

1. Создать `internal/data/post_repo_test.go`:

   Setup: testcontainers PostgreSQL + goose migrations (переиспользовать паттерн
   из `source_repo_test.go`). Перед каждым тестом: создать минимум один Source
   для FK.

   Тесты (все `TestIntegration_*`):

   - **Save — валидный**: создать Post с заполненными полями → возвращается Post
     с ID, SourceInfo(ID+Type), ненулевыми timestamps, metadata сохранён.
    - **Save — идемпотентность**: повторный Save с теми же (source_id, external_id) →
      тот же ID, тот же updated_at (явный assertion: `result.UpdatedAt.Equal(firstSave.UpdatedAt)`),
      кол-во строк не увеличилось.
   - **Save — source не найден**: несуществующий source_id → `ErrPostSourceNotFound`.
   - **Save — author nil**: Post без author → сохранён, `Author == nil`.
   - **Update — валидный**: обновление text, metadata → updated_at увеличился,
     source_id и created_at не изменились, SourceInfo заполнен.
   - **Update — not found**: несуществующий id → `ErrPostNotFound`.
   - **Update — конфликт external_id**: external_id уже занят другой записью того же
     source → `ErrPostAlreadyExists`.
   - **Delete — валидный**: удаление → последующий Get → `ErrPostNotFound`.
   - **Delete — not found**: несуществующий id → `ErrPostNotFound`.
   - **Get — найден**: Post возвращён с заполненным SourceInfo.
   - **Get — not found**.
   - **List — пустая таблица**: `items: []`, `next_page_token: ""`.
   - **List — фильтр по source_id**: только посты указанного источника.
   - **List — source_id без постов**: пустой результат.
   - **List — пагинация**: page_size=1, N>1 постов → обход всех страниц по
     next_page_token, каждый пост ровно один раз, общее кол-во совпадает.
   - **List — sort published_at DESC**: порядок убывания published_at, tie-break по id.
   - **List — sort created_at ASC**: порядок возрастания created_at, tie-break по id ASC.
   - **List — page_size клэмпинг**: page_size=0 → page_size=1; page_size=1000 → page_size=500.
   - **Get — SQL query count**: GetPost выполняет ровно 1 SQL-запрос
     (проверить через Ent driver wrapper / query logger, считающий запросы).
   - **List — SQL query count**: ListPosts(page_size=N) выполняет ≤ 2 SQL-запросов
     (посты + источники batch-load) независимо от N.

**Зависимости:** Steps 3, 4, 7.

**Результат:** Интеграционные тесты data/ с testcontainers.

**Как проверить:** `go test ./internal/data/... -run Integration -v` — все зелёные.

---

### Step 11. Тесты service/

**Цель:** Проверить маппинг proto ↔ домен и конвертацию ошибок в PostService.

**Действия:**

1. Создать `internal/service/post/post_test.go`:

   Setup: mockgen Usecase interface.

   Тесты:

   - **V1CreatePost — валидный**: mock Create возвращает Post с SourceInfo → response
     содержит Post с SourceRef{id, type}.
   - **V1CreatePost — пустой text**: валидация в biz → INVALID_ARGUMENT.
   - **V1CreatePost — source not found**: FK violation → NOT_FOUND: POST_SOURCE_NOT_FOUND.
    - **V1CreatePost — idempotent**: mock Create возвращает существующий Post
      (upsert Save в biz) → корректный маппинг, нет ошибки.
   - **V1UpdatePost — валидный**: mock Update → корректный маппинг.
   - **V1UpdatePost — not found**: → NOT_FOUND: POST_NOT_FOUND.
   - **V1UpdatePost — conflict**: → ALREADY_EXISTS: POST_ALREADY_EXISTS.
   - **V1DeletePost — валидный**: → OK.
   - **V1DeletePost — not found**: → NOT_FOUND.
   - **V1GetPost — найден**: → Post с SourceRef, metadata, author.
   - **V1GetPost — not found**: → NOT_FOUND.
   - **V1ListPosts — валидный**: → items, next_page_token.
   - **V1ListPosts — невалидный order_by**: biz возвращает ошибку → INVALID_ARGUMENT.
   - **V1ListPosts — невалидный order_dir**: biz возвращает ошибку → INVALID_ARGUMENT.
   - **V1ListPosts — page_size clamping**: передан 0 → клэмпится в biz.

2. Сгенерировать мок: `go generate ./internal/service/post/...`.

**Зависимости:** Steps 6, 8.

**Результат:** Unit-тесты service/post.

**Как проверить:** `go test ./internal/service/post/...` — зелёные.

---

### Step 12. Финальная верификация и линтинг

**Цель:** Убедиться, что вся реализация корректна и соответствует стандартам проекта.

**Действия:**

1. `golangci-lint run ./... -c .golangci.yml` — без ошибок.
2. `go test ./...` — все тесты (unit + integration) зелёные.
3. `go vet ./...` — без замечаний.
4. Проверка acceptance criteria из spec по чек-листу (см. секцию Verification).
5. Simplify review: просмотр на premature abstractions, дублирование, dead code,
   overengineering.
6. Обновить документацию:
   - Обновить `memory-bank/features/index.md`: `delivery_status` FT-004 → `done`,
     добавить артефакты (spec, implement) в таблицу.
   - Обновить `memory-bank/index.md`: добавить FT-004 с актуальными артефактами.
   - Добавить cross-references code ↔ docs (по `dna/cross-references.md`):
     комментарии-ссылки в новых Go-файлах на canonical-документы, ссылки из
     документации на файлы реализации.

**Зависимости:** все предыдущие шаги.

**Результат:** Чистая кодовая база, готовая к code review.

**Как проверить:** Все команды выше завершаются без ошибок.

---

## Edge Cases

| # | Сценарий | Ожидание | Слой теста |
|---|----------|----------|------------|
| EC-1 | CreatePost с пустым text (или только whitespace) | `INVALID_ARGUMENT: ERROR_REASON_POST_INVALID_ARGUMENT` | biz/, service/ |
| EC-2 | CreatePost без author | `author = nil` в ответе | biz/, data/ |
| EC-3 | CreatePost с несуществующим source_id | `NOT_FOUND: ERROR_REASON_POST_SOURCE_NOT_FOUND`; БД не изменена | data/, service/ |
| EC-4 | CreatePost с невалидным UUID в source_id | `INVALID_ARGUMENT: ERROR_REASON_POST_INVALID_ARGUMENT` | biz/, service/ |
| EC-5 | Повторный CreatePost с теми же (source_id, external_id) | Возврат существующего Post через upsert Save без модификации, без ошибки | biz/, data/ |
| EC-6 | UpdatePost несуществующего id | `NOT_FOUND: ERROR_REASON_POST_NOT_FOUND` | biz/, data/, service/ |
| EC-7 | UpdatePost с external_id, конфликтующим с другой записью того же source | `ALREADY_EXISTS: ERROR_REASON_POST_ALREADY_EXISTS` | biz/, data/, service/ |
| EC-8 | UpdatePost с пустым text | `INVALID_ARGUMENT: ERROR_REASON_POST_INVALID_ARGUMENT` | biz/, service/ |
| EC-9 | DeletePost несуществующего id | `NOT_FOUND: ERROR_REASON_POST_NOT_FOUND` | biz/, data/, service/ |
| EC-10 | ListPosts на пустой таблице | `items: []`, `next_page_token: ""` | data/ |
| EC-11 | ListPosts с source_id без постов | `items: []`, `next_page_token: ""` | data/ |
| EC-12 | ListPosts с page_size=1 на N>1 постах | Обход всех страниц, каждый пост ровно один раз | data/ |
| EC-13 | ListPosts с order_by=created_at ASC | Корректная сортировка с tie-breaker по id ASC | data/ |
| EC-14 | ListPosts с неизвестным order_by/order_dir | `INVALID_ARGUMENT: ERROR_REASON_POST_INVALID_ARGUMENT` | biz/, service/ |
| EC-15 | ListPosts с невалидным page_token | `INVALID_ARGUMENT` (из decodePageToken) | data/ |
| EC-16 | ListPosts с page_size < 1 или > 500 | Клэмп к границе [1, 500] | biz/ |
| EC-17 | Параллельный CreatePost и UpdatePost/DeletePost другого поста | Не блокируют друг друга (PostgreSQL row-level locking) | implicit (MVCC) |
| EC-18 | Очень длинный text (> 1 МБ) | Принимается, нет бизнес-валидации длины | implicit |
| EC-19 | CreatePost с metadata=nil | Сохраняется как `{}` в БД | biz/, data/ |
| EC-20 | Race: два параллельных CreatePost с одинаковыми (source_id, external_id) | Оба вызывают Save (upsert) — один создаёт, второй получает существующий через ON CONFLICT DO NOTHING + fallback SELECT | data/ |
| EC-21 | ListPosts без фильтра source_id | Все посты, отсортированные по order_by | data/ |

---

## Verification

### Приёмочные тесты (по acceptance criteria из spec)

Проверка выполняется на запущенном процессе (HTTP + gRPC) с реальной PostgreSQL.

1. **AC-CreatePost**: `POST /v1/posts` с валидными полями → 200, ответ содержит Post
   с заполненными `id`, `created_at`, `updated_at`, `source{id,type}`.
   Повторный вызов с теми же (source_id, external_id) → тот же `id`, кол-во строк
   в `posts` не изменилось.

2. **AC-CreatePost-errors**:
    - Несуществующий source_id → `ERROR_REASON_POST_SOURCE_NOT_FOUND`.
    - Пустой/whitespace text или отсутствующее обязательное поле →
      `ERROR_REASON_POST_INVALID_ARGUMENT` с перечнем полей.
   - Без author → 200, `author = null`.

   3. **AC-GetPost**: `GET /v1/posts/{id}` → Post со всеми полями включая metadata и
    SourceRef. Несуществующий id → `ERROR_REASON_POST_NOT_FOUND`.

4. **AC-UpdatePost**: `PUT /v1/posts/{id}` заменяет изменяемые поля, `updated_at`
   увеличивается. Поле `source_id` отсутствует в запросе — нельзя изменить.
    Конфликт external_id → `ERROR_REASON_POST_ALREADY_EXISTS`. Несуществующий id → `ERROR_REASON_POST_NOT_FOUND`.

5. **AC-DeletePost**: `DELETE /v1/posts/{id}` → 200. Последующий GetPost →
    `ERROR_REASON_POST_NOT_FOUND`. Несуществующий id → `ERROR_REASON_POST_NOT_FOUND`.

6. **AC-ListPosts**:
   - Пустая таблица → `items: []`, `next_page_token: ""`.
   - page_size=1, N>1 → корректная пагинация, каждый пост ровно один раз.
   - Фильтр по source_id → только посты этого источника.
   - order_by=published_at DESC (default), order_by=created_at, order_dir=ASC/DESC.
   - page_size вне [1,500] → клэмпится.

7. **AC-Миграция**: `\d posts` показывает FK на sources с ON DELETE RESTRICT,
   UNIQUE(source_id, external_id), оба индекса.

8. **AC-Тесты**: `go test ./...` — все зелёные. `biz/` покрывает валидацию,
   идемпотентность, конфликт. `data/` покрывает CRUD + пагинацию + N+1.

9. **AC-SourceRef**: Каждый Post в ответах Create/Get/Update/List содержит
   вложенный `SourceRef{id, type}` — никогда не пустой.

10. **AC-N+1**: GetPost — 1 SQL. ListPosts(N) — ≤ 2 SQL (покрыто тестом подсчёта
    запросов).

### Автоматизированная верификация

```bash
# Линтинг
golangci-lint run ./... -c .golangci.yml

# Все тесты
go test ./...

# Только unit
go test ./... -short

# Только integration
go test ./... -run Integration

# Покрытие
go test -coverprofile=coverage.out ./...
```

---

## Open Questions

### OQ-1. Proto-представление metadata

Spec определяет `metadata` как `map<string, string>`. Пользователь указал, что в API
контракте должен быть "явный зафиксированный набор полей". Возможные варианты:

- **A**: Оставить `map<string, string>` в proto — соответствует spec, свободный набор
  ключей (url, language, ...).
- **B**: Определить `message PostMetadata { optional string url = 1; optional string language = 2; }`
  в proto — фиксированный набор полей, но нарушает spec ("набор ключей не валидируется",
  свободный JSON-объект).

**Решение:** Вариант A — `map<string, string>`. Соответствует spec (свободный набор ключей,
не валидируется). Принято перед началом Step 2.

### OQ-2. Курсорный токен при смене order_by между запросами

Page token кодирует значение поля сортировки (published_at или created_at), но не
кодирует имя поля. Если клиент меняет `order_by` между paginated-запросами,
токен декодируется как timestamp для другого поля — пагинация выдаёт некорректные
результаты без явной ошибки.

**Варианты:**
- **A**: Кодировать имя поля сортировки в токен (e.g. `base64(published_at|timestamp|uuid)`)
  и валидировать совпадение при декодировании — строго, но утяжеляет токен.
- **B**: Не валидировать — поведение "garbage in, garbage out", ответственность клиента.
  Это стандартный подход (Google API, GitHub API).

**Рекомендация:** B — не кодировать имя поля. Если возникнет потребность,
расширить позже.

### OQ-3. ListPosts без фильтра source_id — производительность

Индексы `(source_id, published_at DESC, id DESC)` оптимизированы для запросов
С фильтром по source_id. При `ListPosts` без фильтра PostgreSQL может выбрать
seq scan на объёме ~10⁶ строк.

Spec не требует оптимизации этого сценария ("покрывают фильтр+сортировку+пагинацию"),
но при отсутствии фильтра p95 может деградировать.

**Решение:** Не добавлять дополнительные индексы в рамках FT-004. Если потребуется —
отдельная задача с benchmark.

### OQ-4. Race condition при идемпотентном Create

Два параллельных CreatePost с одинаковыми (source_id, external_id):

1. Оба вызывают `Save` (upsert).
2. Первый INSERT успешен → возвращает новый Post.
3. Второй INSERT получает ON CONFLICT → SELECT существующего по (source_id, external_id).

**Решение:** В `Save` использовать `ON CONFLICT (source_id, external_id) DO NOTHING`.
Если 0 affected rows → SELECT существующего по (source_id, external_id).
Это гарантирует идемпотентность без ошибки и без race condition.
Предварительный `GetBySourceAndExternalID` в usecase не нужен — Save самодостаточен.

Альтернатива: `ON CONFLICT DO UPDATE SET id = posts.id` (no-op update) +
`RETURNING *` — но это обновляет `updated_at`, что нарушает INV-6 (идемпотентность
без модификации). Поэтому `DO NOTHING` + fallback SELECT — предпочтительнее.

### OQ-5. Ent edge Post → Source: один или два SQL

Необходимо убедиться, что `WithSource()` генерирует JOIN (1 SQL) или batch select
(2 SQL). Для GetPost нужен ровно 1 SQL — при необходимости использовать raw query.

**Решение:** Проверить при Step 4 (Ent schema), что `WithSource()` генерирует
один SQL с JOIN или приемлемый паттерн. Если Ent делает 2 запроса (batch select) —
это допустимо для ListPosts (≤ 2 запросов), но для GetPost нужен ровно 1.
При необходимости для GetPost использовать явный raw query с JOIN.
