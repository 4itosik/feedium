---
title: "FT-020: Cumulative Digest Scoring"
doc_kind: feature
doc_function: canonical
purpose: "Design-решение FT-020: cumulative source-level дайджесты получают собственный score/embedding и попадают в /v1/feed наравне с постами, так что уже существующий 4-часовой OpenClaw-дайджест начинает включать контент групповых чатов без изменений на своей стороне."
derived_from:
  - ../../adr/ADR-003-cumulative-digest-scoring.md
  - ../FT-019-cumulative-source-visibility/brief.md
  - ../FT-010-ai-scoring/feature.md
  - ../../prd/PRD-001-mvp.md
status: active
delivery_status: done
---

# FT-020: Cumulative Digest Scoring

## What

### Problem

FT-019 сделало cumulative-дайджесты (`telegram_group`, `summaries.post_id IS NULL`) видимыми
человеку через отдельную read-only страницу `/digests` (путь B), сознательно не трогая scoring
(`NS-01` в `FT-019/brief.md`). Однако основной канал потребления — 4-часовой Telegram-дайджест
от OpenClaw — читает **только** `GET /v1/feed?min_score=0.5` (`artifacts/openclaw-feedium-skill/
SKILL.md`), а `/v1/feed` структурно исключает cumulative-дайджесты (`INNER JOIN posts ⋈
scores`, `dueCandidatePostIDs` фильтрует `post_id IS NOT NULL`). Оператор явно запросил путь A:
дайджесты должны получать собственный score и попадать в `/v1/feed`, чтобы OpenClaw подхватил их
без единого изменения в своём репозитории.

### Outcome

| Metric ID | Metric | Baseline | Target | Measurement method |
| --- | --- | --- | --- | --- |
| `MET-01` | Cumulative-дайджесты со score в `/v1/feed` | 0 (структурно исключены) | Каждый cumulative-summary без score получает ровно один score и наблюдаем в `/v1/feed` при прохождении `min_score` | `GET /v1/feed?min_score=0&page_size=200` содержит `kind: FEED_ITEM_KIND_DIGEST` для источника с известным существующим summary |
| `MET-02` | Изменения на стороне OpenClaw для получения контента групп | — | 0 | Diff `artifacts/openclaw-feedium-skill/SKILL.md` до/после — файл не меняется |
| `MET-03` | Регрессия post-scored пайплайна | 0 | 0 | `go test ./internal/biz/... ./internal/data/... -race` зелёный; значения существующих post-scored строк не меняются |

### Scope

- `REQ-01` Каждый cumulative source-level summary (`post_id IS NULL`), у которого ещё нет
  score и нет активного score-события, скорится независимо — ровно один score на каждый
  summary (симметрично `INV-02`: ровно один score на пост).
- `REQ-02` Метод, формат и агрегация оценки не меняются (`ADR-001 DEC-01/03/04`): косинус,
  скаляр `[0,1]`, max по интересам. Меняется только то, какая сущность скорится.
- `REQ-03` Embedding дайджеста персистится и переиспользуется между переоценками так же, как
  для постов (`ADR-002`-паттерн), ключ — `summary_id`.
- `REQ-04` `GET /v1/feed` (`ListRanked`) возвращает digest-scored items вперемешку с
  post-scored, с тем же порядком (`value DESC, id ASC`), той же keyset-пагинацией и теми же
  фильтрами (`from`/`to`/`min_score`), что и сегодня для постов.
- `REQ-05` Ответ `FeedItem` явно различает пост и дайджест (`kind`), чтобы клиенты (React UI,
  OpenClaw) могли обработать их по-разному без угадывания по пустым полям.
- `REQ-06` При смене набора интересов (create/update/delete, уже существующий триггер
  `EnqueueScoredPosts` в `interest.go`) уже заскоренные дайджесты тоже переоцениваются —
  иначе их score станет молча устаревшим относительно новых интересов.
- `REQ-07` Существующее post-scored поведение (значения, идемпотентность, устойчивость к
  сбою LLM/провайдера, `EnqueueScoredPosts` для постов) не меняется.

### Non-Scope

- `NS-01` Реакции (лайк/дизлайк) для дайджестов — `post_reactions` (FK → `posts(id)`) не
  меняется; дайджест-элементы в ленте просто не поддерживают реакцию в этой итерации
  (оператор подтвердил явно).
- `NS-02` Любые изменения в репозитории/скилле OpenClaw — контракт `/v1/feed`, который он уже
  читает, расширяется аддитивно; ничего там менять не нужно и не следует.
- `NS-03` Изменение самой cumulative-суммаризации (окно, интервал, как агрегируются сырые
  посты в текст дайджеста) — `FT-005`/`SourceDueScheduler`-поведение не трогается.
- `NS-04` Отдельный batch/backfill джоб для 61 уже существующего исторического дайджеста —
  не нужен: due-очередь ищет по факту «summary без score», а не по дате, так что backlog
  дренируется тем же worker pool (`concurrency: 4`) на общих основаниях, без спец-логики.
- `NS-05` Визуальная перестройка `FeedCard` под дайджест-стиль — минимальное изменение
  (спрятать кнопки реакции при `kind === DIGEST`), полноценный редизайн карточки вне scope.

### Constraints / Assumptions

- `ASM-01` Объём дайджестов остаётся низким (9 cumulative-источников × ~раз в час, `cron.
  interval: 3600s`) — конкуренция за shared `score_events`/worker pool с постами (тысячи
  строк) пренебрежимо мала; отдельная приоритизация не нужна.
- `CON-01` PostgreSQL-only, без новых инфра-зависимостей (мирроринг `ADR-001`/`ADR-002`).
  Docker-образ БД не меняется.
- `CON-02` Миграция обязана быть обратно совместимой: существующие `post_id`-строки во всех
  трёх таблицах (`scores`, `score_events`, `post_embeddings`) не должны менять смысл или
  поведение.
- `DEC-01` Ключ для scoreable cumulative-дайджеста — `summary_id` (не `source_id`, не
  синтетический `Post`) — решено `ADR-003`.

## How

### Solution

`scores`, `score_events`, `post_embeddings` получают nullable `summary_id UUID REFERENCES
summaries(id) ON DELETE CASCADE`; `post_id` становится nullable; каждая таблица получает
`CHECK (num_nonnulls(post_id, summary_id) = 1)` и заменяет единственный `UNIQUE(post_id)` на
два partial-индекса. `ScoreOutboxRepo` получает пару новых методов, зеркальных существующим:
`EnqueueDueDigests` (аналог `EnqueueDuePosts`, но по `summaries.post_id IS NULL`) и
`EnqueueScoredDigests` (аналог `EnqueueScoredPosts`, для re-scoring при смене интересов).
`biz.ScoreUsecase.EnqueueDue` и три места вызова `EnqueueScoredPosts` в `interest.go` начинают
дополнительно вызывать digest-версии. `ScorePost` обобщается: если `event.SummaryID != nil`,
текст берётся напрямую по summary-id (а не через `ListByPost`), дальше — тот же
`Cosine`/`Relevance`/`resolvePostVector`-путь (переименованный в generic
`resolveVector(ctx, key, text, model)`), без новой scoring-логики. `ListRanked` становится
`UNION ALL` существующей пост-ветки (без изменений) и новой digest-ветки (`scores ⋈ summaries
WHERE post_id IS NULL ⋈ sources`). Никакие файлы в `internal/task/` не меняются: воркер-пул и
scheduler уже обобщены над `biz.ScoreEvent`/`biz.ScoreUsecase.EnqueueDue` и не знают, какой
ключ установлен внутри события.

### Change Surface

| Surface | Type | Why it changes |
| --- | --- | --- |
| `migrations/*_scores_summary_id.sql`, `*_score_events_summary_id.sql`, `*_post_embeddings_summary_id.sql` | data | Схема per `ADR-003`: nullable `summary_id`, `CHECK`, partial unique-индексы |
| `internal/ent/schema/score.go`, `score_event.go`, `post_embedding.go` | code | `post_id` → optional, `+summary_id` optional field |
| `internal/biz/score.go` | code | `Score`/`PostEmbedding` — `PostID`/`SummaryID` оба `*string`; `ScorePost` читает текст либо через `ListByPost`, либо напрямую по summary-id; `resolvePostVector` → generic `resolveVector` |
| `internal/biz/score_outbox.go` | code | `ScoreEvent.PostID`/`SummaryID` оба `*string`; интерфейс `ScoreOutboxRepo` — новые методы `EnqueueDueDigests`, `EnqueueScoredDigests` |
| `internal/biz/interest.go` | code | 3 существующих вызова `EnqueueScoredPosts` дополняются вызовом `EnqueueScoredDigests` |
| `internal/data/score_repo.go` | code | `Upsert` пишет либо `post_id`, либо `summary_id`; `ListRanked` — `UNION ALL` |
| `internal/data/score_outbox_repo.go` | code | Новые SQL для digest-веток `EnqueueDueDigests`/`EnqueueScoredDigests`; `ClaimOne`/finalize-запросы читают оба поля |
| `internal/data/post_embedding_repo.go` | code | `Get`/`Upsert` — оба ключа |
| `api/feedium/feed.proto` | code | `FeedItemKind` enum + `FeedItem.kind = 9` |
| `internal/service/feed/feed.go` | code | Маппинг нового поля `kind` |
| `ui/src/api.ts` | code | Тип `FeedItem.kind`, значения enum |
| `ui/src/views/FeedView.tsx` | code | Скрыть кнопки реакции, когда `item.kind === 'FEED_ITEM_KIND_DIGEST'` |
| `memory-bank/adr/ADR-003-cumulative-digest-scoring.md` | doc | Governance-решение (уже написан) |

### Flow

1. `SourceDueScheduler`/`ScoreDueScheduler` (без изменений в `task/`) тикают как раньше.
2. `ScoreUsecase.EnqueueDue` теперь вызывает и `EnqueueDuePosts`, и `EnqueueDueDigests` —
   последний находит summary с `post_id IS NULL`, без score, без активного события, и ставит
   `score_events` с `summary_id`.
3. Воркер-пул (без изменений) забирает событие через `ClaimOne`, зовёт `ScorePost(ctx, event)`.
4. `ScorePost` видит `event.SummaryID != nil` → читает текст этого summary напрямую (не через
   `ListByPost`), считает relevance тем же кодом, что и для постов, апсертит `scores` с
   `summary_id`.
5. `GET /v1/feed` теперь `UNION ALL` двух веток — сортировка/пагинация/фильтры едины.
6. OpenClaw (без изменений) на своём 4-часовом тике вызывает тот же `/v1/feed?min_score=0.5` —
   квалифицирующиеся дайджесты попадают в сообщение автоматически.

### Contracts

| Contract ID | Input / Output | Producer / Consumer | Notes |
| --- | --- | --- | --- |
| `CTR-01` | `scores`/`score_events`/`post_embeddings`: `post_id` nullable + новый nullable `summary_id UUID REFERENCES summaries(id) ON DELETE CASCADE`, `CHECK (num_nonnulls(post_id, summary_id) = 1)`, два partial UNIQUE вместо одного | migrations → `data/` | Существующие `post_id`-строки не меняются; `ON DELETE CASCADE` зеркалит существующее поведение для постов |
| `CTR-02` | `FeedItem.kind` (новое поле 9, enum `FEED_ITEM_KIND_POST` / `FEED_ITEM_KIND_DIGEST`) | `service/feed` → React UI, OpenClaw | Аддитивное поле, обратная совместимость: consumers, игнорирующие поле, продолжают работать как раньше |
| `CTR-03` | Digest-ветка `FeedItem`: `postId` = `summary.id` (стабильный UUID для пагинации/ключа), `author=""`, `url=""`, `reaction=""`, `publishedAt = summary.createdAt` | `data/score_repo.go` (`ListRanked`) | `postId` используется как identity-поле в UI (`key`) и в keyset-курсоре, но не ссылается на реальную строку `posts` |
| `CTR-04` | `ScoreOutboxRepo.EnqueueDueDigests()`/`EnqueueScoredDigests()` — те же idempotency/`ON CONFLICT`-гарантии, что и post-версии | `biz/score_outbox.go` → `data/score_outbox_repo.go` | Партнёрские методы, не замена существующих |

### Failure Modes

- `FM-01` Реакция (`POST /v1/posts/{id}/react`) вызвана с `id` дайджеста (не реального поста)
  → существующий `ErrPostNotFound` путь отрабатывает как есть (в `posts` такой строки нет) —
  никакого спец-кода не требуется, но это acceptance-сценарий (`SC-03`), не просто побочный
  эффект.
- `FM-02` `EnqueueDueDigests` и `EnqueueDuePosts` конкурируют за общий `score_events`/worker
  pool — при заданном объёме (`ASM-01`) не является проблемой; если объём вырастет,
  приоритизация — отдельный follow-up, не решается здесь.
- `FM-03` Смена интересов должна переоценить и посты, и дайджесты атомарно в одной
  транзакции — `EnqueueScoredDigests` вызывается в той же `TxManager.InTx`, что и
  `EnqueueScoredPosts` (три существующих call site в `interest.go`), не отдельным шагом.

### ADR Dependencies

| ADR | Current `decision_status` | Used for | Execution rule |
| --- | --- | --- | --- |
| [../../adr/ADR-003-cumulative-digest-scoring.md](../../adr/ADR-003-cumulative-digest-scoring.md) | `accepted` | Ключевой design-выбор: `summary_id`-ключ, границы (реакции/OpenClaw вне scope) | canonical input, Plan Ready разблокирован |
| [../../adr/ADR-001-scoring-method-and-vector-storage.md](../../adr/ADR-001-scoring-method-and-vector-storage.md) | `accepted` | Метод/формат/агрегация оценки — не пересматриваются, переиспользуются как есть | canonical input |
| [../../adr/ADR-002-post-embedding-persistence.md](../../adr/ADR-002-post-embedding-persistence.md) | `accepted` | Паттерн персистентности/инвалидации embedding-кэша — расширяется тем же ключом | canonical input |

## Verify

### Exit Criteria

- `EC-01` Cumulative summary без score получает ровно один score, ключуемый `summary_id`, без
  необходимости существования поста.
- `EC-02` `GET /v1/feed` возвращает digest-элементы (`kind=DIGEST`) вперемешку с post-элементами,
  корректно упорядоченными/пагинируемыми/фильтруемыми теми же параметрами, что и сегодня.
- `EC-03` Существующее post-scored поведение (значения, идемпотентность, retry, порядок ленты)
  не регрессирует.
- `EC-04` `GET /v1/feed?min_score=0.5` (тот же вызов, что уже делает OpenClaw-скилл) отдаёт
  квалифицирующиеся дайджесты без единого изменения в `artifacts/openclaw-feedium-skill/
  SKILL.md`.

### Traceability matrix

| Requirement ID | Design refs | Acceptance refs | Checks | Evidence IDs |
| --- | --- | --- | --- | --- |
| `REQ-01` | `CTR-01`, `CTR-04`, `DEC-01` | `EC-01`, `SC-01` | `CHK-01`, `CHK-03` | `EVID-01` |
| `REQ-02` | `CTR-01` | `EC-01`, `SC-01` | `CHK-01` | `EVID-01` |
| `REQ-03` | `CTR-01` | `EC-01`, `SC-01` | `CHK-01` | `EVID-01` |
| `REQ-04` | `CTR-02`, `CTR-03` | `EC-02`, `SC-02` | `CHK-01`, `CHK-04` | `EVID-01`, `EVID-02` |
| `REQ-05` | `CTR-02` | `EC-02`, `SC-01`, `SC-03` | `CHK-01`, `CHK-04` | `EVID-02` |
| `REQ-06` | `FM-03` | `SC-02` | `CHK-01` | `EVID-01` |
| `REQ-07` | `CON-02` | `EC-03` | `CHK-01`, `CHK-02` | `EVID-01` |

### Acceptance Scenarios

- `SC-01` Cumulative-источник имеет существующий неоценённый summary (`post_id IS NULL`) →
  после обработки воркером в `scores` появляется одна строка с `summary_id`, и `GET /v1/feed`
  включает её с `kind=DIGEST`, правильными `text`/`sourceId`/`score`/`publishedAt`, пустыми
  `author`/`url`/`reaction`.
- `SC-02` В одном временном окне сосуществуют post-scored и digest-scored элементы →
  `GET /v1/feed` возвращает оба, корректно перемешанные по `score DESC`, keyset-пагинация по
  двум страницам не даёт дублей/пропусков через объединение веток.
- `SC-03` `POST /v1/posts/{digest_id}/react` (id дайджеста, не поста) возвращает штатную ошибку
  "not found", не создавая некорректных данных — граница `NS-01` держится без спец-кода.

### Checks

| Check ID | Covers | How to check | Expected result | Evidence path |
| --- | --- | --- | --- | --- |
| `CHK-01` | `EC-01`, `EC-03`, `SC-01`, `SC-02` | `go test ./internal/biz/... ./internal/data/... ./internal/task/... -race` | Зелёный, включая новые тесты на digest-scoring путь | `artifacts/ft-020/verify/chk-01/` |
| `CHK-02` | `EC-03` | `make lint` | Чисто, без новых замечаний | `artifacts/ft-020/verify/chk-02/` |
| `CHK-03` | `REQ-01`, `CON-02` | `make test` (testcontainers) применяет миграцию к чистой БД и к seed-данным с существующими post-scored строками | Миграция применяется без ошибок; существующие строки не меняются | `artifacts/ft-020/verify/chk-03/` |
| `CHK-04` | `EC-02`, `EC-04`, `SC-01` | После деплоя: `curl "http://127.0.0.1:8000/v1/feed?min_score=0&page_size=200"` (через SSH-туннель на `oc`) | Ответ содержит хотя бы один `kind: FEED_ITEM_KIND_DIGEST` для источника с известным существующим summary | `artifacts/ft-020/verify/chk-04/` |
| `CHK-05` | `EC-04`, `MET-02` | Diff `artifacts/openclaw-feedium-skill/SKILL.md` до/после реализации | Файл не изменился | `artifacts/ft-020/verify/chk-05/` |

### Test matrix

| Check ID | Evidence IDs | Evidence path |
| --- | --- | --- |
| `CHK-01` | `EVID-01` | `artifacts/ft-020/verify/chk-01/` |
| `CHK-02` | `EVID-01` | `artifacts/ft-020/verify/chk-02/` |
| `CHK-03` | `EVID-01` | `artifacts/ft-020/verify/chk-03/` |
| `CHK-04` | `EVID-02` | `artifacts/ft-020/verify/chk-04/` |
| `CHK-05` | `EVID-02` | `artifacts/ft-020/verify/chk-05/` |

### Evidence

- `EVID-01` Логи прогонов `go test`/`make lint`/`make test` (миграция).
- `EVID-02` Сырой ответ `GET /v1/feed` с реальным `kind=DIGEST` элементом + подтверждение
  отсутствия diff в `SKILL.md`.

### Evidence contract

| Evidence ID | Artifact | Producer | Path contract | Reused by checks |
| --- | --- | --- | --- | --- |
| `EVID-01` | test/lint/migration логи | agent | `artifacts/ft-020/verify/chk-01/`, `chk-02/`, `chk-03/` | `CHK-01`, `CHK-02`, `CHK-03` |
| `EVID-02` | sample API response + diff-подтверждение | agent | `artifacts/ft-020/verify/chk-04/`, `chk-05/` | `CHK-04`, `CHK-05` |
