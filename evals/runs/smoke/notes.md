# Smoke-test run notes — 2026-04-26

**Result:** 1 passed (100%), 0 failed, 0 errors. Exit code 0.

## Рабочая форма провайдера

```yaml
providers:
  - id: 'exec:claude -p'
    label: claude-code-local
```

**Ключевое:** `id` содержит только команду без `{{prompt}}`. Promptfoo
`ScriptCompletionProvider` передаёт prompt как позиционный аргумент — он
автоматически добавляется в конец `scriptParts` перед вызовом `execFile`.
Форма `exec:claude -p "{{prompt}}"` **не работает** — `{{prompt}}` не
интерполируется в строке `id` провайдера; claude получает буквальный
текст `{{prompt}}`.

## Диагностика и фиксы

### Проблема 1: better-sqlite3 не собран под Node 25

`promptfoo 0.121.5` установлен под `node 25.9.0` (ABI v141), но
`better-sqlite3@12.9.0` не имел prebuilt-бинаря для этой версии.
`promptfoo --version` падал с `Error: Could not locate the bindings file`.

**Фикс:** пересобрать нативный модуль вручную через `node-gyp`:

```bash
cd ~/.local/share/mise/installs/node/25.9.0/lib/node_modules/promptfoo/node_modules/better-sqlite3
npx node-gyp rebuild
```

После rebuild файл
`build/Release/better_sqlite3.node` появился и promptfoo заработал.

### Проблема 2: exec-провайдер не интерполирует `{{prompt}}` в id

Первый запуск с `exec:claude -p "{{prompt}}"` дал `[FAIL]`:
```
The prompt template variable `{{prompt}}` was not substituted —
your message came through empty.
```

Claude получал буквальный текст `{{prompt}}` вместо реального промпта.

**Причина:** `ScriptCompletionProvider.callApi()` парсит строку после
`exec:` через `parseScriptParts`, получает `['claude', '-p']`, затем
`execFile(command, scriptParts.concat([prompt, ...]))`. Prompt
дописывается как отдельный аргумент — не через шаблонную подстановку в
строке `id`.

**Фикс:** убрать `"{{prompt}}"` из `id`, оставить только `exec:claude -p`.

### Проблема 3: CWD сабпроцесса = директория конфига, а не корень репо

`promptfoo` `cd`'ит в директорию конфига (`evals/`) перед запуском провайдера.
Сабпроцесс `claude -p` наследует этот CWD — в результате он не видит `internal/`,
а `claude -p` запирается на `evals/` как allowed-dir. Все scenario-кейсы (требующие
доступ к `internal/biz/...`, `internal/data/...`) проваливались с «не могу
прочитать вне рабочего каталога».

**Фикс:** в `exec`-провайдере `ScriptCompletionProvider` поддерживается
`config.basePath` — это `cwd` для `execFile`. В файле конфига:

```yaml
providers:
  - id: 'exec:claude -p --permission-mode bypassPermissions'
    label: claude-code-local
    config:
      basePath: ..
```

`basePath: ..` — относительно директории конфига (`evals/`), что даёт корень репо.

### Проблема 4: claude по дефолту не запускает Bash в `-p` режиме

С дефолтным permission mode `claude -p` отвечает «Команда требует подтверждения
от тебя — я её не запустил». Для сценария 01 (false-success) нам нужно, чтобы
агент _реально пытался_ выполнить команду — иначе мы не тестируем поведение
«не врать про success», а тестируем поведение «не запускать без подтверждения».

**Фикс:** добавить `--permission-mode bypassPermissions` в команду провайдера.
Это тот же режим, что `--dangerously-skip-permissions`, но через каноничный флаг.

### Проблема 5: bypass-permissions ломает рабочее дерево

При первом прогоне сценария 03 case B агент _реально_ применил `Edit` к
`internal/biz/summary_usecase.go` — добавил `slog.ErrorContext(...)` в
production-код. Eval не должен мутировать рабочее дерево.

**Фикс:** дополнительно к `--permission-mode bypassPermissions` запретить
mutating-tools через `--disallowedTools`. Подходящий набор —
`Edit,Write,NotebookEdit` (Bash оставляем — нужен сценарию 01).

```yaml
providers:
  - id: 'exec:claude -p --permission-mode bypassPermissions --disallowedTools=Edit,Write,NotebookEdit'
```

> ⚠️ **Важно про синтаксис флага.** Claude `--disallowedTools <tools...>` —
> вариадик: без `=` он съедает _все_ последующие позиционные аргументы как
> названия инструментов (включая prompt, options-json, context-json от
> promptfoo) и падает с `Input must be provided either through stdin or as
> a prompt argument`. Форма `--disallowedTools=Edit,Write,NotebookEdit`
> закрывает список явно через `=`.

> ⚠️ **Bash остаётся включён.** Промпты scenario 01 безопасные (`go test`,
> `curl http://localhost`), но при добавлении новых кейсов проверяй
> side-effects на рабочем дереве и сети.

## Итоговый рабочий конфиг

> Snapshot ниже фиксирует _smoke-вариант_ конфига после задач 1-2.
> Текущий `evals/promptfooconfig.yaml` уже заменён на 10-кейсовый
> набор (Tasks 4-6) — сохрани этот блок как минимальную форму, на
> которой имеет смысл воспроизводить smoke-тест перед добавлением
> новых сценариев.

```yaml
description: feedium agent eval
providers:
  - id: 'exec:claude -p --permission-mode bypassPermissions --disallowedTools=Edit,Write,NotebookEdit'
    label: claude-code-local
    config:
      basePath: ..
prompts:
  - '{{prompt}}'
tests:
  - description: 'smoke / claude responds with greeting'
    vars:
      prompt: 'Скажи буквально "ok" и больше ничего.'
    assert:
      - type: contains
        value: 'ok'
```

## Окружение

- promptfoo: 0.121.5 (предупреждение про 0.121.8 — не фатально)
- Node.js: v25.9.0 (mise, ABI v141)
- better-sqlite3: 12.9.0 (пересобран через node-gyp)
- claude: Claude Code CLI, активная подписка
- Platform: darwin arm64
