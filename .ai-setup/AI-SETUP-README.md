# Setup/bootstrap-репозиторий для курса AI driven development

![AI setup meme](https://i.programmerhumor.io/2025/07/9d40116f39da7b5d83f41899584b86c9c21d5c750c6330ff88d46532ecfb8d59.png)

Этот репозиторий создан для [курса Данила Письменного по ai-driven-development на Thinknetica](https://thinknetica.com/ai/ai_swe_course?utm_source=telegram&utm_medium=post&utm_campaign=ai_swe_course&utm_content=dpismenny).

В первую очередь это базовый setup/bootstrap-репозиторий для окружения и агентских инструментов. Во вторую очередь это шаблон, из которого можно стартовать учебный проект.

## Цели этого репозитория

1. Дать всем участникам общий базовый ai-setup по инструментам.
2. Дать стартовую заготовку для учебного проекта, в котором происходит тренировка заданий.

## Как пользоваться этим репозиторием

1. Выполните инструкции в [SETUP.md](SETUP.md).
2. После установки проверьте окружение командой `make check`.
3. Если вы создаете на основе этого репозитория свой учебный проект, замените этот `README.md` описанием проекта и закоммитьте изменения.

## Полезные команды

- `make check-context` — проверка baseline-контекста для Claude Code и Codex.
- `make codex-context` — сводка по текущей/последней Codex-сессии: окно контекста, live token usage, skills, subagents и грубая оценка baseline.
