---
doc_kind: engineering
doc_function: convention
purpose: Правила работы с БД: Ent ORM (схемы, генерация), goose-миграции, PostgreSQL conventions, constraints.
derived_from:
  - ../dna/governance.md
status: active
---

# Database

## ORM: ent

- Схемы в `ent/schema/`
- Сгенерированный код в `ent/` — коммитится
- Генерация: `go generate ./ent`
- Ent client создаётся в `internal/data/` и инжектится через Wire

### Правила схем

- Одна сущность — один файл в `ent/schema/`
- Имена сущностей — PascalCase единственное число (`Source`, `Post`, `Summary`)
- Поля: camelCase в Go, snake_case в базе (ent делает автоматически)
- Обязательные поля для всех сущностей: `created_at`, `updated_at` (mixin)
- Soft delete: через mixin с полем `deleted_at` если нужен
- Индексы определяем явно в схеме

### Паттерн использования в data/

```go
// internal/data/post_repo.go
type postRepo struct {
    data *Data  // Data содержит ent.Client
}

func (r *postRepo) Save(ctx context.Context, post biz.Post) error {
    _, err := r.data.db.Post.Create().
        SetSourceID(post.SourceID).
        SetExternalID(post.ExternalID).
        SetTitle(post.Title).
        SetText(post.Text).
        Save(ctx)
    return err
}
```

### Транзакции

- Ent Tx для атомарных операций
- Outbox pattern: Post + SummaryEvent в одной транзакции

```go
tx, err := r.data.db.Tx(ctx)
if err != nil {
    return err
}
defer func() {
    if err != nil {
        tx.Rollback()
    }
}()
// операции с tx.Post, tx.SummaryEvent
err = tx.Commit()
```

## Миграции: goose

- SQL-миграции в `migrations/`
- Формат: `YYYYMMDDHHMMSS_description.sql`
- Только SQL-миграции, не Go-миграции
- Каждая миграция — отдельная транзакция
- Не менять существующие миграции

### Создание миграции

```bash
goose -dir migrations create add_posts_table sql
```

### Структура файла миграции

```sql
-- +goose Up
CREATE TABLE posts (
    id UUID PRIMARY KEY,
    source_id UUID NOT NULL REFERENCES sources(id),
    external_id TEXT NOT NULL,
    ...
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_posts_source_external ON posts(source_id, external_id);

-- +goose Down
DROP TABLE IF EXISTS posts;
```

### Ent + Goose: разделение ответственности

- Ent — описание схемы и генерация Go-кода для работы с БД
- Goose — управление миграциями (создание таблиц, изменение схемы)
- Ent auto-migration НЕ используется в production
- Workflow: изменил ent-схему → написал goose-миграцию руками → `go generate ./ent`

## PostgreSQL

- Версия: 18.3
- Все таблицы в схеме `public`
- PK: UUID v7 (`github.com/google/uuid`) для всех таблиц — time-sortable, глобально уникален без координации, обеспечивает стабильную курсорную пагинацию
- Библиотека: `github.com/google/uuid` v1.6+, генерация через `uuid.Must(uuid.NewV7())`
- Тип колонки в БД: `UUID` (PostgreSQL native), генерация значения — на уровне Go (Ent Default hook), не через `DEFAULT gen_random_uuid()`
- JSONB для meta-полей (source meta, media_urls)
- TIMESTAMPTZ для всех дат
- Индексы: создаём явно в миграциях, не полагаемся на ORM

## Constraints

- Foreign keys — обязательны
- UNIQUE constraints — через миграции
- NOT NULL — по умолчанию для всех полей, NULL только если есть бизнес-смысл
- CHECK constraints — для enum-подобных полей если нужно
