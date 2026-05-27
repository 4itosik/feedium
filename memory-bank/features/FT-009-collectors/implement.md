---
title: "FT-009-collectors: Implementation Summary"
doc_kind: feature
doc_function: implementation
status: active
derived_from:
  - ./feature.md
  - ./spec.md
  - ./2026-05-26-telegram-push-design.md
---

# FT-009-collectors — Implementation Summary

## Readback verdict

Spec ↔ feature: **no contradictions**. Spec refines feature without redefining
scope. The hard-to-reverse Telegram-client decision (`DEC-03` `gotd/td`) was
approved 2026-05-26 and superseded by the **push design `DEC-08`**
([2026-05-26-telegram-push-design.md](2026-05-26-telegram-push-design.md)): RSS
(`REQ-01`) and web (`REQ-04`) collect via polling; Telegram (`REQ-02`/`REQ-03`)
collects via a `gotd/td` push session (`updates.Manager`). All four source types
are implemented end-to-end. The only residual manual step is the **live userbot
login** (phone-code), gated by `AG-02`; tests use a fake session.

## What changed in this iteration (code_review fix pass, 2026-05-26)

This run addresses the findings routed to `implement` from the `code_review`
stage. The whole toolchain was available this run, so the gates were actually
executed (see `## Verification`).

| Finding | Source | Severity | Resolution |
|---|---|---|---|
| `implement.md` declared `REQ-02`/`REQ-03` "NOT implemented — escalated" while the tree has a full Telegram push impl | AGENT_1 F1 | **blocking** | This document rewritten to describe the implemented push model (`DEC-08`); `## Что реализовано`, `## Изменённые файлы`, `## Out-of-whitelist changes`, `## Open Questions` all corrected. |
| `## Изменённые файлы` / `## Out-of-whitelist changes` omitted the real Telegram files and claimed `go.sum` untouched / "no new dependency" | AGENT_1 F2 | **blocking** | File list now includes `telegram_collector.go`(+`_test`), `telegram_session.go`(+`_test`), the biz push seam, and the `go.mod`/`go.sum` addition of `gotd/td`; whitelist deviation `telegram_fetcher*` → `telegram_session*`+`telegram_collector*` recorded. |
| `internal/biz/collect.go` named ports `RSSFetcher`/`WebFetcher`: `revive` doc-comment form + `iface` identical-interface lint | AGENT_3 | **blocking** | Split into per-type doc comments starting with the type name; `//nolint:iface` with explanation on `Fetcher`/`RSSFetcher`/`WebFetcher` (intentional named ports, `DEC-06`). |
| `internal/task/rss_collector.go:106` `time.After` not a doc-link (`godoclint`) | AGENT_3 | **blocking** | `time.After` → `[time.After]`. |
| `internal/data/web_fetcher.go:104` non-exhaustive `atom.Atom` switch (`exhaustive`) — **missed by AGENT_3, caught by toolchain run** | this run | **blocking** | `//nolint:exhaustive` with explanation (only `title`/`p`/`meta` carry article content; every other element intentionally ignored). |
| FT-009 test-file lint: `modernize` (`ptr`/`ptrString` → `new(expr)`), `testpackage` (`telegram_session_test.go`), `testifylint require-error` (`collector_internal_test.go`), `unparam` (`collectorsCfg`) | AGENT_3/AGENT_4 | warning | Helpers replaced with Go 1.26 `new(expr)`; `//nolint:testpackage` (white-box of unexported `authorFrom`/`chatTitleFrom`); `assert.ErrorIs`→`require.ErrorIs`; `collectorsCfg` parameter dropped (`testPollInterval` const). |
| `TelegramCollector.Start` used `context.Background()`, inconsistent with `EventWorkerPool` lifecycle ctx | AGENT_3 | warning | `context.WithCancel(ctx)` (parent Start ctx); `cancel` still stored and invoked by `Stop`. |
| `telegram_session.go` auth errors returned bare (no `%w` context) | AGENT_3 | warning | `fmt.Errorf("telegram auth status: %w", err)` / `"telegram self: %w"`. |
| Telegram push resilience branches (resolve-error, persist-error-continue) had no regression test (FM-T01) | AGENT_4 | warning | Added `TestTelegramCollector_handle_ResolveErrorIsLoggedAndStreamContinues` and `..._PersistErrorIsLoggedAndStreamContinues` (white-box, goleak) in `collector_internal_test.go`. |
| `TestTelegramCollector_RoutesMessageToMatchingSource` missing AAA comments | AGENT_4 | warning | Arrange/Act/Assert comments added. |

Deferred (recorded as follow-ups, see `## Open Questions`): peer-type confusion
in `resolve` (AGENT_2 R1), per-message uncached source listing vs the cached
resolver promised in the design doc (AGENT_2 R2), `emit`-mapping unit coverage
incl. the `m.Out` skip (AGENT_4), and the unused generated `MockTelegramSession`.

## Что реализовано (по REQ)

- `REQ-01` (RSS) → `internal/task/rss_collector.go` (`RSSCollector`, polling) +
  `internal/data/rss_fetcher.go` (`rssFetcher.Fetch`) +
  `biz.CollectUsecase.CollectSource` (`internal/biz/collect.go:97`). Covered by
  `SC-01`/`CHK-01` (`TestIntegration_RSSCollector_CollectsAndIsIdempotent`) and
  `TestRSSFetcher_Fetch`.
- `REQ-02` (Telegram channel, self-contained) → **implemented via push**
  (`DEC-08`): `internal/task/telegram_collector.go` (`TelegramCollector`) +
  `internal/data/telegram_session.go` (`telegramSession`, `gotd/td`) +
  `biz.CollectUsecase.CollectItems`. Covered by
  `TestTelegramCollector_RoutesMessageToMatchingSource` and `SC-02`/`CHK-02`
  (`TestIntegration_TelegramCollector_OutboxByMode` — channel enqueues
  `summarize_post`). Live login gated by `AG-02`.
- `REQ-03` (Telegram group, cumulative) → same push collector; the group case of
  `TestIntegration_TelegramCollector_OutboxByMode` asserts cumulative collection
  enqueues **no** per-post event (`SC-03`/`CHK-03`).
- `REQ-04` (web) → `internal/task/web_collector.go` (`WebCollector`,
  single-retry-then-skip) + `internal/data/web_fetcher.go`. Covered by
  `SC-04`/`CHK-04` (`TestIntegration_WebCollector_CollectsArticle`),
  `TestWebFetcher_Fetch`, and white-box `TestWebCollector_collectWithRetry`
  (FM-T02).
- `REQ-05` (post field mapping) → `CollectedItem` + `Create` delegation in
  `CollectUsecase.CollectItems`. Covered by `SC-10` at biz, fetcher and
  end-to-end levels (`TestIntegration_RSSCollector_PersistsFieldsAndEnqueuesSummary`
  reads the stored post and asserts external_id/published_at/metadata/author).
- `REQ-06` (idempotency) → reuse of `PostUsecase.Create` upsert by
  `(source_id, external_id)`; no second dedup layer (`INV-T03`). Covered by
  `SC-05`/`CHK-05` (≥3 cycles, count unchanged) and the biz "duplicate item …
  not fatal" case.
- `REQ-07` (failure isolation) → per-source `continue`-on-error in
  `collectAllOfType`; each collector a separate `transport.Server` (`INV-T07`).
  Push path: `handle` logs and swallows resolve/persist errors so the stream
  survives (`telegram_collector.go:86`). Covered by `SC-06`/`CHK-06`,
  `SC-07`/`CHK-07`, `TestCollectAllOfType_ListFailureIsObservable`, and the two
  new `TestTelegramCollector_handle_*` resilience tests.
- `REQ-08` (configurable poll) → `conf.Collectors`/`conf.CollectorPoll`; RSS/web
  `Start()` read `cfg.GetRss()/GetWeb().GetPollInterval()`. Telegram is push, so
  `REQ-08` does not apply to it (`DEC-08`); `conf.TelegramCollector` carries no
  `poll_interval`. Covered by `SC-08`/`CHK-08`
  (`TestRSSCollector_StartUsesConfiguredPollInterval` + web variant).
- `REQ-09` (FT-005 modes unchanged) → delegation to `PostUsecase.Create`
  (self-contained → `summarize_post`; cumulative → no event); FT-009 adds no
  outbox logic (`CTR-T03`, `INV-T01`). Regression-tested on **both** an RSS
  self-contained path (`...PersistsFieldsAndEnqueuesSummary`) and the Telegram
  channel/group paths (`TestIntegration_TelegramCollector_OutboxByMode`).
- `REQ-10` (text only) → `FilterTextItems` (`internal/biz/collect.go:128`),
  applied inside `CollectItems` so both pull and push paths honour it. Covered by
  `SC-09` (`TestFilterTextItems`) and "non-text item never reaches Create".
- `REQ-11` (autotests) → biz unit (TDD), data httptest, task testcontainers +
  white-box internal tests, all with `goleak`.

## Изменённые файлы

Status is **relative to `main` (`8a26c0f`)**: every FT-009 file is new or
modified vs the base.

### `biz/`
- `internal/biz/collect.go` (new) — `CollectedItem`; ports `Fetcher`/`RSSFetcher`/
  `WebFetcher`/`PostCreator`/`TelegramSession`; `TelegramMessage`;
  `ErrTelegramNotAuthorized`; `CollectUsecase` (`CollectSource` = `Fetch` +
  `CollectItems`); `FilterTextItems`. No `ent`/`http`/`sql`/`gotd` imports
  (`INV-T02`). *Touched this pass:* named-port doc comments + `//nolint:iface`.
- `internal/biz/source.go` (new) — `TelegramChannelConfig`/`TelegramGroupConfig`
  carry `TgID` for push routing.
- `internal/biz/collect_test.go` (new) — `FilterTextItems` + `CollectSource` +
  `CollectItems` cases (happy mapping, non-text skip, duplicate not-fatal,
  fetch-error wrap, `ErrPostSourceNotFound` skip, empty fetch). *Touched:*
  `ptr` → `new(expr)`.
- `internal/biz/mock/mock_collect.go` (new) — mockgen for `Fetcher`/`PostCreator`/
  `TelegramSession`. (Generated; `MockTelegramSession` currently unused — see
  `## Open Questions`.)
- `internal/biz/wire.go` (modified) — `NewCollectUsecase` + `PostCreator` bind.

### `data/`
- `internal/data/rss_fetcher.go` (new) — stdlib `encoding/xml` behind
  `biz.RSSFetcher`.
- `internal/data/web_fetcher.go` (new) — stdlib HTML extraction behind
  `biz.WebFetcher`. *Touched:* `//nolint:exhaustive` on the `atom.Atom` switch.
- `internal/data/telegram_session.go` (new) — `gotd/td` push session behind
  `biz.TelegramSession`; gotd types live only here (`INV-T02`); never auto-login
  (`AG-02`, `ErrTelegramNotAuthorized`). *Touched:* `%w`-wrapped auth errors.
- `internal/data/rss_fetcher_test.go`, `web_fetcher_test.go` (new) — httptest.
- `internal/data/telegram_session_test.go` (new) — white-box unit for
  `authorFrom`/`chatTitleFrom`. *Touched:* `//nolint:testpackage`, `new(expr)`.
- `internal/data/wire.go` (modified) — `New*Fetcher` + `NewTelegramSession`
  providers and named binds.

### `task/`
- `internal/task/rss_collector.go` (new) — `transport.Server` polling loop,
  per-source `continue`-on-error, injectable `after` seam, `WaitGroup` Stop.
  *Touched:* `[time.After]` doc-link.
- `internal/task/web_collector.go` (new) — same lifecycle + single-retry-then-skip.
- `internal/task/telegram_collector.go` (new) — push `transport.Server`:
  `Start` runs the session goroutine (`startOnce`, parent-ctx `WithCancel`),
  `handle` resolves source by `TgID` and persists via `ItemCollector`, `Stop`
  cancels + `wg.Wait`. *Touched:* `context.WithCancel(ctx)` (was `Background()`).
- `internal/task/rss_collector_test.go`, `web_collector_test.go` (new) —
  testcontainers integration. *Touched:* `collectorsCfg()` (param dropped).
- `internal/task/telegram_collector_test.go` (new) — routing unit +
  `TestIntegration_TelegramCollector_OutboxByMode` (channel/group outbox).
  *Touched:* AAA comments.
- `internal/task/collector_internal_test.go` (new, white-box) — deterministic
  tests for unexported helpers (cadence SC-08, `collectWithRetry`,
  `collectAllOfType`), plus the two new `TestTelegramCollector_handle_*`
  resilience tests. Out-of-whitelist — see below. *Touched:* `require.ErrorIs`,
  new tests.
- `internal/task/testhelper_test.go` (modified) — shared task test stack.
- `internal/task/wire.go` (modified) — `NewRSSCollector`/`NewWebCollector`/
  `NewTelegramCollector` providers.

### `cmd/`
- `cmd/feedium/wire.go`, `wire_gen.go` (modified) — three collectors registered
  in `kratos.Server(...)`; reference `bc.GetCollectors()`.

### `configs/`
- `internal/conf/conf.proto` (modified) — `Collectors`/`CollectorPoll`/
  `TelegramCollector` messages + `Bootstrap.collectors = 4`. `TelegramCollector`
  carries `api_id`/`api_hash`/`phone`/`session_path`, **no** `poll_interval`
  (push, `DEC-08`).
- `internal/conf/conf.pb.go` (regen) — `make proto` output; loads without init
  panic (verified, `go test ./internal/conf/...`).
- `configs/config.yaml`, `configs/local.example.yaml` (modified) — `collectors:`
  with per-type `poll_interval` (RSS/web) and Telegram session/secret
  placeholders.

### `deps/`
- `go.mod`, `go.sum` (modified) — **`github.com/gotd/td` added** (Telegram push
  client, `DEC-03`/`DEC-08`); `golang.org/x/net` promoted indirect→direct (web).

## Контракты

- Config (`CTR-T01`): `conf.proto` + `conf.pb.go` expose `Collectors` /
  `CollectorPoll` / `TelegramCollector` and `Bootstrap.GetCollectors()`.
- DB: **unchanged** (`INV-T06`, `CON-05`, `ASM-02`); no schema/migration edits,
  no collection-cursor field (`DEC-05`). Telegram durability is in-memory
  updates-state, no backfill (`DEC-08`).
- Proto API / `service/`: unchanged (`NS-02`).

## Решения и trade-offs

- **Telegram push, not polling (`DEC-08`).** One long-lived `gotd/td` userbot
  connection with `updates.Manager` delivers new channel/group messages in real
  time through the existing biz collect path. `REQ-08` (poll interval) does not
  apply to Telegram; the `biz.Fetcher` port is not used for it (instead
  `biz.TelegramSession` + `CollectUsecase.CollectItems`). RSS/web stay on polling.
- **gotd isolated behind a seam (`INV-T02`).** All `gotd/td` types live only in
  `internal/data/telegram_session.go`; biz/task see `biz.TelegramMessage` /
  `biz.CollectedItem`. Live login is never automated (`AG-02`).
- **Reuse `PostUsecase.Create` (`DEC-04`/`CTR-T03`).** `CollectItems` →
  `FilterTextItems` → `Create` gives `REQ-06`/`REQ-09`/`REQ-10` for both pull and
  push for free; `CollectSource` = `Fetch` + `CollectItems` keeps RSS/web
  behaviour identical (behavioural equivalence confirmed by AGENT_2 §5).
- **RSS/web on stdlib (deviates from `DEC-01`/`DEC-02`).** RSS uses
  `encoding/xml`; web uses the already-present `golang.org/x/net/html`. Both sit
  behind `biz.Fetcher`, so adopting `gofeed`/`go-readability` later is a `data/`
  -only change (`OQ-B`).

## Verification

**Gates executed this run (toolchain available):**

- `go build ./...` — **pass**.
- `go vet ./...` — **pass**.
- `golangci-lint run ./internal/biz/... ./internal/task/... ./internal/data/...`
  — **0 FT-009 findings** (all `code_review` lint findings fixed). NB: the branch
  also carries **pre-existing FT-008 lint debt** (`internal/biz/prompt/*`,
  `internal/biz/summary_usecase_test.go`, `internal/data/llm_provider_test.go`) —
  out of FT-009 scope, but it keeps a repo-wide `make lint` red until addressed
  separately (`OQ-D`).
- `go test ./...` — **all packages pass**, including the testcontainers
  PostgreSQL integration tests (`internal/data` 51s, `internal/task` 21s) and
  `goleak` in every worker test. The Telegram push outbox-by-mode integration and
  the two new push resilience tests are green.
- `go test ./internal/conf/...` — **pass**; `conf.pb.go` descriptor loads without
  an `init()` panic, so byte-parity is instrumentally confirmed (closes `OQ-C`).

## Out-of-whitelist changes

- `internal/task/collector_internal_test.go` (**new**, not in spec `## Files`).
  Reason: deterministic regression coverage for SC-08 (cadence), FM-T02 (retry),
  `collectAllOfType` pagination/list-failure, and the Telegram push resilience
  branches — all exercise **unexported** helpers the black-box `task_test` files
  cannot reach. Same package/dir as the feature code; no production surface.
- **Whitelist deviation (Telegram push, `DEC-08`).** spec `## Files` lists
  `internal/data/telegram_fetcher.go`(+`_test`) and a single
  `internal/task/telegram_collector.go`(+`_test`) under the *polling* assumption.
  The push design replaces the fetcher with `internal/data/telegram_session.go`
  (+`_test`) and adds the biz push seam (`CollectItems`, `TelegramSession`,
  `TelegramMessage`, `ErrTelegramNotAuthorized`). `internal/biz/source.go` was
  also added for `TgID` config. Justified by `DEC-08`
  ([2026-05-26-telegram-push-design.md](2026-05-26-telegram-push-design.md)).
- `go.mod`/`go.sum`: `github.com/gotd/td` added (in spec's `deps` whitelist as
  "Telegram-клиент").

## Open Questions

- `OQ-A` **Telegram live login (`AG-02`).** The push session connects and streams
  but never performs first-time phone-code login (`ErrTelegramNotAuthorized`).
  Provisioning a real userbot session is a manual human step; until done,
  Telegram collection is exercised only by fake-session tests.
- `OQ-B` **RSS/HTML library choice (`DEC-01`/`DEC-02`).** Shipped stdlib; confirm
  whether to keep it for MVP or adopt `gofeed`/`go-readability`. Port-isolated.
- `OQ-C` ~~toolchain regen + test run~~ **resolved this run** — gates executed and
  green (see `## Verification`).
- `OQ-D` **Pre-existing FT-008 lint debt** keeps repo-wide `make lint` red; out of
  FT-009 scope, flagged for the FT-008 owner.
- `OQ-E` **`resolve` peer-type confusion (AGENT_2 R1, follow-up).** `resolve`
  matches an incoming message's `TgID` against both Telegram source types by raw
  `int64`; channel-id and chat-id spaces could in principle collide and misroute.
  Low probability, but the message loses peer-type after `emit`. Follow-up: carry
  the peer type to `resolve`.
- `OQ-F` **Uncached per-message source listing (AGENT_2 R2, follow-up).** The
  design doc promised an in-memory `TgID→Source` index with TTL; the impl lists
  sources on every message. Acceptable at MVP volume (`CON-03`) but a divergence
  from the design and a load risk at scale.
- `OQ-G` **`emit`-mapping unit coverage (AGENT_4, follow-up).** The
  `tg.Message → biz.TelegramMessage` mapping (incl. the `m.Out` self-message skip
  and peer-type selection) is only exercised via the live-gated session, not a
  unit test. Extract `emit` to cover it without a session. Also: remove the unused
  generated `MockTelegramSession` or add a biz-level test that uses it.
