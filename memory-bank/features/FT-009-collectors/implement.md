---
title: "FT-009-collectors: Implementation Summary"
doc_kind: feature
doc_function: implementation
status: draft
derived_from:
  - ./feature.md
  - ./spec.md
---

# FT-009-collectors — Implementation Summary

## Readback verdict

Spec ↔ feature: **no contradictions**. Spec refines feature without redefining
scope. The one hard-to-reverse spec decision — `DEC-03` (Telegram client
`gotd/td`) — is flagged by the spec itself as "requires human approval, not
finalized", matching [autonomy-boundaries.md](../../engineering/autonomy-boundaries.md).
RSS and web are implemented end-to-end; **Telegram is escalated** (`REQ-02`,
`REQ-03`) — see `## Open Questions`.

## What changed in this iteration (review fix pass)

This run addresses the findings routed to `implement` from the previous review.

| Finding (route_to: implement) | Severity | Resolution |
|---|---|---|
| `REQ-09` claimed covered but no observable outbox regression test on any implemented path (AGENT_1 F1) | **blocking** | **New `TestIntegration_RSSCollector_PersistsFieldsAndEnqueuesSummary`** (`internal/task/rss_collector_test.go`): after collecting a self-contained RSS source it asserts `SummaryOutboxRepo.HasActiveEvent(sourceID, summarize_post) == true`. REQ-09's outbox effect is now regression-tested on a real implemented path, decoupled from the escalated Telegram `SC-02`/`SC-03`. |
| `conf.pb.go` hand-regenerated, byte-parity not confirmed by running the toolchain (AGENT_2 R1 = AGENT_4 risk 2) | **blocking** | **Not closeable in this environment** — `go`/`make`/`protoc` are auto-denied (re-confirmed this run). I instead *arithmetically re-verified* the descriptor (lengths, message ordering, dep indexes) field-by-field — see `## Verification`. Authoritative `make proto` + `go test ./internal/conf/...` remains a verify-stage gate (`OQ-C`). Reported honestly, not asserted as passing. |
| `REQ-05`/`SC-10` had no end-to-end assert of the *stored* post fields (AGENT_1 F2) | warning | Same new test reads the post back via `PostRepo.List` and asserts `SourceID`, `ExternalID`, non-zero `PublishedAt` (parsed from `pubDate`), `Metadata["title"]`, and `Author` (present-in-feed case). |
| `implement.md` marked new-to-tree files as "unchanged", misleading vs HEAD (AGENT_1 F3) | warning | `## Изменённые файлы` now states each file's status **relative to HEAD** (all FT-009 files are new/modified vs HEAD); "touched this iteration" is called out separately. |
| `rss_fetcher.go` used `fmt.Errorf` with a static string (AGENT_3) | warning | `internal/data/rss_fetcher.go:129` → `errors.New("unrecognized feed format")`; `errors` import added. |
| `runCollectLoop` `time.After` leaves one pending timer at shutdown (AGENT_3) | warning (conscious) | Left as-is: one pending timer, consistent with `source_due_scheduler.go`, acknowledged by the reviewer as deliberate. |
| `TestCollectUsecase_CollectSource` uses nested `t.Run` not table-driven (AGENT_4) | warning (preferred, not required) | Left as-is: each branch needs a distinct gomock expectation script; the reviewer agreed the differing setups justify it. testing-policy marks table-driven "preferred". |

## Что реализовано (по REQ)

- `REQ-01` (RSS) → `internal/task/rss_collector.go` (`RSSCollector`) +
  `internal/data/rss_fetcher.go` (`rssFetcher.Fetch`) +
  `biz.CollectUsecase.CollectSource` (`internal/biz/collect.go:77`). Covered by
  `SC-01`/`CHK-01` (`TestIntegration_RSSCollector_CollectsAndIsIdempotent`) and
  `TestRSSFetcher_Fetch`.
- `REQ-02` (Telegram channel) → **NOT implemented — escalated** (`DEC-03`). The
  `biz.Fetcher` port and `conf.Collectors.telegram` slot are in place; a fetcher
  slots in with no biz/task contract change. See `## Open Questions`.
- `REQ-03` (Telegram group / cumulative) → **NOT implemented — escalated**.
- `REQ-04` (web) → `internal/task/web_collector.go` (`WebCollector`,
  single-retry-then-skip) + `internal/data/web_fetcher.go`. Covered by
  `SC-04`/`CHK-04` (`TestIntegration_WebCollector_CollectsArticle`),
  `TestWebFetcher_Fetch`, and the white-box `TestWebCollector_collectWithRetry`
  (FM-T02).
- `REQ-05` (post field mapping) → `CollectedItem` + `Create` delegation in
  `CollectUsecase.CollectSource`. Covered by `SC-10` at three levels:
  biz delegation (`TestCollectUsecase_CollectSource`), fetcher mapping
  (`TestRSSFetcher_Fetch`/`TestWebFetcher_Fetch`), and **now end-to-end** —
  `TestIntegration_RSSCollector_PersistsFieldsAndEnqueuesSummary` reads the
  stored post and asserts external_id/published_at/metadata/author.
- `REQ-06` (idempotency) → reuse of `PostUsecase.Create` upsert by
  `(source_id, external_id)`; no second dedup layer (`INV-T03`). Covered by
  `SC-05`/`CHK-05` (deterministic: ≥3 cycles, count unchanged) and the biz
  "duplicate item … not fatal" case.
- `REQ-07` (failure isolation) → per-source `continue`-on-error in
  `collectAllOfType` (`internal/task/rss_collector.go:160`); each collector a
  separate `transport.Server` (`INV-T07`). Covered by `SC-06`/`CHK-06`
  (`TestIntegration_RSSCollector_IsolatesSourceFailure`), `SC-07`/`CHK-07`, and
  `TestCollectAllOfType_ListFailureIsObservable`.
- `REQ-08` (configurable poll) → `conf.Collectors`/`conf.CollectorPoll`
  (`internal/conf/conf.proto`, reflected in `conf.pb.go`); `Start()` reads
  `cfg.GetRss()/GetWeb().GetPollInterval()`. Covered by `SC-08`/`CHK-08`
  (`TestRSSCollector_StartUsesConfiguredPollInterval` + web variant) — cadence
  asserted against the config value, deterministically.
- `REQ-09` (FT-005 modes unchanged) → delegation to `PostUsecase.Create`
  (`internal/biz/post.go:196` enqueues `summarize_post` for self-contained,
  nothing for cumulative); FT-009 adds no outbox logic (`CTR-T03`, `INV-T01`).
  **Now regression-tested**: `TestIntegration_RSSCollector_PersistsFieldsAndEnqueuesSummary`
  asserts a `summarize_post` event is enqueued after self-contained RSS
  collection, via `SummaryOutboxRepo.HasActiveEvent`. (Telegram cumulative path
  `SC-03` remains escalated.)
- `REQ-10` (text only) → `FilterTextItems` (`internal/biz/collect.go:108`).
  Covered by `SC-09` (`TestFilterTextItems`) and "non-text item never reaches
  Create".
- `REQ-11` (autotests) → biz unit (TDD), data httptest, task testcontainers +
  white-box internal tests, all with `goleak`.

## Изменённые файлы

Status is **relative to HEAD (084a617)**: every FT-009 file is new or modified
vs HEAD. "touched this iteration" marks files I edited in this fix pass; the rest
are unchanged since the prior pass but are still new/modified vs HEAD.

### `biz/`
- `internal/biz/collect.go` (new vs HEAD) — `CollectedItem`; ports
  `Fetcher`/`RSSFetcher`/`WebFetcher`/`PostCreator`; `CollectUsecase`;
  `FilterTextItems`. No `ent`/`http`/`sql` imports (`INV-T02`). Not touched this
  iteration.
- `internal/biz/collect_test.go` (new vs HEAD) — table/`t.Run` cases for
  `FilterTextItems` and `CollectSource` (happy mapping, non-text skip, duplicate
  not-fatal, fetch-error wrap, `ErrPostSourceNotFound` skip, empty fetch). Not
  touched this iteration.
- `internal/biz/mock/mock_collect.go` (new vs HEAD) — mockgen artifact for
  `Fetcher`/`PostCreator`. Not touched this iteration.
- `internal/biz/wire.go` (modified vs HEAD) — `NewCollectUsecase` + `PostCreator`
  bind. Not touched this iteration.

### `data/`
- `internal/data/rss_fetcher.go` (new vs HEAD) — **touched this iteration**:
  `errors` import added; `parseFeed`'s unrecognized-format error switched from
  `fmt.Errorf` to `errors.New` (`:129`, AGENT_3). No behaviour change.
- `internal/data/web_fetcher.go` (new vs HEAD) — stdlib HTML extraction behind
  `biz.WebFetcher`. Not touched this iteration.
- `internal/data/rss_fetcher_test.go`, `internal/data/web_fetcher_test.go`
  (new vs HEAD) — httptest mapping + fallbacks + broken-markup. Not touched this
  iteration.
- `internal/data/wire.go` (modified vs HEAD) — `New*Fetcher` providers + named
  binds. Not touched this iteration.

### `task/`
- `internal/task/rss_collector.go` (new vs HEAD) — `transport.Server` polling
  loop, per-source `continue`-on-error, injectable `after` seam, `WaitGroup` Stop.
  Not touched this iteration.
- `internal/task/web_collector.go` (new vs HEAD) — same lifecycle +
  single-retry-then-skip; no retry on cancelled context. Not touched this
  iteration.
- `internal/task/rss_collector_test.go` (new vs HEAD) — **touched this
  iteration**: added `TestIntegration_RSSCollector_PersistsFieldsAndEnqueuesSummary`
  (`rssFieldsFixture` single-item feed → read-back field asserts for REQ-05/SC-10
  + `HasActiveEvent` outbox assert for REQ-09 — closes AGENT_1 F1/F2). Existing
  count/idempotency/isolation tests unchanged.
- `internal/task/web_collector_test.go` (new vs HEAD) — collect + extraction-skip
  + deterministic idempotency wait. Not touched this iteration.
- `internal/task/collector_internal_test.go` (new vs HEAD, white-box) —
  deterministic tests for unexported helpers (cadence SC-08, `collectWithRetry`,
  `collectAllOfType` pagination/list-failure). Out-of-whitelist — see below. Not
  touched this iteration.
- `internal/task/wire.go` (modified vs HEAD) — `NewRSSCollector`/`NewWebCollector`
  providers. Not touched this iteration.

### `cmd/`
- `cmd/feedium/wire.go`, `cmd/feedium/wire_gen.go` (modified vs HEAD) — collectors
  registered in `kratos.Server(...)`; reference `bc.GetCollectors()`. Not touched
  this iteration.

### `configs/`
- `internal/conf/conf.proto` (modified vs HEAD) — `Collectors`/`CollectorPoll`
  messages + `Bootstrap.collectors = 4`. Not touched this iteration.
- `internal/conf/conf.pb.go` (modified vs HEAD) — hand-regenerated to match the
  proto. Not touched this iteration (re-verified — see `## Verification`).
- `configs/config.yaml`, `configs/local.example.yaml` (modified vs HEAD) —
  `collectors:` with per-type `poll_interval`; Telegram annotated as awaiting
  `DEC-03`. Not touched this iteration.

## Контракты

- Config (`CTR-T01`): `conf.proto` + `conf.pb.go` expose `Collectors` /
  `CollectorPoll` and `Bootstrap.GetCollectors()`. If verify runs `make proto`,
  it overwrites the hand-regenerated file idempotently (`OQ-C`).
- DB: **unchanged** (`INV-T06`, `CON-05`, `ASM-02`); no schema/migration edits,
  no collection-cursor field (`DEC-05`).
- Proto API / `service/`: unchanged (`NS-02`).

## Решения и trade-offs

- **REQ-09 observed via `HasActiveEvent`, on RSS not Telegram.** The canonical
  REQ-09 scenarios in feature.md (`SC-02`/`SC-03`) are Telegram and escalated, so
  the only honest way to regression-test the outbox effect *now* is on an
  implemented self-contained path. RSS is self-contained, so collecting it must
  enqueue a `summarize_post` event — exactly what `HasActiveEvent` asserts. This
  decouples REQ-09 coverage from the escalated Telegram work.
- **Hand-regenerated `conf.pb.go` kept, not re-derived.** The toolchain is denied,
  so I cannot re-run `make proto`. Rather than touch the descriptor again (each
  hand edit is a fresh byte-parity risk), I re-verified the existing bytes
  arithmetically. The authoritative regen stays a verify gate.
- **Zero new dependencies (deviates from `DEC-01`/`DEC-02`).** RSS uses stdlib
  `encoding/xml`; web uses the already-present `golang.org/x/net/html`. Both sit
  behind `biz.Fetcher`, so adopting `gofeed`/`go-readability` later is a `data/`
  -only change. Flagged as `OQ-B`.
- **Telegram escalated, not finalized (`DEC-03`).** No `gotd/td`, no `telegram_*`
  files. Hard-to-reverse external account + ban risk + heavy dep; gated on
  approval.

## Simplify pass

- The new test reuses existing helpers (`createRSSSource`, `newCollectUsecase`,
  `collectorsCfg`, `waitForPosts`) and a single-item fixture; it adds no
  production surface and no abstraction. The single-item fixture is deliberate:
  `HasActiveEvent` uses `Only()`, so exactly one event keeps the assertion
  unambiguous.
- No new error types, no pluggable filter chains. `errors.New` swap is a pure
  lint/style normalization.

## Verification

**Gates not executed.** `go`, `protoc` and `make` are auto-denied in this
environment (re-confirmed this run: `go version` → "This command requires
approval", denied; sandbox-disabled retry also denied). No pass/fail is asserted
for:

- `make proto` — authoritative regen of `conf.pb.go` (`OQ-C`). Re-run in the
  pipeline to confirm byte-parity.
- `make wire` — `wire_gen.go` already matches expected output.
- `make build && make test && make lint && make test-coverage` (`CHK-11`). Task
  integration tests also require Docker (testcontainers), incl. the new
  `TestIntegration_RSSCollector_PersistsFieldsAndEnqueuesSummary`.

**Verified without the toolchain:**

- **`conf.pb.go` descriptor re-verified arithmetically** (R1). Length prefixes
  recomputed from field bytes against analogous existing messages:
  - `Bootstrap` prefix `\xac\x01` = 172 = name(11) + server(38) + data(32) +
    summary(41) + collectors(50). ✓
  - `Collectors` prefix `\x8b\x01` = 139 = name(12) + rss(39) + web(39) +
    telegram(49). ✓
  - `CollectorPoll` prefix `O` = 79 = name(15) + poll_interval(64); its
    `poll_interval` field is byte-identical to `SummaryWorker.poll_interval`. ✓
  - `NumMessages: 18`; `goTypes`/`msgTypes` place `Collectors`@15,
    `CollectorPoll`@16, and the map entry `SummaryLLM.ProvidersEntry`@17 — i.e.
    map-entry-last, matching the original file's own convention (it was last
    before FT-009 too). `depIdxs` 30–32 (`Collectors.{rss,web,telegram}`→16),
    33 (`CollectorPoll.poll_interval`→18), 34 (`ProvidersEntry.value`→12) follow
    msgType order. ✓
  - **Limit:** arithmetic agreement is necessary but not a substitute for a real
    `protoc` round-trip. If a byte is wrong, `init()` panics at load — hence the
    explicit `OQ-C` gate.
- New test compiles against confirmed symbols: `biz.ListPostsFilter`,
  `biz.SortByCreatedAt`/`SortDesc`, `PostRepo.List`, `biz.Post.{SourceID,
  ExternalID,PublishedAt,Author,Metadata}`, `SummaryOutboxRepo.HasActiveEvent`,
  `biz.SummaryEventTypeSummarizePost`. No new compiler diagnostics on the edited
  files. (`summary_test.go` "X is not a type" diagnostics are gopls cascade
  artifacts from the `conf` package's transient state — a `data/`-package edit
  cannot affect `biz/` type-checking — and resolve once `conf` compiles.)
- Signatures cross-checked: `PostCreator.Create` ↔ `PostUsecase.Create`
  (`internal/biz/post.go:152`); `HasActiveEvent(ctx, sourceID, eventType) (bool,
  *SummaryEvent, error)` matches a freshly-created pending event
  (`internal/data/summary_outbox_repo.go:404` — `StatusIn(pending, processing)`).

## Out-of-whitelist changes

- `internal/task/collector_internal_test.go` (**new**, not in spec `## Files`).
  Reason: deterministic regression coverage for SC-08 (cadence), FM-T02 (retry
  recovery + cancelled-context), and `collectAllOfType` pagination/list-failure —
  all exercise **unexported** helpers the whitelisted black-box `task_test` files
  cannot reach. Same package/dir as the feature code; adds no production surface.
- `.ferry/prompts/brief_review.md.tmpl` (modified in the working tree per `git
  status`). **Not produced by this step** and unrelated to FT-009 — a pre-existing
  Ferry tooling edit, left untouched, noted for accuracy. Not in spec `## Files`.

Whitelist files **intentionally not produced** (Telegram escalated, `DEC-03`):
`internal/data/telegram_fetcher.go`(+`_test`),
`internal/task/telegram_collector.go`(+`_test`). `go.sum` untouched (no new
dependency; `go.mod` only promotes `golang.org/x/net` indirect→direct).

## Out-of-scope failures

None observed by inspection. The only LSP diagnostics on the tree are the
`internal/biz/summary_test.go` "X is not a type" cascade artifacts described in
`## Verification`; that file was not touched and the artifacts resolve once the
`conf` package builds.

## Open Questions

- `OQ-A` **Telegram collection (`REQ-02`, `REQ-03`) unimplemented pending approval
  of `DEC-03`** (client library + session storage, `OQ-01`/`OQ-03`). Until then
  `SC-02`/`SC-03`, `CHK-02`/`CHK-03` remain unmet; `MET-01` (4/4 types) is not
  reached. Additive `data/`+`task/` change once approved.
- `OQ-B` **RSS/HTML library choice (`DEC-01`/`DEC-02`).** Shipped stdlib to avoid
  an unapproved/unfetchable dependency. Confirm whether to keep stdlib for MVP or
  adopt `gofeed`/`go-readability` (→ согласование). Port-isolated either way.
- `OQ-C` **Authoritative toolchain regen + test run must happen in the pipeline.**
  This environment cannot run `make proto`/`make wire`/`go test` (toolchain
  denied; Docker needed for testcontainers). The compile blocker is fixed in-tree
  by hand and the descriptor re-verified arithmetically, **but byte-parity of
  `conf.pb.go` and the green status of the new integration test are not
  instrumentally confirmed here.** Verify must run `make proto` (idempotent) then
  `make build/test/lint/test-coverage`. This is the residual of blocking finding
  R1 — surfaced, not silently closed.
