# Feature 001 — Implementation Plan (Project Setup Skeleton)

## Summary
Собрать минимальный каркас Feedium без бизнес-логики в рамках артефактов feature 001: создать только целевые директории и 4 файла (`go.mod`, `cmd/feedium/main.go`, `internal/platform/logger/logger.go`, `ARCHITECTURE.md`) как deliverables setup-этапа feature 001, чтобы `go build ./...` и `go run ./cmd/feedium` завершались успешно и при запуске лог содержал `Feedium is starting`.

## Implementation Changes
1. Создать директории (если отсутствуют):  
`cmd/feedium`, `internal/app`, `internal/components`, `internal/platform/logger`, `internal/bootstrap`, `api`, `migrations`.

2. Создать/обновить `go.mod`:  
выполнить `go mod init feedium` (если `go.mod` ещё не существует);  
зафиксировать `module feedium` и `go 1.24`;  
не добавлять внешние зависимости.

3. Реализовать `cmd/feedium/main.go` (только `main()`):  
импорты: стандартная библиотека + `internal/platform/logger`;  
в `main()` ровно один вызов `logger.Init()`, затем лог через возвращённый логгер с сообщением, содержащим `Feedium is starting`;  
без bootstrap/lifecycle/конфигурации/интеграций.

4. Реализовать `internal/platform/logger/logger.go`:  
пакет `logger`, единственная экспортируемая функция `Init() *slog.Logger`;  
`Init()` создаёт non-nil логгер на `slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})`;  
без зависимостей на другие `internal/*` пакеты и без доп. функций.

5. Заполнить `ARCHITECTURE.md`:  
по 1+ абзацу (минимум одно осмысленное предложение) для каждой целевой директории;  
зафиксировать правила взаимодействия пакетов;  
явно записать: “интерфейсы объявляются в пакете-потребителе” и “не создавать интерфейсы без использования”;  
описать ограничения этапа Project Setup;  
добавить пошаговую инструкцию добавления нового компонента.

6. После выполнения всех изменений выполнить `go mod tidy`.

## Definition of Done (ARCHITECTURE.md)
DoD для `ARCHITECTURE.md` считается выполненным только при одновременном выполнении всех условий:

1. Структура документа:
- Присутствуют ровно три обязательные секции с заголовками второго уровня:
  - `## Directories`
  - `## Package Interaction Rules`
  - `## How to Add a New Component`

2. Секция `Directories`:
- Для каждой директории из spec есть отдельный подпункт.
- Каждый подпункт содержит минимум одно непустое предложение.

3. Секция `Package Interaction Rules`:
- Содержит минимум 5 правил.
- Обязательно покрыты правила:
  - допустимое направление импортов;
  - `Интерфейсы объявляются в пакете-потребителе.`
  - `Запрещено создавать интерфейсы без использования.`
  - запрещены зависимости между пакетами `internal/*`, кроме явно разрешённых в spec;
  - в `platform` запрещена бизнес-логика.
- Две policy-фразы выше присутствуют дословно.

4. Секция `How to Add a New Component`:
- Содержит минимум 5 шагов.
- Используется строго нумерованный список (`1.`, `2.`, ...).
- Порядок шагов фиксированный (инструкция выполняется сверху вниз без перестановок).

5. Ограничения setup scope:
- В документе отсутствуют `TODO` и `FIXME`.
- Запрещены ссылки на будущие библиотеки, фреймворки, БД, API и интеграции, не входящие в текущий setup scope.

## Public Interfaces / Contracts
- `internal/platform/logger`:
  - `func Init() *slog.Logger` (единственный публичный API этого этапа).
- `cmd/feedium`:
  - единственная точка входа `main()`.

## Test Plan
1. Структура:
- Проверить наличие всех требуемых директорий и только 4 целевых файлов deliverable setup-этапа feature 001.
- Убедиться, что в рамках артефактов feature 001 не добавлены дополнительные `.go` файлы сверх `cmd/feedium/main.go` и `internal/platform/logger/logger.go`.

2. Статические проверки:
- `rg "^func " cmd internal` → только `main()` и `Init()`.
- Проверить `go.mod`: `module feedium`, `go 1.24`.
- Проверить импорты:  
  `main.go` импортирует только stdlib + `internal/platform/logger`;  
  `logger.go` не импортирует `internal/*`.
- Проверить, что в `logger.Init()` используется `slog.NewTextHandler`, `os.Stdout`, `INFO`-уровень.
- Проверить DoD `ARCHITECTURE.md`:
  - есть только `## Directories`, `## Package Interaction Rules`, `## How to Add a New Component`;
  - в `Directories` есть подпункты по всем директориям из spec и у каждого минимум одно непустое предложение;
  - в `Package Interaction Rules` минимум 5 правил, включая две дословные policy-фразы;
  - в `How to Add a New Component` минимум 5 шагов в нумерованном списке;
  - отсутствуют `TODO` и `FIXME`;
  - отсутствуют ссылки на будущие библиотеки, фреймворки, БД, API и интеграции вне setup scope.

3. Поведенческие проверки:
- `go build ./...` → exit code 0.
- `go run ./cmd/feedium` → exit code 0.
- stdout содержит `Feedium is starting`.
- Логирование выполняется после `logger.Init()`, результат `Init()` используется для `Info(...)`.

## Assumptions
- Требование “только указанные файлы/директории” трактуется строго как scope артефактов feature 001 (setup-deliverables), а не как ограничение на весь репозиторий.
- Существующие миграции не трогаются.
- Новые библиотеки не добавляются; используется только стандартная библиотека Go.
