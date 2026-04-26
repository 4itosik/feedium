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

## Итоговый рабочий конфиг

```yaml
description: feedium agent eval — smoke test
providers:
  - id: 'exec:claude -p'
    label: claude-code-local
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
