# Implement Report: feature 004 (App + Migrations in One Binary)

## Что реализовано

1. Добавлен CLI-роутинг в `cmd/feedium`:
- `go run ./cmd/feedium` запускает сервер (поведение по умолчанию);
- `go run ./cmd/feedium run` запускает сервер;
- `go run ./cmd/feedium run migrate` запускает миграции и завершает процесс.

2. Добавлен bootstrap-путь для миграций:
- `internal/bootstrap/migrate.go` с функцией `Migrate(ctx, log)`;
- чтение `DATABASE_URL` и fail-fast при пустом значении;
- открытие подключения через существующий `internal/platform/postgres`;
- логирование старта/успеха миграций.

3. Интегрирован `goose` как библиотека:
- `internal/platform/migrator/migrator.go` выполняет `goose.Up`;
- установлен диалект `postgres`.

4. Миграции встроены в бинарник:
- добавлен `migrations/embed.go` (`go:embed *.sql`);
- goose читает SQL из embedded FS.

5. Добавлены тесты:
- `cmd/feedium/command_test.go` покрывает парсинг CLI-команд.

6. Обновлены зависимости:
- добавлен `github.com/pressly/goose/v3` и его transitive deps в `go.mod/go.sum`.

## Проверки

Выполнено после изменений:
- `go test ./...` — успешно.

## Ограничения и инварианты

- Существующие SQL-миграции в `migrations/` не изменялись.
- Выбор новой библиотеки уже принят в рамках реализации: используется `goose`.
