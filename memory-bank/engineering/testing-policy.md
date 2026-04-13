---
doc_kind: engineering
doc_function: convention
purpose: Описывает testing policy репозитория: обязательность test case design, требования к automated regression coverage и допустимые manual-only gaps.
derived_from:
  - ../dna/governance.md
status: active
---

# Testing Policy

## Stack

- **Framework:** `go test`, `testify` (assert + require) — **осознанное отклонение** от [Google Go Style Guide](https://google.github.io/styleguide/go/decisions#assert-libraries), который считает assertion-хелперы не-идиоматичными. Проект принял testify ради скорости написания и консистентности. В pull request'ах не смешивать стили в одном пакете.
- **Моки:** `go.uber.org/mock` (mockgen), не ручные моки
- **Goroutine Leak Detection:** `go.uber.org/goleak`
- **Integration:** Testcontainers (PostgreSQL)
- **Запуск:** `go test ./...` (unit), `go test ./... -run Integration` (integration)
- **CI:** GitHub Actions

## Core Rules

- Любое изменение поведения, которое можно проверить детерминированно, обязано получить automated regression coverage.
- Любой новый или измененный contract (proto API, интерфейсы biz/, внешние HTTP-вызовы) обязан получить contract-level automated verification.
- Любой bugfix обязан добавить regression test на воспроизводимый сценарий.
- Required automated tests считаются закрывающими риск только если они проходят локально и в CI.
- Manual-only verify допустим только как явное исключение и не заменяет automated coverage там, где automation реалистична.

## Ownership Split

- Canonical test cases задаются в `features/<name>/spec.md` через `SC-*`, feature-specific `NEG-*`, `CHK-*` и `EVID-*`.
- `features/<name>/plan.md` владеет только стратегией исполнения: какие test surfaces будут добавлены или обновлены, какие gaps временно остаются manual-only и почему.

## Feature Flow Expectations

Canonical lifecycle gates:

- к `Design Ready` `spec.md` уже фиксирует test case inventory;
- к `Plan Ready` `plan.md` содержит `Test Strategy` с planned automated coverage и manual-only gaps;
- к `Done` required tests добавлены, `go test ./...` зелёный локально и CI не противоречит локальному verify.

## Что Считается Sufficient Coverage

- Покрыт основной changed behavior и ближайший regression path.
- Покрыты новые или измененные contracts, события, schema или integration boundaries.
- Покрыты критичные failure modes из `FM-*`, bug history или acceptance risks.
- Покрыты feature-specific negative/edge scenarios, если они меняют verdict.
- Процент line coverage сам по себе недостаточен: нужен scenario- и contract-level coverage.

## Когда Manual-Only Допустим

- Сценарий зависит от live infra, внешних API (Telegram, LLM), недетерминированной среды или human оценки UI.
- Для каждого manual-only gap: причина, ручная процедура, owner follow-up.
- Если manual-only gap оставляет без regression protection критичный путь, feature не считается завершённой.

## Simplify Review

Отдельный проход верификации после функционального тестирования. Цель: убедиться, что реализация минимально сложна.

- Выполняется после прохождения tests, но до closure gate.
- Паттерны: premature abstractions, глубокая вложенность, дублирование логики, dead code, overengineering.
- Три похожие строки лучше premature abstraction. Абстракция оправдана только когда она реально уменьшает риск или повтор.

## Verification Context Separation

Разные этапы верификации — отдельные проходы:

1. **Функциональная верификация** — tests проходят, acceptance scenarios покрыты
2. **Simplify review** — код минимально сложен
3. **Acceptance test** — end-to-end по `SC-*`

Для small features допустимо в одной сессии, но simplify review не пропускается.

## Project-Specific Conventions

### Подход: TDD для biz/, тесты-после для остального

**biz/ — строгий Red-Green-Refactor:**

1. **Red** — написать падающий тест
2. **Green** — написать минимальный код, чтобы тест прошёл
3. **Refactor** — привести код в порядок

- Не пишем код в biz/ без падающего теста
- Один цикл — одно поведение
- Edge cases — отдельными циклами
- Тест пишется с точки зрения потребителя (вызов usecase), не внутренней реализации

**data/, service/, task/ — тесты-после:**

- Сначала реализация, потом тесты
- Покрытие > 80% обязательно
- Допустимо для: маппинг, адаптеры, инфраструктурный код, интеграционные тесты

**Когда TDD не применяется:**

- Сгенерированный код (ent, proto, wire, mocks)
- Конфигурация и DI-провайдеры
- Прототипирование (но перед мержем — покрыть тестами)

### Что тестируем на каждом слое

**biz/** — основной приоритет:
- Бизнес-логика, usecase-ы
- Моки для интерфейсов репозиториев и внешних сервисов (mockgen)
- Проверяем бизнес-правила, edge cases, ошибки

**data/** — интеграционные тесты:
- Testcontainers с реальным PostgreSQL
- Проверяем что ent-запросы работают корректно, маппинг сущностей, транзакции
- Внешние HTTP-клиенты — httptest.Server с фикстурами ответов

**service/** — минимально:
- Тонкий адаптер, тестируем только маппинг DTO → domain и обратно
- Моки для biz usecase-ов

**task/** — интеграционные тесты:
- Testcontainers для полного flow (воркер → biz → data → DB)
- Проверяем lifecycle (Start/Stop), обработку ошибок, ретраи

### Goroutine Leak Detection

- `go.uber.org/goleak` для контроля утечек горутин
- Добавляем в каждый `t.Run` или тестовую функцию, где нет `t.Parallel()`
- `goleak.IgnoreCurrent()` обязателен

```go
func TestPostUsecase_Save(t *testing.T) {
    defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

    // Arrange
    // ...
}
```

### Test helpers и `t.Helper()`

Любая вспомогательная функция, которая вызывает `t.Fatal` / `t.Error` или принимает `*testing.T` первым аргументом, **обязана** начинаться с `t.Helper()`. Без этого репорт об ошибке укажет на строчку внутри хелпера, а не на вызов в тесте — DX ломается.

```go
func setupTestDB(t *testing.T) *ent.Client {
    t.Helper()
    // ...
}

func requireSavedPost(t *testing.T, repo biz.PostRepo, id int64) biz.Post {
    t.Helper()
    post, ok, err := repo.FindByID(context.Background(), id)
    require.NoError(t, err)
    require.True(t, ok)
    return post
}
```

### Examples как исполняемая документация

Для публичных API пакетов `biz/` и `api/` (proto-сгенерированные сервисы) желательны `ExampleXxx`-функции — они компилируются, проверяются `go test` и показываются в godoc.

- Example **не заменяет** regression-тест, а дополняет его читаемым вызовом с точки зрения потребителя.
- Комментарий `// Output:` обязателен — иначе example не запускается.
- Не использовать examples как место для edge cases — для этого есть обычные тесты.

```go
func ExamplePostUsecase_Save() {
    uc := biz.NewPostUsecase(repo, events)
    _ = uc.Save(context.Background(), biz.Post{Title: "hello", SourceID: 1})
    // Output:
}
```

### Testcontainers — паттерн использования

```go
func setupTestDB(t *testing.T) *ent.Client {
    t.Helper()

    ctx := context.Background()
    container, err := postgres.Run(ctx,
        "postgres:18.3-alpine",
        postgres.WithDatabase("feedium_test"),
    )
    require.NoError(t, err)
    t.Cleanup(func() { container.Terminate(ctx) })

    connStr, err := container.ConnectionString(ctx, "sslmode=disable")
    require.NoError(t, err)

    // накатить миграции goose
    // создать ent client
    // вернуть
}
```

### Моки (mockgen)

- Генерируем моки из интерфейсов biz/
- Файлы моков: строго в дочернем пакете `mock/` относительно исходного файла (`biz/mock/`, `data/mock/`)
- Имя файла мока совпадает с исходным: `source.go` → `mock/source_mock.go`
- Команда в `//go:generate` директиве
- Моки коммитятся

```go
//go:generate mockgen -source=source.go -destination=mock/source_mock.go -package=mock
```

### Структура теста: Arrange-Act-Assert

Все тесты следуют паттерну AAA. Три блока разделяются пустой строкой. Без исключений.

- **Arrange** — подготовка: создание моков, входных данных, настройка ожиданий
- **Act** — один вызов тестируемого метода
- **Assert** — проверка результата и побочных эффектов

```go
func TestPostUsecase_Save_CreatesEventForImmediateSource(t *testing.T) {
    // Arrange
    ctrl := gomock.NewController(t)
    postRepo := mock.NewMockPostRepo(ctrl)
    eventRepo := mock.NewMockSummaryEventRepo(ctrl)
    uc := biz.NewPostUsecase(postRepo, eventRepo)

    post := biz.Post{Title: "test", SourceID: 1}
    postRepo.EXPECT().Save(gomock.Any(), post).Return(biz.Post{ID: 1, Title: "test", SourceID: 1}, nil)
    eventRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil)

    // Act
    err := uc.Save(context.Background(), post)

    // Assert
    assert.NoError(t, err)
}
```

Правила:
- Комментарии `// Arrange`, `// Act`, `// Assert` обязательны
- Act — ровно одно действие. Если хочется два — это два теста
- Assert проверяет только то, что заявлено в имени теста

### Table-Driven Tests

Предпочтительный формат для тестов с множеством входных данных:

```go
func TestPostUsecase_Save(t *testing.T) {
    tests := []struct {
        name    string
        input   SavePostInput
        setup   func(m *mock.MockPostRepo)
        wantErr bool
    }{
        // cases
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // ...
        })
    }
}
```

### Что не тестируем

- Сгенерированный код (ent, protoc, wire)
- main.go и wire injection
- Чистые DTO / value objects без логики

### Именование тестов

- `Test<Type>_<Method>` для unit-тестов: `TestPostUsecase_Save`
- `TestIntegration_<Flow>` для интеграционных: `TestIntegration_CollectAndSummarize`
- Имена кейсов в table-driven: описывают сценарий, не реализацию
  - Хорошо: `"duplicate post is skipped"`
  - Плохо: `"returns ErrDuplicate"`

### CI

- Unit-тесты: `go test ./... -short`
- Integration-тесты: `go test ./... -run Integration`
- Покрытие: `go test -coverprofile=coverage.out ./...`
- Линтер: `golangci-lint run` в отдельном шаге
