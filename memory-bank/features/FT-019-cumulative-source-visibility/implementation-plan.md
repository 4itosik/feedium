---
title: "FT-019: Implementation Plan"
doc_kind: feature
doc_function: derived
purpose: "Execution-план FT-019: добавить frontend-only страницу Digests, читающую существующий V1ListSourceSummaries, без изменений backend/proto/схемы."
derived_from:
  - feature.md
status: active
---

# План имплементации

## Цель текущего плана

Реализовать `REQ-01`/`REQ-02` из `feature.md`: страница `/digests`, показывающая агрегированные дайджесты всех cumulative-источников, отсортированные по времени, используя уже существующий и подключённый к HTTP эндпоинт `GET /v1/sources/{source_id}/summaries`.

## Current State / Reference Points

| Path / module | Current role | Why relevant | Reuse / mirror |
| --- | --- | --- | --- |
| `ui/src/api.ts:177` (`listPostSummaries`) | Тонкая обёртка `fetch` над `GET /v1/posts/{post_id}/summaries` | Образец для новой `listSourceSummaries` | Скопировать паттерн apiFetch + query params (см. `listPosts`, строки 162-175, для `page_token`/`page_size`) |
| `ui/src/api.ts:36-43` (`Source`) | Уже содержит `processingMode: string` | Фильтр cumulative-источников не требует нового поля | Использовать как есть, значение `'PROCESSING_MODE_CUMULATIVE'` |
| `ui/src/api.ts:84-95` (`Summary`, `SummariesResponse`) | Уже описывает форму ответа `summaries` (включая `postId?` — у cumulative-записей будет `undefined`) | Переиспользуется для дайджестов один в один, новый тип не нужен | Использовать `Summary[]` напрямую |
| `ui/src/views/PostsView.tsx` | `PostRow`/раскрывающийся summary-блок, паттерн карточки, `formatDate`, `sourceLabel` | Визуальный и структурный эталон для `DigestsView` | Скопировать стиль карточки и `formatDate`; локальная копия `sourceLabel` (как уже задублирована в `FeedView.tsx`/`PostsView.tsx`) |
| `ui/src/App.tsx`, `ui/src/components/Layout.tsx` | Роутинг и nav-бар | Нужно добавить один роут и один пункт меню | Скопировать существующий паттерн `<Route path="posts" .../>` и `{ to: '/posts', label: 'Posts' }` |
| `internal/service/summary/summary.go` | Backend-реализация `V1ListSourceSummaries` | Подтверждает, что эндпоинт уже отдаёт нужные данные (включая `post_id IS NULL`) | Не трогать |
| `api/feedium/summary.proto:69-77` | Контракт `GET /v1/sources/{source_id}/summaries` (`page_size`, `page_token` → `summaries`, `next_page_token`) | Источник правды для query-параметров и формы ответа новой обёртки | Не менять, только читать |

## Test Strategy

| Test surface | Canonical refs | Existing coverage | Planned automated coverage | Required local suites / commands | Required CI suites / jobs | Manual-only gap / justification | Manual-only approval ref |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `ui/src/api.ts` (`listSourceSummaries`) | `REQ-01` | Нет UI-тестов в проекте (`ui/package.json` не содержит test-раннера; `ui/` не имеет ни одного `*.test.*`) | Не добавляется — proto-typed thin wrapper без бизнес-логики, риск минимален | `cd ui && npm run build` (включает `tsc -b`) — типобезопасность как единственный automated guard | то же, что локально (CI прогоняет тот же build) | Полноценный unit/component test harness для `ui/` отсутствует в проекте — заводить vitest/RTL только ради одной тонкой обёртки и одного read-only view нарушает YAGNI этой итерации | `none` — соответствует существующей практике проекта (`PostsView`/`FeedView` тоже без тестов) |
| `ui/src/views/DigestsView.tsx` | `REQ-01`, `REQ-02`, `EC-01`, `SC-01` | Нет | Не добавляется (см. выше) | `npm run build` + ручная проверка в браузере (`CHK-01`) | то же | Ручная проверка страницы — единственный практичный verify для React UI в этом проекте (см. testing-policy.md: «Сценарий зависит от... human оценки UI») | `none` |

## Open Questions / Ambiguities

Открытых вопросов нет: Design (`feature.md`) зафиксировал путь B (отдельная страница, без изменений backend) однозначно, конкретный backend-эндпоинт и форма данных уже существуют и проверены чтением кода.

## Environment Contract

| Area | Contract | Used by | Failure symptom |
| --- | --- | --- | --- |
| setup | `ui/` зависимости установлены (`node_modules` присутствует или `npm ci` отрабатывает в `ui/`) | `STEP-01`..`STEP-03` | `npm run build` падает с ошибкой разрешения модулей |
| test | `cd ui && npm run build` (`tsc -b && vite build`) — эталонная команда verify для типобезопасности и сборки | `CHK-01` | Ненулевой exit code / TS-ошибки в выводе |
| access / network / secrets | Ручная проверка страницы требует доступа к работающему backend (локальный `feedium` с тестовыми данными или прод через `oc`, см. `project_ft019_cumulative_brief` в памяти) с хотя бы одним cumulative-источником, у которого уже есть дайджест | `STEP-03` | Если live backend с cumulative-данными недоступен, `CHK-01` подтверждается только пустым/loading-состоянием — это допустимый, но более слабый evidence; зафиксировать в `EVID-01` какой именно вариант проверен |

## Preconditions

| Precondition ID | Canonical ref | Required state | Used by steps | Blocks start |
| --- | --- | --- | --- | --- |
| `PRE-01` | `CON-01` | `ui/` собирается и проходит lint/build на текущем `main` до начала изменений (baseline зелёный) | `STEP-01` | yes |

## Workstreams

| Workstream | Implements | Result | Owner | Dependencies |
| --- | --- | --- | --- | --- |
| `WS-1` | `REQ-01`, `REQ-02`, `CON-01` | Рабочая страница `/digests` в `ui/`, доступная из nav | agent | `PRE-01` |

## Approval Gates

Approval gates не требуются: изменения локальны для `ui/`, не затрагивают backend/схему/прод-данные и полностью обратимы через git revert.

## Порядок работ

| Step ID | Actor | Implements | Goal | Touchpoints | Artifact | Verifies | Evidence IDs | Check command / procedure | Blocked by | Needs approval | Escalate if |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `STEP-01` | agent | `REQ-01` | Добавить `listSourceSummaries()` в `ui/src/api.ts` по образцу `listPostSummaries`/`listPosts` | `ui/src/api.ts` | Новая экспортируемая функция, типизированная через существующие `Summary`/`SummariesResponse` | `CHK-01` (частично, через build) | `EVID-01` | `cd ui && npx tsc -b --noEmit` проходит без ошибок | `PRE-01` | `none` | TS не может вывести тип ответа — значит контракт `SummariesResponse` не подходит, остановиться и свериться с `summary.proto` |
| `STEP-02` | agent | `REQ-01`, `REQ-02`, `CON-01` | Создать `ui/src/views/DigestsView.tsx`: фильтр cumulative-источников, параллельный fetch дайджестов, merge+sort, карточки, loading/error/empty состояния | `ui/src/views/DigestsView.tsx` | Новый компонент, экспортируемый как default | `CHK-01` (частично, через build) | `EVID-01` | `cd ui && npx tsc -b --noEmit` проходит без ошибок | `STEP-01` | `none` | Компонент не компилируется из-за расхождения типов `Source`/`Summary` — свериться с `ui/src/api.ts` |
| `STEP-03` | agent | `REQ-01`, `CON-01` | Подключить роут `/digests` (`ui/src/App.tsx`) и пункт навигации `Digests` (`ui/src/components/Layout.tsx`) | `ui/src/App.tsx`, `ui/src/components/Layout.tsx` | Страница доступна из общей навигации | `CHK-01` (частично) | `EVID-01` | `cd ui && npm run build` (полный `tsc -b && vite build`) проходит | `STEP-02` | `none` | Build падает — откатить роут/nav правки, не трогая `STEP-01`/`STEP-02` |
| `STEP-04` | agent | `EC-01`, `SC-01` | Финальная verify: полный `npm run build` + ручная проверка `/digests` в браузере на live backend с cumulative-данными; сохранить evidence | — (verify-only) | `artifacts/ft-019/verify/chk-01/` с build-логом и подтверждением страницы | `CHK-01` | `EVID-01` | См. `CHK-01` в `feature.md` | `STEP-03` | `none` | Live backend с cumulative-источниками недоступен — задокументировать как известное ограничение verify, не блокировать `delivery_status: done`, если build зелёный и код-ревью кода пройден |

## Parallelizable Work

- `PAR-01` Нет — `STEP-01`→`STEP-04` строго последовательны (каждый следующий шаг компилирует/использует артефакт предыдущего, общий write-surface — `ui/`).

## Checkpoints

| Checkpoint ID | Refs | Condition | Evidence IDs |
| --- | --- | --- | --- |
| `CP-01` | `STEP-03`, `CHK-01` | `npm run build` зелёный после всех правок | `EVID-01` |
| `CP-02` | `STEP-04`, `CHK-01` | Страница `/digests` вручную подтверждена (с реальными данными или задокументированным known-limitation, если backend недоступен) | `EVID-01` |

## Execution Risks

| Risk ID | Risk | Impact | Mitigation | Trigger |
| --- | --- | --- | --- | --- |
| `ER-01` | На момент ручной проверки ни у одного cumulative-источника ещё нет готового summary (cron `interval: 3600s` на проде) | `EC-01` подтверждается только через пустое состояние, не через реальные данные | Использовать прод (`oc`), где по памяти `project_ft019_cumulative_brief` уже есть 61 готовый source-level summary — данные точно есть | Пустой список на `/digests` при проверке локально без прод-данных |

## Stop Conditions / Fallback

| Stop ID | Related refs | Trigger | Immediate action | Safe fallback state |
| --- | --- | --- | --- | --- |
| `STOP-01` | `CON-01` | `npm run build` не проходит после любого шага и причина не очевидна за 1 итерацию правки | Откатить последний шаг (`git checkout -- <file>` для незакоммиченных правок этого шага) | Последний зелёный коммит (после `STEP-01`/`STEP-02`/`STEP-03` соответственно — шаги коммитятся по отдельности) |

## Готово для приемки

`delivery_status: done` достижим, когда: `STEP-01`..`STEP-04` выполнены, `CP-01` и `CP-02` подтверждены, `EVID-01` сохранён в `artifacts/ft-019/verify/chk-01/`, и `Verify` секция `feature.md` (`EC-01`, `SC-01`, `CHK-01`) закрыта.
