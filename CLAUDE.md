Прочитай главный индекс Memory Bank: [memory-bank/index.md](memory-bank/index.md)

## Tool-use rules (lessons learned)

- **Не параллелить Bash-вызовы, которые мутируют файловую систему или делят CWD** (`mockgen`, `go generate`, `go build` в подпапке, `git stash`, генерация артефактов). Параллельно запускай только независимые read-only пробы (`git status`, `git diff`, `git log`, `Read`, `Grep`).
  - Почему: при ошибке одного параллельного звена все остальные отменяются с `Cancelled: parallel tool call Bash(...)`, и работа теряется.

- **Не полагаться на shell-globs в zsh** (`internal/task/*_test.go`, `*.go`). Для перечисления файлов используй инструмент `Glob` или `find`.
  - Почему: zsh без `nullglob` фейлит команду целиком — `(eval):1: no matches found: ...`.

- **Перед вызовом `Skill` сверяться со списком из system-reminder; не выдумывать неймспейсы.**
  - Почему: повторяется `Unknown skill: superpowers:code-reviewer` — корректные имена в этом проекте: `code-reviewer` (без префикса) или `superpowers:requesting-code-review`.

- **Если `Edit` упал с `String to replace not found` — не ретраить тот же `old_string`. Сначала `Read` нужный диапазон заново и подобрать уникальный якорь по фактическому содержимому.**
  - Почему: содержимое уже дрифтнуло (линтер, автоформат, чужая правка), и повтор того же `old_string` гарантированно даст ту же ошибку.
