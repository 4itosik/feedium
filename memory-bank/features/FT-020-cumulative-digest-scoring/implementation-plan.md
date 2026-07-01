---
title: "FT-020: Implementation Plan"
doc_kind: feature
doc_function: derived
purpose: "Execution-план реализации FT-020: cumulative source-level дайджесты получают собственный score/embedding и попадают в /v1/feed. Фиксирует discovery context, шаги, риски и test strategy без переопределения canonical feature-фактов."
derived_from:
  - feature.md
status: archived
---

# План имплементации

## Цель текущего плана

Провести FT-020 от Design Ready до приёмки по секции `Verify` в `feature.md`: cumulative-дайджест
(`summaries.post_id IS NULL`) получает ровно один score, ключуемый `summary_id`, персистит embedding
по `summary_id`, попадает в `GET /v1/feed` вперемешку с постами (`kind=DIGEST`, тот же
порядок/пагинация/фильтры), переоценивается при смене интересов — без изменений в
`artifacts/openclaw-feedium-skill/SKILL.md` и без регрессии post-scored пути.

## Current State / Reference Points

Grounding выполнен по всему change surface `feature.md` (12 файлов + миграции + тесты). Ключевые
факты, которые нельзя игнорировать:

| Path / module | Current role | Why relevant | Reuse / mirror |
| --- | --- | --- | --- |
| `migrations/20260602100100_create_scores_table.sql` | `scores(id, post_id NOT NULL FK posts, value, model, ...)`, `UNIQUE idx_scores_post_id`, `idx_scores_value(value DESC)` | Целевая таблица `CTR-01`; `post_id` станет nullable, `+summary_id`, `CHECK`, 2 partial UNIQUE | goose `-- +goose Up/Down`, timestamp-имя `YYYYMMDDHHMMSS_snake.sql` |
| `migrations/20260602100200_create_score_events_table.sql` | `score_events`; **partial** `UNIQUE idx_score_events_unique_active_post ON (post_id) WHERE status IN ('pending','processing')` + `idx_score_events_claim` + `idx_score_events_processing_lease` | Идемпотентность enqueue живёт в этом partial-index + `ON CONFLICT`; нужен зеркальный по `summary_id` | mirror partial-unique + claim index |
| `migrations/20260607120000_create_post_embeddings_table.sql` | `post_embeddings(post_id NOT NULL FK, vector jsonb, model, input_hash, ...)`, `UNIQUE idx_post_embeddings_post_id` | Кэш вектора `CTR-01`/`REQ-03`; тот же nullable+CHECK+2 partial UNIQUE | mirror scores-migration |
| `migrations/20260415100000_create_summaries_and_events.sql` | `summaries(id, post_id NULL, source_id NOT NULL, text, word_count, created_at)` | FK-цель `summary_id`; текст дайджеста = `summaries.text`; `post_id IS NULL` = cumulative | `REFERENCES summaries(id) ON DELETE CASCADE` |
| `internal/ent/schema/{score,score_event,post_embedding}.go` | ent-модели; `post_id` = `field.UUID(...)`, `index.Fields("post_id").Unique()` | `feature.md` требует правки; **НО** goose — единственный DDL-owner (`cmd/migrate/main.go`: "server never auto-migrates") | `post_id`→Optional/Nillable, `+summary_id` Optional; правка держит ent-структуры честными, DDL не трогает |
| `internal/biz/score.go` | `Score{ID,PostID string,...}`, `PostEmbedding{PostID string,...}`, `ScoreUsecase.ScorePost` читает `summaryRepo.ListByPost(event.PostID)[0].Text`, `resolvePostVector(ctx,postID,text,model)`; `Cosine`/`Relevance` — pure | Ядро `REQ-01/02/03`; generalize ключ, не метод | `Relevance`=max cosine [0,1]; `resolvePostVector` reuse когда `stored.Model==model && stored.InputHash==inputHash` |
| `internal/biz/summary.go` | `SummaryRepo.Get(ctx, id) (Summary, error)` **уже существует** | Чтение текста дайджеста напрямую по `summary_id` без нового метода интерфейса | `summaryRepo.Get(ctx, *event.SummaryID)` |
| `internal/biz/score_outbox.go` | `ScoreEvent{ID,PostID string,...}`; интерфейс `ScoreOutboxRepo` (`EnqueueDuePosts`, `EnqueueScoredPosts`, `ClaimOne`, `Finalize...`, ...) | `+EnqueueDueDigests`, `+EnqueueScoredDigests`; `PostID`→`*string`, `+SummaryID *string` | mockgen через `//go:generate` |
| `internal/biz/interest.go` | 3 call site `outbox.EnqueueScoredPosts(ctx)` внутри `uc.tx.InTx` (Create :116-125, Update :156-165, Delete :176-184), мутация→enqueue атомарно | `FM-03`/`REQ-06`: в тот же `InTx` добавить `EnqueueScoredDigests(ctx)` | wrap `fmt.Errorf("enqueue rescore: %w", e)` |
| `internal/data/score_repo.go` | `Upsert` (`ON CONFLICT (post_id)`), `ListRanked` (`JOIN posts`, LATERAL summaries, LEFT JOIN post_reactions, `!= 'dislike'`, ORDER `CASE like→+0.1 DESC, p.id ASC`, keyset `s.value`, token `value|uuid`) | `CTR-01/02/03`, `REQ-04`; `Upsert` пишет либо `post_id` либо `summary_id`; `ListRanked`=`UNION ALL` | `encodeScoreToken(value,id)`; PageSize+1 fetch |
| `internal/data/score_outbox_repo.go` | `scoreEventInsertQuery` (`ON CONFLICT (post_id) WHERE ... DO NOTHING`), `dueCandidatePostIDs`/`scoredCandidatePostIDs`, `ClaimOne` CTE, `scanScoreEvent(id,post_id,status,error,attempt,created,processed)` | Новые digest-ветки enqueue; scan обоих ключей | mirror insert-const + candidate query |
| `internal/data/post_embedding_repo.go` | `Get(ctx,postID)`, `Upsert` (`ON CONFLICT (post_id)`) | `Get`/`Upsert` по обоим ключам | `biz.ErrPostEmbeddingNotFound` на `sql.ErrNoRows` |
| `internal/task/*` | worker pool / scheduler / reaper: interfaces `ScoreProcessor.ScorePost(ctx,biz.ScoreEvent)`, `ScoreScheduler.EnqueueDue`; **grep `summary` → 0 matches** | `NS`/Solution: task/ не меняется — **verified**. Единственный риск: `worker_pool.go:181` логирует `event.PostID` (станет `*string`) → см. `OQ-01` | не трогать без нужды |
| `api/feedium/feed.proto` | `FeedItem` поля 1..8 (max=8), enum-паттерн из `error_reason.proto` (`*_UNSPECIFIED=0`, screaming-snake) | `CTR-02`: `+FeedItemKind` enum, `+FeedItem.kind=9` | `make proto` (protoc + go/go-grpc/go-http) |
| `internal/service/feed/feed.go` | `V1ListFeed` маппит `biz.FeedItem`→`feedium.FeedItem` пофайлово (:69-78) | `CTR-02`: маппинг `kind` | добавить поле в литерал |
| `internal/biz/feed.go` | `biz.FeedItem{Post, Score, URL, Reaction}` (domain) | Носитель `kind` от data к service | добавить `Kind` |
| `ui/src/api.ts` | `FeedItem` (camelCase, snake→camel transcode); enum = string-literal union | `+kind` поле, значения enum | mirror `SourceType` union |
| `ui/src/views/FeedView.tsx` | `FeedCard` reaction-кнопки `+`/`−` (:144-165), `handleReact→onReact(item.postId,signal)` | `NS-05`: спрятать реакции при `kind==='FEED_ITEM_KIND_DIGEST'` | conditional render |
| `internal/data/source_repo_test.go:454` (`setupTestData`) | testcontainers `postgres:18.3-alpine` + `goose.Up(migrations)` | Интеграционные тесты применяют **реальные** миграции → `CHK-03` авто; seed-helpers `seedSource/seedPost/seedSummary/seedScore` | mirror seed helpers |
| `internal/biz/score_test.go:136` (`newScoreUsecase`) | `biz.NewScoreUsecase(scoreRepo,interestRepo,summaryRepo,outboxRepo,scorer,postEmbeddingRepo,&conf.Scoring{})` (7 arg), gomock+goleak, happy-path :154 | Мираринг для новых biz-тестов digest-пути | AAA + `goleak.VerifyNone` |
| `Makefile` | `proto`, `wire`, `generate`(=proto+wire), `test`(`go test ./...`), `lint`(`golangci-lint ... -c .golangci.yml`), `migrate`(goose up). **ent-generate НЕ в Makefile** → `go generate ./...` | Команды verify/regen | `go generate ./internal/...` для ent+mocks |

## Test Strategy

| Test surface | Canonical refs | Existing coverage | Planned automated coverage | Required local suites / commands | Required CI suites / jobs | Manual-only gap / justification | Manual-only approval ref |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `internal/biz/score_test.go` (ScorePost digest branch, resolveVector generic) | `REQ-01`,`REQ-02`,`REQ-03`,`SC-01`,`EC-01`,`CHK-01` | post-branch happy/reuse | +digest happy-path (`SummaryID!=nil`→`summaryRepo.Get`→`Relevance`→`Upsert{SummaryID}`), +vector reuse by summary_id | `go test ./internal/biz/... -race` | `go test ./... -race` | none | none |
| `internal/biz/score_outbox_test.go` (EnqueueDue delegates both) | `REQ-01`,`REQ-06`,`CHK-01` | post enqueue | +`EnqueueDue` вызывает `EnqueueDuePosts`+`EnqueueDueDigests` (mock assert) | `go test ./internal/biz/... -race` | `go test ./... -race` | none | none |
| `internal/biz/interest_test.go` (rescore both in one InTx) | `REQ-06`,`FM-03`,`SC-02`,`CHK-01` | post rescore на 3 call site | +assert `EnqueueScoredDigests` вызывается в том же `InTx` на Create/Update/Delete | `go test ./internal/biz/... -race` | `go test ./... -race` | none | none |
| `internal/data/score_outbox_repo_test.go` (digest candidates + claim + idempotency) | `REQ-01`,`CTR-04`,`CON-02`,`CHK-01`,`CHK-03` | post candidates/claim | +`EnqueueDueDigests` находит summary `post_id IS NULL` без score/актив-события; +повторный вызов идемпотентен; +`ClaimOne` отдаёт `SummaryID` | `go test ./internal/data/... -race` (testcontainers) | `go test ./... -race` | none | none |
| `internal/data/score_repo_test.go` (Upsert summary_id + ListRanked UNION) | `REQ-04`,`CTR-01`,`CTR-03`,`SC-01`,`SC-02`,`EC-02`,`CHK-01` | post Upsert/ListRanked | +Upsert по `summary_id`; +ListRanked отдаёт digest-строку (author/url/reaction пустые, `kind=DIGEST`, `text=summary.text`, `publishedAt=summary.created_at`); +**mixed keyset pagination 2 стр. без дублей/пропусков** | `go test ./internal/data/... -race` (testcontainers) | `go test ./... -race` | none | none |
| `internal/data/post_embedding_repo_test.go` (both keys) | `REQ-03`,`CTR-01`,`CHK-01` | post Get/Upsert | +Get/Upsert по `summary_id` | `go test ./internal/data/... -race` | `go test ./... -race` | none | none |
| migration up/down (backward compat) | `REQ-01`,`CON-02`,`CHK-03` | goose.Up в каждом data-тесте | существующие post-scored строки не меняют смысл; up применяется к seed-БД | `make test` (testcontainers через `setupTestData`) | `go test ./... -race` | none | none |
| `internal/service/feed` + proto | `CTR-02`,`REQ-05`,`EC-02` | `feed_test.go` post-mapping | +маппинг `kind` (POST/DIGEST) | `go test ./internal/service/... -race` | `go test ./... -race` | none | none |
| `ui/src/views/FeedView.tsx` | `NS-05`,`REQ-05` | — (нет UI-тестов в проекте) | manual smoke: digest-карточка без reaction-кнопок | `npm run build` (ui) | — | UI не покрыт автотестами в проекте; проверяется визуально | `AG-02` |
| deployed `/v1/feed` (live) | `EC-04`,`MET-01`,`SC-01`,`CHK-04` | — | `curl "…/v1/feed?min_score=0&page_size=200"` содержит `kind: FEED_ITEM_KIND_DIGEST` | — | live prod через SSH-туннель `oc` | требует деплоя миграции на прод-данные | `AG-01` |
| `artifacts/openclaw-feedium-skill/SKILL.md` | `MET-02`,`EC-04`,`CHK-05` | — | git-diff до/после = пусто | `git diff --stat` | — | none | none |

## Open Questions / Ambiguities

| Open Question ID | Question | Why unresolved | Blocks | Default action / escalation owner |
| --- | --- | --- | --- | --- |
| `OQ-01` | `ScoreEvent.PostID` становится `*string`; `internal/task/score_event_worker_pool.go:181` логирует `event.PostID`. `feature.md` Solution утверждает «никакие файлы в `internal/task/` не меняются». `*string` в slog деградирует лог (адрес/`<nil>`). | Solution-prose vs факт: строка лога. Не `REQ`/`NS`, а нарратив дизайна. | `STEP-05`, `STEP-13` | Default: сделать **минимальную** правку лога в `task/` (`derefOr(event.PostID)` helper в `biz` или inline `""`-fallback), сохранив семантику; зафиксировать 1-строчное отклонение как заметку в `feature.md` §Solution. Эскалация оператору только если правка выходит за пределы logging. |
| `OQ-02` | Ключ keyset-пагинации в `ListRanked`: WHERE фильтрует по `s.value`, а ORDER — по boosted `CASE like→+0.1`. При `UNION ALL` нужен единый ordering-столбец `effective_value` в обеих ветках. | Существующий post-код имеет тонкую (value vs effective) асимметрию, которую нельзя «чинить» (`REQ-07`). | `STEP-10` | Default: обернуть обе ветки в подзапросы, выбрать общий `effective_value` (posts: `CASE like`; digests: `s.value`, реакций нет), keyset+ORDER+LIMIT по `(effective_value DESC, id ASC)` над union; поведение post-ветки сохранить 1-в-1; доказать `SC-02`-тестом. Эскалация если mixed-pagination тест выявит дубли/пропуски, неустранимые без изменения post-семантики. |
| `OQ-03` | Down-миграция должна восстановить single `UNIQUE(post_id)` и `post_id NOT NULL`. Если к моменту отката существуют digest-строки (`post_id IS NULL`), откат `NOT NULL` упадёт. | Обратимость требуется `ADR-003` Риски + `STOP-01`. | `STEP-01`..`STEP-03` (Down-секции) | Default: Down удаляет digest-строки (`DELETE WHERE post_id IS NULL`) перед `SET NOT NULL` — откат = возврат к «только посты», что и есть безопасное pre-FT-020 состояние. Задокументировать в Down-комментарии. Оператор подтверждает откат через `AG-01` (тот же prod-gate). |

## Environment Contract

| Area | Contract | Used by | Failure symptom |
| --- | --- | --- | --- |
| setup | Go toolchain из `go.mod`; Docker daemon запущен (testcontainers `postgres:18.3-alpine`); `protoc`+плагины (`protoc-gen-go`,`-go-grpc`,`-go-http`) в PATH для `make proto`; Node/npm для `ui` build | `STEP-*` | `make test` виснет/падает «cannot connect to Docker»; `make proto` — `protoc: command not found` |
| test | Эталон verify: `go test ./... -race` (юнит+testcontainers), `make lint`, `make test`. Data-тесты применяют реальные миграции через `goose.Up` в `setupTestData` | `CHK-01`,`CHK-02`,`CHK-03` | Тест «зелёный» без запущенного Docker = недостоверно (testcontainers-тесты скипаются/падают на setup) |
| generate | `go generate ./internal/...` для ent (`internal/ent`) и mockgen (`internal/biz/mock`); `make proto` для proto; `make wire` при смене DI-графа | `STEP-04`,`STEP-06`,`STEP-11` | Компиляция падает: mock не реализует новый метод интерфейса / ent-структура без нового поля |
| access / network / secrets | Прод-верификация (`CHK-04`) — SSH на `oc` (порт 2222), туннель к `127.0.0.1:8000`; деплой миграции = `cmd/migrate up` на прод-БД (external `core_storage_pg`, `COMPOSE_FILE=docker-compose.server.yml`); после деплоя — `docker restart` app | `STEP-14`,`STEP-15` | Нет доступа/тумбера → работа останавливается на `AG-01`, не деплоит вслепую |

## Preconditions

| Precondition ID | Canonical ref | Required state | Used by steps | Blocks start |
| --- | --- | --- | --- | --- |
| `PRE-01` | `ADR-003` (`decision_status: accepted`) | Governance-решение о `summary_id`-ключе финализировано | все `STEP-*` | yes |
| `PRE-02` | `feature.md` (`status: active`) | Design Ready достигнут; этот план — derived | все `STEP-*` | yes |
| `PRE-03` | Environment `setup` | Docker up, `protoc`/плагины, Node в PATH | `STEP-01`+ | yes |
| `PRE-04` | `CON-02` | Ветка от актуального `main`; рабочее дерево без несвязанных изменений в change surface | `STEP-01` | yes |

## Workstreams

| Workstream | Implements | Result | Owner | Dependencies |
| --- | --- | --- | --- | --- |
| `WS-1` Schema | `CTR-01`,`REQ-01`,`REQ-03`,`CON-02` | 3 миграции (nullable `post_id`+`summary_id`+CHECK+2 partial UNIQUE) + ent-схемы + regen; `make test` зелёный | agent | `PRE-01..04` |
| `WS-2` biz scoring | `REQ-01`,`REQ-02`,`REQ-03`,`REQ-07` | `Score/PostEmbedding/ScoreEvent` двухключевые; `ScorePost` digest-ветка; `resolveVector` generic; biz-тесты | agent | `WS-1` |
| `WS-3` outbox+interest | `REQ-01`,`REQ-06`,`CTR-04`,`FM-03` | `EnqueueDueDigests`/`EnqueueScoredDigests`; `EnqueueDue` + 3 interest call site зовут digest-версии; тесты | agent | `WS-2` |
| `WS-4` data repos | `CTR-01`,`CTR-03`,`REQ-04` | `score_repo.Upsert`/`ListRanked` (UNION ALL); `score_outbox_repo` digest SQL + двухключевой scan; `post_embedding_repo` оба ключа; data-тесты | agent | `WS-1`,`WS-2`,`WS-3` |
| `WS-5` API+UI | `CTR-02`,`REQ-05`,`NS-05` | `FeedItemKind` proto + `kind=9`; regen; `biz.FeedItem.Kind`; service-маппинг; `ui` тип+enum+скрытие реакций | agent | `WS-4` |
| `WS-6` verify+close | `EC-01..04`,`SC-01..03`,`CHK-01..05` | Evidence в `artifacts/ft-020/verify/`; simplify review; prod-verify; `feature.md`→done, план→archived | agent+human | `WS-1..5` |

## Approval Gates

| Approval Gate ID | Trigger | Applies to | Why approval is required | Approver / evidence |
| --- | --- | --- | --- | --- |
| `AG-01` | Деплой миграции + кода на прод; откат миграции | `STEP-14`,`STEP-15`,`OQ-03` | Необратимая schema-surgery на 3 таблицах с живыми prod-данными; неверный `COMPOSE_FILE` уже вызывал прод-аутэйдж (`feedback_feedium_deploy_composefile`) | Оператор; evidence = вывод `cmd/migrate up` + `docker restart` лог |
| `AG-02` | Приёмка UI (нет автотестов на React в проекте) | `STEP-12` | Визуальная проверка «реакции скрыты для digest» не автоматизирована | Оператор; evidence = скриншот/подтверждение |

## Порядок работ

Каждый code-STEP в biz следует TDD (Red→Green): сначала тест, прогон-fail, минимальная реализация,
прогон-pass, commit. Для data-репо (интеграция) тест пишется до/вместе с SQL. Коммиты частые,
Conventional Commits (`feat(ft-020): ...`).

### WS-1 — Schema & ent

| Step ID | Actor | Implements | Goal | Touchpoints | Artifact | Verifies | Evidence IDs | Check command / procedure | Blocked by | Needs approval | Escalate if |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `STEP-01` | agent | `CTR-01`,`REQ-01` | Миграция `scores`: `ALTER post_id DROP NOT NULL`; `ADD summary_id UUID REFERENCES summaries(id) ON DELETE CASCADE`; `ADD CHECK (num_nonnulls(post_id,summary_id)=1)`; `DROP idx_scores_post_id`; `CREATE UNIQUE idx_scores_post_id ON scores(post_id) WHERE post_id IS NOT NULL`; `CREATE UNIQUE idx_scores_summary_id ON scores(summary_id) WHERE summary_id IS NOT NULL`. Down: `DELETE FROM scores WHERE post_id IS NULL` (`OQ-03`), drop summary_id/check/partial-idx, восстановить single `UNIQUE(post_id)` + `SET NOT NULL` | `migrations/<ts>_scores_summary_id.sql` | goose up/down файл | `CHK-03` | `EVID-01` | `make test` (goose.Up в data-тестах применяет миграцию) | `PRE-01..04` | none | `num_nonnulls`/partial-index не поддерживается целевой PG (не 18.3) |
| `STEP-02` | agent | `CTR-01`,`REQ-01` | Миграция `score_events` по тому же паттерну + partial `UNIQUE idx_score_events_unique_active_summary ON (summary_id) WHERE status IN ('pending','processing')`; claim-index не трогаем (не зависит от ключа) | `migrations/<ts>_score_events_summary_id.sql` | goose файл | `CHK-03` | `EVID-01` | `make test` | `STEP-01` | none | как `STEP-01` |
| `STEP-03` | agent | `CTR-01`,`REQ-03` | Миграция `post_embeddings` по паттерну `scores` (`summary_id`, CHECK, 2 partial UNIQUE) | `migrations/<ts>_post_embeddings_summary_id.sql` | goose файл | `CHK-03` | `EVID-01` | `make test` | `STEP-02` | none | как `STEP-01` |
| `STEP-04` | agent | change surface | ent-схемы: `post_id`→`.Optional().Nillable()`, `+field.UUID("summary_id",uuid.UUID{}).Optional().Nillable()`; `index.Fields("post_id").Unique()`→partial через `.Annotations(entsql.IndexWhere("post_id IS NOT NULL"))` (или снять Unique если ent не строит DDL) + аналог summary; `go generate ./internal/ent/...`; `go build ./...` | `internal/ent/schema/{score,score_event,post_embedding}.go`, generated | компилируемый ent | `CHK-01` | `EVID-01` | `go build ./...` | `STEP-03` | none | ent regen ломает несвязанные схемы |
| `CP-1` checkpoint | — | — | Миграции применяются к чистой и seed-БД, существующие post-строки не меняются, проект компилируется | — | — | `CHK-03` | `EVID-01` | `make test` зелёный | — | — | — |

### WS-2 — biz scoring (TDD)

| Step ID | Actor | Implements | Goal | Touchpoints | Artifact | Verifies | Evidence IDs | Check command / procedure | Blocked by | Needs approval | Escalate if |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `STEP-05` | agent | `REQ-01`,`REQ-07` | Расширить структуры: `Score.PostID *string`,`+SummaryID *string`; `PostEmbedding.PostID *string`,`+SummaryID *string`; `ScoreEvent.PostID *string`,`+SummaryID *string`. Обновить все post-call-site на `&id`/deref; `OQ-01` — минимальный fallback для `task` log | `internal/biz/score.go`,`score_outbox.go`, зависимые | компилируемые двухключевые типы | `CHK-01` | `EVID-01` | `go build ./...` | `CP-1` | none | blast radius типов выходит за change surface (напр. service ломается неаддитивно) |
| `STEP-06` | agent | `CTR-04` | В интерфейсы `ScoreOutboxRepo` добавить `EnqueueDueDigests(ctx)(int,error)`,`EnqueueScoredDigests(ctx)(int,error)`; `go generate ./internal/biz/...` (mockgen) | `internal/biz/score_outbox.go`, `internal/biz/mock/*` | обновлённые mocks | `CHK-01` | `EVID-01` | `go build ./...` | `STEP-05` | none | mockgen не видит `//go:generate` директиву |
| `STEP-07` | agent | `REQ-01`,`REQ-02`,`REQ-03`,`SC-01`,`EC-01` | **RED**: тест `TestScorePost_DigestBranch` в `score_test.go` — `event.SummaryID=&"sum-1"`, `summaryRepo.Get("sum-1")→{Text:"digest text"}`, `scorer.Model()`, `interestRepo.ListInterestVectors`, `postEmbeddingRepo.Get→ErrPostEmbeddingNotFound`, `scorer.Embed(...,"digest text")`, `postEmbeddingRepo.Upsert(SummaryID=&"sum-1")`, `scoreRepo.Upsert(SummaryID=&"sum-1", PostID=nil)`; assert value | `internal/biz/score_test.go` | failing test | `CHK-01` | `EVID-01` | `go test ./internal/biz/ -run DigestBranch` → FAIL | `STEP-06` | none | — |
| `STEP-08` | agent | `REQ-01`,`REQ-02`,`REQ-03` | **GREEN**: `ScorePost` — если `event.SummaryID!=nil`: текст = `summaryRepo.Get(ctx,*event.SummaryID).Text`, ключ embedding/score = `summary_id`; иначе существующая post-ветка без изменений. `resolvePostVector`→`resolveVector(ctx, key VectorKey{PostID,SummaryID *string}, text, model)`; `postEmbeddingRepo.Get/Upsert` по ключу. `Cosine`/`Relevance` не трогаются | `internal/biz/score.go` | зелёный тест | `CHK-01`,`EC-01`,`SC-01` | `EVID-01` | `go test ./internal/biz/ -run DigestBranch` → PASS; +full `go test ./internal/biz/... -race` | `STEP-07` | none | post-ветка регрессирует (existing biz-тесты краснеют) |
| `STEP-09` | agent | `REQ-06`,`FM-03` | **RED→GREEN**: `EnqueueDue` (biz `ScoreUsecase`) дополнительно вызывает `outboxRepo.EnqueueDueDigests` (тест на mock); `interest.go` — в каждом из 3 `InTx` после `EnqueueScoredPosts` добавить `EnqueueScoredDigests` (тесты на 3 call site) | `internal/biz/score.go`,`interest.go`,`score_outbox_test.go`,`interest_test.go` | зелёные тесты | `CHK-01`,`SC-02` | `EVID-01` | `go test ./internal/biz/... -race` | `STEP-08` | none | rescore не атомарен с мутацией интереса |
| `CP-2` checkpoint | — | — | biz-слой двухключевой, digest-путь покрыт, post-путь не регрессирует | — | — | `CHK-01` | `EVID-01` | `go test ./internal/biz/... -race` зелёный | — | — | — |

### WS-3/WS-4 — data repos

| Step ID | Actor | Implements | Goal | Touchpoints | Artifact | Verifies | Evidence IDs | Check command / procedure | Blocked by | Needs approval | Escalate if |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `STEP-10` | agent | `CTR-04`,`REQ-01`,`REQ-06`,`CON-02` | `score_outbox_repo`: `digestCandidateSummaryIDs` (`SELECT su.id FROM summaries su WHERE su.post_id IS NULL AND NOT EXISTS(scores WHERE summary_id=su.id) AND NOT EXISTS(score_events active WHERE summary_id=su.id) LIMIT 500`); insert-const с `ON CONFLICT (summary_id) WHERE status IN('pending','processing') DO NOTHING`; `EnqueueDueDigests`/`EnqueueScoredDigests`; `ClaimOne`/`scanScoreEvent`/finalize читают `summary_id` (nullable→`*string`). Тест: digest enqueue находит due-summary, идемпотентен, claim отдаёт `SummaryID` | `internal/data/score_outbox_repo.go`,`score_outbox_repo_test.go` | зелёные data-тесты | `CHK-01`,`CHK-03`,`CTR-04` | `EVID-01` | `go test ./internal/data/ -run ScoreOutbox -race` | `CP-2` | none | claim-CTE ломает post-порядок |
| `STEP-11` | agent | `REQ-03`,`CTR-01` | `post_embedding_repo`: `Get`/`Upsert` по обоим ключам (`WHERE post_id=$1`/`WHERE summary_id=$1`; `ON CONFLICT` на соответствующий partial-index). Тест: round-trip по `summary_id` | `internal/data/post_embedding_repo.go`,`post_embedding_repo_test.go` | зелёные тесты | `CHK-01`,`CHK-03` | `EVID-01` | `go test ./internal/data/ -run PostEmbedding -race` | `CP-2` | none | — |
| `STEP-12` | agent | `CTR-01`,`REQ-04` | `score_repo.Upsert`: писать `post_id` или `summary_id` c соответствующим `ON CONFLICT` target; scan `*string`. Тест: Upsert по `summary_id` | `internal/data/score_repo.go`,`score_repo_test.go` | зелёный тест | `CHK-01` | `EVID-01` | `go test ./internal/data/ -run ScoreUpsert -race` | `CP-2` | none | — |
| `STEP-13` | agent | `REQ-04`,`CTR-03`,`SC-01`,`SC-02`,`EC-02` | `ListRanked`=`UNION ALL`: post-ветка **без изменений семантики** + digest-ветка (`scores s JOIN summaries su ON su.id=s.summary_id LEFT JOIN sources src ON src.id=su.source_id WHERE s.summary_id IS NOT NULL AND su.post_id IS NULL`, поля per `CTR-03`: `id=su.id`,`author=''`,`text=su.text`,`published_at=su.created_at`,`url=''`,`reaction=''`,`effective_value=s.value`,`kind='DIGEST'`). Единый ordering `effective_value DESC, id ASC` + keyset над union (`OQ-02`). Тесты: (a) digest-строка присутствует с верными полями; (b) **mixed keyset пагинация 2 стр. без дублей/пропусков** | `internal/data/score_repo.go`,`score_repo_test.go` | зелёные тесты | `CHK-01`,`SC-01`,`SC-02`,`EC-02` | `EVID-01` | `go test ./internal/data/... -race` | `STEP-10..12` | none | keyset даёт дубли/пропуски без изменения post-семантики (`OQ-02` эскалация) |
| `CP-3` checkpoint | — | — | data-слой отдаёт digest в ленту, enqueue/claim/embedding по `summary_id`, post-путь цел | — | — | `CHK-01`,`CHK-03` | `EVID-01` | `go test ./internal/... -race` зелёный | — | — | — |

### WS-5 — API + service + UI

| Step ID | Actor | Implements | Goal | Touchpoints | Artifact | Verifies | Evidence IDs | Check command / procedure | Blocked by | Needs approval | Escalate if |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `STEP-14` | agent | `CTR-02`,`REQ-05` | `feed.proto`: `enum FeedItemKind { FEED_ITEM_KIND_UNSPECIFIED=0; FEED_ITEM_KIND_POST=1; FEED_ITEM_KIND_DIGEST=2; }` + `FeedItemKind kind = 9;`; `make proto`; `biz.FeedItem.Kind`; `data.ListRanked` заполняет `Kind`; `service/feed` маппит. Тест: `feed_test.go` — POST/DIGEST маппинг | `api/feedium/feed.proto`, generated, `internal/biz/feed.go`, `internal/data/score_repo.go`, `internal/service/feed/feed.go`, `feed_test.go` | proto+маппинг | `CTR-02`,`EC-02`,`CHK-01` | `EVID-01`,`EVID-02` | `make proto` + `go test ./internal/service/... -race` | `CP-3` | none | proto-ломающее изменение (не аддитивное поле ≥9) |
| `STEP-15` | agent | `REQ-05`,`NS-05` | `ui/src/api.ts`: `FeedItem.kind` + union `'FEED_ITEM_KIND_POST'|'FEED_ITEM_KIND_DIGEST'|'FEED_ITEM_KIND_UNSPECIFIED'`; `FeedView.tsx`: скрыть `+`/`−` кнопки при `item.kind==='FEED_ITEM_KIND_DIGEST'`; `npm run build` | `ui/src/api.ts`,`ui/src/views/FeedView.tsx` | собранный UI | `NS-05`,`REQ-05` | `EVID-02` | `cd ui && npm run build` | `STEP-14` | `AG-02` | build падает / карточка ломается |
| `CP-4` checkpoint | — | — | Контракт `/v1/feed` аддитивно расширен `kind`, UI скрывает реакции для digest | — | — | `CHK-01` | `EVID-02` | сборка+тесты зелёные | — | — | `AG-02` |

### WS-6 — Verify & Close

| Step ID | Actor | Implements | Goal | Touchpoints | Artifact | Verifies | Evidence IDs | Check command / procedure | Blocked by | Needs approval | Escalate if |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `STEP-16` | agent | `EC-03`,`CHK-01`,`CHK-02`,`CHK-03` | Полный прогон + сохранить логи | вся кодовая база, `artifacts/ft-020/verify/chk-0{1,2,3}/` | evidence-логи | `CHK-01`,`CHK-02`,`CHK-03` | `EVID-01` | `go test ./... -race`, `make lint`, `make test` → логи в artifacts | `CP-4` | none | любой красный тест/линт |
| `STEP-17` | agent | simplify review | Проход на минимальную сложность (dup-ветки post/digest оправданы `CON-02`/`CTR-01`?), убрать premature abstraction | change surface | заметка review | — | — | самоаудит по `Test Ownership §5` | `STEP-16` | none | сложность не оправдана `CON/FM/DEC` |
| `STEP-18` | human+agent | `EC-04`,`MET-01`,`MET-02`,`CHK-04`,`CHK-05` | Деплой (миграция `cmd/migrate up` + код, `COMPOSE_FILE=docker-compose.server.yml`, `docker restart`); `curl "…/v1/feed?min_score=0&page_size=200"` → есть `kind:FEED_ITEM_KIND_DIGEST`; `git diff SKILL.md`=пусто | prod `oc`, `artifacts/ft-020/verify/chk-0{4,5}/` | prod-evidence | `EC-04`,`CHK-04`,`CHK-05` | `EVID-02` | curl-вывод + diff в artifacts | `STEP-17` | `AG-01` | миграция падает на прод / нет digest в ленте |
| `STEP-19` | agent | close | `feature.md`→`delivery_status: done` c evidence-ссылками; `implementation-plan.md`→`status: archived`; `README.md` актуализировать; `adr/index.md`/`features/index.md` при необходимости | memory-bank | обновлённые доки | Execution→Done gate | `EVID-01`,`EVID-02` | feature-flow §Execution→Done чеклист | `STEP-18` | none | остался незакрытый `CHK-*`/`EVID-*` |

## Parallelizable Work

- `PAR-01` `STEP-01`,`STEP-02`,`STEP-03` (три миграции) — независимые файлы, но применяются goose'ом последовательно; писать можно параллельно, `make test` — один раз после всех (см. `CLAUDE.md`: не параллелить мутирующие Bash).
- `PAR-02` `STEP-10`,`STEP-11`,`STEP-12` (три data-репо) — независимые файлы после `CP-2`.
- `PAR-03` НЕ распараллеливать: `STEP-13` (`ListRanked`) с остальными data-STEP — общий `score_repo.go`; `STEP-05` (типы) блокирует всё в biz/data — единый write-surface структур.

## Checkpoints

| Checkpoint ID | Refs | Condition | Evidence IDs |
| --- | --- | --- | --- |
| `CP-1` | `STEP-01..04`,`CHK-03` | Миграции применяются к чистой и seed-БД без ошибок; post-строки не меняются; проект компилируется | `EVID-01` |
| `CP-2` | `STEP-05..09`,`CHK-01` | biz двухключевой; digest-scoring покрыт; post-путь не регрессирует | `EVID-01` |
| `CP-3` | `STEP-10..13`,`CHK-01`,`CHK-03` | data отдаёт digest в `/v1/feed`; mixed-пагинация без дублей/пропусков | `EVID-01` |
| `CP-4` | `STEP-14..15` | `kind` в контракте аддитивно; UI скрывает реакции digest | `EVID-02` |
| `CP-5` | `STEP-16..17` | Все локальные suite зелёные; simplify review пройден | `EVID-01` |

## Execution Risks

| Risk ID | Risk | Impact | Mitigation | Trigger |
| --- | --- | --- | --- | --- |
| `ER-01` | `PostID string→*string` шире change surface, чем ожидалось (ломает несвязанные call site/тесты) | Разрастание диффа, регрессия | Держать deref локально; аддитивно; прогонять `go build ./...` после `STEP-05` до логики | компиляция падает вне change surface |
| `ER-02` | `ListRanked` keyset-инконсистентность при `UNION ALL` (`OQ-02`) → дубли/пропуски на границе страниц | `SC-02` красный, битая лента | Единый `effective_value`, обязательный mixed-pagination тест на 2 страницы | тест `SC-02` падает |
| `ER-03` | Миграция необратима при живых digest-строках (`OQ-03`) | Откат невозможен | Down удаляет `post_id IS NULL` строки перед `SET NOT NULL`; протестировать down в testcontainers | `goose down` падает на NOT NULL |
| `ER-04` | mockgen/ent regen не запущены → компиляция/тесты падают на старых mock | Ложный red | `go generate ./internal/...` после каждого изменения интерфейса/схемы | «mock does not implement» |
| `ER-05` | Прод-деплой неверным `COMPOSE_FILE` (история аутэйджа) | Прод-аутэйдж | `AG-01`: оператор, `COMPOSE_FILE=docker-compose.server.yml`, `docker restart` после (`project_tg_collector_restart`, `feedback_feedium_deploy_composefile`) | — |

## Stop Conditions / Fallback

| Stop ID | Related refs | Trigger | Immediate action | Safe fallback state |
| --- | --- | --- | --- | --- |
| `STOP-01` | `ADR-003` Риски,`OQ-03`,`ER-03` | Миграция падает на прод-БД | Остановить деплой, `cmd/migrate down` (удаляет digest-строки, восстанавливает `UNIQUE(post_id)`+`NOT NULL`) | Pre-FT-020: только посты скорятся, `/v1/feed` без digest |
| `STOP-02` | `OQ-02`,`ER-02`,`REQ-07` | mixed-pagination нельзя починить без изменения post-семантики | Остановиться, поднять в `feature.md`/`ADR-003` | biz digest-scoring есть, но `ListRanked` не мержится → откат `STEP-13`, digest не в ленте (как FT-019 путь B) |
| `STOP-03` | `EC-04`,`CHK-04` | После деплоя digest не появляется в `/v1/feed?min_score=0` | Не закрывать фичу; диагностировать enqueue/claim/ListRanked на проде | `delivery_status: in_progress`, эскалация оператору |

## Готово для приемки

План исчерпан, когда: все `STEP-01..19` выполнены; `CP-1..5` пройдены; `CHK-01..CHK-05` имеют
pass-evidence в `artifacts/ft-020/verify/`; `EVID-01`/`EVID-02` заполнены конкретными путями;
simplify review (`STEP-17`) выполнен; прод-verify (`STEP-18`) подтверждён под `AG-01`; после чего
переход на финальную приёмку по секции `Verify` в `feature.md` и Execution→Done gate feature-flow.

## Execution Status (2026-07-01)

Код-полностью реализовано и локально верифицировано. Резюме: `artifacts/ft-020/verify/SUMMARY.md`.

| Item | Status |
| --- | --- |
| `STEP-01..03` migrations, `STEP-04` ent | done — `CP-1` пройден (goose применяет 3 миграции к fresh+seed PG) |
| `STEP-05..09` biz (two-key structs, `ScorePost` digest, `resolveVector`, `EnqueueDue`, interest rescore) | done — `CP-2` (biz `-race` green) |
| `STEP-10..13` data (outbox digest enqueue, `post_embedding` two-key, `score_repo.Upsert`+`ListRanked` UNION ALL) | done — `CP-3` (data+task `-race` green, mixed keyset pagination `SC-02` proven) |
| `STEP-14..15` proto `kind`/service map/UI hide reactions | done — `CP-4` (proto regen, `go build`, `ui` `tsc` build green) |
| `STEP-16` verify | `CHK-01` PASS, `CHK-02` PASS (FT-020 files clean; residual pre-existing), `CHK-03` PASS, `CHK-05` PASS |
| `STEP-17` simplify review | done |
| `STEP-18` prod-verify `CHK-04` | done — деплой `openclaw@oc` (`COMPOSE_FILE=docker-compose.server.yml`) 2026-07-01 17:10Z; 3 миграции применены к `core_storage_pg` (v20260701120200); `/v1/feed` отдаёт `kind=FEED_ITEM_KIND_DIGEST` (score 0.63); backlog скорится worker pool'ом (`EnqueueDueDigests`) |
| `STEP-19` close (feature.md→done, plan→archived) | done |

`OQ-01` resolved: `ScoreEvent.PostID`→`*string`; `task/score_event_worker_pool.go` лог использует
новый `ScoreEvent.EntityID()` helper (одна строка, поле лога `post_id`→`entity_id`).
`OQ-02` resolved: единый `effective_value` ordering + `value` keyset over `UNION ALL`; post-семантика
не изменена (like-boost сохранён); `SC-02` mixed-pagination тест зелёный.
`OQ-03` resolved: Down-миграции удаляют `post_id IS NULL` строки перед `SET NOT NULL`.
