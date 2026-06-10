---
title: "FT-011: React UI"
doc_kind: feature
doc_function: canonical
purpose: "Canonical feature-документ FT-011: React SPA встроенная в Go-бинарник через go:embed — дашборд для потребления ленты и управления источниками, интересами и постами."
derived_from:
  - ../../domain/problem.md
  - ../../prd/PRD-001-mvp.md
  - ../../domain/architecture.md
  - ../../engineering/api-contracts.md
status: active
delivery_status: done
---

# FT-011: React UI

Выбран `large.md`. Rationale: фича пронизывает несколько поверхностей (новый `ui/` фронтенд, `internal/server/` статика, `Makefile` build-chain), вводит новый config-контракт (go:embed path, Vite proxy), требует явных `CON-*` / `CTR-*` / `FM-*`, нескольких acceptance scenarios (по одному на каждый view) и нескольких `CHK-*` / `EVID-*`.

## What

### Problem

Взаимодействие с Feedium сегодня возможно только через HTTP API и CLI-инструменты. Отсутствует визуальный интерфейс для потребления отранжированной ленты — главная цель продукта (G-01, G-02, G-03 из [PRD-001](../../prd/PRD-001-mvp.md#goals)). Без UI пользователь не может быстро просматривать ленту, видеть score постов, управлять источниками и интересами через браузер.

Feature-specific delta: общий контекст consumption workflow зафиксирован в [problem.md → WF-03](../../domain/problem.md#core-workflows). FT-011 закрывает конкретный отсутствующий шаг — web UI как основной интерфейс потребления.

### Outcome

| Metric ID | Metric | Baseline | Target | Measurement method |
| --- | --- | --- | --- | --- |
| `MET-01` | Лента открывается и показывает посты | Нет UI | Feed view доступен по `/` и отображает посты из GET /v1/feed | Ручная проверка: открыть браузер, убедиться в наличии списка постов |
| `MET-02` | Управление источниками из браузера | Нет UI | Sources view позволяет просматривать, создавать и удалять источники | Ручная проверка: добавить и удалить источник через UI |
| `MET-03` | Управление интересами из браузера | Нет UI | Interests view позволяет просматривать, создавать и удалять интересы | Ручная проверка: добавить и удалить интерес через UI |
| `MET-04` | SPA встроена в бинарник | Отдельный сервер | `go build` → бинарник отдаёт SPA без внешних зависимостей | `go build && ./feedium` → GET / возвращает HTML |

### Scope

- `REQ-01` **Feed view**: отображает отранжированную ленту постов из `GET /v1/feed`; показывает текст поста, автора, дату публикации, score; поддерживает фильтрацию по min_score.
- `REQ-02` **Sources view**: список источников из `GET /v1/sources`; форма создания нового источника (типы: Telegram channel, Telegram group, RSS, HTML); удаление источника через `DELETE /v1/sources/{id}`.
- `REQ-03` **Interests view**: список интересов из `GET /v1/interests`; форма добавления нового интереса (topic string); удаление через `DELETE /v1/interests/{id}`.
- `REQ-04` **Posts view**: список постов из `GET /v1/posts` с пагинацией; при клике на пост — просмотр саммари через `GET /v1/posts/{id}/summaries`.
- `REQ-05` **go:embed**: собранные статические файлы SPA встроены в Go-бинарник (`ui/dist`); Go HTTP server отдаёт их по `/` и `/*`; API-роуты (`/v1/*`) не перекрываются.
- `REQ-06` **Dev mode**: Vite dev server проксирует `/v1/*` на Go backend (`:8000`), позволяя вести разработку с hot-reload без пересборки бинарника.
- `REQ-07` **Build chain**: `make ui-build` запускает `npm run build` в `ui/`, результат попадает в `ui/dist/`, который подхватывает go:embed.

### Non-Scope

- `NS-01` Аутентификация и авторизация — solo-use без auth (PCON-01).
- `NS-02` Редактирование (update) источников и интересов — только create/delete в MVP.
- `NS-03` Создание и редактирование постов из UI.
- `NS-04` OpenClaw-бот и его интеграция с UI.
- `NS-05` Аналитика, графики потребления, статистика.
- `NS-06` Push/realtime-уведомления, WebSocket.
- `NS-07` Мобильный responsive дизайн (не исключён, но не является обязательным требованием MVP).
- `NS-08` Dark mode.
- `NS-09` Суммаризация источника (`POST /v1/sources/{id}/summarize`) через UI — post-MVP.

### Constraints / Assumptions

- `CON-01` SPA встраивается в Go-бинарник через `go:embed` — статика не может быть внешним сервисом (PCON-06, [architecture.md](../../domain/architecture.md)).
- `CON-02` UI минимальный; скорость отображения и потребления ленты важнее визуального богатства (PCON-06).
- `CON-03` Стек: Vite + React + TypeScript + Tailwind CSS. Вне этого стека ничего не вводится.
- `CON-04` Backend API не меняется в рамках FT-011 — UI потребляет существующие proto-endpoints.
- `ASM-01` API доступно по `http://localhost:8000` при локальной разработке; Vite proxy настраивается на этот адрес.
- `ASM-02` Node.js ≥ 20 и npm доступны в dev-окружении.

## How

### Solution

React SPA, собранная Vite, встраивается в Go-бинарник через `go:embed`. В production режиме Go server отдаёт `ui/dist/` как статические файлы; API-роуты `/v1/*` регистрируются раньше и не перекрываются SPA fallback-ом. В dev режиме запускается Vite dev server с proxy `/v1/*` → Go backend. Tailwind CSS обеспечивает минимальную стилизацию без лишних зависимостей. Навигация через React Router DOM.

**Главный trade-off:** `go:embed` требует наличия собранного `ui/dist/` до `go build`. Решение — отдельный `make ui-build` target; в dev режиме используется Vite сервер напрямую.

### Change Surface

| Surface | Type | Why it changes |
| --- | --- | --- |
| `ui/` | code (new) | Новый Vite + React + TypeScript проект |
| `ui/src/` | code (new) | Компоненты и страницы SPA |
| `internal/server/static.go` | code (new) | go:embed + HTTP handler для статики |
| `internal/server/http.go` | code (modified) | Подключение static handler к kratos HTTP server |
| `internal/server/server.go` | code (modified) | Добавление static handler в Wire-граф, если нужно |
| `Makefile` | config (modified) | Добавление `ui-build`, `ui-dev` targets |
| `memory-bank/features/index.md` | doc (modified) | Регистрация FT-011 |

### Flow

1. Пользователь открывает `http://localhost:8000/` в браузере.
2. Go server находит запрос на `/`, не совпадающий с `/v1/*`, и отдаёт `index.html` из embedded `ui/dist/`.
3. Браузер загружает JS/CSS bundle, React монтируется.
4. React Router рендерит нужный view в зависимости от URL.
5. View делает fetch на соответствующий `/v1/*` endpoint, Go возвращает JSON.
6. UI рендерит данные.

### Contracts

| Contract ID | Input / Output | Producer / Consumer | Notes |
| --- | --- | --- | --- |
| `CTR-01` | `GET /v1/feed?min_score=&from=&to=` → `{items: FeedItem[], next_page_token}` | Go FeedService / React FeedView | Основной endpoint для Feed view |
| `CTR-02` | `GET /v1/sources` → `{items: Source[]}` | Go SourceService / React SourcesView | Список источников |
| `CTR-03` | `POST /v1/sources` + `DELETE /v1/sources/{id}` | React SourcesView / Go SourceService | Создание и удаление |
| `CTR-04` | `GET /v1/interests` → `{items: Interest[]}` | Go InterestService / React InterestsView | Список интересов |
| `CTR-05` | `POST /v1/interests` + `DELETE /v1/interests/{id}` | React InterestsView / Go InterestService | Создание и удаление |
| `CTR-06` | `GET /v1/posts` → `{items: Post[]}` | Go PostService / React PostsView | Список постов |
| `CTR-07` | `GET /v1/posts/{id}/summaries` → `{summaries: Summary[]}` | Go SummaryService / React PostsView | Саммари поста |
| `CTR-08` | `GET /*` (non-API) → `index.html` | Go static handler / browser | SPA fallback |
| `CTR-09` | `ui/dist/` → Go `//go:embed ui/dist` | Vite build / Go binary | Build artifact contract |

### Failure Modes

- `FM-01` `ui/dist/` отсутствует при `go build` → compile error от `//go:embed`. Mitigation: `make build` зависит от `ui-build`; CI явно запускает `make ui-build` перед `make build`.
- `FM-02` SPA fallback перехватывает `/v1/*` роуты → API недоступно. Mitigation: регистрировать API-роуты до static handler; fallback работает только для не-API-путей.
- `FM-03` CORS в dev режиме — Vite proxy снимает проблему для dev; в production всё на одном origin.
- `FM-04` Бесконечная загрузка при пустой БД — UI должен корректно отображать пустые состояния без паники.
- `FM-05` API возвращает ошибку — UI показывает error state, не падает.

## Verify

### Exit Criteria

- `EC-01` `go build` успешен при наличии `ui/dist/`.
- `EC-02` `GET /` на запущенном сервере возвращает HTML SPA.
- `EC-03` Feed view отображает данные из `/v1/feed`.
- `EC-04` Sources view позволяет создать и удалить источник.
- `EC-05` Interests view позволяет создать и удалить интерес.
- `EC-06` Posts view отображает список постов и саммари при клике.
- `EC-07` Dev mode работает: `vite` запускается, proxy /v1/* работает.

### Traceability matrix

| Requirement ID | Design refs | Acceptance refs | Checks | Evidence IDs |
| --- | --- | --- | --- | --- |
| `REQ-01` | `CTR-01`, `FM-04`, `FM-05` | `EC-03`, `SC-01` | `CHK-01` | `EVID-01` |
| `REQ-02` | `CTR-02`, `CTR-03`, `FM-05` | `EC-04`, `SC-02` | `CHK-02` | `EVID-02` |
| `REQ-03` | `CTR-04`, `CTR-05`, `FM-05` | `EC-05`, `SC-03` | `CHK-03` | `EVID-03` |
| `REQ-04` | `CTR-06`, `CTR-07`, `FM-04`, `FM-05` | `EC-06`, `SC-04` | `CHK-04` | `EVID-04` |
| `REQ-05` | `CON-01`, `CTR-08`, `CTR-09`, `FM-01`, `FM-02` | `EC-01`, `EC-02`, `SC-05` | `CHK-05` | `EVID-05` |
| `REQ-06` | `ASM-01`, `ASM-02`, `FM-03` | `EC-07`, `SC-06` | `CHK-06` | `EVID-06` |
| `REQ-07` | `CTR-09`, `FM-01` | `EC-01`, `SC-05` | `CHK-05` | `EVID-05` |

### Acceptance Scenarios

- `SC-01` **Feed view happy path**: запущен сервер с данными → открыть `/` → список FeedItem с текстом, score и датой.
- `SC-02` **Sources CRUD**: открыть `/sources` → создать RSS-источник через форму → источник появился в списке → удалить → источник исчез.
- `SC-03` **Interests CRUD**: открыть `/interests` → создать интерес "Go programming" → появился в списке → удалить → исчез.
- `SC-04` **Posts + summaries**: открыть `/posts` → список постов → кликнуть на пост с саммари → саммари отображается; кликнуть на пост без саммари → пустое состояние без ошибки.
- `SC-05` **go:embed build**: `make ui-build && go build` успешен; `./feedium -conf configs/ &` → `curl http://localhost:8000/` возвращает HTML с `<div id="root">`.
- `SC-06` **Dev mode**: `cd ui && npm run dev` → Vite запустился; `curl http://localhost:5173/v1/feed` проксируется на Go backend.

### Checks

| Check ID | Covers | How to check | Expected result | Evidence path |
| --- | --- | --- | --- | --- |
| `CHK-01` | `EC-03`, `SC-01` | Открыть `http://localhost:8000/` в браузере с запущенным сервером | Feed view с постами или empty state | `artifacts/ft-011/verify/chk-01/` |
| `CHK-02` | `EC-04`, `SC-02` | Открыть `/sources`, создать и удалить RSS-источник | Источник появляется и исчезает из списка | `artifacts/ft-011/verify/chk-02/` |
| `CHK-03` | `EC-05`, `SC-03` | Открыть `/interests`, создать и удалить интерес | Интерес появляется и исчезает | `artifacts/ft-011/verify/chk-03/` |
| `CHK-04` | `EC-06`, `SC-04` | Открыть `/posts`, кликнуть на пост | Список постов, потом саммари | `artifacts/ft-011/verify/chk-04/` |
| `CHK-05` | `EC-01`, `EC-02`, `SC-05` | `make ui-build && go build -o bin/feedium ./cmd/feedium && ./bin/feedium -conf configs/ & sleep 2 && curl -s http://localhost:8000/ \| grep '<div id="root">'` | HTML с `<div id="root">` | `artifacts/ft-011/verify/chk-05/` |
| `CHK-06` | `EC-07`, `SC-06` | `cd ui && npm run dev` → проверить proxy | Vite запущен, `/v1/feed` проксируется | `artifacts/ft-011/verify/chk-06/` |

### Test matrix

| Check ID | Evidence IDs | Evidence path |
| --- | --- | --- |
| `CHK-01` | `EVID-01` | `artifacts/ft-011/verify/chk-01/` |
| `CHK-02` | `EVID-02` | `artifacts/ft-011/verify/chk-02/` |
| `CHK-03` | `EVID-03` | `artifacts/ft-011/verify/chk-03/` |
| `CHK-04` | `EVID-04` | `artifacts/ft-011/verify/chk-04/` |
| `CHK-05` | `EVID-05` | `artifacts/ft-011/verify/chk-05/` |
| `CHK-06` | `EVID-06` | `artifacts/ft-011/verify/chk-06/` |

### Evidence

- `EVID-01` Скриншот Feed view с постами или empty state.
- `EVID-02` Скриншот Sources view после создания и после удаления источника.
- `EVID-03` Скриншот Interests view после создания и после удаления интереса.
- `EVID-04` Скриншот Posts view со списком и с раскрытым саммари.
- `EVID-05` Вывод `curl http://localhost:8000/ | grep root` показывающий HTML.
- `EVID-06` Терминальный вывод `npm run dev` с активным proxy.

### Evidence contract

| Evidence ID | Artifact | Producer | Path contract | Reused by checks |
| --- | --- | --- | --- | --- |
| `EVID-01` | Скриншот браузера | human | `artifacts/ft-011/verify/chk-01/` | `CHK-01` |
| `EVID-02` | Скриншот браузера | human | `artifacts/ft-011/verify/chk-02/` | `CHK-02` |
| `EVID-03` | Скриншот браузера | human | `artifacts/ft-011/verify/chk-03/` | `CHK-03` |
| `EVID-04` | Скриншот браузера | human | `artifacts/ft-011/verify/chk-04/` | `CHK-04` |
| `EVID-05` | curl output или скриншот | human/agent | `artifacts/ft-011/verify/chk-05/` | `CHK-05` |
| `EVID-06` | Скриншот терминала | human | `artifacts/ft-011/verify/chk-06/` | `CHK-06` |
