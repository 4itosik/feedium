---
doc_kind: engineering
doc_function: convention
purpose: Канонические идиомы Go — читаемость, простота, консистентность. Правила уровня языка, независимые от проектной структуры.
derived_from:
  - ../dna/governance.md
status: active
---

# Go Style

Языковые идиомы Go. Читать **перед** `coding-style.md`: здесь правила уровня языка, там — проектные конвенции (слои kratos, DI, линтер).

Источники: [Effective Go](https://go.dev/doc/effective_go), [Google Go Style Guide](https://google.github.io/styleguide/go/).

---

## Порядок приоритетов при сомнении

1. **Clarity** — смысл и мотивация понятны читателю
2. **Simplicity** — минимально достаточно для задачи
3. **Concision** — высокое соотношение сигнал/шум
4. **Maintainability** — код читается и правится чаще, чем пишется
5. **Consistency** — похожий код выглядит и ведёт себя похоже

Сначала понятно, потом оптимально.

---

## Комментарии

Комментарий объясняет *почему*, а не *что*. Не пересказывать имя функции или операции — это видно из кода. Писать про неочевидные решения, trade-off'ы, скрытые инварианты, edge cases, бизнес- и инфраструктурные ограничения.

```go
// плохо
// increment retries
retries++

// хорошо
// upstream нестабилен — ограничиваем ретраи, чтобы не зациклиться
retries++
```

---

## Doc comments

- Все **экспортируемые** символы (типы, функции, методы, константы, переменные) обязаны иметь doc comment.
- Комментарий начинается с имени символа.
- Один package comment на пакет. Для больших пакетов — отдельный `doc.go`.

```go
// Post represents a content item aggregated from an external source.
type Post struct { ... }

// Save persists a post and emits a summary event for immediate sources.
func (u *PostUsecase) Save(ctx context.Context, p Post) error { ... }
```

Проверять через `go doc ./...` или `pkg.go.dev`-viewer.

---

## Context

- `context.Context` — **первый параметр** любой функции, принимающей контекст. Имя — `ctx`.
- **Никогда** не храни `context.Context` в полях struct. Передавай параметром каждый раз.
- Контекст — для cancellation, deadline, tracing. Не для передачи бизнес-данных.

```go
// плохо — ctx в struct
type Worker struct {
    ctx  context.Context
    repo PostRepo
}

// хорошо — ctx параметром
type Worker struct{ repo PostRepo }
func (w *Worker) Run(ctx context.Context) error { ... }
```

---

## Error handling

### Error strings

Начинаются со строчной буквы (исключения: имя экспортируемого символа, proper noun, акроним). Не заканчиваются пунктуацией. Содержат контекст — имя операции или пакета-источника.

### Оборачивание

Всегда оборачивай ошибку через `%w`. `%w` ставим **в конец** строки — ошибки читаются newest-to-oldest. Для программной проверки — `errors.Is` / `errors.As`.

```go
// плохо
return fmt.Errorf("Failed to save post.")
return fmt.Errorf("%w: save post", err)

// хорошо
return fmt.Errorf("save post: %w", err)
```

### Early return / indent error flow

Обрабатывай ошибку и выходи — нормальный поток идёт без лишнего отступа и без `else`.

```go
// плохо
if err == nil {
    return post, nil
} else {
    return Post{}, fmt.Errorf("save post: %w", err)
}

// хорошо
if err != nil {
    return Post{}, fmt.Errorf("save post: %w", err)
}
return post, nil
```

### Panic

`panic` — только для unrecoverable ситуаций (невозможность инициализации библиотеки, нарушенный инвариант). Для нормальных ошибок — `error`.

---

## Интерфейсы

- **Принимай интерфейсы, возвращай конкретные типы.** Касается всех функций, не только конструкторов.
- Интерфейс определяется **на стороне потребителя**, не реализации.
- Маленькие: 1–3 метода. Большой — признак смешения ответственностей.
- Без префикса `I`. Суффикс `-er` где уместно (`Reader`, `Scorer`).

```go
// плохо — возвращаем интерфейс, потребитель теряет доступ к конкретным методам
func NewPostRepo(db *ent.Client) PostRepo { ... }

// хорошо
func NewPostRepo(db *ent.Client) *postRepo { ... }
```

### Compile-time assertion реализации

Для пары «интерфейс в `biz/`, реализация в `data/`» — compile-time проверка, что реализация не разъедется с интерфейсом. Ловит рассинхрон моментально, без тестов.

```go
// internal/data/post_repo.go
var _ biz.PostRepo = (*postRepo)(nil)
```

---

## Values vs pointers

По умолчанию — значение. Указатель — только когда:

- метод должен мутировать receiver;
- структура содержит не-копируемые поля (`sync.Mutex`, `*ent.Client`, `sync.WaitGroup`);
- нужна nil-семантика (optional значение);
- структура действительно большая (десятки полей, тяжёлые вложения).

### Zero-value design

Проектируй типы так, чтобы **zero value был пригоден к использованию** без инициализации — базовый Go-паттерн (`bytes.Buffer`, `sync.Mutex`).

```go
var buf bytes.Buffer
buf.WriteString("hello") // zero value готов к работе
```

### Dangerous copying

Не копируй структуры с `sync.Mutex`, `sync.WaitGroup`, каналами, пулами, внутренними буферами — копирование ломает синхронизацию. Такие типы оформляй с pointer-receiver методами и передавай только по указателю.

```go
// плохо — копирование Cache ломает mu
func useCopy(c Cache) { ... }

// хорошо
func use(c *Cache) { ... }
```

---

## Slices и maps

**Slices:**
- Пустой локальный slice — `var xs []T` (nil), не `make([]T, 0)`. `append`, `range`, `len` на nil работают идентично пустому.
- В API не полагайся на различие nil vs empty — проверяй через `len(xs) == 0`.

**Maps:**
- Чтение из `nil` map безопасно — возвращает zero value.
- **Запись в `nil` map паникует** — перед первой записью `m = make(map[K]V)`.

```go
// плохо
tags := make([]string, 0)
var m map[string]int
m["k"] = 1 // panic

// хорошо
var tags []string
m := make(map[string]int)
m["k"] = 1
```

---

## Naming

- `MixedCaps` / `mixedCaps`, не `snake_case`. Константы тоже: `MaxRetries`, не `MAX_RETRIES`.
- Пакеты — одно слово, lowercase, без подчёркиваний.
- Getters без префикса `Get`: `obj.Owner()`, не `obj.GetOwner()`. Исключение — если «get» осмыслен в домене (HTTP verbs).
- Длина имени пропорциональна размеру scope: `i`, `n`, `err` в коротком scope — норма.
- Не повторяй имя пакета в именах символов: `post.New`, не `post.NewPost`.

### Receiver names

Короткие (1–2 буквы, аббревиатура типа) и **одинаковые** во всех методах одного типа.

```go
// плохо
func (postUsecase *PostUsecase) Save(...) { ... }
func (this *PostUsecase) Find(...)        { ... }

// хорошо
func (u *PostUsecase) Save(...) { ... }
func (u *PostUsecase) Find(...) { ... }
```

---

## Function signatures

- Сигнатура умещается в одну строку. Если не умещается — рефакторинг (группировка параметров в struct), а не перенос.
- **Именованные возвраты** — только когда улучшают читаемость или нужны для `defer`-модификации (`defer func() { err = wrap(err) }()`).
- **Naked return** запрещён в функциях длиннее нескольких строк — теряется видимость того, что возвращается.

```go
// плохо — naked return
func Process(in Input) (out Output, err error) {
    // 30 строк логики
    return
}
```

---

## Imports

- Группы в порядке: (1) stdlib, (2) сторонние пакеты, (3) пакеты проекта. `goimports` делает автоматически.
- `import .` запрещён.
- Rename импорта — только для устранения конфликта имён.
- Blank imports (`import _ "..."`) допустимы только в `main` или в тестах (driver-регистрация).

---

## init()

Только для инициализации, которую нельзя выразить простым присваиванием. Сложная логика, которая может упасть, — в явный конструктор, возвращающий `error`. Не использовать для регистрации глобальных клиентов, метрик, хендлеров — это скрытое глобальное состояние, ломающее тесты.

```go
// плохо — side effect в init
func init() { metrics.Register(myCollector) }

// хорошо — регистрация в конструкторе
func NewCollector(reg prometheus.Registerer) *Collector {
    c := &Collector{...}
    reg.MustRegister(c)
    return c
}
```

---

## Зависимости: least dependency

Порядок выбора инструмента: (1) средства языка, (2) stdlib, (3) уже используемая в проекте внешняя библиотека, (4) новая внешняя зависимость — **только** после согласования.

> «A little copying is better than a little dependency.» — Rob Pike

Три похожие строки лучше premature abstraction или нового пакета ради DRY.
