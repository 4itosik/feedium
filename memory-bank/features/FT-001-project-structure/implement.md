---
doc_kind: feature
doc_function: implement
purpose: "Пошаговый план реализации FT-001: исполняемый каркас Go-проекта Feedium."
derived_from:
  - spec.md
status: done
---

# FT-001: Implementation Plan

## Steps

### 1. Инициализация модуля

- **Цель.** Создать `go.mod` с правильным module path и версией Go.
- **Действия.** Выполнить `go mod init github.com/4itosik/feedium`; выставить `go 1.26.1` в `go.mod`.
- **Зависимости.** Нет.
- **Результат.** Файл `go.mod` в корне с `module github.com/4itosik/feedium` и `go 1.26.1`.
- **Проверка.** `grep` по `go.mod`: строки `module github.com/4itosik/feedium` и `go 1.26.1` присутствуют.

### 2. Обновление `.gitignore`

- **Цель.** Убедиться, что git видит `cmd/feedium/` и другие служебные директории, но игнорирует артефакты сборки.
- **Действия.** В существующем `.gitignore` заменить строку `feedium` на `/bin/`. Добавить строку `/feedium` (для корневого бинарника, если сборка идёт в корень). Других строк не трогать.
- **Зависимости.** Нет.
- **Результат.** `.gitignore` не матчит `cmd/feedium/`; директория `bin/` и корневой бинарник `feedium` игнорируются.
- **Проверка.** `git status cmd/feedium/` (после создания на шаге 10) показывает файлы как untracked, а не игнорируемые.

### 3. Добавление зависимостей

- **Цель.** Подтянуть только разрешённые библиотеки (NFR-4).
- **Действия.** `go get github.com/go-kratos/kratos/v2`, `go get github.com/google/wire`, `go get google.golang.org/protobuf`.
- **Зависимости.** Шаг 1.
- **Результат.** В `go.mod` присутствуют только go-kratos, Wire, protobuf runtime и их транзитивные зависимости.
- **Проверка.** `go mod tidy` не добавляет посторонних прямых зависимостей; `go.sum` сгенерирован.

### 4. Установка protoc toolchain

- **Цель.** Обеспечить доступность `protoc` и `protoc-gen-go` для генерации Go-кода из `.proto`.
- **Действия.**
  1. Проверить `which protoc`; если не найден — установить через системный пакетный менеджер (brew install protobuf на macOS, apt install protobuf-compiler на Ubuntu) или скачать binary release.
  2. Установить Go-плагин: `go install google.golang.org/protobuf/cmd/protoc-gen-go@latest`.
  3. Убедиться, что `$(go env GOPATH)/bin` в `$PATH`: `which protoc-gen-go` возвращает путь.
- **Зависимости.** Шаг 1.
- **Результат.** `protoc --version` и `which protoc-gen-go` отрабатывают без ошибок; bundled Google proto-файлы (`google/protobuf/duration.proto` и др.) доступны через `--proto_path`.
- **Проверка.** Эхо-команда `echo 'syntax = "proto3"; message M {}' | protoc --proto_path=. --go_out=. --go_opt=paths=source_relative /dev/stdin` завершается exit 0.

### 5. Обновление `.golangci.yml`

- **Цель.** Зафиксировать `local-prefixes` для нового модуля (FR-11).
- **Действия.** В существующем `.golangci.yml` заменить значение `local-prefixes` на `github.com/4itosik/feedium`. Других полей не трогать.
- **Зависимости.** Шаг 1.
- **Результат.** `.golangci.yml` содержит `local-prefixes: github.com/4itosik/feedium`.
- **Проверка.** `grep "local-prefixes" .golangci.yml` показывает новое значение; `golangci-lint run ./... -c .golangci.yml` стартует без ошибок конфигурации.

### 6. Конфигурационный proto

- **Цель.** Описать структуру конфигурации kratos-style.
- **Действия.** Создать `internal/conf/conf.proto` с сообщениями `Bootstrap { Server server }`, `Server { HTTP http; GRPC grpc }`, `HTTP { string addr; google.protobuf.Duration timeout }`, `GRPC { string addr; google.protobuf.Duration timeout }`. Указать `option go_package = "github.com/4itosik/feedium/internal/conf;conf"`.
- **Зависимости.** Шаг 1.
- **Результат.** `internal/conf/conf.proto`, описывающий все поля из спеки (`server.http.addr`, `server.http.timeout`, `server.grpc.addr`, `server.grpc.timeout`).
- **Проверка.** Файл существует; имена полей и типы соответствуют спеке.

### 7. Генерация `conf.pb.go`

- **Цель.** Получить Go-код для конфигурации.
- **Действия.** Запустить `protoc --proto_path=. --go_out=. --go_opt=paths=source_relative internal/conf/conf.proto`; результат `internal/conf/conf.pb.go` закоммитить.
- **Зависимости.** Шаги 4, 6.
- **Результат.** `internal/conf/conf.pb.go` существует и компилируется.
- **Проверка.** `go build ./internal/conf/...` exit 0; повторный запуск той же команды protoc не создаёт diff.

### 8. `configs/config.yaml`

- **Цель.** Дефолтный конфиг для локального запуска.
- **Действия.** Создать `configs/config.yaml` со структурой `server.http.addr`, `server.http.timeout` (10s), `server.grpc.addr`, `server.grpc.timeout` (10s). Адреса — непустые валидные `host:port` (например `0.0.0.0:8000` и `0.0.0.0:9000`).
- **Зависимости.** Шаг 6.
- **Результат.** YAML-файл, парсимый kratos config в Bootstrap из шага 7.
- **Проверка.** YAML валиден; все четыре поля присутствуют и непусты.

### 9. Пакет `internal/server`

- **Цель.** Конструкторы HTTP- и gRPC-серверов kratos без зарегистрированных сервисов (FR-6).
- **Действия.** Создать `internal/server/http.go` с `NewHTTPServer(c *conf.Server, logger *slog.Logger) *http.Server` (kratos `transport/http`), применяющий `addr` и `timeout` из конфига. Аналогично `internal/server/grpc.go` с `NewGRPCServer`. Объявить `ProviderSet = wire.NewSet(NewHTTPServer, NewGRPCServer)` в `internal/server/server.go`. Добавить `internal/server/doc.go` со ссылкой на раздел `coding-style.md §server/`.
- **Зависимости.** Шаг 7.
- **Результат.** Пакет `internal/server` с двумя конструкторами и provider set; роуты не регистрируются.
- **Проверка.** `go build ./internal/server/...` exit 0; конструкторы возвращают сконфигурированный сервер.

### 10. Wire graph и точка входа

- **Цель.** Bootstrap процесса: config → logger → серверы → `kratos.New().Run()` (FR-4..FR-9, INV-2).
- **Действия.**
  1. `cmd/feedium/main.go`: парсинг флага `-conf` (путь к директории конфигов), инициализация `*slog.Logger` (JSON handler, level=info, stdout), загрузка конфигурации через `kratos/config` (`config.New(config.WithSource(file.NewSource(flagconf)))`), `c.Load()`, `c.Scan(&bc)` в `Bootstrap`. После загрузки — fail-fast валидация: если `bc.Server.Http.Addr` или `bc.Server.Grpc.Addr` пустые, логировать error с именем поля и `os.Exit(1)`. Затем вызов `wireApp(bc.Server, logger)` и `app.Run()`.
  2. `cmd/feedium/wire.go` с build tag `//go:build wireinject`: функция `wireApp(*conf.Server, *slog.Logger) (*kratos.App, func(), error)`, использующая `wire.Build(server.ProviderSet, newApp)`.
  3. Локальный `newApp(logger *slog.Logger, hs *http.Server, gs *grpc.Server) *kratos.App` — `kratos.New(kratos.Name("feedium"), kratos.Server(hs, gs))`.
  4. Сгенерировать `cmd/feedium/wire_gen.go` командой `go run github.com/google/wire/cmd/wire ./cmd/feedium/`; закоммитить.
- **Зависимости.** Шаги 7, 9.
- **Результат.** Запускаемый бинарь, использующий Wire-граф; в `main.go` нет бизнес-логики и упоминаний конкретных сервисов.
- **Проверка.** `go build ./...` exit 0; `go run ./cmd/feedium/ -conf configs/` стартует HTTP+gRPC; повторный запуск wire не создаёт diff.

### 11. `internal/conf/doc.go`

- **Цель.** Документация пакета конфигурации.
- **Действия.** Создать `internal/conf/doc.go` (`package conf`) с комментарием-ссылкой на `memory-bank/engineering/coding-style.md §Конфигурация`.
- **Зависимости.** Шаг 7.
- **Результат.** Файл `doc.go` присутствует в `internal/conf`.
- **Проверка.** `go build ./internal/conf/...` exit 0; ссылка соответствует canonical документу.

### 12. `Makefile`

- **Цель.** Стандартизованные команды разработки (FR-12).
- **Действия.** Создать `Makefile` с таргетами:
  - `build` — `go build -o bin/feedium ./cmd/feedium`
  - `run` — `./bin/feedium -conf configs/`
  - `lint` — `golangci-lint run ./... -c .golangci.yml`
  - `test` — `go test ./...`
  - `proto` — `protoc --proto_path=. --go_out=. --go_opt=paths=source_relative $$(find internal -name '*.proto')`
  - `wire` — `go run github.com/google/wire/cmd/wire ./...`
  - `generate` — последовательность `make proto wire`
- **Зависимости.** Шаги 1, 5, 7, 10 (все файлы, на которые ссылается Makefile, должны существовать).
- **Результат.** `Makefile` с семью таргетами.
- **Проверка.** `make build`, `make lint`, `make test`, `make proto`, `make wire`, `make generate` — все exit 0 на чистом репо; `make run` запускает бинарь.

### 13. Финальная верификация

- **Цель.** Подтвердить все Acceptance Criteria из `spec.md`.
- **Действия.** Прогнать `make lint && make build && make test`; запустить `make run`, проверить `nc -vz` на оба адреса; послать SIGTERM, проверить exit 0; вручную выставить пустой `server.http.addr`, проверить fail-fast выход с exit 1; в локальной (некоммитимой) ветке проверить FR-10.
- **Зависимости.** Все предыдущие шаги.
- **Результат.** Все чекбоксы Acceptance Criteria закрыты.
- **Проверка.** См. раздел Verification ниже.

## Edge Cases

- **Отсутствует флаг `-conf` или несуществующий путь.** Kratos config возвращает ошибку чтения файла; main логирует сообщение, содержащее подстроку `config` и путь, выходит с exit code 1 до старта серверов.
- **Невалидный YAML.** `c.Load()` или `c.Scan()` возвращают ошибку парсинга; main выходит с ненулевым кодом до старта серверов.
- **Пустые `server.http.addr` или `server.grpc.addr`.** Fail-fast проверка после `c.Scan` логирует error с именем пустого поля и завершает процесс с exit code 1 до создания серверов.
- **Невалидный `addr` (не `host:port`).** Listener kratos возвращает ошибку из `Start()`; kratos останавливает уже запущенные компоненты, процесс выходит с ненулевым кодом.
- **Порт занят.** Аналогично — ошибка `Start()`, корректная остановка остальных серверов, ненулевой exit.
- **SIGTERM во время старта.** Kratos прерывает запуск, вызывает `Stop` у уже стартовавших серверов, процесс завершается без зависших горутин.
- **SIGTERM/SIGINT в нормальном режиме.** Kratos lifecycle вызывает `Stop` в пределах `server.*.timeout`, процесс выходит с exit code 0.
- **HTTP-запрос на незарегистрированный путь.** Возвращается 404 (поведение kratos http по умолчанию).
- **gRPC-вызов незарегистрированного метода.** Возвращается `UNIMPLEMENTED`.
- **Повторный `make wire` / `make proto`.** Diff отсутствует — генерация детерминирована.
- **Добавление нового `transport.Server` (FR-10).** Изменяется только новый пакет и `ProviderSet` (либо ProviderSet нового пакета, добавленный в `wire.Build`); `cmd/feedium/main.go` и `internal/conf/conf.proto` не трогаются.

## Verification

1. `make lint` — exit 0.
2. `make build` — exit 0; бинарь `bin/feedium` создан.
3. `make test` — exit 0.
4. `make run` в одном терминале; в другом — `nc -vz <http_host> <http_port>` и `nc -vz <grpc_host> <grpc_port>` отвечают.
5. `kill -TERM <pid>` запущенного процесса — exit code 0, в логах виден kratos graceful shutdown, нет зависших горутин.
6. Установить пустую `server.http.addr` в `configs/config.yaml`, запустить — в stdout JSON-лог с level=error и именем пустого поля, exit code 1, серверы не стартовали.
7. Аналогично пустая `server.grpc.addr` — тот же результат.
8. Удалить `configs/config.yaml` и запустить — ошибка содержит подстроку `config` и путь, exit code != 0.
9. `make wire` повторно — `git diff` пуст.
10. `make proto` повторно — `git diff` пуст.
11. Ручной dry-run FR-10: в локальной ветке создать пакет с фейковым `transport.Server`, добавить в provider set, `make wire && make build` — `git diff cmd/feedium/main.go internal/conf/conf.proto` пуст.
12. Чек-лист Acceptance Criteria из `spec.md` — все пункты закрыты.

## Open Questions

Нет. `spec.md §Open Questions` явно фиксирует отсутствие неоднозначностей; форматы `addr`, дефолтный timeout, набор полей конфига, список Makefile-таргетов и список зависимостей — все определены в спеке.
