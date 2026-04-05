# Health Check Endpoint

## Цель

Предоставить HTTP-эндпоинт для автоматической проверки доступности сервиса внешними системами (мониторинг, балансировщики).

---

## Reference

Brief: memory-bank/features/002/brief.md

---

## Scope

### Входит:
- Добавление Connect-go (`connectrpc.com/connect`) в зависимости проекта
- Создание HTTP-сервера в `internal/bootstrap`
- Реализация эндпоинта `GET /healthz`
- Graceful shutdown по SIGINT/SIGTERM
- Логирование старта и остановки сервера
- Unit-тест health check хендлера
- Обновление `cmd/feedium/main.go` для запуска сервера через bootstrap

### НЕ входит:
- Proto-файлы и кодогенерация (будет с первым бизнес-сервисом)
- Проверка зависимостей (БД, внешние сервисы)
- Бизнес-логика
- Аутентификация / авторизация
- Конфигурация через файлы (порт берётся из env)

---

## Требования

### 1. Зависимости

- В `go.mod` добавлен модуль `connectrpc.com/connect`
- Зависимость устанавливается через `go get`

### 2. Bootstrap

Создать пакет `internal/bootstrap` с файлом `bootstrap.go`.

Пакет содержит функцию:
```go
func Run(ctx context.Context, log *slog.Logger) error
```

Функция:
- Создаёт `http.NewServeMux()`
- Регистрирует health check хендлер на путь `/healthz`
- Создаёт `http.Server` с адресом `:<port>`
- Порт читается из переменной окружения `PORT`, по умолчанию `8080`
- Если значение `PORT` не является валидным числовым портом (1–65535), функция возвращает ошибку до запуска сервера
- Запускает сервер в горутине
- Если `ListenAndServe` возвращает ошибку (кроме `http.ErrServerClosed`), `Run` возвращает эту ошибку
- Ожидает отмены `ctx` (graceful shutdown)
- При получении сигнала завершения вызывает `server.Shutdown` с таймаутом 5 секунд
- Если shutdown не завершился за таймаут, `Run` возвращает ошибку от `Shutdown`
- Логирует через переданный `*slog.Logger`:
  - при старте: сообщение содержит подстроку `listening` и порт
  - при остановке: сообщение содержит подстроку `shutting down`

### 3. Health Check Handler

Хендлер обрабатывает `GET /healthz`:

- Ответ: HTTP 200
- Content-Type: `application/json`
- Body: `{"status":"ok"}`
- Не содержит бизнес-логики
- Не проверяет внешние зависимости
- Запросы к неизвестным путям и не-GET запросы к `/healthz` обрабатываются стандартным поведением `http.ServeMux` (404/405 соответственно)

### 4. main.go

Файл `cmd/feedium/main.go` обновлён:

- Создаёт контекст, отменяемый по SIGINT/SIGTERM
- Вызывает `logger.Init()`
- Вызывает `bootstrap.Run(ctx, log)`
- При ошибке от `bootstrap.Run` — логирует и завершается с exit code 1
- Сообщение `Feedium is starting` сохраняется

### 5. Unit-тест

Создать файл `internal/bootstrap/bootstrap_test.go`.

Тест проверяет health check хендлер:
- Использует `httptest.NewRequest` и `httptest.NewRecorder`
- Проверяет HTTP статус код = 200
- Проверяет Content-Type = `application/json`
- Проверяет тело ответа содержит `{"status":"ok"}`
- Тест запускается через `go test ./...`

---

## Инварианты

- Проект собирается через `go build ./...`
- Приложение запускается через `go run ./cmd/feedium/main.go`
- Все тесты проходят через `go test ./...`
- Пакеты `internal/platform/*` не содержат бизнес-логики
- Импорты направлены строго внутрь: `cmd` → `bootstrap` → `platform`
- Пакет `bootstrap` не импортирует `internal/app` и `internal/components`
- Пакет `logger` не изменён и не зависит от других внутренних пакетов
- Health check не содержит бизнес-логики и не проверяет зависимости

---

## Acceptance Criteria

- [ ] `go.mod` содержит зависимость `connectrpc.com/connect`
- [ ] Существует файл `internal/bootstrap/bootstrap.go`
- [ ] `bootstrap.Run(ctx, log)` запускает HTTP-сервер
- [ ] `GET /healthz` возвращает 200 с телом `{"status":"ok"}` и Content-Type `application/json`
- [ ] Сервер слушает порт из env `PORT` (дефолт `8080`)
- [ ] Graceful shutdown работает по отмене контекста
- [ ] `main.go` вызывает `bootstrap.Run` и обрабатывает ошибку
- [ ] При старте логируется сообщение с подстрокой `listening`
- [ ] При остановке логируется сообщение с подстрокой `shutting down`
- [ ] Существует файл `internal/bootstrap/bootstrap_test.go`
- [ ] Unit-тест проверяет статус 200, Content-Type и тело ответа
- [ ] `go test ./...` проходит без ошибок
- [ ] `go build ./...` проходит без ошибок
- [ ] При невалидном `PORT` (не число или вне диапазона 1–65535) `bootstrap.Run` возвращает ошибку
- [ ] Если порт занят, `bootstrap.Run` возвращает ошибку от `ListenAndServe`
- [ ] Если shutdown не завершился за таймаут, `bootstrap.Run` возвращает ошибку
- [ ] Все инварианты не нарушены

---

## Ограничения

- Не изменять `internal/platform/logger/logger.go`
- Не создавать proto-файлы и не запускать кодогенерацию
- Не добавлять бизнес-логику в health check
- Не создавать пакеты в `internal/app` и `internal/components`
- Не добавлять зависимости, кроме `connectrpc.com/connect` и её транзитивных
