# Implementation Plan

## Steps

### Step 1: Миграция БД — `migrations/002_create_posts.sql`

**Цель:** Создать таблицу `posts` в PostgreSQL.

**Действия:**
- Создать файл `migrations/002_create_posts.sql`
- В секции `+goose Up`:
  - Таблица `posts` с полями: `id` (UUID PK, default `gen_random_uuid()`), `source_id` (UUID NOT NULL, FK → `sources.id` ON DELETE RESTRICT), `title` (TEXT NOT NULL), `content` (TEXT NOT NULL), `author` (VARCHAR(255) NOT NULL DEFAULT ''), `published_at` (TIMESTAMPTZ NOT NULL), `created_at` (TIMESTAMPTZ NOT NULL DEFAULT now()), `updated_at` (TIMESTAMPTZ NOT NULL DEFAULT now())
  - Индекс `idx_posts_published_at` на `published_at`
  - Индекс `idx_posts_source_id` на `source_id`
- В секции `+goose Down`:
  - `DROP TABLE IF EXISTS posts;`

**Зависимости:** Миграция `001_create_sources.sql` уже существует.

**Результат:** Таблица `posts` создаётся при запуске миграции.

**Проверка:** `go run ./cmd/feedium run migrate` выполняется без ошибок; таблица `posts` и индексы видны в БД (`\d posts` в psql); FK constraint проверяется попыткой вставить пост с несуществующим `source_id` — должна быть ошибка.

---

### Step 2: Доменная модель — `internal/app/post/post.go`

**Цель:** Определить структуру `Post` и `ListFilter`.

**Действия:**
- Создать пакет `internal/app/post/`
- Файл `post.go` с:
  - Структура `Post`: `ID uuid.UUID`, `SourceID uuid.UUID`, `Title string`, `Content string`, `Author string`, `PublishedAt time.Time`, `CreatedAt time.Time`, `UpdatedAt time.Time`
  - Структура `ListFilter`: `PublishedAfter *time.Time`, `PublishedBefore *time.Time`, `Page int`, `PageSize int`

**Зависимости:** Нет.

**Результат:** Доменные типы доступны для использования в service и адаптерах.

**Проверка:** `go build ./internal/app/post/` компилируется.

---

### Step 3: Ошибки домена — `internal/app/post/errors.go`

**Цель:** Определить доменные ошибки модуля `post`.

**Действия:**
- Файл `errors.go` в пакете `post`:
  - `var ErrNotFound = errors.New("post not found")`
  - Тип `ValidationError` со строковым полем `msg` и методом `Error() string` (аналогично `source`)
  - Функция-конструктор `validationError(msg string) error`

**Зависимости:** Нет.

**Результат:** Ошибки `ErrNotFound` и `ValidationError` доступны для service и адаптеров.

**Проверка:** `go build ./internal/app/post/` компилируется.

---

### Step 4: Интерфейс репозитория — `internal/app/post/repository.go`

**Цель:** Определить контракт хранения данных и сгенерировать мок.

**Действия:**
- Файл `repository.go` в пакете `post`:
  - Директива `//go:generate mockgen -source=repository.go -destination=mocks/repository_mock.go -package=mocks`
  - Интерфейс `Repository`:
    - `Create(context.Context, *Post) error`
    - `GetByID(context.Context, uuid.UUID) (*Post, error)`
    - `Update(context.Context, *Post) error`
    - `Delete(context.Context, uuid.UUID) error`
    - `List(context.Context, ListFilter) ([]Post, int, error)`
- Запустить `go generate ./internal/app/post/` для генерации мока в `mocks/repository_mock.go`

**Зависимости:** Step 2 (доменная модель).

**Результат:** Интерфейс репозитория и мок готовы.

**Проверка:** `go generate ./internal/app/post/` завершается без ошибок; файл `mocks/repository_mock.go` создан.

---

### Step 5: Сервис — `internal/app/post/service.go`

**Цель:** Реализовать бизнес-логику CRUD для постов.

**Действия:**
- Файл `service.go` в пакете `post`:
  - Структура `Service` с полями `repo Repository`, `log *slog.Logger`
  - Конструктор `NewService(repo Repository, log *slog.Logger) *Service`
  - Константы `defaultPageSize = 50`, `maxPageSize = 100`
  - Метод `Create(ctx, *Post) (*Post, error)`:
    - Вызывает `validatePost(post)`
    - Вызывает `repo.Create(ctx, post)`
    - Возвращает пост с заполненными `id`, `created_at`, `updated_at`
  - Метод `Get(ctx, uuid.UUID) (*Post, error)`:
    - Вызывает `repo.GetByID(ctx, id)`
  - Метод `Update(ctx, *Post) (*Post, error)`:
    - Вызывает `validatePost(post)`
    - Проверяет существование через `repo.GetByID(ctx, post.ID)`
    - Вызывает `repo.Update(ctx, post)`
    - Возвращает обновлённый пост
  - Метод `Delete(ctx, uuid.UUID) error`:
    - Вызывает `repo.Delete(ctx, id)`
  - Метод `List(ctx, ListFilter) ([]Post, int, error)`:
    - Нормализует `PageSize` (<=0 → 50, >100 → 100)
    - Нормализует `Page` (<=0 → 1)
    - Вызывает `repo.List(ctx, filter)`
  - Функция `validatePost(p *Post) error`:
    - `p` не nil
    - `SourceID` не нулевой UUID
    - `Title` после trim — не пустая строка
    - `Content` после trim — не пустая строка
    - `PublishedAt` не нулевой (`time.Time{}`)
    - Trim `Title` и `Content` перед сохранением

**Зависимости:** Steps 2, 3, 4.

**Результат:** Бизнес-логика модуля `post` реализована.

**Проверка:** `go build ./internal/app/post/` компилируется.

---

### Step 6: Unit-тесты сервиса

**Цель:** Покрыть бизнес-логику сервиса unit-тестами > 80%.

**Действия:**
- Файл `service_test.go` (exported-тесты) и/или `service_internal_test.go` (internal-тесты), по аналогии с `source`.
- Тестовые сценарии с использованием мока из `mocks/repository_mock.go`:
  - **Create:** валидный пост; пустой title; пустой content; нулевой source_id; нулевой published_at; ошибка репозитория
  - **Get:** существующий пост; несуществующий (ErrNotFound)
  - **Update:** валидный update; несуществующий пост; невалидные поля
  - **Delete:** существующий; несуществующий
  - **List:** с фильтрами; без фильтров; нормализация page/pageSize
  - **validatePost:** все ветки валидации
  - **normalizePageSize / normalizePage:** граничные значения (0, -1, 1, 50, 100, 101)

**Зависимости:** Steps 4, 5.

**Результат:** Тесты проходят, покрытие > 80%.

**Проверка:** `go test ./internal/app/post/ -cover` — покрытие > 80%, все тесты зелёные.

---

### Step 7: Postgres-адаптер — `internal/app/post/adapters/postgres/repository.go`

**Цель:** Реализовать интерфейс `Repository` для PostgreSQL через GORM.

**Действия:**
- Создать директорию `internal/app/post/adapters/postgres/`
- Файл `repository.go`:
  - Структура `Repository` с полем `db *gorm.DB`
  - Конструктор `New(db *gorm.DB) *Repository`
  - Внутренняя структура `postRow` с GORM-тегами:
    - `ID uuid.UUID` — `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
    - `SourceID uuid.UUID` — `gorm:"type:uuid;not null"`
    - `Title string` — `gorm:"type:text;not null"`
    - `Content string` — `gorm:"type:text;not null"`
    - `Author string` — `gorm:"type:varchar(255);not null;default:''"`
    - `PublishedAt time.Time` — `gorm:"type:timestamptz;not null"`
    - `CreatedAt time.Time` — `gorm:"type:timestamptz;not null;autoCreateTime"`
    - `UpdatedAt time.Time` — `gorm:"type:timestamptz;not null;autoUpdateTime"`
  - Метод `TableName() string` — возвращает `"posts"`
  - Функции конвертации `fromDomain(*post.Post) postRow`, `toDomain(*postRow) post.Post`, `applyRow(*post.Post, *postRow)`
  - Метод `Create`:
    - Конвертирует в `postRow`, вызывает `db.Create`, применяет сгенерированные поля через `applyRow`
    - FK violation (source_id не существует) — транслирует ошибку Postgres (код 23503) в `post.ValidationError` с сообщением о невалидном source_id
  - Метод `GetByID`:
    - `db.First(&row, "id = ?", id)`; `gorm.ErrRecordNotFound` → `post.ErrNotFound`
  - Метод `Update`:
    - `db.Model(&postRow{}).Where("id = ?", id).Updates(...)` обновляет все поля кроме `id`, `created_at`
    - `RowsAffected == 0` → `post.ErrNotFound`
    - FK violation — аналогично Create
  - Метод `Delete`:
    - `db.Delete(&postRow{}, "id = ?", id)`; `RowsAffected == 0` → `post.ErrNotFound`
  - Метод `List`:
    - Применяет фильтр `published_at >= published_after` если задан
    - Применяет фильтр `published_at < published_before` если задан
    - Считает `total` через `Count` (с фильтрами)
    - Сортирует по `published_at DESC`
    - Применяет `Limit` и `Offset` для пагинации

**Зависимости:** Steps 2, 3.

**Результат:** Репозиторий реализует интерфейс `post.Repository`.

**Проверка:** `go build ./internal/app/post/adapters/postgres/` компилируется.

---

### Step 8: Unit-тесты postgres-адаптера

**Цель:** Покрыть postgres-адаптер тестами с использованием `go-mocket`.

**Действия:**
- Файл `repository_internal_test.go` в пакете `postgres` (по аналогии с `source`).
- Тесты с `go-mocket` для:
  - **Create:** успех; FK violation
  - **GetByID:** успех; not found
  - **Update:** успех; not found; FK violation
  - **Delete:** успех; not found
  - **List:** без фильтров; с `published_after`; с `published_before`; с обоими; пустой результат

**Зависимости:** Step 7.

**Результат:** Тесты проходят, покрытие > 80%.

**Проверка:** `go test ./internal/app/post/adapters/postgres/ -cover` — покрытие > 80%.

---

### Step 9: Proto-определение — `api/post/v1/post.proto`

**Цель:** Определить gRPC/Connect API для модуля `post`.

**Действия:**
- Создать директорию `api/post/v1/`
- Файл `post.proto`:
  - `package post.v1;`
  - `option go_package = "feedium/api/post/v1;postv1";`
  - `import "google/protobuf/timestamp.proto";`
  - Message `Post`: `string id`, `string source_id`, `string title`, `string content`, `string author`, `google.protobuf.Timestamp published_at`, `google.protobuf.Timestamp created_at`, `google.protobuf.Timestamp updated_at`
  - Message `CreatePostRequest`: `string source_id`, `string title`, `string content`, `string author`, `google.protobuf.Timestamp published_at`
  - Message `CreatePostResponse`: `Post post`
  - Message `GetPostRequest`: `string id`
  - Message `GetPostResponse`: `Post post`
  - Message `UpdatePostRequest`: `string id`, `string source_id`, `string title`, `string content`, `string author`, `google.protobuf.Timestamp published_at`
  - Message `UpdatePostResponse`: `Post post`
  - Message `DeletePostRequest`: `string id`
  - Message `DeletePostResponse`: (пустой)
  - Message `ListPostsRequest`: `google.protobuf.Timestamp published_after`, `google.protobuf.Timestamp published_before`, `int32 page`, `int32 page_size`
  - Message `ListPostsResponse`: `repeated Post posts`, `int32 total_count`
  - Service `PostService`:
    - `rpc CreatePost(CreatePostRequest) returns (CreatePostResponse)`
    - `rpc GetPost(GetPostRequest) returns (GetPostResponse)`
    - `rpc UpdatePost(UpdatePostRequest) returns (UpdatePostResponse)`
    - `rpc DeletePost(DeletePostRequest) returns (DeletePostResponse)`
    - `rpc ListPosts(ListPostsRequest) returns (ListPostsResponse)`
- Файл `generate.go` с директивой `//go:generate ../../../scripts/gen-proto.sh`
- Запустить `go generate ./api/post/v1/` для генерации Go-кода

**Зависимости:** Нет.

**Результат:** Сгенерированные файлы `post.pb.go` и `postv1connect/post.connect.go`.

**Проверка:** `go generate ./api/post/v1/` без ошибок; `go build ./api/post/v1/...` компилируется.

---

### Step 10: Connect-хэндлер — `internal/app/post/adapters/connect/handler.go`

**Цель:** Реализовать HTTP-хэндлер Connect-go для `PostService`.

**Действия:**
- Создать директорию `internal/app/post/adapters/connect/`
- Файл `handler.go`:
  - Структура `Handler` с полями `svc *post.Service`, `log *slog.Logger`
  - Конструктор `New(svc *post.Service, log *slog.Logger) *Handler`
  - Compile-time проверка: `var _ postv1connect.PostServiceHandler = (*Handler)(nil)`
  - Метод `CreatePost`:
    - Конвертирует proto-request → доменную модель `Post` (source_id парсит как UUID)
    - Если source_id невалидный UUID → `connect.CodeInvalidArgument`
    - Конвертирует `published_at` из `timestamppb.Timestamp` в `time.Time`
    - Вызывает `svc.Create`
    - Конвертирует результат → proto-response
  - Метод `GetPost`:
    - Парсит `id` как UUID, ошибка → `CodeInvalidArgument`
    - Вызывает `svc.Get`
  - Метод `UpdatePost`:
    - Парсит `id` и `source_id` как UUID
    - Конвертирует все поля
    - Вызывает `svc.Update`
  - Метод `DeletePost`:
    - Парсит `id` как UUID
    - Вызывает `svc.Delete`
  - Метод `ListPosts`:
    - Конвертирует `published_after`, `published_before` из `timestamppb` (если не nil)
    - Конвертирует `page`, `page_size`
    - Вызывает `svc.List`
    - Конвертирует список постов → proto, добавляет `total_count`
  - Функция `mapError(err) error`:
    - `post.ErrNotFound` → `connect.CodeNotFound`
    - `post.ValidationError` → `connect.CodeInvalidArgument`
    - остальное → `connect.CodeInternal` с логированием
  - Вспомогательные функции конвертации: `toProto(*post.Post) *postv1.Post`, `fromCreateRequest`, `fromUpdateRequest`

**Зависимости:** Steps 5, 9.

**Результат:** Хэндлер реализует интерфейс `postv1connect.PostServiceHandler`.

**Проверка:** `go build ./internal/app/post/adapters/connect/` компилируется.

---

### Step 11: Unit-тесты Connect-хэндлера

**Цель:** Покрыть хэндлер тестами > 80%.

**Действия:**
- Файлы `handler_test.go` и/или `handler_internal_test.go` (по аналогии с `source`).
- Тесты:
  - **CreatePost:** валидный запрос; невалидный source_id (не UUID); пустой title; ошибка сервиса
  - **GetPost:** валидный запрос; невалидный id; not found
  - **UpdatePost:** валидный запрос; невалидный id; not found; невалидные поля
  - **DeletePost:** валидный; невалидный id; not found
  - **ListPosts:** без фильтров; с фильтрами по дате; с пагинацией
  - **mapError:** все ветки (nil, ErrNotFound, ValidationError, unknown)
  - Конвертация proto ↔ domain: корректность маппинга всех полей

**Зависимости:** Steps 4, 10.

**Результат:** Тесты проходят, покрытие > 80%.

**Проверка:** `go test ./internal/app/post/adapters/connect/ -cover` — покрытие > 80%.

---

### Step 12: Регистрация в bootstrap — `internal/bootstrap/bootstrap.go`

**Цель:** Подключить модуль `post` к HTTP-серверу.

**Действия:**
- В `internal/bootstrap/bootstrap.go`:
  - Добавить импорты: `postv1connect`, `postsvc` (`internal/app/post`), `postconnect` (`internal/app/post/adapters/connect`), `postpg` (`internal/app/post/adapters/postgres`)
  - После блока регистрации `source` (строка 54) добавить аналогичный блок:
    - `postRepo := postpg.New(db)`
    - `postService := postsvc.NewService(postRepo, log)`
    - `postHandler := postconnect.New(postService, log)`
    - `postPath, postH := postv1connect.NewPostServiceHandler(postHandler)`
    - `mux.Handle(postPath, postH)`

**Зависимости:** Steps 7, 9, 10.

**Результат:** API модуля `post` доступен по HTTP.

**Проверка:** `go build ./...` компилируется; `go run ./cmd/feedium` стартует без ошибок; HTTP-запрос на `/post.v1.PostService/` возвращает корректный ответ (не 404).

---

### Step 13: Финальная проверка

**Цель:** Убедиться, что всё собирается, тесты проходят, покрытие достаточное.

**Действия:**
- `go build ./...` — компиляция всего проекта
- `go vet ./...` — статический анализ
- `golangci-lint run ./... -c .golangci.yml` — линтер
- `go test ./... -cover` — все тесты зелёные
- Проверить покрытие модуля `post`: `go test ./internal/app/post/... -cover` — > 80% по каждому пакету

**Зависимости:** Все предыдущие шаги.

**Результат:** Проект собирается, линтер проходит, тесты зелёные с покрытием > 80%.

**Проверка:** Все команды завершаются с кодом 0.

---

## Edge Cases

1. **Create/Update с несуществующим `source_id`** — FK violation Postgres (код 23503) должен транслироваться в `ValidationError`/`CodeInvalidArgument`, а не в `CodeInternal`
2. **Create/Update с `title`/`content` из одних пробелов** — после trim становятся пустыми → `InvalidArgument`
3. **Create/Update с `published_at` = zero time** — должен возвращать `InvalidArgument`
4. **Create/Update с `source_id` = UUID нулей (`00000000-...`)** — невалидный, должен возвращать `InvalidArgument`
5. **Get/Update/Delete с невалидным UUID** — хэндлер парсит UUID, при ошибке → `CodeInvalidArgument`
6. **Get/Update/Delete несуществующего поста** — `ErrNotFound` → `CodeNotFound`
7. **List с `published_after` > `published_before`** — пустой список, `total_count = 0`, без ошибки
8. **List с `page` = 0 или отрицательным** — нормализуется до 1
9. **List с `page_size` = 0** — нормализуется до 50
10. **List с `page_size` > 100** — нормализуется до 100
11. **List с `page_size` < 0** — нормализуется до 50
12. **List без постов в БД** — пустой массив (не null), `total_count = 0`
13. **List с `page` за пределами данных** (page=999, данных мало) — пустой список, `total_count` отражает реальное количество
14. **Create с `author` не передан / пустая строка** — сохраняется как пустая строка
15. **Update — `created_at` не должен изменяться** — при Update обновляются все поля кроме `id` и `created_at`
16. **Delete source, у которого есть посты** — ошибка на стороне модуля `source` из-за `ON DELETE RESTRICT` (не в scope post, но FK должен быть настроен корректно)
17. **Конкурентное удаление одного поста** — второй вызов Delete возвращает `NotFound` (RowsAffected == 0)
18. **`total_count` при int overflow** — при конвертации `int` → `int32` для proto: если total > MaxInt32, ограничить до `math.MaxInt32` (по аналогии с source)

## Verification

1. **Компиляция:** `go build ./...` проходит без ошибок
2. **Статический анализ:** `go vet ./...` без предупреждений
3. **Линтер:** `golangci-lint run ./... -c .golangci.yml` без ошибок
4. **Тесты:** `go test ./...` — все зелёные
5. **Покрытие:**
   - `go test ./internal/app/post/ -cover` > 80%
   - `go test ./internal/app/post/adapters/postgres/ -cover` > 80%
   - `go test ./internal/app/post/adapters/connect/ -cover` > 80%
6. **Миграция:** `go run ./cmd/feedium run migrate` создаёт таблицу `posts` с FK, индексами
7. **Smoke test API:** сервер стартует, CRUD-операции через Connect-запросы возвращают корректные ответы и коды ошибок
8. **Кодогенерация:** `go generate ./...` не показывает diff (все сгенерированные файлы актуальны)

## Open Questions

1. **FK violation detection.** Postgres возвращает ошибку с кодом 23503 при FK violation. В модуле `source` такая обработка не реализована (нет FK). Нужно определить способ детекции: парсить строку ошибки GORM, или использовать `lib/pq` / `pgx` error codes. Какой подход предпочтительнее?

2. **Update — полная перезагрузка после update.** Spec требует возвращать обновлённый Post со всеми полями. В модуле `source` метод `Update` репозитория не возвращает обновлённую запись. Для `post` после `repo.Update` нужно либо делать `repo.GetByID` (дополнительный запрос), либо изменить `repo.Update` чтобы он возвращал обновлённый объект. В service модуля `source` уже делается `GetByID` перед `Update` — можно использовать тот же паттерн (Get → Update → вернуть post с обновлённым `updated_at`), но `updated_at` после Update будет отличаться от того, что вернул Get. Нужно решить: делать дополнительный Get после Update, или возвращать объект из Update через GORM `Returning`.

3. **Proto: `published_after`/`published_before` как optional.** В proto3 message-типы (включая `google.protobuf.Timestamp`) уже nullable (nil = не задано). Подтвердить, что отсутствие `published_after`/`published_before` в запросе ListPosts означает отсутствие фильтра (а не фильтр по zero time).
