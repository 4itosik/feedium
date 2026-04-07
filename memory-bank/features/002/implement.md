# Implement Plan: Health Check Endpoint

## Reference

- Spec: memory-bank/features/002/spec.md
- Brief: memory-bank/features/002/brief.md

---

## Pre-conditions

- Текущий `cmd/feedium/main.go` — минимальный: инициализирует логгер, выводит сообщение
- `internal/platform/logger/logger.go` — существует, не меняется
- `go.mod` — чистый, без внешних зависимостей
- Директория `internal/bootstrap/` не существует

---

## Steps

### Step 1. Добавить зависимость Connect-go

```bash
go get connectrpc.com/connect
```

**Результат:** `go.mod` и `go.sum` содержат `connectrpc.com/connect` и транзитивные зависимости.

---

### Step 2. Создать health check handler

**Файл:** `internal/bootstrap/health.go`

Содержит функцию-хендлер:

```go
func healthHandler(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    io.WriteString(w, `{"status":"ok"}`)
}
```

Почему отдельный файл: изолирует хендлер для тестирования и не загромождает bootstrap.go lifecycle-кодом.

---

### Step 3. Создать `internal/bootstrap/bootstrap.go`

**Файл:** `internal/bootstrap/bootstrap.go`

Функция `Run(ctx context.Context, log *slog.Logger) error`:

1. Читает `PORT` из `os.Getenv`, дефолт `"8080"`
2. Валидирует порт: `strconv.Atoi` + проверка диапазона 1–65535. При ошибке — `return fmt.Errorf(...)` до запуска сервера
3. Создаёт `http.NewServeMux()`, регистрирует `GET /healthz` → `healthHandler`
4. Создаёт `http.Server{Addr: ":"+port, Handler: mux}`
5. Запускает `server.ListenAndServe()` в горутине. Ошибку (кроме `http.ErrServerClosed`) отправляет в канал `errCh`
6. Логирует `log.Info("listening", "port", port)`
7. Ожидает:
   - `<-ctx.Done()` — переход к shutdown
   - `<-errCh` — немедленный возврат ошибки
8. Shutdown:
   - Логирует `log.Info("shutting down")`
   - Создаёт `shutdownCtx` с таймаутом 5 секунд
   - Вызывает `server.Shutdown(shutdownCtx)`
   - Возвращает ошибку от `Shutdown` (или `nil`)

Ключевые решения:
- `select` на двух каналах (`ctx.Done()` и `errCh`) — стандартный паттерн, гарантирует что ошибка `ListenAndServe` не потеряется
- Таймаут shutdown = 5 секунд — hardcoded, конфигурация вне скоупа

---

### Step 4. Обновить `cmd/feedium/main.go`

Изменения:

```go
package main

import (
    "context"
    "os"
    "os/signal"
    "syscall"

    "feedium/internal/bootstrap"
    "feedium/internal/platform/logger"
)

func main() {
    log := logger.Init()
    log.Info("Feedium is starting")

    ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
    defer stop()

    if err := bootstrap.Run(ctx, log); err != nil {
        log.Error("server error", "error", err)
        os.Exit(1)
    }
}
```

Ключевые решения:
- `signal.NotifyContext` — контекст отменяется по SIGINT/SIGTERM, передаётся в `bootstrap.Run`
- Сообщение `"Feedium is starting"` сохранено (требование спеки)
- `os.Exit(1)` при ошибке — exit code 1

---

### Step 5. Написать unit-тест

**Файл:** `internal/bootstrap/bootstrap_test.go`

Тест `TestHealthHandler`:

```go
func TestHealthHandler(t *testing.T) {
    req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
    rec := httptest.NewRecorder()

    healthHandler(rec, req)

    // Проверить статус 200
    // Проверить Content-Type = "application/json"
    // Проверить тело = `{"status":"ok"}`
}
```

Тестируем только хендлер напрямую — без поднятия сервера. Это быстро и детерминировано.

---

### Step 6. Проверка инвариантов

```bash
go build ./...
go test ./...
go vet ./...
```

Проверить:
- [ ] Сборка проходит
- [ ] Тесты проходят
- [ ] `internal/bootstrap` не импортирует `internal/app` и `internal/components`
- [ ] `internal/platform/logger/logger.go` не изменён
- [ ] Нет proto-файлов и кодогенерации

---

## File Change Summary

| Файл | Действие |
|---|---|
| `go.mod` | Изменён (добавлена зависимость connect) |
| `go.sum` | Создан (транзитивные зависимости) |
| `internal/bootstrap/health.go` | Создан |
| `internal/bootstrap/bootstrap.go` | Создан |
| `internal/bootstrap/bootstrap_test.go` | Создан |
| `cmd/feedium/main.go` | Изменён |

---

## Risks

- **Порт занят при запуске** — обрабатывается: ошибка `ListenAndServe` пробрасывается наверх через `errCh`
- **Race между `ListenAndServe` и логом `listening`** — лог выводится до фактического начала прослушивания (после вызова `go server.ListenAndServe()`). Для health check это допустимо; для интеграционных тестов в будущем потребуется `net.Listen` + `server.Serve`
