---
doc_kind: feature
doc_function: implementation-plan
purpose: "Пошаговый план реализации FT-002: health-check эндпоинт /healthz (HTTP+gRPC) с проверкой PostgreSQL, первичный setup БД и goose-миграций."
derived_from:
  - spec.md
status: active
delivery_status: done
---

# FT-002: Implementation Plan

## Steps

### 1. Расширение `internal/conf/conf.proto` блоком `Data.Database`

- **Цель.** Описать конфигурацию подключения к PostgreSQL (FR-7).
- **Действия.** В `internal/conf/conf.proto` добавить сообщение `Data { Database database }` и сообщение `Database` с полями `string host`, `int32 port`, `string database`, `string user`, `string password`, `string sslmode`. Расширить `Bootstrap` полем `Data data`. Строковый тип пароля оставляем как есть — секрет приходит из env.
- **Зависимости.** Нет (инфраструктура FT-001 уже готова).
- **Результат.** `conf.proto` описывает `bootstrap.data.database.*` со всеми шестью полями.
- **Проверка.** Файл содержит сообщение `Database` с указанными полями; имена и типы совпадают с FR-7.

### 2. Регенерация `conf.pb.go`

- **Цель.** Получить Go-типы для новой секции конфигурации.
- **Действия.** Запустить `make proto` (или точечно `protoc` для `internal/conf/conf.proto`); сгенерированный `internal/conf/conf.pb.go` закоммитить.
- **Зависимости.** Шаг 1.
- **Результат.** Типы `conf.Data` и `conf.Database` доступны в Go.
- **Проверка.** `go build ./internal/conf/...` exit 0; повторная генерация не создаёт diff.

### 3. Дефолты и валидация конфигурации БД

- **Цель.** Реализовать дефолты `host=127.0.0.1`, `port=5432` и fail-fast валидацию обязательных полей (FR-7, FR-8).
- **Действия.** В `internal/conf/` (или отдельном файле `internal/conf/database.go`) добавить функцию, которая после `c.Scan(&bc)` в `cmd/feedium/main.go` подставляет дефолты, если `Host` пуст и `Port == 0`, и проверяет, что `Database`, `User`, `Password`, `Sslmode` непусты. При отсутствии любого обязательного поля — вернуть ошибку с именем поля.
- **Зависимости.** Шаг 2.
- **Результат.** Единая точка нормализации/валидации DSN-полей.
- **Проверка.** Unit-тест на четыре случая отсутствия обязательного поля + один happy path + случай применения дефолтов.

### 4. `configs/config.yaml` — секция `data.database`

- **Цель.** Локальный дефолтный конфиг с подключением к PostgreSQL для dev-окружения.
- **Действия.** В существующем `configs/config.yaml` добавить секцию `data: { database: { host: 127.0.0.1, port: 5432, database: feedium, user: feedium, password: feedium, sslmode: disable } }`. Пароль в dev-конфиге допустим; для прод — env-overrides.
- **Зависимости.** Шаг 1.
- **Результат.** YAML содержит валидный блок `data.database` со всеми шестью полями.
- **Проверка.** `kratos config` парсит файл в `Bootstrap` без ошибок; `c.Scan(&bc)` заполняет `bc.Data.Database`.

### 5. Подключение драйвера PostgreSQL

- **Цель.** Добавить единственную разрешённую зависимость на драйвер БД для `database/sql` (используется как Ent, так и прямой пул).
- **Действия.** `go get github.com/lib/pq` (стандартный `database/sql` драйвер, совместимый с Ent). `go mod tidy`.
- **Зависимости.** Нет.
- **Результат.** В `go.mod` присутствует прямая зависимость на драйвер PostgreSQL.
- **Проверка.** `go build ./...` exit 0; `go mod tidy` не чистит зависимость.

### 6. Решение по Ent: отложить до первой таблицы

- **Цель.** Зафиксировать, что в FT-002 Ent не подключается; пул БД — голый `*sql.DB`.
- **Действия.** Ничего не добавляется. Директория `ent/` не создаётся. Решение фиксируется комментарием в `internal/data/data.go` (шаг 7): «Ent client будет создан поверх `Data.db` через `entsql.OpenDB(dialect.Postgres, Data.db)` в фиче, вводящей первую сущность; физический пул останется тем же».
- **Зависимости.** Нет.
- **Результат.** Явное архитектурное решение, зафиксированное в коде и плане; FR-2 соблюдается тривиально, т.к. health-check и (будущий) Ent будут использовать единственный `*sql.DB`, которым владеет `internal/data`.
- **Проверка.** В `go.mod` нет зависимости на `entgo.io/ent`; в репо отсутствует директория `ent/`.

### 7. Пакет `internal/data` — пул БД и провайдеры Wire

- **Цель.** Создать единственный пул `*sql.DB`, используемый как health-check, так и будущим Ent client (FR-2, NFR-3).
- **Действия.** Создать `internal/data/data.go` с типом `Data { db *sql.DB }` и конструктором `NewData(c *conf.Data, logger *slog.Logger) (*Data, func(), error)`. Конструктор:
  1. Формирует DSN из полей `c.Database` (`postgres://user:password@host:port/dbname?sslmode=...`).
  2. Вызывает `sql.Open("postgres", dsn)`.
  3. Параметры пула (`MaxOpenConns`, `MaxIdleConns`, `ConnMaxLifetime`) явно **не** устанавливает — используются дефолты `database/sql`. Тюнинг откладывается до появления нагрузочных требований.
  4. Выполняет стартовый `db.PingContext` с таймаутом 5 секунд (fail-fast, FR-8).
  5. Возвращает cleanup-функцию, закрывающую `db.Close()`.
  Добавить интерфейс `type Pinger interface { Ping(ctx context.Context) error }` — `*HealthRepo` его реализует. `HealthService` (шаг 11) принимает `data.Pinger`, а не конкретный `*HealthRepo`: это делает возможным подстановку тест-дублера в шаге 16 (тест 5).
  Добавить `ProviderSet = wire.NewSet(NewData, NewHealthRepo)`. Создать `internal/data/doc.go` со ссылкой на `engineering/database.md` и комментарием из шага 6 об интеграции Ent.
- **Зависимости.** Шаги 3, 5, 6.
- **Результат.** Пакет `internal/data` с пулом, cleanup и провайдером для health-check.
- **Проверка.** `go build ./internal/data/...` exit 0; unit-тест конструктора с невалидным DSN (невалидный sslmode) возвращает ошибку до возврата `*Data`.

### 8. Proto-контракт `api/feedium/health.proto`

- **Цель.** Зафиксировать контракт health-check метода как источник правды для HTTP и gRPC (NFR-4, FR-1, FR-4..FR-6).
- **Действия.** Создать `api/feedium/health.proto`:
  - `syntax = "proto3"`, `package feedium`, `option go_package = "feedium/api/feedium;feedium"`.
  - `import "google/api/annotations.proto"`.
  - `service HealthService { rpc V1Check(V1CheckRequest) returns (V1CheckResponse) { option (google.api.http) = { get: "/healthz" }; } }`.
  - `message V1CheckRequest {}`.
  - `message V1CheckResponse { string status = 1; }`.
- **Зависимости.** Нет.
- **Результат.** Единственный proto-файл, описывающий HTTP+gRPC контракт health-check с ровно одним полем `status`.
- **Проверка.** Файл существует; ровно одно поле в `V1CheckResponse`; HTTP аннотация `GET /healthz`.

### 9. Генерация кода для `health.proto`

- **Цель.** Получить gRPC (и HTTP, для документации) stubs.
- **Действия.**
  1. Обновить таргет `proto` в `Makefile`: заменить `find internal` на `find internal api` и добавить недостающие флаги генерации. Итоговый таргет:
     ```makefile
     proto:
         protoc --proto_path=. --proto_path=third_party \
             --go_out=. --go_opt=paths=source_relative \
             --go-grpc_out=. --go-grpc_opt=paths=source_relative \
             --go-http_out=. --go-http_opt=paths=source_relative \
             $$(find internal api -name '*.proto')
     ```
     Без этого изменения `make proto` молча пропустит `api/feedium/health.proto`, а gRPC/HTTP стабы не будут сгенерированы.
  2. Убедиться, что в `third_party/` присутствуют `google/api/annotations.proto` и `google/api/http.proto` (если их нет — добавить). Запустить `make proto`; результат (`health.pb.go`, `health_grpc.pb.go`, `health_http.pb.go`) закоммитить. HTTP-stub генерируется вместе с остальными, но **не регистрируется** в рантайме (см. шаги 12, 13) — аннотация `(google.api.http) = { get: "/healthz" }` сохраняется в proto как документация контракта и на будущее.
- **Зависимости.** Шаг 8.
- **Результат.** Сгенерированные файлы в `api/feedium/`.
- **Проверка.** `go build ./api/feedium/...` exit 0; повторный `make proto` не создаёт diff.

### 10. Троттл-логгер неуспешных проверок

- **Цель.** Реализовать FR-10: не более одной `error`-записи в секунду, подавленные события не копятся.
- **Действия.** В `internal/service/health/` создать тип `throttledLogger` с полями `logger *slog.Logger`, `last atomic.Int64` (UnixNano последней записи). Метод `LogError(ctx, err)`:
  1. `now := time.Now().UnixNano()`.
  2. `prev := t.last.Load()`; если `now - prev < time.Second.Nanoseconds()` — return (подавить; счётчик не инкрементируется).
  3. `CAS(prev, now)`; в случае успеха — вызвать `logger.ErrorContext(ctx, "healthz ping failed", "err", err)`; в случае проигрыша CAS — также return (другой goroutine уже записал).
  Успешные проверки не логируются. `HealthService` (шаг 11) создаёт `throttledLogger` внутри конструктора.
- **Зависимости.** Нет (только стандартная библиотека).
- **Результат.** Компонент, гарантирующий инвариант «подавленный лог-эвент не эмитится позже и не агрегируется».
- **Проверка.** Unit-тест: 10 параллельных вызовов `LogError` в течение <1с → ровно 1 запись в тестовом `slog.Handler`; после `time.Sleep(1s)` ещё один вызов → 2-я запись.

### 11. Сервис `internal/service/health`

- **Цель.** Реализация единого внутреннего метода проверки БД и gRPC-обработчика `V1Check` (FR-2..FR-6, FR-11, INV).
- **Действия.** Создать `internal/service/health/health.go`:
  1. Тип `HealthService` с полем `throttle *throttledLogger` (шаг 10), реализующий сгенерированный `feedium.HealthServiceServer`.
  2. Конструктор `NewHealthService(p data.Pinger, logger *slog.Logger) *HealthService`; инициализирует `throttle` внутри.
  3. **Внутренний метод** `check(ctx context.Context) (status string, ok bool)` — единая точка истины для обоих транспортов:
     - `ctx, cancel := context.WithTimeout(ctx, 1*time.Second)`; `defer cancel()`.
     - `err := p.Ping(ctx)`.
     - `err == nil` → `return "ok", true`.
     - Иначе → `hs.throttle.LogError(ctx, err)` и `return "unavailable", false`.
  4. **gRPC метод** `V1Check(ctx, *V1CheckRequest) (*V1CheckResponse, error)`:
     - Вызывает `check(ctx)`.
     - `ok == true` → `return &V1CheckResponse{Status: "ok"}, nil`.
     - `ok == false` → `return nil, status.Error(codes.Unavailable, "unavailable")` (через `google.golang.org/grpc/status` и `codes`).
  5. Метод `check` экспортируется под именем `Check` — чтобы plain HTTP handler (шаг 12) мог его вызывать; handler живёт в том же пакете `internal/service/health`.
  6. Добавить `ProviderSet = wire.NewSet(NewHealthService)`.
  7. `doc.go` со ссылкой на `coding-style.md §service/`.
  Запрет на импорт `internal/biz` соблюдается: сервис зависит только от `internal/data`.
- **Зависимости.** Шаги 7, 9, 10.
- **Результат.** Сервисный пакет с единым методом `Check` и gRPC-обработчиком `V1Check`. HTTP и gRPC гарантированно отдают согласованный результат, потому что оба вызывают `Check`.
- **Проверка.** `go build ./internal/service/health/...` exit 0; `go vet` и depguard-правило на запрет импорта `internal/biz` проходят.

### 12. Plain HTTP handler для `/healthz`

- **Цель.** Отдать ровно `200 / {"status":"ok"}` или `503 / {"status":"unavailable"}` без вмешательства в глобальный kratos error encoder (FR-4, FR-5, FR-6, инвариант согласованности).
- **Действия.** В `internal/service/health/http.go`:
  1. Функция-фабрика `HTTPHandler(hs *HealthService) http.Handler`.
  2. Внутри: `status, ok := hs.Check(r.Context())`.
  3. `w.Header().Set("Content-Type", "application/json")`.
  4. `code := 200; if !ok { code = 503 }`; `w.WriteHeader(code)`.
  5. Записать тело: `json.NewEncoder(w).Encode(map[string]string{"status": status})`. При необходимости — `fmt.Fprintf` с литералом для исключения trailing newline.
  6. Для запросов не-GET возвращать `405 Method Not Allowed` с пустым телом.
- **Зависимости.** Шаг 11.
- **Результат.** HTTP-ответ на `GET /healthz`: `200 + {"status":"ok"}` при ok, `503 + {"status":"unavailable"}` при ошибке. `Content-Type: application/json` в обоих случаях. Глобальный kratos error encoder не трогается.
- **Проверка.** Интеграционные тесты шага 16 фиксируют код, тело (byte-to-byte) и Content-Type для обоих исходов.

### 13. Регистрация `HealthService` в серверах kratos

- **Цель.** Подключить сервис к HTTP и gRPC серверам (FR-1). gRPC — через сгенерированный stub; HTTP — через plain handler из шага 12.
- **Действия.**
  1. В `internal/server/grpc.go`: изменить сигнатуру `NewGRPCServer(c *conf.Server, hs *healthservice.HealthService, logger *slog.Logger) *grpc.Server`; внутри вызвать `feedium.RegisterHealthServiceServer(srv, hs)`.
  2. В `internal/server/http.go`: изменить сигнатуру `NewHTTPServer(c *conf.Server, hs *healthservice.HealthService, logger *slog.Logger) *http.Server`; **не вызывать** сгенерированный `RegisterHealthServiceHTTPServer`; зарегистрировать handler: `srv.HandleFunc("/healthz", health.HTTPHandler(hs).ServeHTTP)` (имя метода kratos `http.Server` — уточнить по актуальной версии; может быть `Route` или прямой доступ к роутеру).
  3. `ProviderSet` в `internal/server/server.go` по составу не меняется, но граф Wire теперь знает о `HealthService`.
- **Зависимости.** Шаги 9, 11, 12.
- **Результат.** gRPC отдаёт метод через штатный stub; HTTP отдаёт `/healthz` через plain handler. Оба пути вызывают `HealthService.Check`, что гарантирует согласованность.
- **Проверка.** `go build ./...` exit 0; интеграционный тест (шаг 16) подтверждает оба транспорта.

### 14. Wire graph

- **Цель.** Собрать граф зависимостей: `conf.Data → data.Data → data.Pinger → service/health.HealthService → server.HTTP/gRPC → kratos.App`.
- **Действия.**
  1. В `cmd/feedium/wire.go` добавить в `wire.Build` пакеты `data.ProviderSet` и `health.ProviderSet`.
  2. Уточнить `wireApp` так, чтобы принимать `*conf.Bootstrap` (или отдельно `*conf.Server` и `*conf.Data`) и возвращать `*kratos.App, func(), error`; cleanup из `NewData` должен пробрасываться наружу.
  3. Регенерировать `cmd/feedium/wire_gen.go` командой `make wire`; закоммитить.
- **Зависимости.** Шаги 7, 11, 13.
- **Результат.** Wire-граф содержит data и health-сервис; cleanup закрывает пул при завершении процесса.
- **Проверка.** `go build ./...` exit 0; повторный `make wire` не создаёт diff.

### 15. Fail-fast при старте и обновление `main.go`

- **Цель.** FR-8: процесс не открывает порты, если БД недоступна или конфиг невалиден.
- **Действия.** В `cmd/feedium/main.go`:
  1. После `c.Scan(&bc)` вызвать нормализацию/валидацию из шага 3; при ошибке — `logger.Error` + `os.Exit(1)` до вызова `wireApp`.
  2. Вызвать `app, cleanup, err := wireApp(&bc, logger)`; если `err != nil` (включая ошибку стартового ping из `NewData`) — `logger.Error` + `os.Exit(1)`; `cleanup` в этой ветке не вызывается (он ещё не сформирован).
  3. Зарегистрировать `defer cleanup()` после успешного возврата.
  4. Вызвать `app.Run()`.
- **Зависимости.** Шаги 3, 7, 14.
- **Результат.** При невалидной конфигурации или недоступной БД процесс завершает работу с кодом `1` и error-логом; серверы не открываются.
- **Проверка.** Ручной запуск с выключенным PostgreSQL → exit 1, отсутствие слушающих портов (`nc -vz` connection refused).

### 16. Интеграционные тесты health-check

- **Цель.** Покрыть Acceptance Criteria автоматическими тестами (согласно `engineering/testing-policy.md`).
- **Действия.** Создать `internal/service/health/health_test.go` с использованием `testcontainers-go` (PostgreSQL) и `goleak`:
  1. **Happy path HTTP.** Поднять контейнер PostgreSQL → инициализировать реальный `*sql.DB` → HealthService → вызов `V1Check` → проверить `status="ok"`.
  2. **Happy path HTTP через сервер.** Поднять kratos HTTP-сервер с реальным handler → `http.Get("/healthz")` → `200` + `{"status":"ok"}` + `Content-Type: application/json`.
  3. **Happy path gRPC.** Поднять kratos gRPC-сервер → вызов через gRPC клиент → `OK` + `status="ok"`.
  4. **БД остановлена.** `container.Stop()` → HTTP `503` + `{"status":"unavailable"}`; gRPC `UNAVAILABLE` + `status="unavailable"`.
  5. **Медленный ping.** Настроить `iptables`/прокси не получится в unit-тесте; вместо этого создать тестовый struct, реализующий `data.Pinger` и спящий 2 с в `Ping`, передать его в `NewHealthService` → получить `503` + `unavailable` за ≤1.1 с (проверка таймаута). Требует `data.Pinger` из шага 7.
  6. **Общий пул.** Тест через reflection или интерфейсный assertion проверяет, что `HealthService` получает тот же `*sql.DB`, что регистрируется в Wire-графе (AC-5). Достигается передачей пула как единственного источника.
  7. **Троттлинг логов.** В HealthService подменить `slog.Handler` на тестовый счётчик; с остановленной БД сделать ≥10 вызовов `V1Check` в течение 1 с → счётчик `error`-записей равен 1. После `time.Sleep(1s)` ещё 1 вызов → счётчик = 2.
  8. **Успех не логируется.** Happy-path тест дополнительно проверяет, что счётчик `error`-записей равен 0.
  9. **Запрет импорта biz.** Тест уровня пакета: `go list -deps ./internal/service/health/...` не содержит `internal/biz` (альтернатива — статический lint через `depguard` в `.golangci.yml`).
  10. **Анонимный доступ.** HTTP-тест отправляет запрос без заголовков `Authorization` и получает `200`.
  Все тесты оборачиваются в `defer goleak.VerifyNone(t)`.
- **Зависимости.** Шаги 10–13.
- **Результат.** Тестовый пакет покрывает AC-1..AC-4, AC-5, AC-9..AC-12.
- **Проверка.** `go test ./internal/service/health/...` exit 0 при запущенном docker.

### 17. Тесты fail-fast на старте

- **Цель.** Покрыть AC-6, AC-7, AC-8 (fail-fast при недоступной БД и невалидной конфигурации).
- **Действия.** Создать `cmd/feedium/main_test.go` (или `internal/conf/database_test.go`):
  1. **Недоступная БД.** Запустить `exec.Command` с `go run ./cmd/feedium -conf testdata/unreachable/` (в `testdata/unreachable/config.yaml` прописать `host=127.0.0.1`, `port=1` — порт, на котором БД гарантированно недоступна). Ожидать exit code != 0 за ≤5 с. Проверить отсутствие прослушивания портов (опционально).
  2. **Отсутствующее обязательное поле.** Для каждого из `database`, `user`, `password`, `sslmode` — отдельная `testdata/missing-XXX/config.yaml`. Запустить, ожидать exit != 0 и error-лог, упоминающий имя поля.
  3. **Дефолты host/port.** Unit-тест на функцию нормализации из шага 3.
- **Зависимости.** Шаги 3, 15.
- **Результат.** AC-6, AC-7, AC-8 закрыты автотестами.
- **Проверка.** `go test ./...` exit 0.

### 18. Goose-инфраструктура и baseline-миграция

- **Цель.** FR-9 и AC по goose: ровно одна пустая baseline-миграция, штатная команда применения работает.
- **Действия.**
  1. `go install github.com/pressly/goose/v3/cmd/goose@latest` (устанавливается локально, не в `go.mod`).
  2. Создать директорию `migrations/`.
  3. Создать файл `migrations/20260413000000_baseline.sql` со структурой:
     ```
     -- +goose Up
     -- baseline: no schema changes
     -- +goose Down
     -- baseline: no schema changes
     ```
  4. Добавить в `Makefile` таргет `migrate` (или `db-migrate`), который вызывает `goose -dir migrations postgres "$(DSN)" up`, где `DSN` собирается из env-переменных или параметра.
  5. Добавить в `README.md` проекта строку о команде применения миграций (опционально — за рамками spec, не обязательно).
- **Зависимости.** Шаг 4 (нужен DSN dev-окружения).
- **Результат.** В репозитории присутствует `migrations/` с одной пустой миграцией; `make migrate` проходит успешно на чистой БД.
- **Проверка.** `ls migrations/ | wc -l` = 1; `make migrate` на чистой локальной БД → exit 0; вторая прогонка — также exit 0 (goose отмечает миграцию как применённую).

### 19. Lint-правило на запрет импорта `internal/biz` из `internal/service/health`

- **Цель.** Статическая гарантия FR-11.
- **Действия.** В `.golangci.yml` добавить `depguard`-правило: для пакета `internal/service/health` запретить импорт `internal/biz/...`. Альтернатива — архитектурный тест в шаге 16.9.
- **Зависимости.** Шаг 10.
- **Результат.** Нарушение FR-11 ловится линтером до merge.
- **Проверка.** `make lint` exit 0; искусственная попытка добавить `import "github.com/4itosik/feedium/internal/biz"` в health-пакет приводит к ошибке линтера.

### 20. Финальная верификация

- **Цель.** Закрыть все Acceptance Criteria `spec.md`.
- **Действия.** Прогнать `make lint && make build && make test && make migrate` против локальной БД; запустить `make run`; вручную пройтись по чек-листу из раздела Verification ниже.
- **Зависимости.** Все предыдущие шаги.
- **Результат.** Все чекбоксы Acceptance Criteria закрыты.
- **Проверка.** См. раздел Verification.

## Edge Cases

- **БД доступна, но `sslmode` требует сертификата, которого нет.** `sql.Open` не ошибается, но `PingContext` при старте возвращает ошибку → fail-fast exit 1.
- **БД поднимается позже сервиса.** Стартовый ping падает → процесс выходит с exit 1 (spec: fail-fast, а не ожидание).
- **БД падает в работе.** Ping возвращает ошибку → `503` / `UNAVAILABLE`; троттл-логгер пишет не более 1 error/сек; процесс продолжает работать (spec не требует self-heal или остановки).
- **БД отвечает медленно (>1 с).** `context.DeadlineExceeded` → `503` / `UNAVAILABLE`; пул не удерживает соединение дольше таймаута (cancel освобождает соединение, NFR-1).
- **Пул исчерпан.** Ожидание свободного соединения превышает 1 с → `context.DeadlineExceeded` → `503` / `UNAVAILABLE`.
- **Частые запросы при недоступной БД (10+ rps).** Каждый вызов вызывает ping; в лог попадает ровно одна error-запись в секунду; подавленные не агрегируются.
- **Параллельные ошибки в одну и ту же наносекунду.** CAS в троттл-логгере гарантирует, что запись произойдёт ровно один раз.
- **gRPC и HTTP одновременно вызывают /healthz.** Оба обработчика используют один и тот же `HealthRepo` и пул; результат согласован (инвариант `status` ↔ код).
- **HTTP-клиент без `Accept: application/json`.** Сервер всё равно отдаёт `Content-Type: application/json` (FR-4).
- **Повторный `make proto` / `make wire`.** Генерация детерминирована, diff отсутствует.
- **Применение миграций на БД с уже существующей `goose_db_version`.** Goose пропускает уже применённые миграции, exit 0.
- **Отсутствие env `PGPASSWORD`-style override.** Пароль берётся из `configs/config.yaml`; если и там пусто — валидация из шага 3 отклоняет старт.
- **Невалидный `port` (0 или отрицательный).** Функция нормализации считает `port == 0` признаком «не задано» → подставляет 5432. Отрицательные/> 65535 — ошибка конфигурации.
- **Отсутствует `google/api/annotations.proto` в `third_party/`.** `make proto` падает — решается шагом 9 (добавление файлов).

## Verification

1. `make lint` — exit 0 (включая depguard-правило шага 19).
2. `make build` — exit 0.
3. `make test` — exit 0 (интеграционные тесты шагов 16, 17 запущены).
4. `make migrate` на чистой локальной БД — exit 0; таблица `goose_db_version` содержит запись о baseline-миграции.
5. `make run` с доступной PostgreSQL → `curl -i http://<http_addr>/healthz` отвечает `200`, `Content-Type: application/json`, тело `{"status":"ok"}`.
6. Вызов через gRPC-клиент (например, `grpcurl -plaintext <grpc_addr> feedium.HealthService/V1Check`) → `OK` + `{"status":"ok"}`.
7. Остановить PostgreSQL → повторить шаги 5 и 6: HTTP `503` + `{"status":"unavailable"}`; gRPC `UNAVAILABLE` + `status="unavailable"`.
8. Во время шага 7 запустить `for i in $(seq 1 20); do curl -s http://<http_addr>/healthz; done` за <1 с → в stdout-логе процесса ровно одна error-запись.
9. Повторить шаг 5 ещё раз при поднятой БД → в логе отсутствуют error-записи.
10. Остановить PostgreSQL, запустить `make run` → процесс завершается с exit 1; `nc -vz` на HTTP/gRPC порты — connection refused.
11. В `configs/config.yaml` удалить поле `database` → `make run` → exit 1, error-лог упоминает `database`. Повторить для `user`, `password`, `sslmode`.
12. В `configs/config.yaml` удалить `host` и `port` → `make run` при поднятой локальной БД на `127.0.0.1:5432` → процесс стартует (дефолты применены).
13. `go list -deps ./internal/service/health/...` не содержит `github.com/4itosik/feedium/internal/biz`.
14. `curl -i http://<http_addr>/healthz` без заголовков Authorization → `200` (анонимный доступ).
15. Чек-лист Acceptance Criteria `spec.md` — все 12 пунктов закрыты.

## Open Questions

Нет. Все ранее открытые вопросы закрыты решениями, зафиксированными в шагах:

- **Ent vs прямой `database/sql`** → откладываем Ent до первой фичи с таблицей; `internal/data.Data` владеет `*sql.DB` (шаг 6). Ent при появлении сядет на тот же пул через `entsql.OpenDB`.
- **HTTP-маппинг `503 + {"status":"unavailable"}`** → plain HTTP handler для `/healthz`, gRPC штатно через stub; оба вызывают общий `HealthService.Check` (шаги 10, 12, 13).
- **HTTP-аннотация в proto** → оставляем как документацию контракта; сгенерированный HTTP-stub не регистрируется в рантайме (шаг 9).
- **Имя baseline-миграции** → `20260413000000_baseline.sql` (шаг 18).
- **Параметры пула** → Go-дефолты `database/sql`, без явного тюнинга (шаг 7).
- **`sslmode=disable` в dev-конфиге** → допустимо для локального окружения; прод-конфиги будут использовать `require`/`verify-full` через env-overrides (шаг 4).
