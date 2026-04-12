---
doc_kind: feature
doc_function: spec
purpose: "Спецификация FT-001: исполняемый каркас Go-проекта Feedium по kratos-layout."
derived_from:
  - brief.md
  - ../../engineering/coding-style.md
  - ../../domain/architecture.md
status: done
---

# FT-001: Структура проекта Feedium

## Цель

Создать минимальный исполняемый каркас Go-проекта по kratos-layout, в котором процесс запускается, загружает конфигурацию, поднимает HTTP/gRPC серверы, корректно останавливается по SIGTERM и готов принять FT-002 (Health Check) добавлением одного компонента без изменений в `cmd/feedium/main.go` и в `internal/conf/conf.proto`.

## Reference

- Brief: `memory-bank/features/FT-001-project-structure/brief.md`
- Coding style: `memory-bank/engineering/coding-style.md`
- Architecture: `memory-bank/domain/architecture.md`

## Scope

### Входит

- `go.mod` с module path `github.com/4itosik/feedium`, Go `1.26.1`
- Точка входа `cmd/feedium/main.go` + Wire (`wire.go` + сгенерированный `wire_gen.go`) с реальным минимальным графом зависимостей
- Пакет конфигурации kratos-style на protobuf: `internal/conf/conf.proto` + сгенерированный `conf.pb.go`, загрузка через `kratos/config` из `configs/config.yaml`
- Поля конфигурации: `server.http.addr`, `server.http.timeout`, `server.grpc.addr`, `server.grpc.timeout` (`addr` — строка вида `host:port`)
- Fail-fast валидация конфигурации после загрузки: обязательные поля (`server.http.addr`, `server.grpc.addr`) непустые, иначе процесс падает с ненулевым exit code до старта серверов
- Пакет `internal/server/` с конструкторами HTTP и gRPC kratos-серверов, без зарегистрированных сервисов (готовы принимать регистрацию роутов)
- Инициализация `*slog.Logger` (JSON handler, level=info, writer=stdout) в `main.go` и проброс через Wire как зависимость
- Запуск через `kratos.New(...).Run()` с передачей HTTP- и gRPC-серверов; kratos обрабатывает SIGTERM/SIGINT
- Обновление `local-prefixes` в `.golangci.yml` на `github.com/4itosik/feedium`
- `Makefile` с таргетами: `build`, `run`, `lint`, `test`, `generate`, `wire`, `proto`
- `doc.go` в каждой создаваемой `internal/*` директории со ссылкой на соответствующий раздел `memory-bank/engineering/coding-style.md`

### НЕ входит

- Реализация health-check эндпоинта (FT-002) и любая прикладная логика
- Пакеты `internal/biz`, `internal/data`, `internal/service`, `internal/task` — создаются в рамках фичи, которой они нужны
- `api/feedium/*.proto` сервисных API, `ent/`, `migrations/`, `third_party/`
- Docker, docker-compose, развёртывание
- Изменения в `ci.yml`, кроме случаев несовместимости с новым каркасом
- Любая бизнес-логика, клиенты внешних сервисов, доменные модели
- Корневой `README.md` / `ARCHITECTURE.md` как навигационная точка по layout (отложено до момента, когда в проекте будет больше пакетов)

## Контекст

Репозиторий содержит только документацию. FT-002 (Health Check) заблокирован отсутствием точки входа, конфигурации и серверного слоя. Архитектура и стек зафиксированы в `memory-bank/domain/architecture.md` и `memory-bank/engineering/coding-style.md`; задача исполнительская, не проектная.

## Функциональные требования

1. **FR-1.** `go build ./...` завершается с exit code 0.
2. **FR-2.** `golangci-lint run ./... -c .golangci.yml` завершается с exit code 0.
3. **FR-3.** `go test ./...` завершается с exit code 0 (пустой набор тестов допустим).
4. **FR-4.** Бинарь `feedium`, запущенный с флагом пути к конфигу (`-conf configs/`), читает `configs/config.yaml` и парсит его в kratos-config struct, сгенерированный из `internal/conf/conf.proto`.
5. **FR-5.** После загрузки конфигурации выполняется fail-fast валидация: при пустых `server.http.addr` или `server.grpc.addr` процесс пишет ошибку в логгер и завершается с ненулевым exit code до старта серверов.
6. **FR-6.** После успешной валидации kratos запускает HTTP-сервер на `server.http.addr` и gRPC-сервер на `server.grpc.addr`; оба сервера готовы принимать TCP-соединения, даже если в них не зарегистрированы сервисы.
7. **FR-7.** При получении SIGTERM или SIGINT процесс завершает работу с exit code 0 в пределах `server.*.timeout` (graceful shutdown через kratos lifecycle); после остановки не остаётся зависших горутин.
8. **FR-8.** В `main.go` инициализируется `*slog.Logger` (JSON handler, level=info, writer=stdout) и передаётся во все компоненты через Wire. Глобальный логгер не используется (sloglint: no-global).
9. **FR-9.** Dependency graph собирается через Wire: `wire.go` объявляет provider sets, `wire_gen.go` закоммичен в репозиторий. Команда `make wire` (`go run github.com/google/wire/cmd/wire ./...`) на чистом репозитории не создаёт diff.
10. **FR-10.** Добавление нового kratos `transport.Server` (например, health-хендлера в FT-002) требует: (а) создания нового пакета с конструктором `New*`, (б) добавления провайдера в существующий `ProviderSet`, (в) регенерации Wire. Файлы `cmd/feedium/main.go` и `internal/conf/conf.proto` не изменяются.
11. **FR-11.** `local-prefixes` в `.golangci.yml` установлен в `github.com/4itosik/feedium`.
12. **FR-12.** `Makefile` содержит таргеты `build`, `run`, `lint`, `test`, `generate`, `wire`, `proto`; каждый таргет (кроме `run`) выполняется без ошибок на чистом репозитории.

## Нефункциональные требования

1. **NFR-1.** Структура каталогов строго соответствует kratos-layout из `coding-style.md §Структура проекта`; новых корневых директорий не вводится.
2. **NFR-2.** Код на английском, файлы в snake_case, пакеты — одно слово lowercase (`coding-style.md §Язык и именование`).
3. **NFR-3.** Максимальная длина строки — 120 символов (golines).
4. **NFR-4.** Зависимости в `go.mod`: только go-kratos (`github.com/go-kratos/kratos/v2`), Wire (`github.com/google/wire`), protobuf runtime. Новые библиотеки за пределами этого списка — через отдельное согласование.
5. **NFR-5.** Graceful shutdown timeout — из конфигурации (`server.*.timeout`), значение по умолчанию 10s согласно `coding-style.md §Graceful Shutdown`.

## Сценарии и edge cases

- **Happy path.** `make build` → `./bin/feedium -conf configs/` → логи старта HTTP и gRPC серверов → TCP listener на обоих `addr` отвечает → SIGTERM → логи graceful shutdown → exit 0.
- **Отсутствует конфиг.** Запуск без `-conf` или с несуществующим путём — процесс пишет в stderr сообщение, содержащее подстроку `config` и путь к файлу, и завершается с exit code 1 до старта серверов.
- **Невалидный YAML.** Парсер kratos возвращает ошибку, процесс падает с ненулевым exit code до старта серверов.
- **Пустые обязательные поля.** Если `server.http.addr` или `server.grpc.addr` пустые после загрузки — fail-fast валидация (FR-5) завершает процесс с ненулевым exit code до старта серверов.
- **Порт занят.** Один из серверов не может поднять listener — kratos возвращает ошибку из `Start()`, процесс завершает уже стартовавшие компоненты и выходит с ненулевым кодом.
- **SIGTERM во время старта.** Kratos корректно прерывает запуск и завершает процесс без зависших горутин.
- **Нет зарегистрированных роутов.** HTTP-запрос на любой путь возвращает 404; gRPC-запрос на незарегистрированный метод возвращает `UNIMPLEMENTED`.

## Инварианты

- **INV-1.** Никакого глобального состояния (логгер, конфиг, клиенты) — всё через Wire.
- **INV-2.** `cmd/feedium/main.go` не содержит бизнес-логики и не знает про конкретные сервисы; только bootstrap (config → logger → wireApp → run).
- **INV-3.** `internal/conf` не импортируется из `biz/` (когда тот появится).
- **INV-4.** Сгенерированные файлы (`wire_gen.go`, `conf.pb.go`) закоммичены в репозиторий.
- **INV-5.** `go build ./...`, `golangci-lint run ./... -c .golangci.yml`, `go test ./...` зелёные на каждом коммите.

## Acceptance Criteria

- [ ] `go build ./...` завершается с exit code 0
- [ ] `golangci-lint run ./... -c .golangci.yml` завершается с exit code 0
- [ ] `go test ./...` завершается с exit code 0
- [ ] `go.mod` содержит `module github.com/4itosik/feedium` и `go 1.26.1`
- [ ] `.golangci.yml` содержит `local-prefixes: github.com/4itosik/feedium`
- [ ] `configs/config.yaml` существует и содержит поля `server.http.addr`, `server.http.timeout`, `server.grpc.addr`, `server.grpc.timeout`
- [ ] `internal/conf/conf.proto` определяет соответствующие сообщения; `conf.pb.go` сгенерирован и закоммичен
- [ ] `cmd/feedium/main.go` инициализирует slog JSON logger (level=info), загружает конфиг, выполняет fail-fast валидацию обязательных полей, вызывает wire-инициализатор, запускает `kratos.New(...).Run()`
- [ ] Запуск с пустым `server.http.addr` или `server.grpc.addr` завершается ненулевым exit code до старта серверов
- [ ] `wire.go` и `wire_gen.go` присутствуют; `make wire` на чистом репо не создаёт diff
- [ ] `internal/server/` содержит конструкторы HTTP и gRPC kratos-серверов, оба возвращают настроенный сервер без зарегистрированных роутов
- [ ] Запущенный процесс: TCP listeners на обоих `addr` активны; SIGTERM → exit 0, зависших горутин нет
- [ ] *(manual)* Демонстрация FR-10 (dry-run, не коммитится): добавление dummy `transport.Server` в отдельном пакете требует правок только в provider set и регенерации Wire — `cmd/feedium/main.go` и `internal/conf/conf.proto` не меняются
- [ ] `Makefile` содержит таргеты `build`, `run`, `lint`, `test`, `generate`, `wire`, `proto`; каждый (кроме `run`) завершается без ошибок
- [ ] В каждой созданной директории `internal/*` есть `doc.go` со ссылкой на соответствующий раздел `memory-bank/engineering/coding-style.md`

## Ограничения

- Не менять существующий `.golangci.yml`, кроме поля `local-prefixes`
- Не трогать `ci.yml`, кроме случаев, когда текущий CI падает на новом каркасе (тогда — правки ограничены изменением полей `run:` в существующих шагах, не более 3 строк; добавление новых шагов или изменение `uses:` — через отдельное согласование)
- Не создавать пакеты `biz/`, `data/`, `service/`, `task/` до фичи, которая их требует
- Не вводить новые внешние библиотеки сверх перечисленных в NFR-4

## Open Questions

Нет.

## Верификация

1. `make lint && make build && make test` — все зелёные.
2. `make run` → в другом терминале `nc -vz` на оба `addr` — listeners отвечают.
3. `kill -TERM <pid>` — процесс завершается с exit code 0, в логах виден graceful shutdown.
4. Запуск с пустым `server.http.addr` в `configs/config.yaml` — процесс пишет в лог (JSON, level=error) имя пустого поля и завершается с exit code 1 до старта серверов.
5. Ручной dry-run FR-10: в локальной (некоммитимой) ветке создать пакет с фейковым `transport.Server`, добавить в provider set, `make wire && make build` — подтвердить, что `cmd/feedium/main.go` и `internal/conf/conf.proto` не менялись.
