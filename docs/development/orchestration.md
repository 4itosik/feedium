# Orchestration setup (recommended)

Этот документ описывает рекомендуемый сетап разработчика для параллельного запуска задач в отдельных сессиях. Это не правило проекта — другой разработчик может использовать иной мультиплексор/агент или вообще обойтись без оркестрации.

Сетап опирается на готовые инструменты: каркас не пишем сами.

## Цель

Из текущей сессии (терминала или Claude-чата) одной командой:

1. Получить задачу из GitHub issue.
2. Создать ветку с конвенциональным именем и git worktree вне основного чекаута.
3. Открыть отдельную вкладку в zellij с этим worktree.
4. Запустить в ней coding-агента с уже проклассифицированной задачей через `/route-task`.

## Стек

| Компонент | Роль | Источник |
|---|---|---|
| `zellij` | TUI-мультиплексор, видимость параллельных сессий | `brew install zellij` |
| `gh` | CLI GitHub: чтение issue, авторизация | `brew install gh` |
| `git` >= 2.5 | worktree | system |
| `jq` | парсинг JSON в скриптах | `brew install jq` |
| `direnv` | per-worktree окружение через `.envrc` | `brew install direnv` |
| `mise` | toolchain (уже используется проектом) | `brew install mise` |
| `start-issue` | issue → branch → worktree → агент | https://github.com/dapi/start-issue |
| `zellij-tab-status` | переименование таба под issue (опц.) | https://github.com/dapi/zellij-tab-status |
| `task-router` (Claude plugin) | `/route-task <url>` — классификация и запуск feature-dev / subagent-driven-dev / brainstorming | marketplace `dapi` |
| `feature-dev` (Claude plugin) | workflow «discovery → architecture → implementation → review»; зависимость `task-router` | marketplace `claude-code-plugins` (официальный Anthropic) |
| `superpowers` (Claude plugin) | brainstorming, writing-plans, subagent-driven-development и др.; зависимость `task-router` | marketplace `superpowers-marketplace` (obra) |
| `zellij-workflow` (Claude plugin, опц.) | команды `/run-in-new-tab`, `/start-issue-in-new-tab` для триггера из чата | marketplace `dapi` |

## Установка (macOS)

```bash
# 1) base tools
brew install zellij gh jq direnv mise git

# 2) start-issue
git clone https://github.com/dapi/start-issue.git ~/src/start-issue
cd ~/src/start-issue && make install      # → ~/.local/bin/start-issue
# убедиться, что ~/.local/bin есть в PATH

# 3) zellij-tab-status (опциональная интеграция переименования табов)
git clone https://github.com/dapi/zellij-tab-status.git ~/src/zellij-tab-status
cd ~/src/zellij-tab-status && make install

# 4) GitHub auth
gh auth login

# 5) Claude Code плагины (внутри Claude Code)
#    плагины живут в трёх разных маркетплейсах — добавляем те, что ещё не подключены:
/plugin marketplace add anthropics/claude-code         # claude-code-plugins (официальный)
/plugin marketplace add obra/superpowers-marketplace   # superpowers-marketplace
/plugin marketplace add dapi/claude-code-marketplace   # dapi

#    установка:
/plugin install feature-dev@claude-code-plugins
/plugin install superpowers@superpowers-marketplace    # может быть уже установлен из
                                                       # @claude-plugins-official (Anthropic
                                                       # форкает obra-овский) — это нормально
/plugin install task-router@dapi                       # требует feature-dev и superpowers
/plugin install zellij-workflow@dapi                   # опционально
```

Проверка установки:

```bash
which start-issue zellij gh jq direnv
gh auth status
zellij --version
```

## Конфигурация проекта

Сейчас Feedium **не требует** проектного конфига для `start-issue`. По умолчанию он:

- использует агента `claude` (`START_ISSUE_AGENT` → `claude`);
- запускает Claude с командой `/task-router:route-task {ISSUE_URL}`;
- кладёт worktree в `~/worktrees/`;
- запускает `./init.sh` в worktree, если он существует (а он у нас есть).

Если когда-то понадобится override — добавить `.start-issue/prompt.md` или `.start-issue/agent` в корне репо. Пока такие файлы не создаются.

## Использование

### Из терминала

```bash
# по issue номеру
start-issue 42

# по URL
start-issue https://github.com/4itosik/feedium/issues/42

# с другим агентом
start-issue 42 --agent codex

# только подготовить worktree, без запуска агента
start-issue 42 --no-agent

# preview без побочных эффектов
start-issue 42 --dry-run

# sibling-layout вместо ~/worktrees/
start-issue 42 --worktree-dir .. --flat

# другой репо (если запускаешь не из чекаута того проекта)
start-issue 42 --repo 4itosik/feedium
```

**Как `start-issue` определяет, из какого репо брать issue:**

1. Если передан полный URL — берёт его как есть.
2. Если передан только номер — читает `git remote get-url origin` из CWD и собирает URL автоматически. Поэтому запускай команду из чекаута нужного проекта.
3. Если remote не `origin` или нужен кросс-репо запуск — `--repo OWNER/REPO`.

**Имена веток и кириллические заголовки.** По умолчанию `start-issue` строит имя ветки эвристикой из issue title. Для полностью кириллических заголовков получится мусор вроде `feature/issue-3-`. В таких случаях добавляй `--ai` (агент сгенерирует осмысленное имя) или `--branch <name>` явно:

```bash
start-issue 42 --ai
start-issue 42 --branch feature/source-management
```

**Запуск агента с `--dangerously-skip-permissions`.** `start-issue` зовёт Claude в новой вкладке именно с этим флагом — внутри изолированного worktree агент должен работать автономно, иначе будет постоянно спрашивать разрешения. Это сознательный default `dapi/start-issue`, а не баг.

### Из Claude-чата (если стоит `zellij-workflow` плагин)

```
/start-issue-in-new-tab https://github.com/4itosik/feedium/issues/42
/run-in-new-tab Execute plan from memory-bank/features/FT-008/implementation-plan.md
```

Если `zellij-workflow` не стоит — агент в чате может сам вызвать `start-issue 42` через bash.

## Конвенция для FT-XXX (memory-bank features)

`start-issue` принимает только GitHub issues, а не локальные feature-пакеты. Чтобы запускать FT-фичи через тот же поток:

1. Завести GitHub issue с заголовком, начинающимся с `FT-XXX:`, и в теле — ссылку на `memory-bank/features/FT-XXX/feature.md` (или содержимое feature-пакета).
2. Запустить `start-issue <N>` по номеру созданной issue.

Это явная ручная шага. Если она станет рутиной, можно добавить тонкий wrapper, который ищет issue по `FT-XXX in:title` через `gh issue list` и зовёт `start-issue` уже с найденным номером. Сейчас такой wrapper не пишем.

## Где живут worktrees

По умолчанию: `~/worktrees/<branch-with-slashes-as-dirs>/`.

Это вне основного чекаута Feedium, не попадает в `.gitignore` репо, удаляется через `git worktree remove`.

Запись `.worktrees/` в `.gitignore` (commit `2e39069`) — артефакт прежнего подхода. Её можно убрать в отдельной чистке после миграции на этот сетап.

## Перенос на Linux server

```bash
# 1) base tools (Ubuntu/Debian)
sudo apt-get update
sudo apt-get install -y git gh jq direnv

# zellij — нет в apt, ставим из бинарника или cargo
curl -fsSL https://github.com/zellij-org/zellij/releases/latest/download/zellij-x86_64-unknown-linux-musl.tar.gz \
  | tar -xz -C ~/.local/bin/

# mise
curl https://mise.run | sh

# 2) start-issue + zellij-tab-status
git clone https://github.com/dapi/start-issue.git ~/src/start-issue
( cd ~/src/start-issue && make install )

git clone https://github.com/dapi/zellij-tab-status.git ~/src/zellij-tab-status
( cd ~/src/zellij-tab-status && make install )

# 3) GitHub auth (если headless server — gh auth login --with-token < ~/token)
gh auth login

# 4) Claude Code: ставится отдельно (см. официальную инструкцию), плагины:
#    /plugin install task-router@dapi
#    /plugin install zellij-workflow@dapi

# 5) clone репо
git clone git@github.com:4itosik/feedium.git ~/work/feedium
cd ~/work/feedium

# 6) запуск zellij и проверка
zellij
# внутри zellij:
start-issue --dry-run 1
```

Все артефакты — bash-скрипты, единственная неперемещаемая часть — Claude Code сам по себе.

## Триггер из текущей Claude-сессии

Чтобы текущий чат мог запустить задачу в соседней вкладке, варианты:

1. `zellij-workflow` плагин — команды `/start-issue-in-new-tab <url>` и `/run-in-new-tab <prompt>` (рекомендуется).
2. Без плагина — попросить агента: «запусти `start-issue 42` через Bash» — агент шеллит CLI напрямую.

Оба способа работают одинаково; плагин даёт чуть удобнее UX и хуки статусов вкладок (Ready/Working/Needs input).

## Что осознанно не делаем сейчас

- **Scheduler** (слайд 31 курса) — добавим, когда уже будет 2–3 рабочих параллельных задачи и появится потребность. Сейчас YAGNI.
- **Свой роутер вместо `task-router`** — готовый покрывает GitHub issues и Google Docs, нам этого достаточно.
- **Свой start-issue вместо dapi/** — упомянутая утилита покрывает 100% потребностей, разница только в FT-XXX, и та решается конвенцией (см. выше).
- **Декларативные zellij layouts (KDL)** — для одиночной задачи лишние, добавятся при необходимости (например, отдельная панель под `gh pr checks --watch`).

## Troubleshooting

| Симптом | Что проверить |
|---|---|
| `zellij action: not in zellij session` | Запусти команду внутри активной zellij-сессии. |
| `gh: not authenticated` | `gh auth login`. |
| Worktree не создаётся, «branch already exists» | `git worktree list`, `git branch -D <name>` или другой issue-номер. |
| `direnv: untrusted` в новой вкладке | После первого `cd` в новый worktree выполни `direnv allow`. Это зашито в `init.sh`. |
| `task-router` не находит | `/plugin install task-router@dapi`, перезапусти Claude Code. |
| Порт уже занят между worktrees | `port-selector` в `.envrc` уже распределяет; если нет — экспортируй `PORT` вручную. |
