# Posts CRUD API

## Цель

Обеспечить хранение и управление постами (создание, чтение, обновление, удаление) через API без изменения кода и деплоя. Посты агрегируются из внешних источников (Telegram, RSS и др.) и должны быть доступны другим компонентам системы.

## Reference
- Brief: `memory-bank/features/006/brief.md`

## Scope

### Входит
- CRUD API для постов (Create, Read, Update, Delete, List)
- Хранение постов в PostgreSQL с FK на таблицу `sources`
- Фильтрация списка постов по `published_at`
- Пагинация списка постов
- Валидация входных данных
- Unit-тесты с покрытием > 80%

### НЕ входит
- Аутентификация и авторизация
- Integration и e2e тесты
- Сбор постов из источников (ingestion pipeline)
- Проверка доступности источников
- Пользовательский интерфейс
- Обработка медиа-контента (только текст)

## Контекст

Feedium агрегирует посты из различных источников. Модуль `source` уже реализован и хранит информацию об источниках. Модуль `post` будет вторым доменным модулем, следующим тем же архитектурным паттернам:
- Домен: `internal/app/post/`
- Адаптеры: `internal/app/post/adapters/postgres/`, `internal/app/post/adapters/connect/`
- Proto: `api/post/v1/post.proto`
- Миграция: `migrations/002_create_posts.sql`
- Регистрация: `internal/bootstrap/bootstrap.go`

## Функциональные требования

### FR-1: Модель данных Post

Поля:
| Поле | Тип | Обязательное | Описание |
|------|-----|:---:|-----------|
| `id` | UUID | да (генерируется БД) | Уникальный идентификатор |
| `source_id` | UUID (FK → sources.id) | да | Ссылка на источник |
| `title` | string | да | Заголовок поста |
| `content` | text (без ограничения длины) | да | Текстовое содержимое поста |
| `author` | string | нет | Автор поста (metadata) |
| `published_at` | timestamptz | да | Дата публикации в источнике |
| `created_at` | timestamptz | да (генерируется) | Дата создания в системе |
| `updated_at` | timestamptz | да (генерируется) | Дата последнего обновления в системе |

- FK `source_id` → `sources.id` с constraint `ON DELETE RESTRICT`
- `published_at` — дата из источника, задаётся клиентом
- `created_at`, `updated_at` — системные, управляются GORM

### FR-2: Create Post

- Принимает: `source_id`, `title`, `content`, `author` (опционально), `published_at`
- Валидация:
  - `source_id` — обязателен, UUID формат
  - `title` — обязателен, не пустая строка (после trim)
  - `content` — обязателен, не пустая строка (после trim)
  - `published_at` — обязателен, валидный timestamp
  - Если `source_id` ссылается на несуществующий source — ошибка (FK constraint)
- Возвращает: созданный Post со всеми полями (включая сгенерированные `id`, `created_at`, `updated_at`)

### FR-3: Get Post

- Принимает: `id` (UUID)
- Валидация: `id` — обязателен, UUID формат
- Если пост не найден — ошибка NotFound
- Возвращает: Post со всеми полями

### FR-4: Update Post (full replace)

- Принимает: `id`, `source_id`, `title`, `content`, `author` (опционально), `published_at`
- Семантика: полная замена (PUT), все обязательные поля должны быть переданы
- Валидация: аналогична Create + `id` должен существовать
- Если пост не найден — ошибка NotFound
- Если новый `source_id` ссылается на несуществующий source — ошибка FK constraint
- Возвращает: обновлённый Post со всеми полями

### FR-5: Delete Post

- Принимает: `id` (UUID)
- Hard delete (физическое удаление из БД)
- Если пост не найден — ошибка NotFound
- Возвращает: пустой ответ (подтверждение удаления)

### FR-6: List Posts

- Принимает (все опционально):
  - `published_after` (timestamptz) — фильтр: `published_at >= published_after`
  - `published_before` (timestamptz) — фильтр: `published_at < published_before`
  - `page` (int, default: 1)
  - `page_size` (int, default: 50, max: 100)
- Сортировка: по `published_at DESC` (новые первыми)
- Возвращает: список постов + `total_count` (общее количество с учётом фильтров)

## Нефункциональные требования

### NFR-1: Производительность
- Операции CRUD ≤ 5 секунд при нагрузке до 10,000 постов/день
- Индексы: `published_at` (для фильтрации и сортировки), `source_id` (для FK lookups)

### NFR-2: Тестирование
- Unit-тесты с покрытием > 80%
- Мок репозитория через `go.uber.org/mock` (mockgen)
- Тесты postgres-адаптера с `go-mocket`

### NFR-3: Совместимость
- Следует архитектурным паттернам модуля `source`
- Connect-go API (protobuf)

## Сценарии и edge cases

### Основной сценарий
1. Клиент создаёт пост через `CreatePost` с валидными данными
2. Пост сохраняется в БД, возвращается с `id`, `created_at`, `updated_at`
3. Клиент получает пост через `GetPost` по `id`
4. Клиент обновляет пост через `UpdatePost` (full replace)
5. Клиент получает список постов через `ListPosts` с фильтрами по дате
6. Клиент удаляет пост через `DeletePost`

### Ошибки
- Create/Update с невалидным `source_id` → `InvalidArgument` (FK violation обрабатывается как ошибка валидации)
- Get/Update/Delete несуществующего поста → `NotFound`
- Create/Update с пустым `title` или `content` → `InvalidArgument`
- Create/Update без `published_at` → `InvalidArgument`
- Удаление source, у которого есть посты → ошибка на стороне source (RESTRICT), не в модуле post

### Невалидный ввод
- Пустой UUID → `InvalidArgument`
- Невалидный UUID → `InvalidArgument`
- `page` < 1 → нормализуется до 1
- `page_size` < 1 → нормализуется до 50 (default)
- `page_size` > 100 → нормализуется до 100 (max)
- `published_after` > `published_before` → пустой список (не ошибка)

### Пустые состояния
- `ListPosts` без постов → пустой список, `total_count = 0`
- `ListPosts` с фильтрами, не совпадающими ни с одним постом → пустой список, `total_count = 0`
- `author` не передан → сохраняется как пустая строка

## Инварианты

1. Каждый пост всегда ссылается на существующий source (`source_id` FK NOT NULL + RESTRICT)
2. `id` генерируется базой данных, клиент не может задать его
3. `created_at` устанавливается один раз при создании, не изменяется при update
4. `updated_at` обновляется автоматически при каждом update
5. Удаление поста — физическое, без возможности восстановления
6. Операция Update — полная замена всех полей (кроме `id`, `created_at`)

## Acceptance Criteria

- [ ] Миграция `002_create_posts.sql` создаёт таблицу `posts` с FK на `sources.id` (ON DELETE RESTRICT)
- [ ] Proto `api/post/v1/post.proto` определяет `PostService` с 5 RPC методами
- [ ] `CreatePost` сохраняет пост и возвращает его с `id`, `created_at`, `updated_at`
- [ ] `GetPost` возвращает пост по `id` или `NotFound`
- [ ] `UpdatePost` заменяет все поля поста (full replace) или `NotFound`
- [ ] `DeletePost` физически удаляет пост или `NotFound`
- [ ] `ListPosts` возвращает посты с пагинацией (default 50, max 100) и `total_count`
- [ ] `ListPosts` фильтрует по `published_after` и `published_before`
- [ ] `ListPosts` сортирует по `published_at DESC`
- [ ] Валидация: пустые обязательные поля → `InvalidArgument`
- [ ] Валидация: невалидный/несуществующий `source_id` → ошибка
- [ ] Unit-тесты покрытие > 80% для service, handler, repository
- [ ] `go build ./...` и `go test ./...` проходят
- [ ] Регистрация в `internal/bootstrap/bootstrap.go`

## Ограничения

- Не добавлять `ON DELETE CASCADE` — используется `RESTRICT`
- Не создавать soft delete механизм
- Не добавлять аутентификацию
- Не модифицировать существующие миграции
- Следовать паттернам модуля `source` (структура пакетов, именование, error handling)

## Open Questions

- Нет открытых вопросов. Все неоднозначности уточнены.
