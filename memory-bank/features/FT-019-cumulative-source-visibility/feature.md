---
title: "FT-019: Cumulative Telegram Group Posts Missing From Feed"
doc_kind: feature
doc_function: canonical
purpose: "Design-решение FT-019: cumulative-источники (telegram_group) становятся видимы пользователю через отдельный read-only раздел 'Digests', читающий уже существующий backend-эндпоинт source-summaries — без изменений API/схемы/scoring."
derived_from:
  - brief.md
  - ../../prd/PRD-001-mvp.md
status: active
delivery_status: done
---

# FT-019: Cumulative Telegram Group Posts Missing From Feed

## What

### Problem

См. `brief.md`: посты cumulative-источников (`telegram_group`) собираются, но их агрегированный source-level summary (`summaries.post_id IS NULL`) никогда не скорится и не попадает в ranked feed, и при этом нигде не читается ни одним сервисом — контент полностью невидим пользователю.

Design выбрал путь **B** из `brief.md`: отдельное read-only представление дайджестов, не путь A (встраивание в общую ranked-ленту со своим score, требующее менять схему `scores`/`score_events`). `GET /v1/sources/{source_id}/summaries` (`V1ListSourceSummaries`) уже реализован, уже отдаёт source-level summary (включая записи с `post_id = null`) и уже подключён к HTTP (`internal/service/summary/summary.go`, `internal/server/http.go`) — backend-изменений не требуется.

### Scope

- `REQ-01` Пользователь видит отдельную страницу со списком агрегированных дайджестов всех cumulative-источников (`processingMode === PROCESSING_MODE_CUMULATIVE`), отсортированных по времени создания (новые сверху).
- `REQ-02` Каждая карточка дайджеста показывает метку источника (как в `PostsView`/`FeedView`), текст дайджеста и время создания.

### Non-Scope

- `NS-01` Дайджесты не получают score и не попадают в общую ranked-ленту (Feed) — путь A из `brief.md` остаётся нереализованным и не исключается на будущее.
- `NS-02` Без серверной кросс-источниковой пагинации: на первой итерации фронтенд берёт первую страницу (`page_size=50`) `summaries` на каждый cumulative-источник и мержит на клиенте. Источник с >50 дайджестами или общее число cumulative-источников, при котором это станет узким местом, — известное ограничение, не закрывается здесь.
- `NS-03` Backend, proto, миграции не меняются — переиспользуется существующий `V1ListSourceSummaries`.

### Constraints

- `CON-01` Не менять поведение существующих views (`FeedView`, `PostsView`, `SourcesView`, `InterestsView`) — только новая страница и общий пункт навигации.

## How

### Solution

Новый React-вид `DigestsView` фильтрует источники (уже получаемые через существующий `listSources()`) по `processingMode === 'PROCESSING_MODE_CUMULATIVE'`, параллельно запрашивает `GET /v1/sources/{id}/summaries` для каждого через новую тонкую обёртку `listSourceSummaries()` в `ui/src/api.ts`, мержит и сортирует результаты на клиенте по `createdAt` desc, рендерит карточки в стиле `PostsView` (источник + текст + время). Главный trade-off: пагинация на клиенте по источникам вместо единого backend-эндпоинта — нулевой риск для backend, но не масштабируется при большом числе cumulative-источников/дайджестов (`NS-02`).

### Change Surface

| Surface | Why |
| --- | --- |
| `ui/src/api.ts` | добавить `listSourceSummaries(sourceId, pageToken?)`, тонкая обёртка над существующим `GET /v1/sources/{id}/summaries` |
| `ui/src/views/DigestsView.tsx` | новая страница: фильтр cumulative-источников, fetch+merge+sort дайджестов, карточки |
| `ui/src/App.tsx` | новый роут `/digests` |
| `ui/src/components/Layout.tsx` | новый пункт навигации `Digests` |

### Flow

1. `DigestsView` монтируется → `listSources()` → фильтр по `processingMode === PROCESSING_MODE_CUMULATIVE`.
2. Для каждого cumulative-источника — `listSourceSummaries(sourceId)` параллельно (`Promise.all`).
3. Результаты мержатся в один список, сортируются по `createdAt` desc, рендерятся карточками; пустое состояние, если cumulative-источников нет или ни у одного ещё нет дайджеста.

## Verify

### Exit Criteria

- `EC-01` На странице `/digests` пользователь видит дайджесты существующих cumulative-источников (если они есть), отсортированные по времени, без обращения к Feed/Posts/scores-пайплайну.

### Acceptance Scenarios

- `SC-01` Открыть `/digests` при наличии хотя бы одного cumulative-источника с готовым source-level summary → дайджест виден в списке с текстом, меткой источника и временем создания.

### Traceability matrix

| Requirement ID | Design refs | Acceptance refs | Checks | Evidence IDs |
| --- | --- | --- | --- | --- |
| `REQ-01` | `CON-01` | `EC-01`, `SC-01` | `CHK-01` | `EVID-01` |
| `REQ-02` | `CON-01` | `EC-01`, `SC-01` | `CHK-01` | `EVID-01` |

### Checks

| Check ID | Covers | How to check | Expected |
| --- | --- | --- | --- |
| `CHK-01` | `EC-01`, `SC-01` | `cd ui && npm run build` (tsc -b + vite build) без ошибок; открыть `/digests` в браузере на dev-сервере, указывающем на бэкенд с реальными cumulative-источниками (прод через `oc`, или локально с тестовыми данными) | Build проходит без ошибок; страница показывает хотя бы один дайджест с текстом/источником/временем, если данные есть в БД |

### Test matrix

| Check ID | Evidence IDs | Evidence path |
| --- | --- | --- |
| `CHK-01` | `EVID-01` | `artifacts/ft-019/verify/chk-01/` |

### Evidence

- `EVID-01` Лог успешного `npm run build` + скриншот/текстовое подтверждение страницы `/digests` с реальными данными.

### Evidence contract

| Evidence ID | Artifact | Producer | Path contract | Reused by checks |
| --- | --- | --- | --- | --- |
| `EVID-01` | build-лог + скриншот/подтверждение страницы | agent | `artifacts/ft-019/verify/chk-01/` | `CHK-01` |
