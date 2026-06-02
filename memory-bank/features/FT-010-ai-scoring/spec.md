---
title: "FT-010-ai-scoring: AI Scoring & Ranking — Technical Spec"
doc_kind: feature
doc_function: derived
status: draft
delivery_status: planned
derived_from:
  - feature.md
  - ../../domain/architecture.md
  - ../../engineering/coding-style.md
  - ../../engineering/go-style.md
  - ../../engineering/api-contracts.md
  - ../../engineering/database.md
  - ../../engineering/testing-policy.md
  - ../FT-007-scalable-event-processing/feature.md
  - ../../adr/ADR-001-scoring-method-and-vector-storage.md
---

# FT-010-ai-scoring — Technical Spec

Technical-исполнимая спецификация поверх [`feature.md`](feature.md). Резолвит Design-фазу
`OQ-01..07` feature.md как `DEC-01..07`. Scope, продуктовые контракты (`CTR-01..07`),
acceptance scenarios (`SC-01..09`), checks (`CHK-01..08`) и evidence (`EVID-01..08`) — owned
by [`feature.md`](feature.md), здесь только ссылки по ID и technical-расширения.

## Solution

Вводим две управляемые сущности и downstream-конвейер скоринга поверх уже построенного
PostgreSQL-каркаса FT-007. `Interest` — глобальный для инстанса набор интересов с CRUD-поверхностью,
повторяющей FT-003 (`REQ-01`); при создании/изменении интереса его текст один раз кодируется в
embedding-вектор через внешний AI-клиент и кэшируется в БД. `Score` — ровно одна актуальная скалярная
оценка релевантности на пост (`REQ-02`, `REQ-05`), гарантируемая `UNIQUE(post_id)` + upsert. Скоринг
запускается автоматически downstream события готовности `Summary`: отдельный `score_due_scheduler`
(`task/`) находит просуммированные, но ещё не оценённые посты и кладёт `score_event` в новый outbox;
пул воркеров `score_event` (точная калька `EventWorkerPool` FT-007 — `ClaimOne` через
`FOR UPDATE SKIP LOCKED`, lease + heartbeat, retry с backoff, reaper зависших) кодирует текст Summary
в embedding, считает в `biz/` чистой функцией косинусную близость к интересам и сохраняет максимум как
оценку (`REQ-02`, `REQ-03`). Отранжированная лента — новый read-контракт (proto + service + feediumctl),
отдающий посты `ORDER BY score.value DESC` с фильтрами по времени и порогу (`REQ-04`). Ключевой
trade-off (`DEC-01`/`DEC-02`): берём **embeddings + косинус**, а не LLM-судью, и храним **скалярную
оценку плюс кэш embedding'ов интересов как JSONB**, а не вводим pgvector/vector-store — это укладывается
в экономику провайдера при потоке 10k постов/день (`CON-01`, `RISK-02`) и в «PostgreSQL-only»
инфраструктуру MVP (`REQ-07`, `CON-04`) ценой того, что ретроактивный пере-скоринг при смене интересов
не делается (это и так `NS-05`), а лента ранжируется по предвычисленному скаляру.

## Change Surface

| Path/Component | Layer | Why | REQ |
| --- | --- | --- | --- |
| `api/feedium/interest.proto` (new) | api | CRUD-контракт интересов, форма как `SourceService` (`source.proto:105-137`) | `REQ-01` |
| `api/feedium/feed.proto` (new) | api | Read-контракт отранжированной ленты | `REQ-04` |
| `api/feedium/error_reason.proto:1` (modify) | api | Новые reasons `INTEREST_NOT_FOUND`, `INVALID_THRESHOLD` | `REQ-01`, `REQ-04` |
| `internal/ent/schema/interest.go` (new) | ent/schema | Таблица `interests` + кэш embedding | `REQ-01`, `REQ-07` |
| `internal/ent/schema/score.go` (new) | ent/schema | Таблица `scores`, `UNIQUE(post_id)` | `REQ-02`, `REQ-05`, `REQ-07` |
| `internal/ent/schema/score_event.go` (new) | ent/schema | Outbox-таблица событий скоринга (калька `summary_event.go`) | `REQ-02`, `REQ-06` |
| `migrations/<ts>_create_interests_table.sql` (new) | migrations | DDL `interests` | `REQ-01`, `REQ-07` |
| `migrations/<ts>_create_scores_table.sql` (new) | migrations | DDL `scores` + уникальный/ранжирующий индексы | `REQ-04`, `REQ-05`, `REQ-07` |
| `migrations/<ts>_create_score_events_table.sql` (new) | migrations | DDL `score_events` + lease/claim/uniqueness индексы | `REQ-02`, `REQ-05`, `REQ-06` |
| `internal/biz/interest.go` (new) | biz | `Interest`, `InterestRepo`, `Scorer` порт, `InterestUsecase` | `REQ-01` |
| `internal/biz/score.go` (new) | biz | `Score` VO, `ScoreRepo`, чистый `Relevance`/`Cosine`, `ScoreUsecase` | `REQ-02`, `REQ-03`, `REQ-05` |
| `internal/biz/score_outbox.go` (new) | biz | `ScoreOutboxRepo`, типы/статусы `score_event`, sentinels, `RetryPolicy` | `REQ-02`, `REQ-06` |
| `internal/biz/feed.go` (new) | biz | `FeedUsecase`, `FeedQueryRepo`, `DateRange`/`Threshold` VO | `REQ-04` |
| `internal/biz/wire.go:1` (modify) | biz | Регистрация новых usecase/provider'ов | `REQ-01`, `REQ-02`, `REQ-04` |
| `internal/data/interest_repo.go` (new) | data | Реализация `InterestRepo` (Ent) | `REQ-01` |
| `internal/data/score_repo.go` (new) | data | Upsert актуальной оценки + ранжированный запрос ленты | `REQ-04`, `REQ-05` |
| `internal/data/score_outbox_repo.go` (new) | data | Claim/lease/retry/finalize (калька `summary_outbox_repo.go`) | `REQ-02`, `REQ-05`, `REQ-06` |
| `internal/data/embedding_provider.go` (new) | data | Реализация `Scorer` через OpenAI-совместимый embeddings endpoint | `REQ-02`, `REQ-06` |
| `internal/data/wire.go:1` (modify) | data | Регистрация новых repo/клиента | `REQ-01`, `REQ-02`, `REQ-04` |
| `internal/service/interest/` (new) | service | proto DTO → `InterestUsecase` | `REQ-01` |
| `internal/service/feed/` (new) | service | proto DTO → `FeedUsecase` | `REQ-04` |
| `internal/task/score_due_scheduler.go` (new) | task | Enqueue `score_event` для summarized-unscored постов | `REQ-02` |
| `internal/task/score_event_worker_pool.go` (new) | task | Пул скоринг-воркеров (lease/retry/heartbeat) | `REQ-02`, `REQ-05`, `REQ-06` |
| `internal/task/stuck_score_event_reaper.go` (new) | task | Санация зависших `score_event` (калька `stuck_event_reaper.go`) | `REQ-06` |
| `internal/task/wire.go:1` (modify) | task | Регистрация воркеров как `transport.Server` | `REQ-02`, `REQ-06` |
| `internal/server/grpc.go:1`, `internal/server/http.go:1` (modify) | server | Регистрация `InterestService`/`FeedService` | `REQ-01`, `REQ-04` |
| `internal/conf/conf.proto:14` (modify) | configs | Новое поле `Scoring scoring` + message-контракт | `REQ-02`, `REQ-06` |
| `configs/config.yaml:16`, `configs/local.example.yaml` (modify) | configs | Блок `scoring.*` (provider/worker/scheduler/reaper/threshold) | `REQ-02`, `REQ-06` |
| `cmd/feediumctl/internal/app/interest.go` (new) | cmd | CLI CRUD интересов | `REQ-01` |
| `cmd/feediumctl/internal/app/feed.go` (new) | cmd | CLI отранжированной ленты | `REQ-04` |
| `cmd/feediumctl/internal/app/root.go:1` (modify) | cmd | Регистрация команд `interest`/`feed` | `REQ-01`, `REQ-04` |
| `cmd/feedium/wire.go:1` (modify) | cmd | Wire provider set новых слоёв | `REQ-01`, `REQ-02`, `REQ-04` |

## Files

Полный whitelist для handoff-верификации working tree. Пути вне списка при реализации не трогать.
Сгенерированные артефакты (`ent/`, `*.pb.go`, `wire_gen.go`, `mock/`) перечислены явно как
производимые `go generate ./...` / `make proto` (`coding-style.md §Кодогенерация`).

**api**
- `api/feedium/interest.proto` (new)
- `api/feedium/feed.proto` (new)
- `api/feedium/error_reason.proto` (modify)
- `api/feedium/interest.pb.go`, `api/feedium/interest_grpc.pb.go`, `api/feedium/interest_http.pb.go` (new, generated)
- `api/feedium/feed.pb.go`, `api/feedium/feed_grpc.pb.go`, `api/feedium/feed_http.pb.go` (new, generated)
- `api/feedium/error_reason.pb.go` (regenerated)

**ent/schema + generated ent**
- `internal/ent/schema/interest.go` (new)
- `internal/ent/schema/score.go` (new)
- `internal/ent/schema/score_event.go` (new)
- `internal/ent/interest/`, `internal/ent/score/`, `internal/ent/scoreevent/` (new, generated packages)
- `internal/ent/client.go`, `internal/ent/ent.go`, `internal/ent/mutation.go`, `internal/ent/runtime.go`,
  `internal/ent/runtime/runtime.go`, `internal/ent/migrate/schema.go`, `internal/ent/hook/hook.go`,
  `internal/ent/predicate/predicate.go` (regenerated) + новые `internal/ent/interest*.go`,
  `internal/ent/score*.go`, `internal/ent/scoreevent*.go` CRUD-файлы (generated)

**migrations**
- `migrations/<ts>_create_interests_table.sql` (new)
- `migrations/<ts>_create_scores_table.sql` (new)
- `migrations/<ts>_create_score_events_table.sql` (new)

**biz**
- `internal/biz/interest.go`, `internal/biz/interest_test.go` (new)
- `internal/biz/score.go`, `internal/biz/score_test.go` (new)
- `internal/biz/score_outbox.go`, `internal/biz/score_outbox_test.go` (new)
- `internal/biz/feed.go`, `internal/biz/feed_test.go` (new)
- `internal/biz/mock/interest_mock.go`, `internal/biz/mock/score_mock.go`,
  `internal/biz/mock/score_outbox_mock.go`, `internal/biz/mock/feed_mock.go` (new, generated)
- `internal/biz/wire.go` (modify)

**data**
- `internal/data/interest_repo.go`, `internal/data/interest_repo_test.go` (new)
- `internal/data/score_repo.go`, `internal/data/score_repo_test.go` (new)
- `internal/data/score_outbox_repo.go`, `internal/data/score_outbox_repo_test.go` (new)
- `internal/data/embedding_provider.go`, `internal/data/embedding_provider_test.go` (new)
- `internal/data/wire.go` (modify)

**service**
- `internal/service/interest/interest.go`, `internal/service/interest/interest_test.go`,
  `internal/service/interest/wire.go`, `internal/service/interest/mock/mock_usecase.go` (new)
- `internal/service/feed/feed.go`, `internal/service/feed/feed_test.go`,
  `internal/service/feed/wire.go`, `internal/service/feed/mock/mock_usecase.go` (new)

**task**
- `internal/task/score_due_scheduler.go`, `internal/task/score_due_scheduler_test.go` (new)
- `internal/task/score_event_worker_pool.go`, `internal/task/score_event_worker_pool_test.go` (new)
- `internal/task/stuck_score_event_reaper.go`, `internal/task/stuck_score_event_reaper_test.go` (new)
- `internal/task/wire.go` (modify)

**server**
- `internal/server/grpc.go` (modify)
- `internal/server/http.go` (modify)

**configs / conf**
- `internal/conf/conf.proto` (modify)
- `internal/conf/conf.pb.go` (regenerated)
- `configs/config.yaml` (modify)
- `configs/local.example.yaml` (modify)

**cmd**
- `cmd/feediumctl/internal/app/interest.go`, `cmd/feediumctl/internal/app/interest_test.go` (new)
- `cmd/feediumctl/internal/app/feed.go`, `cmd/feediumctl/internal/app/feed_test.go` (new)
- `cmd/feediumctl/internal/app/root.go` (modify)
- `cmd/feedium/wire.go` (modify)
- `cmd/feedium/wire_gen.go` (regenerated)

## Flow

### F1. Управление интересом (`REQ-01`, `SC-01`)
1. **Вход:** HTTP/gRPC запрос `V1CreateInterest` (или CLI `feediumctl interest create`).
2. `service/interest` маппит proto DTO → `biz.Interest`, вызывает `InterestUsecase.Create`.
3. `InterestUsecase.Create` вызывает порт `Scorer.Embed(ctx, topic)` → embedding-вектор, формирует
   `Interest{Topic, Embedding}` (чистая сборка, без мутации входа — `coding-style.md §Чистые функции`),
   вызывает `InterestRepo.Save`.
4. `data/interest_repo` пишет строку `interests` (Ent), embedding в JSONB.
5. **Выход:** proto `V1CreateInterestResponse` с `Interest`. Аналогично List/Update/Delete; Update
   пере-кодирует topic. Эффект на оценку — через F2 для постов, оцениваемых после изменения (`SC-08`).

### F2. Авто-скоринг просуммированного поста (`REQ-02`, `REQ-03`, `REQ-05`, `SC-02`)
1. **Вход (cron tick):** `score_due_scheduler` опрашивает БД с `poll_interval` (как
   `source_due_scheduler.go`), вызывает `ScoreOutboxRepo.EnqueueDuePosts`.
2. `data/score_outbox_repo` находит посты, у которых есть `summaries` и нет `scores` и нет активного
   `score_event`, и вставляет `score_event{status=pending}` с `INSERT ... ON CONFLICT DO NOTHING` по
   уникальному частичному индексу `(post_id) WHERE status IN ('pending','processing')` (`INV-04`).
3. **Вход (worker loop):** `score_event_worker_pool` (N горутин) вызывает
   `ScoreOutboxRepo.ClaimOne` → `FOR UPDATE SKIP LOCKED`, ставит `status=processing`, `locked_by`,
   `locked_until`, `attempt_count++` (`INV-03`).
4. Воркер вызывает `ScoreUsecase.ScorePost(ctx, event)`: usecase читает текст `Summary` поста
   (`ASM-01`) и текущие `Interest`-embedding'и, вызывает `Scorer.Embed(ctx, summaryText)`, считает
   чистой функцией `Relevance(postEmb, interestEmbs)` (max косинус, нормировка в `[0,1]`), вызывает
   `ScoreRepo.Upsert(ctx, Score{PostID, Value, Model})`.
5. `data/score_repo.Upsert` — `INSERT ... ON CONFLICT (post_id) DO UPDATE` (`INV-02`, `CTR-02`).
6. Воркер `FinalizeWithLease(status=completed)` под защитой lease (heartbeat продлевает lease).
7. **Выход:** у поста ровно одна `Score`; нерелевантный пост получает `Value≈0` и уходит в хвост (F3).
   `slog` строка `score completed` с `post_id`, `score_event_id` (`coding-style.md §Логирование`).

### F3. Отранжированная лента (`REQ-04`, `SC-03`, `SC-04`)
1. **Вход:** `V1ListFeed` (HTTP `GET /v1/feed?from=&to=&min_score=&page_size=&page_token=`) или CLI
   `feediumctl feed list`.
2. `service/feed` маппит DTO → `biz.FeedQuery{DateRange, Threshold, Pagination}`, валидирует порог в
   `[0,1]` (иначе `INVALID_THRESHOLD`), вызывает `FeedUsecase.RankedFeed`.
3. `FeedUsecase.RankedFeed` вызывает `FeedQueryRepo.ListRanked`.
4. `data/score_repo.ListRanked` — join `posts`⋈`scores`, `WHERE published_at` в диапазоне `AND value
   >= min_score`, `ORDER BY value DESC, post.id`, keyset-пагинация (`data/page_token.go`).
5. **Выход:** proto `V1ListFeedResponse` (посты по убыванию оценки) / машиночитаемый вывод CLI как у
   `summary`/`post` команд. Непросуммированные/непровалидированные посты в ленту не попадают, но
   доступны по существующему `V1GetPost` (`DEC-06`, `FM-02`).

## Contracts

Технические контракты, owned by этим spec. Продуктовые `CTR-01..07` — в [`feature.md`](feature.md).

- `CTR-08` **proto interest/feed.** Новые `api/feedium/interest.proto` (`InterestService`: V1Create/V1List/
  V1Get/V1Update/V1DeleteInterest, форма как `SourceService`) и `api/feedium/feed.proto`
  (`FeedService.V1ListFeed`, `GET /v1/feed`). Имена методов и messages с **V-префиксом**, версия в
  HTTP-пути, package `feedium`, suffix `Response`, пустые запросы — свой message
  ([`api-contracts.md §Версионирование`, `§Правила proto-файлов`](../../engineering/api-contracts.md)).
  Реализует `CTR-01`, `CTR-04`.
- `CTR-09` **Ent schema + goose миграции (раздельно).** `interests`, `scores`, `score_events` —
  отдельные таблицы; PK `UUID v7` (Ent Default hook, не `gen_random_uuid()`), `TIMESTAMPTZ` для дат,
  FK на `posts(id)` обязательны, JSONB для embedding. Ent-схема и goose up/down пишутся руками
  раздельно, `go generate ./internal/ent` после ([`database.md §Ent + Goose`, `§PostgreSQL`,
  `§Constraints`](../../engineering/database.md)). `posts` структурно не меняется — `post_id` хранится
  плоским UUID-полем без edge на `Post` (прецедент `summary_event.go`), `CON-05`/`CTR-06`. Реализует
  `CTR-06`.
- `CTR-10` **`scores`.** `id UUID PK`, `post_id UUID NOT NULL REFERENCES posts(id)`,
  `value DOUBLE PRECISION NOT NULL` (диапазон `[0,1]`, `DEC-03`), `model TEXT NOT NULL`,
  `created_at`/`updated_at TIMESTAMPTZ`. `CREATE UNIQUE INDEX ON scores(post_id)` (`CON-03`, `CTR-02`,
  `INV-02`); `CREATE INDEX ON scores(value DESC)` для ранжированной выдачи (`REQ-04`).
- `CTR-11` **`score_events` (outbox).** Структурная калька `summary_events`: `id`, `post_id`, `status`
  (`pending|processing|completed|failed|expired`), `error`, `attempt_count`, `locked_by`,
  `locked_until`, `next_attempt_at`, `created_at`, `processed_at`. Индексы: `(status, created_at)`;
  частичный `UNIQUE(post_id) WHERE status IN ('pending','processing')` (`INV-04`). Производит
  scheduler, потребляет worker pool. Реализует `CTR-03`.
- `CTR-12` **`interests`.** `id UUID PK`, `topic TEXT NOT NULL`, `embedding JSONB NOT NULL`,
  `model TEXT NOT NULL`, `created_at`/`updated_at TIMESTAMPTZ`. CRUD-поверхность зеркалит FT-003
  (`ASM-03`). Реализует `CTR-01`, `CTR-05`.
- `CTR-13` **config `scoring.*`.** Новый message `Scoring` в `internal/conf/conf.proto`
  (`Bootstrap.scoring = 5`), под-сообщения по образцу `Summary`:
  `worker{poll_interval=5s, concurrency=4, lease_ttl=5m, heartbeat_interval=1m, graceful_timeout=10s,
  max_attempts=5, backoff_base=10s, backoff_max=10m}`, `scheduler{poll_interval=5s}`,
  `reaper{interval=1m, grace=30s}`, `embedding{timeout=30s, max_retries=3, provider, providers{...},
  model}`, `default_threshold=0.0` (`DEC-07`). Парсинг через kratos config; секреты — не коммитить
  ([`coding-style.md §Конфигурация`](../../engineering/coding-style.md)). Реализует `CTR-07`.
- `CTR-14` **`Scorer` порт.** Интерфейс в `biz/` (`Embed(ctx, text) ([]float32, error)`), реализация
  в `data/embedding_provider.go`; интерфейс на стороне потребителя, `var _ biz.Scorer = (*...)(nil)`
  ([`coding-style.md §Интерфейсы по слоям`, `go-style.md §Интерфейсы`](../../engineering/coding-style.md)).
  Transient-сбой → ошибка наверх в `task/`, где решается retry (`CTR-07`, `FM-01`).

## Invariants

Технические инварианты; каждый наблюдаем через указанный `CHK-*` из [`feature.md`](feature.md).

- `INV-01` **Слоевые границы.** `biz/` не импортирует ent/http/sql/proto; `service/` обращается только к
  `biz/` usecase, не к `data/`; `task/` вызывает `biz/`, не `data/` напрямую
  ([`architecture.md §Module Boundaries`](../../domain/architecture.md)). Наблюдаемо: `make lint` +
  отсутствие запрещённых импортов (`CHK-08`).
- `INV-02` **Все записи оценок — через `biz.ScoreRepo.Upsert`**, одна актуальная строка на пост
  (`UNIQUE(post_id)`). Никаких прямых INSERT мимо repo. Наблюдаемо: `CHK-05`.
- `INV-03` **Захват события — атомарный** (`FOR UPDATE SKIP LOCKED` + lease), финализация под защитой
  lease; leaf-функции `biz/`/`data/` не логируют, лог/retry/state — только в `task/`
  ([`architecture.md §Failure Handling`](../../domain/architecture.md)). Наблюдаемо: `CHK-05`, `CHK-06`.
- `INV-04` **Не более одного активного `score_event` на пост** (частичный UNIQUE), enqueue
  идемпотентен (`ON CONFLICT DO NOTHING`). Наблюдаемо: `CHK-02`, `CHK-05`.
- `INV-05` **`Relevance`/`Cosine` — чистые функции** в `biz/`: без побочных эффектов, не мутируют вход,
  детерминированы при фиксированных embedding'ах ([`coding-style.md §Чистые функции`](../../engineering/coding-style.md)).
  Наблюдаемо: unit-тесты `score_test.go` (`CHK-04`).
- `INV-06` **Пост не теряется.** Ни transient-, ни permanent-сбой скоринга не удаляет пост и не
  блокирует его последующее появление в ленте; факт сбоя — в `score_events.status/error` + `slog`.
  Наблюдаемо: `CHK-06`.
- `INV-07` **FT-005/FT-007 не модифицируются** (`NS-08`): summary-транзакция, `SummaryEvent`,
  `next_summary_at`, summary worker lifecycle не трогаются; скоринг — отдельный downstream-конвейер.
  Наблюдаемо: отсутствие изменений вне `## Files` (handoff-верификация) + зелёные FT-005/FT-007 тесты
  (`CHK-08`).
- `INV-08` **Конфиг-дефолты не хардкодятся в leaf-функциях.** Параметры воркера/scheduler/reaper и
  порог читаются из `conf.Scoring` с безопасными fallback-константами (как `defaultPollInterval` в
  `event_worker_pool.go`). Наблюдаемо: `CHK-02`, `CHK-03`.

## Concurrency & Lifecycle

Скоринг добавляет три `transport.Server` в `task/`, регистрируемых через kratos lifecycle (Wire,
`cmd/feedium/wire.go` → `kratos.Server(...)`), shutdown — автоматический через `Stop()`
([`coding-style.md §Graceful Shutdown`, `architecture.md §Воркеры`](../../domain/architecture.md)).

- **`score_event_worker_pool`** — точная калька `EventWorkerPool` (`event_worker_pool.go`): `Start`
  запускает `concurrency` горутин через `wg.Go`, каждая в цикле `ClaimOne`→`dispatch`→`finalize`;
  per-event heartbeat-горутина продлевает lease и отменяет `processCtx` при `ErrLeaseLost`; `Stop`
  закрывает `stopCh`, ждёт `wg` до `graceful_timeout`, затем `cancel()`. Shared state — только через
  PostgreSQL (`architecture.md §Concurrency`).
- **`score_due_scheduler`** — калька `source_due_scheduler.go`: тикер с `poll_interval`, один enqueue-
  проход за тик, реагирует на `ctx.Done()`/`stopCh`.
- **`stuck_score_event_reaper`** — калька `stuck_event_reaper.go`: периодически переводит
  `processing` с истёкшим `locked_until` обратно в `pending` (или `expired` по TTL), `grace` из конфига.
- **goroutine leak coverage:** каждый воркерный тест содержит
  `defer goleak.VerifyNone(t, goleak.IgnoreCurrent())` (`testing-policy.md §Goroutine Leak Detection`),
  evidence `EVID-06` (`CHK-06`).

## Failure Modes

Дополняют `FM-01..06` [`feature.md`](feature.md) техническими detection/recovery; retry-политика — по
[`architecture.md §Retry policy`](../../domain/architecture.md) (exponential backoff, конфигурируемые
`max_attempts`/`backoff_base`/`backoff_max`).

- `FM-01` (feature) **Embeddings endpoint таймаут/5xx/обрыв** (`RISK-05`). Detection: ошибка из
  `Scorer.Embed` наверх в `task/`. Recovery: `policy.ShouldRetry` → `MarkForRetry` с backoff; по
  исчерпании `max_attempts` → `FinalizeWithLease(failed)`. Пост сохранён (`INV-06`).
- `FM-02` (feature) **Permanent-ошибка провайдера (4xx/валидация).** Detection: классификация ошибки
  как non-retryable в `biz`. Recovery: сразу `failed`, без лишних попыток; пост доступен по `V1GetPost`,
  в ленту не попадает до успешного скоринга (`DEC-06`).
- `FM-03` (feature) **Гонка воркеров за один пост.** `FOR UPDATE SKIP LOCKED` + `UNIQUE(post_id)` на
  `scores` → ровно одна оценка, 0 дубликатов (`INV-02`, `INV-03`, `CHK-05`).
- `FM-04` (feature) **Повторная оценка / recovery после crash / смена интересов.** Upsert обновляет
  оценку на месте (`CTR-02`, `INV-02`).
- `FM-05` (feature) **10k постов/день на платном провайдере** (`RISK-02`). Митигировано `DEC-01`
  (embeddings ≪ LLM-судья) + `DEC-04` (кэш embedding'ов интересов: 1 embed-вызов на пост, не на пару
  пост×интерес); деградация наблюдаема через рост `score_events.pending`.
- `FM-06` (feature) **Явно нерелевантный контент.** `Value≈0`, хвост ленты, доступен по get (`REQ-03`).
- `FM-07` (technical) **Дубль enqueue от scheduler'а / гонка scheduler×worker.** Detection: частичный
  `UNIQUE(post_id)` на активных `score_events`. Recovery: `ON CONFLICT DO NOTHING` (`INV-04`), без
  ошибки.
- `FM-08` (technical) **Malformed embedding response (пустой/несовпадающая размерность).** Detection:
  валидация длины вектора в `data/embedding_provider` (ошибка) или в `biz.Cosine` при несовпадении
  размерностей (доменная ошибка). Recovery: классифицируется как permanent (`FM-02`) → `failed`,
  пост сохранён.
- `FM-09` (technical) **Интересы не заданы (пустой набор).** Detection: `len(interestEmbs)==0` в
  `ScoreUsecase`. Recovery: пост получает низшую оценку `Value=0` (`DEC-03`), скоринг завершается
  `completed` (не сбой); пост остаётся доступным и пере-оценится при появлении интересов через F2.

## Tests

Ownership: `SC-*`/`CHK-*`/`EVID-*` owned by [`feature.md`](feature.md); ниже — стратегия исполнения
(`testing-policy.md`). TDD для `biz/`, test-after для `data/`/`service/`/`task/`; AAA, table-driven,
mockgen, testcontainers, goleak.

**Unit (biz/, TDD Red-Green-Refactor — `autonomy-boundaries.md §TDD`):**
- `interest_test.go`: `InterestUsecase` Create/List/Update/Delete; Create/Update вызывают `Scorer.Embed`
  (mock); валидация пустого topic. → `SC-01`.
- `score_test.go`: чистые `Cosine`/`Relevance` (table-driven: ортогональные/идентичные/нерелевантные
  векторы, нормировка в `[0,1]`, пустой набор интересов → 0 — `FM-09`); `ScoreUsecase.ScorePost` с
  mock `Scorer`/`ScoreRepo`/`InterestRepo` (happy, transient-err, permanent-err классификация). →
  `SC-02`, `SC-04`, `SC-07`.
- `feed_test.go`: `FeedUsecase.RankedFeed` маппинг/валидация порога с mock `FeedQueryRepo`. → `SC-03`.
- `score_outbox_test.go`: `RetryPolicy.ShouldRetry`/`CalculateBackoff` (границы `max_attempts`,
  transient vs permanent). → `SC-07`.
- Моки: `//go:generate mockgen` для `Scorer`, `ScoreRepo`, `InterestRepo`, `ScoreOutboxRepo`,
  `FeedQueryRepo` в `internal/biz/mock/` (`testing-policy.md §Моки`).

**Integration (data/, testcontainers PostgreSQL — test-after, >80%):**
- `interest_repo_test.go`: CRUD + JSONB embedding round-trip.
- `score_repo_test.go`: `Upsert` идемпотентность (`UNIQUE(post_id)`, повтор не плодит дубликат);
  `ListRanked` — порядок по `value DESC`, фильтры времени и порога, keyset-пагинация. → `CHK-03`,
  `CHK-05`.
- `score_outbox_repo_test.go`: `ClaimOne` (`FOR UPDATE SKIP LOCKED`), lease/`ExtendLease`/
  `MarkForRetry`/`FinalizeWithLease`, `EnqueueDuePosts` идемпотентность (`INV-04`); конкурентный
  claim M×N без двойной обработки (`-race`). → `CHK-02`, `CHK-05`.
- `embedding_provider_test.go`: `httptest.Server` с фикстурами (200 / 5xx / malformed) —
  `testing-policy.md §data/ §Когда Manual-Only`. → `CHK-06`.

**Service-level (proto contract, минимально):**
- `service/interest`, `service/feed`: маппинг DTO↔domain с mock usecase; коды ошибок
  (`INTEREST_NOT_FOUND`, `INVALID_THRESHOLD`). → `SC-01`, `SC-03`.

**Task-level (integration, testcontainers + goleak):**
- `score_event_worker_pool_test.go`: полный flow воркер→biz→data→DB; lifecycle Start/Stop;
  transient→retry→success и permanent→failed (mock `Scorer`); конкурентность без дубликатов;
  `goleak.VerifyNone`. → `CHK-02`, `CHK-05`, `CHK-06`.
- `score_due_scheduler_test.go`: enqueue только summarized-unscored; повторный тик не дублирует.
- `stuck_score_event_reaper_test.go`: возврат зависших `processing`→`pending`/`expired`. → `CHK-06`.
- `feediumctl` `interest_test.go`/`feed_test.go`: CLI через фейковый client factory (как
  `summary_test.go`), машиночитаемый вывод. → `CHK-01`, `CHK-03`.

**Manual-only gaps:** живой платный embeddings-провайдер не вызывается в CI — подменяется mock/
`httptest` (детерминированный double, `feature.md §Verify`). Реальный прогон против провайдера —
manual smoke вне CI; не блокирует Done, regression покрыт mock'ами. Требует approval gate в
`implementation-plan.md` (`AG-*`) при первом живом вызове.

## Decisions

- `DEC-01` **Метод скоринга — embeddings + косинусная близость, не LLM-судья** (резолвит `OQ-01`).
  Alternatives: (a) LLM-судья per-post — выше качество нюансов, но ~×100 стоимость и латентность,
  бьёт по `RISK-02`/`MET-03`; (b) гибрид — сложнее, преждевременно для MVP. Выбор (embeddings)
  минимизирует стоимость/латентность при потоке 10k/день (`CON-01`, `FM-05`) и переиспользует
  существующий provider-паттерн (`data/llm_provider.go`). Обратимость: оценка скалярна (`DEC-03`),
  смена метода не ломает контракт ленты.
- `DEC-02` **Хранение — скалярная оценка + кэш embedding'ов интересов в JSONB; без pgvector**
  (резолвит `OQ-02`). Alternatives: (a) pgvector + `vector(N)` + ANN — новая инфра-зависимость
  (расширение PostgreSQL), избыточна, т.к. ретро-пере-скоринг вне scope (`NS-05`) и лента
  ранжируется по предвычисленному скаляру; (b) хранить post-embedding — раздувает БД без пользы при
  скалярной оценке. Выбор укладывается в «PostgreSQL-only» (`REQ-07`, `CON-04`), не вводит
  зависимостей (`go-style.md §least dependency`). Косинус считается в `biz/` (`INV-05`). Необратимая
  часть (миграции) — `AG-*` в плане (`autonomy-boundaries.md §Супервизия`).
- `DEC-03` **Формат оценки — `float64` в `[0,1]`, «низшая» = `0.0`, порог = min-value фильтр**
  (резолвит `OQ-03`). Косинус `[-1,1]` нормируется `(x+1)/2` либо clamp negatives→0; нерелевантный/
  без-интересов → `0` (`FM-06`, `FM-09`). Простая, монотонная, machine-friendly семантика.
- `DEC-04` **Агрегация — max косинус по интересам** (резолвит `OQ-05`). Пост релевантен, если близок
  хотя бы к одному интересу. Alternatives: mean (размывает узкие интересы). Интересы — свободный
  текст с кэшированным embedding; смена интересов влияет на посты, оцениваемые после (`CTR-05`), без
  ретро-пере-скоринга (`NS-05`).
- `DEC-05` **Очередь — отдельный outbox `score_events` + scheduler + worker pool, калька FT-007**
  (резолвит `OQ-04`). Alternatives: добавить `event_type=score_post` в `summary_events` — нарушает
  `NS-08` (модификация FT-005/FT-007 outbox). Отдельная таблица изолирует конвейер (`INV-07`),
  переиспользует проверенный паттерн lease/retry. Ключ идемпотентности — `UNIQUE(post_id)` на
  `scores` + частичный UNIQUE на активных `score_events` (`CTR-02`, `INV-04`).
- `DEC-06` **Непросуммированные/непроскоренные посты в ленту не включаются** (резолвит `OQ-06`).
  Лента — `posts ⋈ scores`; пост без оценки не теряется (доступен `V1GetPost`, `INV-06`) и появится
  после успешного скоринга. Alternatives: показывать в хвосте с sentinel-оценкой — путает порядок и
  семантику порога. Гарантия «пост не теряется» сохранена (`REQ-06`).
- `DEC-07` **Дефолтные параметры — конвенции FT-007** (резолвит `OQ-07`): `poll_interval=5s`,
  `concurrency=4`, `lease_ttl=5m`, `heartbeat=1m`, `graceful_timeout=10s`, `max_attempts=5`,
  `backoff_base=10s`, `backoff_max=10m`, `reaper.grace=30s`, `default_threshold=0.0`. Совпадение с
  `summary.worker` снижает операционную нагрузку и когнитивную стоимость (`ASM-02`, `CON-01`).

## Traceability matrix (technical)

| REQ | SC (feature) | Layers | Technical refs | CHK | EVID |
| --- | --- | --- | --- | --- | --- |
| `REQ-01` | `SC-01`, `SC-08` | api, ent/schema, migrations, biz, data, service, cmd | `CTR-08`, `CTR-12`, `CTR-14`, `INV-01` | `CHK-01`, `CHK-07` | `EVID-01`, `EVID-07` |
| `REQ-02` | `SC-02` | ent/schema, migrations, biz, data, task, configs | `CTR-09`, `CTR-11`, `CTR-13`, `DEC-01`, `DEC-04`, `DEC-05`, `INV-03`, `INV-04` | `CHK-02` | `EVID-02` |
| `REQ-03` | `SC-04` | biz, data | `DEC-03`, `DEC-04`, `INV-05`, `FM-06`, `FM-09` | `CHK-04` | `EVID-04` |
| `REQ-04` | `SC-03` | api, biz, data, service, cmd | `CTR-08`, `CTR-10`, `DEC-03`, `DEC-06` | `CHK-03` | `EVID-03` |
| `REQ-05` | `SC-05`, `SC-06` | ent/schema, migrations, data, task | `CTR-10`, `CTR-11`, `INV-02`, `INV-03`, `INV-04`, `FM-03`, `FM-04`, `FM-07` | `CHK-05` | `EVID-05` |
| `REQ-06` | `SC-07` | biz, data, task, configs | `CTR-13`, `CTR-14`, `DEC-05`, `DEC-07`, `INV-06`, `FM-01`, `FM-02`, `FM-08` | `CHK-06` | `EVID-06` |
| `REQ-07` | `SC-01`, `SC-03` | ent/schema, migrations | `CTR-09`, `CTR-10`, `CTR-12`, `DEC-02` | `CHK-01`, `CHK-03` | `EVID-01`, `EVID-03` |
| `REQ-08` | `SC-09` | all | `INV-01`, `INV-07` | `CHK-08` | `EVID-08` |

## Out of scope

Повтор `NS-01..09` [`feature.md`](feature.md): React UI (`NS-01`), OpenClaw (`NS-02`), файнтюнинг/
обучение на обратной связи (`NS-03`), тематическая классификация/дедуп (`NS-04`), ретроактивный
массовый пере-скоринг (`NS-05`), мультипрофили/auth (`NS-06`), а также — раз `DEC-01..07` резолвят
`NS-07`/`OQ-01..07` — любые альтернативные методы скоринга/хранения за пределами принятых решений.
Не переопределяем `MET-03`/`MET-04` PRD-001 (`NS-09`); FT-005/FT-007 не модифицируем (`NS-08`,
`INV-07`).

Технический non-scope: pgvector и любой внешний vector-store (`DEC-02`); рефакторинг существующих
`summary_*`/`source_*`/collector-воркеров и их миграций (`coding-style.md §Не трогай существующие
миграции`); изменение схемы `posts` (`CON-05`); добавление новых Go-зависимостей сверх stdlib/уже
используемых без отдельного `DEC-*` (`go-style.md §least dependency`); оптимизация embeddings
(batch/кэш post-embedding) сверх кэша интересов — кандидат на отдельную фичу, не блокирует FT-010.
