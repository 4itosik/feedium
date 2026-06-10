---
title: "FT-011: Implementation Plan"
doc_kind: feature
doc_function: derived
purpose: "Execution-план реализации FT-011 React UI. Фиксирует discovery context, workstreams, шаги, риски и test strategy без переопределения canonical feature-фактов."
derived_from:
  - feature.md
status: archived
---

# План имплементации FT-011: React UI

## Цель текущего плана

Создать React SPA (Vite + TypeScript + Tailwind CSS) с четырьмя views (Feed, Sources, Interests, Posts), встроить её в Go-бинарник через `go:embed`, настроить dev proxy. Delivery outcome: `go build` → бинарник отдаёт SPA; все четыре views работают с живым API.

## Current State / Reference Points

| Path / module | Current role | Why relevant | Reuse / mirror |
| --- | --- | --- | --- |
| `internal/server/http.go` | Регистрирует kratos HTTP routes | Нужно добавить static file handler после API routes | Паттерн `srv.Handle(...)` |
| `api/feedium/feed.proto` | `GET /v1/feed` contract | Основной endpoint для Feed view | `FeedItem{post_id,source_id,author,text,published_at,score}` |
| `api/feedium/source.proto` | Source CRUD contract | Sources view | `Source{id,type,processing_mode,config}` |
| `api/feedium/interest.proto` | Interest CRUD contract | Interests view | `Interest{id,topic}` |
| `api/feedium/post.proto` | Post list + get contract | Posts view | `Post{id,source,text,published_at,author}` |
| `api/feedium/summary.proto` | `GET /v1/posts/{id}/summaries` | Post summaries | `Summary{id,text,word_count}` |
| `Makefile` | Build targets | Добавить `ui-build` и обновить `build` | Паттерн существующих phony targets |

## Test Strategy

| Test surface | Canonical refs | Existing coverage | Planned automated coverage | Required local suites | Required CI suites | Manual-only gap | Approval ref |
| --- | --- | --- | --- | --- | --- | --- | --- |
| go:embed + static handler | `REQ-05`, `SC-05`, `CHK-05` | Нет | `CHK-05`: shell-команда (build + curl) | `make ui-build && go build` | CI job: `make ui-build && go build` | — | `none` |
| Feed view render | `REQ-01`, `SC-01`, `CHK-01` | Нет | Нет (UI без test framework) | Ручная проверка CHK-01 | — | Нет Vitest в MVP | `AG-01` |
| Sources CRUD | `REQ-02`, `SC-02`, `CHK-02` | Нет | Нет | Ручная проверка CHK-02 | — | Нет Vitest в MVP | `AG-01` |
| Interests CRUD | `REQ-03`, `SC-03`, `CHK-03` | Нет | Нет | Ручная проверка CHK-03 | — | Нет Vitest в MVP | `AG-01` |
| Posts + summaries | `REQ-04`, `SC-04`, `CHK-04` | Нет | Нет | Ручная проверка CHK-04 | — | Нет Vitest в MVP | `AG-01` |

## Open Questions / Ambiguities

| OQ ID | Question | Why unresolved | Blocks | Default / escalation |
| --- | --- | --- | --- | --- |
| `OQ-01` | FeedItem не содержит `summary` поле в proto — показывать ли саммари в Feed view? | summary — отдельный endpoint | `STEP-17` | По умолчанию Feed view не показывает саммари; Posts view показывает через отдельный запрос |
| `OQ-02` | Нужна ли пагинация в Feed view UI или достаточно `page_size`=50? | UX решение | `STEP-17` | По умолчанию `page_size`=50, scroll вниз загружает следующую страницу (infinite scroll) — если сложно, просто кнопка "Загрузить ещё" |

## Environment Contract

| Area | Contract | Used by | Failure symptom |
| --- | --- | --- | --- |
| Node.js | ≥ 20, npm ≥ 10 | `STEP-01..07` | `npm install` или `npm run build` падает |
| Go backend | Go backend запущен на `:8000` при dev | `STEP-18` (CHK-06) | Vite proxy возвращает 502 |
| `ui/dist/` | Должен существовать до `go build` | `STEP-20`, `CHK-05` | compile error `//go:embed ui/dist` |
| PostgreSQL | Запущена (docker-compose) при ручном verify | `CHK-01..04` | API возвращает 500 |

## Preconditions

| PRE ID | Canonical ref | Required state | Used by steps | Blocks start |
| --- | --- | --- | --- | --- |
| `PRE-01` | `ASM-02` | Node.js ≥ 20, npm доступны в shell | `STEP-01` | yes |
| `PRE-02` | `CON-04` | API endpoints реализованы (FT-003..010 done) | `STEP-14..18` | no (scaffolding можно без API) |
| `PRE-03` | `CON-01` | Go проект собирается (`go build` зелёный) | `STEP-19..20` | no |

## Workstreams

| Workstream | Implements | Result | Dependencies |
| --- | --- | --- | --- |
| `WS-1` Frontend scaffold | `REQ-06`, `REQ-07` | `ui/` с Vite, React, TS, Tailwind; `npm run build` отдаёт `ui/dist/` | `PRE-01` |
| `WS-2` API client | `CTR-01..07` | `ui/src/api.ts` — typed fetch-функции для всех используемых endpoints | `WS-1` |
| `WS-3` Views | `REQ-01..04` | FeedView, SourcesView, InterestsView, PostsView + роутинг | `WS-2` |
| `WS-4` go:embed | `REQ-05`, `REQ-07` | `internal/server/static.go`, обновлённый `http.go`, `Makefile` | `WS-1` |

## Approval Gates

| AG ID | Trigger | Applies to | Why approval required | Approver |
| --- | --- | --- | --- | --- |
| `AG-01` | UI views без automated tests в MVP | `WS-3` | Manual-only gap требует явного подтверждения | human: ручная проверка CHK-01..04 после STEP-18 |

## Порядок работ

| Step ID | Actor | Implements | Goal | Touchpoints | Artifact | Verifies | Evidence IDs | Check | Blocked by | Needs approval | Escalate if |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `STEP-01` | agent | `REQ-06` | Создать Vite проект в `ui/` | `ui/package.json`, `ui/vite.config.ts`, `ui/tsconfig.json`, `ui/index.html` | `ui/` scaffold | — | — | `cd ui && npm run build` не падает | `PRE-01` | none | npm/node версия несовместима |
| `STEP-02` | agent | `CON-03` | Установить и настроить Tailwind CSS v4 | `ui/src/index.css`, `ui/vite.config.ts` | Tailwind работает | — | — | Класс `bg-blue-500` применяется в dev | `STEP-01` | none | Tailwind v4 несовместим с Vite |
| `STEP-03` | agent | `REQ-06` | Добавить React Router DOM v6 | `ui/src/main.tsx`, `ui/src/App.tsx` | Router с 4 routes | — | — | Навигация между `/`, `/sources`, `/interests`, `/posts` | `STEP-01` | none | — |
| `STEP-04` | agent | `REQ-06` | Добавить Vite proxy `/v1/*` → backend `:8000` | `ui/vite.config.ts` | proxy config | `EC-07` | `EVID-06` | `CHK-06` | `STEP-01` | none | proxy 502 |
| `STEP-05` | agent | `CTR-01..07` | Создать API client `api/src/api.ts` | `ui/src/api.ts` | typed fetch functions | — | — | TypeScript compile без ошибок | `STEP-01` | none | Типы не совпадают с proto |
| `STEP-06` | agent | `REQ-01` | Создать FeedView | `ui/src/views/FeedView.tsx` | Feed page | `EC-03` | `EVID-01` | `CHK-01` | `STEP-03`, `STEP-05` | none | API возвращает неожиданный shape |
| `STEP-07` | agent | `REQ-02` | Создать SourcesView | `ui/src/views/SourcesView.tsx` | Sources page | `EC-04` | `EVID-02` | `CHK-02` | `STEP-03`, `STEP-05` | none | — |
| `STEP-08` | agent | `REQ-03` | Создать InterestsView | `ui/src/views/InterestsView.tsx` | Interests page | `EC-05` | `EVID-03` | `CHK-03` | `STEP-03`, `STEP-05` | none | — |
| `STEP-09` | agent | `REQ-04` | Создать PostsView | `ui/src/views/PostsView.tsx` | Posts page | `EC-06` | `EVID-04` | `CHK-04` | `STEP-03`, `STEP-05` | none | — |
| `STEP-10` | agent | `REQ-05` | Создать `internal/server/static.go` с go:embed | `internal/server/static.go` | static handler | `EC-01`, `EC-02` | `EVID-05` | `CHK-05` | `WS-1` done | none | go:embed compile error |
| `STEP-11` | agent | `REQ-05` | Обновить `internal/server/http.go` — добавить static handler | `internal/server/http.go` | обновлённый http.go | `EC-02` | `EVID-05` | `CHK-05` | `STEP-10` | none | API routes перекрыты |
| `STEP-12` | agent | `REQ-07` | Обновить `Makefile` — `ui-build`, зависимость в `build` | `Makefile` | обновлённый Makefile | — | — | `make ui-build` работает | `WS-1` done | none | — |
| `STEP-13` | agent | `REQ-05`, `REQ-07` | Запустить `make ui-build && go build` и убедиться, что компилируется | shell | зелёный build | `EC-01` | `EVID-05` | `CHK-05` | `STEP-10..12` | none | compile error |
| `STEP-14` | human | `REQ-01..04` | Запустить сервер, провести ручную проверку CHK-01..04 | browser | скриншоты EVID-01..04 | `EC-03..06` | `EVID-01..04` | `CHK-01..04` | `STEP-13` | `AG-01` | Любой view не работает |

## Parallelizable Work

- `PAR-01` STEP-06, STEP-07, STEP-08, STEP-09 можно писать параллельно — разные файлы без shared write surface.
- `PAR-02` STEP-10..12 (go:embed) можно начинать параллельно с STEP-06..09, так как затрагивают разные поверхности (`internal/server/` vs `ui/src/`).

## Checkpoints

| CP ID | Refs | Condition | Evidence IDs |
| --- | --- | --- | --- |
| `CP-01` | `STEP-01..04` | `npm run build` в `ui/` успешен, dist выдаёт файлы | — |
| `CP-02` | `STEP-05..09` | TypeScript compile без ошибок, все views рендерятся в dev | `EVID-01..04` (dev screenshots) |
| `CP-03` | `STEP-10..13` | `go build` успешен, `curl localhost:8000/` возвращает HTML | `EVID-05` |
| `CP-04` | `STEP-14` | Все CHK-01..04 пройдены вручную | `EVID-01..04` |

## Execution Risks

| ER ID | Risk | Impact | Mitigation | Trigger |
| --- | --- | --- | --- | --- |
| `ER-01` | Tailwind CSS v4 breaking changes с Vite | CSS не применяется | Использовать Tailwind v3 если v4 несовместим | `npm run build` ошибки с PostCSS |
| `ER-02` | `go:embed` не поддерживает пустой `ui/dist/` | compile error в CI | `make build` зависит от `make ui-build` | CI падает на `go build` без `ui-build` |
| `ER-03` | kratos HTTP server не поддерживает `http.FileServer` pattern | static serving не работает | Использовать `srv.HandlePrefix` или `http.StripPrefix` | 404 на static assets |

## Stop Conditions / Fallback

| STOP ID | Related refs | Trigger | Immediate action | Safe fallback state |
| --- | --- | --- | --- | --- |
| `STOP-01` | `FM-02` | API routes перекрыты static handler → 404 на `/v1/*` | Перенести static handler после API registration | API работает, static — нет |
| `STOP-02` | `ER-01` | Tailwind v4 несовместим | Downgrade до Tailwind v3 | Работающий CSS |

## Готово для приёмки

- `go build` успешен при наличии `ui/dist/` (`EC-01`)
- `curl http://localhost:8000/` возвращает HTML с `<div id="root">` (`EC-02`)
- Все четыре views работают и получают данные от API (`EC-03..EC-06`)
- Dev mode с Vite proxy работает (`EC-07`)
- EVID-01..06 заполнены
- feature.md → `delivery_status: done`
- implementation-plan.md → `status: archived`
