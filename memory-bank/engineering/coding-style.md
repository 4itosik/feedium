---
doc_kind: engineering
doc_function: convention
purpose: Coding style Feedium: kratos layout, правила по слоям, именование, значения vs указатели, error handling, logging, DI.
derived_from:
  - ../dna/governance.md
status: active
---

# Coding Style

## Язык и именование

- Весь код, комментарии — на английском
- Именование по стандартам Go: camelCase для приватных, PascalCase для экспортируемых
- Пакеты — одно слово, lowercase, без подчёркиваний (`summary`, `source`, `post`)
- Интерфейсы — без префикса `I`. Суффикс `-er` где уместно (`Collector`, `Scorer`)
- Файлы — snake_case (`summary_worker.go`, `post_repo.go`)

## Структура проекта (go-kratos layout)

```
├── api/                    ← proto-файлы и сгенерированный код
│   └── feedium/
├── cmd/
│   └── feedium/            ← main.go, wire injection
├── configs/                ← yaml-конфигурация
├── internal/
│   ├── biz/                ← доменная логика, интерфейсы репозиториев
│   ├── data/               ← реализация репозиториев, внешние клиенты
│   ├── service/            ← тонкий адаптер API → biz
│   ├── task/               ← воркеры (transport.Server)
│   └── server/             ← настройка HTTP/gRPC серверов
├── ent/                    ← ent-схемы и сгенерированный код
│   └── schema/
├── migrations/             ← goose SQL-миграции
└── third_party/            ← proto-зависимости
```

## Правила по слоям

### biz/

- Доменные сущности, бизнес-правила, usecase-ы
- Интерфейсы репозиториев и внешних сервисов определяются здесь
- Нулевой импорт инфраструктуры: никаких ent, http, sql
- Один файл — один домен (`source.go`, `post.go`, `summary.go`)

### data/

- Реализует интерфейсы из biz/
- Знает про ent, HTTP-клиенты, внешние API
- Не содержит бизнес-логику — только маппинг и вызовы

### service/

- Принимает proto DTO, конвертирует в доменные объекты, вызывает biz
- Без бизнес-логики, без прямого доступа к data
- Один сервис — один proto service

### task/

- Воркеры реализуют `transport.Server`
- Kratos управляет lifecycle (Start/Stop)
- Бизнес-функции возвращают структурированный результат (status, retry, error)
- Логирование, ретраи, state transitions — только здесь, не в leaf-функциях

### server/

- Настройка HTTP и gRPC серверов
- Middleware, interceptors
- Регистрация сервисов

## Интерфейсы

- Определяй интерфейс там, где он используется, не где реализуется
- Интерфейс в biz/, реализация в data/ — основной паттерн
- Минимальные интерфейсы: 1-3 метода. Если больше — разбивай
- Каждый пакет — минимум зависимостей

## Значения и указатели

По значению по умолчанию. Указатель — только когда есть конкретная причина.

**По значению:**
- Доменные сущности в biz/: `Post`, `Source`, `Summary`
- Value objects: `Pagination`, `DateRange`, `Score`
- DTO между слоями (кроме proto-generated, там указатели — ок)
- Любая структура, которую не нужно мутировать

**Указатель нужен когда:**
- Структура с состоянием: `*ent.Client`, `*sql.DB`, `*sync.Mutex`
- Семантика nil (optional значение, отсутствие)
- Pointer receiver для реализации интерфейса
- Структура реально большая (десятки полей, тяжёлые вложения)

**По слоям:**
- `biz/` — сущности и интерфейсы репозиториев работают со значениями. Usecase-ы — указатели (есть зависимости)
- `data/` — принимает и возвращает доменные значения. `*ent.Client` — указатель
- `service/` — принимает proto (указатели, так генерирует protoc), конвертирует в значения для biz/

```go
type PostRepo interface {
    Save(ctx context.Context, post Post) (Post, error)
    FindByExternalID(ctx context.Context, sourceID int64, externalID string) (Post, bool, error)
}
```

## Чистые функции в biz/

Не мутируй входные аргументы — возвращай новый результат. Функция получает всё через аргументы, не читает глобальное состояние.

```go
// плохо — мутация входного аргумента
func EnrichPost(post *Post) {
    post.Slug = slugify(post.Title)
}

// хорошо — возвращает новое значение
func EnrichPost(post Post) Post {
    post.Slug = slugify(post.Title)
    return post
}
```

Правила:
- Бизнес-функции (scoring, validation, enrichment) — чистые, без побочных эффектов
- Побочные эффекты (БД, HTTP, логи) — только в usecase-методах и выше
- Контекст — для cancellation и tracing, не для передачи бизнес-данных

## Dependency Injection

- Wire для DI (стандарт kratos)
- Конструкторы `New*` принимают интерфейсы, возвращают конкретные типы
- Не проверяй nil для зависимостей, инициализированных в `New*`

## Error Handling

- Kratos status errors для API-ошибок
- Доменные ошибки в biz/ как sentinel errors (`var ErrPostNotFound = errors.New(...)`)
- Всегда оборачивай ошибки с контекстом: `fmt.Errorf("save post: %w", err)`
- Не логируй ошибку и не возвращай её одновременно — выбери одно

### Правила по слоям

- **data/** — только возвращает ошибку наверх, не логирует. Оборачивает с контекстом операции: `fmt.Errorf("postRepo.Save: %w", err)`
- **biz/** — оборачивает ошибку бизнес-контекстом, не логирует. Может конвертировать инфраструктурные ошибки в доменные
- **service/** — конвертирует доменные ошибки в kratos status errors. Может логировать unexpected errors
- **task/** — точка логирования. Здесь принимается решение: логировать, ретраить, менять state. Leaf-функции не логируют

## Логирование

- slog, глобальные логгеры запрещены (sloglint: no-global: all)
- Логгер передаётся через DI
- Structured logging keys: только стабильные идентификаторы (event_id, source_id) и error
- Не засоряй логи лишними key-value парами — лаконичное сообщение + несколько ключей
- Логирование — на уровне оркестрации (воркеры, сервисы), не в leaf-функциях

## Кодогенерация

- При изменении входных данных генерации: `go generate ./...` и коммит результатов
- Сгенерированные файлы коммитятся в репозиторий
- Это касается: ent, protoc, mockgen, wire

## Линтер

- golangci-lint v2, конфиг в `.golangci.yml` в корне проекта
- Конфиг зафиксирован, не менять без обсуждения
- `local-prefixes` в goimports — обновить на реальный module path
- Максимальная длина строки: 120 символов (golines)
- Каноническая команда запуска: `golangci-lint run ./... -c .golangci.yml`. Запуск без `./...` и `-c .golangci.yml` запрещён — `./...` гарантирует обход всех пакетов, `-c` фиксирует путь к конфигу и исключает неявный pickup из окружения

## Конфигурация

- Вся конфигурация, включая секреты, в YAML-файлах в `configs/`.
- Kratos config для парсинга.
- Конфиг с секретами не коммитить.

### Ownership

| Owner | Отвечает за |
| --- | --- |
| `configs/*.yaml` | структура, defaults и секреты (LLM API key, Telegram credentials) |
| `cmd/feedium/main.go` | загрузка конфигурации через kratos |
| `internal/data/` | DSN, параметры подключения к БД |
| `internal/task/` | интервалы воркеров, rate limits |

### Workflow при изменении конфигурации

1. Обновить YAML в `configs/`.
2. Если новое поле — обновить kratos config struct.

## Graceful Shutdown и Health Checks

- Kratos управляет lifecycle — не реализуй свой signal handling
- Readiness probe: выключена до старта серверов, выключена после SIGTERM
- Liveness probe: всегда ok, ошибка только при fatal (требует рестарт)
- При SIGTERM: прекратить приём новых запросов, дождаться завершения текущих, закрыть соединения с БД и внешними сервисами
- Воркеры (task/) реализуют `transport.Server` — kratos вызывает Stop() автоматически
- Таймаут graceful shutdown: задаётся в конфигурации сервера, по умолчанию 10s

```go
// cmd/feedium/main.go
app := kratos.New(
    kratos.Name("feedium"),
    kratos.Server(httpSrv, grpcSrv, summaryWorker),
    // kratos обрабатывает SIGTERM/SIGINT и вызывает Stop() на всех серверах
)
```

## Общие правила

- Решай проблему, а не последствие
- Выбор библиотеки — только после согласования
- Не трогай существующие миграции
- Коммитить сгенерированный код
