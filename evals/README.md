# evals — promptfoo eval suite for feedium agent behavior

Eval-инфраструктура для проверки поведения `claude -p` (Claude Code с
полным стеком AGENTS.md + memory-bank + skills) против сценариев,
найденных в реальных сессиях через ccbox.

## Что внутри

- `spec.md` — что и зачем мы измеряем. Прочитать первым.
- `plan.md` — implementation plan (как этот каталог построен).
- `promptfooconfig.yaml` — главный конфиг: 1 provider + 10 tests.
- `scenarios/` — описания 3 поведенческих сценариев и evidence.
- `runs/` — артефакты прогонов; `output.{json,html}` в gitignore,
  `summary.md` коммитится вручную как evidence фиксов.
- `runs/smoke/notes.md` — рабочая форма exec-провайдера и обнаруженные
  гочи (basePath, permission mode, disallowed tools, синтаксис флага).

## Установка

```bash
npm i -g promptfoo
# или brew install promptfoo
```

`claude` CLI с активной подпиской — required.

## Как запустить

**Из корня репозитория** (CWD критичен — `claude -p` подгружает
AGENTS.md + memory-bank только из CWD; провайдер дополнительно
выставляет `basePath: ..`, чтобы сабпроцесс стартовал из корня):

```bash
cd <repo-root>
promptfoo eval -c evals/promptfooconfig.yaml \
    --output evals/runs/$(date +%Y-%m-%dT%H-%M-%S)/output.json
```

Открыть HTML-отчёт:

```bash
promptfoo view evals/runs/<ts>/output.json
```

Только один сценарий:

```bash
promptfoo eval -c evals/promptfooconfig.yaml --filter-pattern '^01:'
```

## Provider-конфиг

Провайдер вызывает `claude -p` со следующими флагами:

- `--permission-mode bypassPermissions` — иначе агент в `-p` режиме
  отказывается запускать Bash («команда требует подтверждения») и
  сценарий 01 (false-success) не тестируется по существу.
- `--disallowedTools=Edit,Write,NotebookEdit` — запрещает мутирующие
  tools, чтобы prompt вроде «измени файл X» не дрейфил рабочее дерево.
  Bash остаётся доступен (нужен сценарию 01).
- `config.basePath: ..` в YAML провайдера — без этого promptfoo
  стартует сабпроцесс из директории конфига (`evals/`), и claude
  ограничен только evals/, не видит `internal/`.

Подробности и история фиксов — в `runs/smoke/notes.md`.

## Tuning loop

1. Прогнал → есть failed cases.
2. Понял причину: дыра в документации (нет правила) или агент не
   читает имеющееся правило.
3. Если первое — добавить правило в `AGENTS.md` или
   `memory-bank/engineering/*.md`. Закоммитить.
4. Если второе — добавить триггер на чтение в `AGENTS.md`, или
   принять, что fix требует структурного изменения (вне scope).
5. Перезапустить, сохранить новый run в `runs/<ts>-after-<change>/`.
6. Скопировать `summary.md` обоих ранов в коммит как evidence.

## Как добавить новый кейс

1. Если новый сценарий — создать `scenarios/0N-<name>.md`.
2. В `promptfooconfig.yaml` в секцию `tests:` добавить запись с
   `description: '0N:<name> / case X: <short>'`.
3. Assertions — детерминистические. В promptfoo 0.121.5 `regex` есть,
   но `not-regex` и `not-contains-regex` — нет. Для негативных
   regex-проверок используем `type: javascript` с
   `value: '!/pattern/i.test(output)'`. LLM-as-judge не используем
   (см. spec, раздел «Что НЕ делаем»).
4. Прогнать `--filter-pattern '^0N:'`, убедиться, что кейс хотя бы
   корректно отрабатывает (pass или fail — дело tuning-а).
