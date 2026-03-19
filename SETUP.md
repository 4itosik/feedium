# Базовый сетап для курса AI driven development

Основное взаимодействие с окружающей системой происходит через CLI-утилиты. Поэтому важно иметь рабочий базовый набор инструментов и agent skills, а после установки проверить, что они не только поставились, но и реально настроены под ваши аккаунты.

## Автоматическая установка

Подходит для базовых Linux-дистрибутивов (Ubuntu-tested) и macOS.

```bash
make
```

По умолчанию `make` запускает target `ai`, который последовательно:

1. ставит `mise`, если его еще нет;
2. устанавливает инструменты из `mise.toml`;
3. ставит CLI-агенты;
4. ставит CLI-утилиты для агентов;
5. ставит curated skills для `codex` и `claude-code`;
6. добавляет marketplace и plugins для `claude-code`.

## Проверка установки

После `make` стоит прогнать базовую проверку окружения:

```bash
make check
```

`make check` проверяет:

- обязательный toolchain из локального setup;
- установку agent CLI;
- обязательные проверки авторизации для `claude`, `codex` и `gh`;
- установку вспомогательных CLI для агентов: `playwright-cli`, `tgcli`, `googleworkspace/cli` (`gws`) и `himalaya`;
- опциональные проверки настройки `tgcli`, `googleworkspace/cli`, `himalaya`, curated skills и Claude plugins.

Команда завершается с ошибкой только на обязательных проверках. Для опциональных интеграций она оставляет `WARN`, чтобы было видно, что еще не настроено, но базовая установка уже пригодна к работе.

## Что устанавливается автоматически

### Инструменты через `mise`

Из `mise.toml` ставятся: `direnv`, `gh`, `himalaya`, `gitleaks`, `jq`, `node`, `port-selector`, `ruby`, `tmux`, `yarn`, `zellij`.

### Кодинговые агенты

- `@anthropic-ai/claude-code`
- `@openai/codex`

### CLI-утилиты для агентов

Обязательные:

| Утилита | Для чего | Как проверить |
| --- | --- | --- |
| [@playwright/cli](https://github.com/microsoft/playwright-cli) | Автоматизация работы с сайтами и тестирование веба | Попросить агента зайти на сайт и сделать скриншот |
| [gh](https://github.com/cli/cli) | Работа с GitHub API за пределами `git`: просмотр и создание issue, pull request, projects | Попросить агента посмотреть или создать issue в репозитории |
| [port-selector](https://github.com/dapi/port-selector) | Автоматический выбор свободного порта из диапазона для локальных dev-серверов и e2e при параллельной работе агентов | Выполнить `port-selector` и убедиться, что команда возвращает номер свободного порта |
| [tgcli](https://github.com/dapi/tgcli) | Сбор требований из переписки | Попросить агента найти что-то в личной переписке в Telegram или закинуть пост в Избранное |
| [googleworkspace/cli](https://github.com/googleworkspace/cli) (`gws-docs`, `gws-docs-write`, `gws-drive`, `gws-sheets`) | Сбор требований и формирование проектной документации | Дать агенту ссылку на закрытый Google Doc и попросить прочитать его и дать выдержку |
| [himalaya](https://github.com/pimalaya/himalaya) | Работа с почтой через IMAP/SMTP из CLI | Попросить агента прочитать письмо или найти письмо по теме после настройки почтового аккаунта |

Сами CLI ставятся автоматически и их наличие проверяется через `make check`. При этом доступ к конкретным аккаунтам и системам для `tgcli`, `googleworkspace/cli` и `himalaya` остается опциональной настройкой: если вы ими пока не пользуетесь, `make check` покажет `WARN`, а не завершится ошибкой.

Что не ставится автоматически, но желательно поставить:

Методы трекинга задачи и хранения документов зависят от конкретной компании или сценария и должны ставиться отдельно, если они вам нужны:

| Утилита | Для чего | Как проверить |
| --- | --- | --- |
| [jira-cli](https://github.com/ankitpokhrel/jira-cli) | Работа с Jira | Попросить агента прочитать или создать issue |
| [linear-cli](https://github.com/schpet/linear-cli) | Работа с Linear | Попросить агента прочитать или создать issue |
| [trello-cli](https://github.com/mheap/trello-cli) | Работа с Trello | Попросить агента прочитать или создать карточку |

### Skills для агентов

Эти skills ставятся для `codex` и `claude-code`:

`tgcli`, `playwright-cli`, `prompt-engeneering`, `gws-docs`, `gws-docs-write`, `gws-drive`, `gws-sheets`.

### Plugins для Claude Code

Во время установки добавляются marketplace:

- `dapi/claude-code-marketplace`

И ставятся plugins:

`himalaya@dapi`, `pr-review-fix-loop@dapi`, `spec-reviewer@dapi`, `zellij-workflow@dapi`.

Если нужно ставить Claude plugins из вашего marketplace, это можно переопределить при запуске `make`, например:

```bash
make agents-claude-plugins \
  CLAUDE_PLUGINS_MARKETPLACES=your-org/claude-code-marketplace \
  CLAUDE_MARKETPLACE_NAMES=your-org \
  CLAUDE_PLUGIN_NAMESPACE=your-org \
  CLAUDE_PLUGINS='zellij-workflow@your-org'
```

## Что нужно настроить руками после установки

Установка инструмента, skill или plugin еще не означает, что агент уже сможет работать с конкретной системой. После `make` обычно нужно отдельно пройти авторизацию и базовую настройку:

- `claude`: выполнить `claude auth login`
- `codex`: выполнить `codex login`
- `gh`: выполнить `gh auth login`
- `tgcli`: подключить Telegram-аккаунт
- `googleworkspace/cli`: подключить Google Workspace
- `himalaya`: настроить почтовый аккаунт и доступ к IMAP/SMTP

После настройки аккаунтов стоит повторно запустить `make check`, чтобы убедиться, что обязательные CLI доступны, а статусные команды для аккаунтов проходят успешно.

## direnv

В проекте используется `direnv`:

1. настройте интеграцию с вашей shell: https://direnv.net/docs/hook.html
2. разрешите локальный `.envrc` в корне проекта:

```bash
direnv allow
```
