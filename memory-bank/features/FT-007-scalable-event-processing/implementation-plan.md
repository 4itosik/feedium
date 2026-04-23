---
title: "FT-007: Implementation Plan"
doc_kind: feature
doc_function: derived
purpose: "Execution-план реализации FT-007. Фиксирует discovery context, шаги, риски и test strategy без переопределения canonical feature-фактов."
derived_from:
  - feature.md
status: archived
---

# План имплементации

## Цель текущего плана

Перевести обработку `summary_events` на pull-модель с row-level захватом (Postgres `FOR UPDATE SKIP LOCKED`), добавить lease/retry, заменить in-process ticker на per-source планировщик через `sources.next_summary_at`, удалить устаревшие пути. Результат: горизонтально масштабируемый обработчик, соответствующий REQ-01..09 из `feature.md`, без внешних зависимостей и без внесения новых ADR.

## Current State / Reference Points

| Path / module | Current role | Why relevant | Reuse / mirror |
| --- | --- | --- | --- |
| `internal/task/summary_worker.go` | Пакетный обработчик `ListPending` → sequential `processEvent`. Содержит `processSummarizePost`, `processSummarizeSource`, `summarizeWithRetry`. | Полностью переписывается в `EventWorkerPool` с per-goroutine захват-циклом + heartbeat, но доменные обработчики `processSummarizePost/processSummarizeSource` сохраняются как есть. | Переиспользовать тело `processSummarizePost` / `processSummarizeSource` внутри новой loop-структуры; `summarizeWithRetry` не трогаем (NS-04). |
| `internal/task/cron_worker.go` | In-process ticker: обход cumulative-источников, создание `summarize_source` событий. | Удаляется, заменяется `SourceDueScheduler`. | Бизнес-логика «проверить, есть ли активное событие; есть ли новые посты» — переносится как есть в новый воркер. |
| `internal/data/summary_outbox_repo.go` | CRUD + `ListPending` (чистый SELECT без лока) + `HasActiveEvent`. | Добавляются: `ClaimOne`, `ExtendLease`, `MarkForRetry`, `ListLeaseExpired`. `ListPending` остаётся для совместимости тестов, но не используется в task. | Сохраняем pattern `clientFromContext(ctx, r.data.Ent)` для транзакционности; SKIP LOCKED пишем через raw SQL (`r.data.Ent.QueryContext`), Ent нативно не поддерживает. |
| `internal/data/source_repo.go` | CRUD + `List` с фильтром по `ProcessingMode`. | Добавляются: `ClaimDueCumulative`, `BumpNextSummaryAt`. | Тот же паттерн `clientFromContext`; SKIP LOCKED через raw SQL. |
| `internal/ent/schema/summary_event.go` | Схема: status, event_type, post_id, source_id, summary_id, error, created_at, processed_at. | Добавляются поля `locked_until`, `attempt_count`, `next_attempt_at`, `locked_by`. | Повторить паттерн `SchemaType(timestamptz)`. |
| `internal/ent/schema/source.go` | Схема Source. | Добавляется `next_summary_at TIMESTAMPTZ NULL`. | — |
| `migrations/20260415100000_create_summaries_and_events.sql` | Baseline схемы FT-005. Уникальные partial-индексы: `idx_summary_events_unique_active_post`, `idx_summary_events_unique_active_source`. | Новые миграции не должны ломать эти индексы. | Полностью сохраняем инварианты FT-005. |
| `internal/conf/conf.proto` + `configs/config.yaml` | `summary.worker.{poll_interval,batch_size}`, `summary.cron.interval`, `summary.outbox.event_ttl`, `summary.cumulative.*`, `summary.llm.*`. | Добавить: `summary.worker.{concurrency, lease_ttl, heartbeat_interval, graceful_timeout, max_attempts, backoff_base, backoff_max}`, `summary.source_scheduler.{concurrency, poll_interval, lease_ttl}`, `summary.reaper.{interval, grace}`. `batch_size` удаляется. | Регенерировать proto: `make proto`. |
| `internal/biz/summary.go` | Интерфейсы `SummaryOutboxRepo`, `SummaryRepo`. | Расширяются новыми методами (CTR-01..03). Mockgen регенерируется. | `//go:generate mockgen` директивы остаются. |
| `internal/biz/source.go` | Интерфейс `SourceRepo`. | Расширяется (CTR-04, CTR-05). | — |
| `internal/biz/summary_usecase.go` | `TriggerSourceSummarize` создаёт событие и возвращает existing-id при гонке. | Опционально: сбрасывать `next_summary_at = now()` при ручном trigger для немедленного подхвата планировщиком (не меняет контракт API). | — |
| `internal/task/wire.go` | DI provider set воркеров. | Обновить: убрать `NewCronWorker`, добавить `NewEventWorkerPool`, `NewSourceDueScheduler`, `NewStuckEventReaper`. | — |
| `engineering/testing-policy.md` | TDD для biz/, testcontainers для data/task/, goleak. | Все новые data-методы покрываются testcontainers-тестами; все task-воркеры — интеграционные тесты с `goleak.VerifyNone`. | — |

## Test Strategy

| Test surface | Canonical refs | Existing coverage | Planned automated coverage | Required local suites / commands | Required CI suites / jobs | Manual-only gap / justification | Manual-only approval ref |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `biz/summary.go` retry policy (`calculateBackoff`, `shouldRetry`) | `REQ-05`, `SC-04`, `CHK-04` | нет | Unit-тесты в `biz/summary_retry_test.go`: monotonic backoff, cap на `backoff_max`, terminal после `max_attempts`, разделение transient vs permanent. | `go test ./internal/biz/... -race` | `make test` | — | `none` |
| `data/summary_outbox_repo.go` ClaimOne + ExtendLease + MarkForRetry | `REQ-01`, `REQ-04`, `CTR-01..03`, `SC-02`, `CHK-02` | `ListPending` (integration) | Integration tests на testcontainers: конкурентные `ClaimOne` из N горутин → уникальность; expired-lease строки захватываются повторно; `ExtendLease` no-op на completed. | `go test ./internal/data/... -race` | `make test` | — | `none` |
| `data/source_repo.go` ClaimDueCumulative + BumpNextSummaryAt | `REQ-06`, `CTR-04..05`, `SC-05`, `CHK-05` | List (integration) | Integration: источник становится due → захват; повторный захват в пределах интервала пропускается. | `go test ./internal/data/... -race` | `make test` | — | `none` |
| `task/event_worker_pool.go` | `REQ-01..04`, `REQ-08`, `SC-01..03`, `SC-07`, `CHK-01..03`, `CHK-07` | `summary_worker` покрыт для single-goroutine batch | Integration с testcontainers + goleak: параллельная обработка; multi-pool race; crash recovery (cancel context в процессе); graceful drain. | `go test ./internal/task/... -race` | `make test` | — | `none` |
| `task/source_due_scheduler.go` | `REQ-06`, `SC-05`, `CHK-05` | нет (новый файл) | Integration: due источник → событие; next_summary_at сдвинут; повторный tick дубль не создаёт. | `go test ./internal/task/... -race` | `make test` | — | `none` |
| `task/stuck_event_reaper.go` | `REQ-07`, `SC-06`, `CHK-04`, `CHK-06` | нет (новый файл) | Integration: событие в processing с истёкшим lease и max_attempts → failed; логирование stuck без max_attempts не переводит статус. | `go test ./internal/task/... -race` | `make test` | — | `none` |
| Build & lint after removal | `REQ-09`, `EC-07`, `CHK-08` | — | Проверка, что `CronWorker`, `processBatch`, `ListPending` (если не используется) удалены. | `make build && make lint` | `make build && make test && make lint` | — | `none` |

## Open Questions / Ambiguities

Все четыре вопроса разрешены на этапе brainstorming-сессии; фиксируются здесь для traceability. Новые неизвестности вносить как `OQ-05+` и не прятать в prose.

| Open Question ID | Question | Resolution | Rationale | Affects |
| --- | --- | --- | --- | --- |
| `OQ-01` | `ListPending` остаётся в интерфейсе или удаляется? | **Удалить в STEP-12.** | Не нужен после перехода на `ClaimOne`. Если понадобится диагностика очереди — заведём отдельный метод `InspectPending` с явной read-only semantics в рамках Observability (NS-06). | `STEP-12` |
| `OQ-02` | `TriggerSourceSummarize` — только enqueue или дополнительно сброс `next_summary_at`? | **Enqueue + `BumpNextSummaryAt(ctx, sourceID, now()+cron.interval)` в той же транзакции.** | Ручной trigger ведёт себя как досрочно выполненный плановый тик: следующая плановая суммаризация через полный `cron.interval` после ручной, без «съедания» остаточного окна. | `STEP-10` |
| `OQ-03` | Default для `summary.worker.concurrency`. | **Code fallback при unset = `2`; `configs/config.yaml` = `4`** с комментарием `# увеличить под нагрузку; ограничено LLM rate limits и БД connection pool`. Согласовано с `feature.md` CTR-08 и ASM-03. | `2` в коде — минимум, на котором конкурентная семантика (захват, lease, multi-worker race) реально проявляется в dev/test, а не маскируется single-goroutine путём; так же упрощает отладку поведения concurrency. `4` в yaml — консервативное умолчание для dev/prod. | `STEP-03` |
| `OQ-04` | Как heartbeat продлевает lease. | **Условное продление, только живой lease:** `UPDATE summary_events SET locked_until = now()+$lease_ttl WHERE id=$event_id AND locked_by=$worker_id AND status='processing' AND locked_until > now()`. Финализирующие update-ы воркера (`UpdateStatus`, `MarkForRetry`) имеют ту же guard `locked_by=$worker_id AND locked_until > now()`. Если `rows affected = 0` — воркер потерял lease, прекращает обработку, не финализирует событие. | Строгий инвариант «ни одна запись не делается поверх истёкшего lease»; согласуется с FM-05 (recovery создаёт новую попытку с новым `locked_by`). Нет семантики «воскрешения» своего старого lease. | `STEP-05`, `STEP-07` |

## Environment Contract

| Area | Contract | Used by | Failure symptom |
| --- | --- | --- | --- |
| setup | Postgres 18.3 доступен (testcontainers для тестов, локальный docker-compose для dev); `make proto` регенерирует protobuf после правки `conf.proto`. | `STEP-01..03` | goose migrate завершился ошибкой / `make proto` падает. |
| test | `go test -race` обязателен для всех новых тестов; `goleak.VerifyNone` — для task-тестов. Все integration-тесты идут через testcontainers-go (без моков БД). | все `CHK-*` | Тест зелёный без `-race` и красный с ним — гонка реальна. |
| access / network / secrets | Не требуется — вся работа локальна, LLM-провайдер мокается в тестах. | `STEP-06..09` | — |
| migrations | Новые миграции: `2026*_summary_events_lease.sql`, `2026*_sources_next_summary_at.sql`. Обе — ADD COLUMN с DEFAULT, совместимые с online deployment. | `STEP-01`, `STEP-03` | Миграция блокирует таблицу / падает ROLLBACK на большой таблице. |

## Preconditions

| Precondition ID | Canonical ref | Required state | Used by steps | Blocks start |
| --- | --- | --- | --- | --- |
| `PRE-01` | `ASM-01` | Postgres доступен, поддерживает `FOR UPDATE SKIP LOCKED` (>=9.5). Все deploy-окружения проекта имеют PG 18.3 (engineering/database.md). | `STEP-04`, `STEP-05` | yes |
| `PRE-02` | `CON-01` | Существующие уникальные partial-индексы на `summary_events` сохранены; ни одна новая миграция не пересоздаёт эти индексы. | `STEP-01` | yes |
| `PRE-03` | feature.md `status: active` | feature.md в Design Ready — все REQ-* прослежены в traceability matrix; `implementation-plan.md` стартует от утверждённых инвариантов. | whole plan | yes |
| `PRE-04` | — | Тесты FT-005 `summary_outbox_repo_test.go` (единственный существующий тест на FT-005 pull-поверхности) зелёный перед началом; `make build && make test && make lint` на baseline зелёные. | whole plan | yes |
| `PRE-05` | `AG-01` | Отсутствие task-level unit/integration тестов у FT-005 (`internal/task/` не содержит `_test.go`) зафиксировано как risk для regression-защиты: подтверждение, что новые воркеры покрывают scope старых, выполняется через новые testcontainers-тесты `CHK-01..07` + partial-index инвариант на уровне БД. | `STEP-11` | yes |

## Workstreams

| Workstream | Implements | Result | Owner | Dependencies |
| --- | --- | --- | --- | --- |
| `WS-1` Schema & config | `REQ-04`, `REQ-05`, `REQ-06`, `CTR-06`, `CTR-07`, `CTR-08` | Миграции, Ent-схемы, proto + yaml с новыми полями; регенерированный proto/Ent. | agent | `PRE-01`, `PRE-02` |
| `WS-2` Biz interfaces & retry policy | `REQ-05`, `CTR-01..05` | Расширенные интерфейсы `SummaryOutboxRepo`, `SourceRepo`; чистая retry-политика в `biz/`; регенерированные моки. | agent | `WS-1` |
| `WS-3` Data layer (SKIP LOCKED) | `REQ-01`, `REQ-03`, `REQ-06`, `CTR-01..05` | Реализации ClaimOne / ExtendLease / MarkForRetry / ListLeaseExpired / ClaimDueCumulative / BumpNextSummaryAt через raw SQL; integration-тесты. | agent | `WS-2` |
| `WS-4` Task workers | `REQ-02`, `REQ-04`, `REQ-07`, `REQ-08` | EventWorkerPool, SourceDueScheduler, StuckEventReaper; heartbeat-горутина; graceful drain; integration-тесты с goleak. | agent | `WS-3` |
| `WS-5` Wire DI & cleanup | `REQ-09` | Обновлённый provider set; удалённые `CronWorker` и `processBatch`; проверка `make build && make test && make lint`. | agent | `WS-4` |
| `WS-6` Documentation & commit | — | Обновление `memory-bank/features/index.md`, cross-reference кода на `feature.md`, commit с conventional-commit сообщением. | agent | `WS-5` |

## Approval Gates

| Approval Gate ID | Trigger | Applies to | Why approval is required | Approver / evidence |
| --- | --- | --- | --- | --- |
| `AG-01` | Перед удалением `internal/task/cron_worker.go` и старого `SummaryWorker.processBatch` | `STEP-11`, `WS-5` | Потенциальный regression для FT-005 acceptance (idempotency). Нужно подтверждение, что новые воркеры покрывают scope старых. | human / user; evidence — зелёный прогон `CHK-01..08`. |
| `AG-02` | Перед регенерацией `internal/ent/*` и `internal/conf/conf.pb.go` | `STEP-01`, `STEP-02` | Изменение автогенерённых файлов массовое; нужна проверка, что команды запущены корректно. | human / user; evidence — diff `make proto` output. |

## Порядок работ

| Step ID | Actor | Implements | Goal | Touchpoints | Artifact | Verifies | Evidence IDs | Check command / procedure | Blocked by | Needs approval | Escalate if |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `STEP-01` | agent | `CTR-06`, `REQ-04`, `REQ-05` | Миграция `summary_events`: `locked_until TIMESTAMPTZ NULL`, `locked_by TEXT NULL`, `attempt_count INT NOT NULL DEFAULT 0`, `next_attempt_at TIMESTAMPTZ NULL`; индекс `(status, coalesce(next_attempt_at, created_at))` для захват-запроса. | `migrations/2026*_summary_events_lease.sql`, `internal/ent/schema/summary_event.go` | Новая SQL-миграция + обновлённая схема Ent + регенерация `internal/ent/*`. | — | — | `goose up && go generate ./internal/ent/...` зелёные, существующие индексы FT-005 сохранены. | `PRE-01`, `PRE-02` | `AG-02` | Нарушен `CON-01` / сломана идемпотентность. |
| `STEP-02` | agent | `CTR-07` | Миграция `sources.next_summary_at`. | `migrations/2026*_sources_next_summary_at.sql`, `internal/ent/schema/source.go` | Миграция + обновлённая схема + регенерация. | — | — | `goose up && go generate ./internal/ent/...` зелёные. | `STEP-01` | `AG-02` | — |
| `STEP-03` | agent | `CTR-08`, `REQ-02`, `REQ-08` | Расширить `conf.proto`: новые поля `summary.worker.{concurrency, lease_ttl, heartbeat_interval, graceful_timeout, max_attempts, backoff_base, backoff_max}`, `summary.source_scheduler.*`, `summary.reaper.*`. В Go-коде fallback при `concurrency==0` → `2` (OQ-03). Обновить `configs/config.yaml`: `concurrency: 4 # увеличить под нагрузку; ограничено LLM rate limits и БД connection pool`, `lease_ttl=5m`, `heartbeat_interval=1m`, `graceful_timeout=10s`, `max_attempts=5`, `backoff_base=10s`, `backoff_max=10m`, `source_scheduler.concurrency=1`, `source_scheduler.poll_interval=5s`, `source_scheduler.lease_ttl=1m`, `reaper.interval=1m`, `reaper.grace=30s`. | `internal/conf/conf.proto`, `configs/config.yaml`, `internal/conf/conf.pb.go` | Регенерированный proto + yaml. | — | — | `make proto && make build` зелёные. | `STEP-02` | `AG-02` | — |
| `STEP-04` | agent | `CTR-01..05`, `REQ-05` | Расширить интерфейсы `SummaryOutboxRepo` (ClaimOne / ExtendLease / MarkForRetry / ListLeaseExpired) и `SourceRepo` (ClaimDueCumulative / BumpNextSummaryAt); добавить retry-политику `biz/summary_retry.go` (`CalculateBackoff`, `IsTransientError`, `ShouldRetry`). Терминальный перевод «max attempts exceeded» выполняется санатором (STEP-09) через обычный `UpdateStatus(failed, error)` — отдельный метод не нужен. | `internal/biz/summary.go`, `internal/biz/source.go`, `internal/biz/summary_retry.go`, `internal/biz/mock/*` | Обновлённые интерфейсы + регенерированные моки + unit-тесты retry. | `CHK-04` | `EVID-04` | `go test ./internal/biz/... -race`. | `STEP-03` | — | — |
| `STEP-05` | agent | `CTR-01..03`, `REQ-01`, `REQ-03`, `REQ-04` | Реализация новых методов в `data/summary_outbox_repo.go` через raw SQL. `ClaimOne`: одна транзакция, `UPDATE summary_events SET status='processing', locked_until=now()+$lease, locked_by=$worker, attempt_count=attempt_count+1 WHERE id = (SELECT id FROM summary_events WHERE (status='pending' AND (next_attempt_at IS NULL OR next_attempt_at<=now())) OR (status='processing' AND locked_until<now()) ORDER BY coalesce(next_attempt_at, created_at) ASC LIMIT 1 FOR UPDATE SKIP LOCKED) RETURNING *`. `ExtendLease` (OQ-04, guarded): `UPDATE ... SET locked_until=now()+$lease WHERE id=$event AND locked_by=$worker AND status='processing' AND locked_until>now()`; при `rows affected=0` возвращает `ErrLeaseLost`. `UpdateStatus` и `MarkForRetry` — с тем же `WHERE locked_by=$worker AND locked_until>now()`; любой нулевой-affected update возвращает `ErrLeaseLost` (воркер не пишет финализацию поверх чужого lease). `MarkForRetry` дополнительно сбрасывает `locked_until=NULL, locked_by=NULL, status='pending', next_attempt_at=$2, error=$3`. `ListLeaseExpired`: SELECT по `status='processing' AND locked_until < now() - $grace LIMIT $limit`. | `internal/data/summary_outbox_repo.go`, `internal/data/summary_outbox_repo_test.go` | Реализация + integration-тесты (testcontainers). | `CHK-02`, `CHK-03` | `EVID-02`, `EVID-03` | `go test ./internal/data/... -race -run TestSummaryOutbox` | `STEP-04` | — | Race-тесты падают — пересмотреть SQL. |
| `STEP-06` | agent | `CTR-04..05`, `REQ-06` | Реализация `ClaimDueCumulative` и `BumpNextSummaryAt` в `data/source_repo.go` по той же схеме SKIP LOCKED. | `internal/data/source_repo.go`, `internal/data/source_repo_test.go` | Реализация + integration-тесты. | `CHK-05` | `EVID-05` | `go test ./internal/data/... -race -run TestSourceRepo` | `STEP-05` | — | — |
| `STEP-07` | agent | `REQ-02`, `REQ-04`, `REQ-08` | Реализовать `EventWorkerPool` (`internal/task/event_worker_pool.go`). На старте пула генерируется `processID = hostname + pid + uuid`; каждой горутине присваивается `workerID = fmt.Sprintf("%s-%d", processID, idx)`. `Start(ctx)` запускает `cfg.Concurrency` (fallback `2` при unset) горутин, каждая крутит `for { claim → spawn heartbeat → process → cancel heartbeat → finalize }`. Heartbeat-горутина (отдельная на каждое активное событие) раз в `heartbeat_interval` вызывает `ExtendLease(eventID, workerID, lease_ttl)`; при `ErrLeaseLost` отменяет основной `processCtx` и завершается, чтобы основная горутина не записала финализацию поверх чужого lease. `Stop(ctx)` закрывает input-гейт (новые claim-ы не стартуют), ждёт `WaitGroup` до `graceful_timeout`, после — `cancel` подпроцессов (неоконченное событие будет подхвачено через истечение lease). Внутри `process` — доменные обработчики FT-005 (`processSummarizePost`/`processSummarizeSource`), адаптированные: при transient-ошибке вызывается `MarkForRetry`, при permanent — `UpdateStatus(Failed)`. Идемпотентность (FM-05): `processSummarizePost` перед LLM-вызовом проверяет `summaryRepo.ListByPost(postID)` — если Summary уже есть, событие закрывается `completed` без LLM. | `internal/task/event_worker_pool.go`, `internal/task/event_worker_pool_test.go` | Новый воркер + integration-тесты (parallel, multi-instance, crash recovery, graceful drain). | `CHK-01`, `CHK-02`, `CHK-03`, `CHK-07` | `EVID-01..03`, `EVID-07` | `go test ./internal/task/... -race -run TestEventWorkerPool` | `STEP-06` | — | goleak детектит утечку — исправить drain. |
| `STEP-08` | agent | `REQ-06` | Реализовать `SourceDueScheduler` (`internal/task/source_due_scheduler.go`): захват одного источника по `next_summary_at <= now()`, проверка `HasActiveEvent`, создание события (`SummaryEventTypeSummarizeSource`) в одной транзакции, bump `next_summary_at = now() + cron.interval`. | `internal/task/source_due_scheduler.go`, `internal/task/source_due_scheduler_test.go` | Новый воркер + integration-тесты. | `CHK-05` | `EVID-05` | `go test ./internal/task/... -race -run TestSourceDueScheduler` | `STEP-06` | — | — |
| `STEP-09` | agent | `REQ-07` | Реализовать `StuckEventReaper` (`internal/task/stuck_event_reaper.go`): раз в `reaper.interval` вызывает `ListLeaseExpired`, для каждого: если `attempt_count >= max_attempts` → `UpdateStatus(failed, error='max attempts exceeded')`; иначе логировать warning и не трогать (захват-loop подхватит). | `internal/task/stuck_event_reaper.go`, `internal/task/stuck_event_reaper_test.go` | Новый воркер + integration-тесты. | `CHK-04`, `CHK-06` | `EVID-04`, `EVID-06` | `go test ./internal/task/... -race -run TestStuckEventReaper` | `STEP-07` | — | — |
| `STEP-10` | agent | `OQ-02` | Обновить `biz/summary_usecase.go` `TriggerSourceSummarize`: enqueue события и `sourceRepo.BumpNextSummaryAt(ctx, sourceID, now() + cron.interval)` выполняются **в одной транзакции** через `TxManager`. Следующий плановый тик произойдёт через полный `cron.interval` после ручного trigger. | `internal/biz/summary_usecase.go`, `internal/biz/summary_usecase_test.go` | Обновлённый use-case + unit-тесты. | — | — | `go test ./internal/biz/... -race` | `STEP-04` | — | — |
| `STEP-11` | agent | `REQ-09`, `EC-07` | Удалить `internal/task/cron_worker.go` и `internal/task/summary_worker.go` (включая `processBatch`) — их роль после STEP-07..09 полностью покрыта новыми воркерами. Обновить provider set в `internal/task/wire.go`: убрать `NewCronWorker` и `NewSummaryWorker`, добавить `NewEventWorkerPool`, `NewSourceDueScheduler`, `NewStuckEventReaper`. | `internal/task/summary_worker.go` (удаление), `internal/task/cron_worker.go` (удаление), `internal/task/wire.go` (правка) | Чистый diff без dead code. | `CHK-08` | `EVID-08` | `make build && make test && make lint` | `STEP-07`, `STEP-08`, `STEP-09`, `STEP-10` | `AG-01` | Любой существующий тест (biz/, data/) стал красным. |
| `STEP-12` | agent | `REQ-09`, `OQ-01` | Удалить `ListPending` из интерфейса `SummaryOutboxRepo` (`internal/biz/summary.go`), реализации (`internal/data/summary_outbox_repo.go`), integration-тестов (`TestIntegration_OutboxRepo_ListPending*` в `internal/data/summary_outbox_repo_test.go`) и регенерируемого мока (`internal/biz/mock/mock_summary.go` — через `go generate`). Предварительно `grep -rn "ListPending" internal/` должен возвращать только эти точки — `service/summary/mock/` не содержит `ListPending` (это мок сервисного интерфейса, не репозитория). | `internal/biz/summary.go`, `internal/data/summary_outbox_repo.go`, `internal/data/summary_outbox_repo_test.go`, `internal/biz/mock/mock_summary.go` | Упрощение контракта. | `CHK-08` | `EVID-08` | `go test ./... -race && make lint` | `STEP-11` | — | Нашёлся внешний потребитель `ListPending` вне task — эскалация. |
| `STEP-13` | agent | — | Обновить `memory-bank/features/index.md` (FT-007 → `in_progress` после начала работ, затем `done`), добавить cross-reference из обновлённых файлов на `feature.md` (см. `engineering/cross-references.md`). | `memory-bank/features/index.md`, `internal/task/event_worker_pool.go` (doc comment с ссылкой на feature.md) | Актуальная документация. | — | — | `make lint` | `STEP-11` | — | — |

## Parallelizable Work

- `PAR-01` `STEP-08` (SourceDueScheduler) и `STEP-09` (StuckEventReaper) могут идти параллельно после `STEP-07` (EventWorkerPool готов) — у них нет общего write-surface.
- `PAR-02` `STEP-01` и `STEP-02` (миграции summary_events и sources) могут идти параллельно — независимые таблицы.
- `PAR-03` **Нельзя распараллелить:** `STEP-04 → STEP-05 → STEP-06` — биз-интерфейс сначала, потом data-layer (моки и тесты зависят от интерфейсов).

## Checkpoints

| Checkpoint ID | Refs | Condition | Evidence IDs |
| --- | --- | --- | --- |
| `CP-01` | `STEP-01..03` | Миграции применены, Ent и proto регенерированы, `make build` зелёный — инфраструктурный слой готов к biz-изменениям. | — |
| `CP-02` | `STEP-04..06` | Biz-интерфейсы и data-слой реализованы; все unit и integration тесты data/biz зелёные; моки пересобраны. | `EVID-02`, `EVID-04`, `EVID-05` |
| `CP-03` | `STEP-07..09` | Все новые воркеры работают, integration-тесты с race-detector и goleak зелёные. | `EVID-01`, `EVID-03`, `EVID-06`, `EVID-07` |
| `CP-04` | `STEP-11..13` | Старые пути удалены, build/test/lint зелёные локально и в CI, индекс feature обновлён. | `EVID-08` |

## Execution Risks

| Risk ID | Risk | Impact | Mitigation | Trigger |
| --- | --- | --- | --- | --- |
| `ER-01` | Ent не даёт удобного способа использовать `FOR UPDATE SKIP LOCKED`, приходится падать в raw SQL | Смешение стилей в data/, сложность мейнтейнса | Написать тонкую helper-функцию `execSkipLocked(ctx, client, sql, args)` и использовать её единообразно; покрыть integration-тестами. | Если helper становится больше 50 строк или требует mock-обхода — эскалация. |
| `ER-02` | Heartbeat-горутина может не успеть продлить lease при stop-the-world (GC, IO-блок) | Ложное истечение lease, двойная обработка | `lease_ttl >> heartbeat_interval` (5m / 1m = 5x запас); идемпотентность обработчика через проверку существующей Summary (FM-05). | — |
| `ER-03` | Testcontainers медленные в CI (Postgres startup ~5s × N тестов) | Длинный CI feedback-loop | Переиспользовать один контейнер через `testify suite` на пакет; truncate между тестами. | CI job > 5 минут. |
| `ER-04` | При rolling deploy старый процесс (batch) и новый процесс (pull) работают одновременно | Двойная обработка события во время deploy | Claim-запрос сам по себе устойчив к этому (SKIP LOCKED защитит на стороне нового; старый не использует lock, но уникальный partial index защитит от двойной Summary). | — |
| `ER-05` | Migration `ADD COLUMN ... DEFAULT` блокирует таблицу на больших объёмах | Downtime при релизе | PG 11+ для nullable/non-volatile defaults — online. Fact-чек перед релизом: PG 18.3 ✓. | — |

## Stop Conditions / Fallback

| Stop ID | Related refs | Trigger | Immediate action | Safe fallback state |
| --- | --- | --- | --- | --- |
| `STOP-01` | `CON-01`, `CON-02` | Integration-тест обнаружил дубликат Summary или нарушенный уникальный partial index | Откатить изменения текущего STEP через `git reset`; зафиксировать test case как regression; вернуться на `STEP-05`. | `main` branch + FT-005 baseline. |
| `STOP-02` | `ER-01` | Raw SQL выходит за границы 1-2 helper-ов, проникает в доменный код | Остановить STEP-05/06, обсудить введение тонкой репозитории-абстракции | Остановка до согласования дизайна helper. |
| `STOP-03` | `AG-01` | STEP-11 (удаление старых воркеров) падает без explicit approval | Оставить код, отметить как технический долг; продолжить остальные STEP-ы. | feature.md + implementation-plan, ожидающие approval. |

## Готово для приемки

План считается исчерпанным, когда:

1. Все `STEP-01..13` завершены или явно пропущены с обоснованием.
2. Все `CHK-01..08` из `feature.md` имеют pass-результат в `artifacts/ft-007/verify/chk-XX/`.
3. `make build && make test && make lint` зелёные локально и в CI.
4. `AG-01` approved (удаление старых воркеров).
5. `feature.md` → `delivery_status: done`, `implementation-plan.md` → `status: archived`.
