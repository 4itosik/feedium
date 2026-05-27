# Run lifecycle & reading `ferry status`

## The 8 stages

The MVP pipeline is hard-wired:

```
brief_review → feature → feature_review → spec → spec_review → implement → code_review → verify
```

| Stage | Kind | Produces / judges | Notes |
| --- | --- | --- | --- |
| `brief_review` | review | judges the brief | Non-routing: on any ambiguity it **escalates immediately**, no retries. Briefs are fixed by a human. |
| `feature` | prompt | `feature.md` | Product decomposition. |
| `feature_review` | review | judges `feature.md` | Routes back to `feature` only. |
| `spec` | prompt | `spec.md` | Technical spec, incl. a `## Files` whitelist used by handoff. |
| `spec_review` | review | judges `spec.md` | Routes back to `feature` or `spec`. |
| `implement` | implement | `implement.md` (+ code) | The only stage that can be human-in-the-loop (handoff). |
| `code_review` | review | judges the implementation (quality gate) | Routes back to `implement` only — quality signals must not change scope. |
| `verify` | review | judges the implementation (conformance gate) | Includes a smoke build; routes back to `feature`, `spec`, or `implement`. |

## Route-back on review failure

A review stage returns `pass`/`fail`; on `fail` it sets `route_to: <stage>` and the run jumps
**back**, incrementing that review stage's attempt counter. Allowed routes are fixed:

| Review stage | May route to |
| --- | --- |
| `brief_review` | nowhere (escalates instead) |
| `feature_review` | `feature` |
| `spec_review` | `feature`, `spec` |
| `code_review` | `implement` |
| `verify` | `feature`, `spec`, `implement` |

Review stages have `MaxAttempts = 3`. After 3 consecutive `fail`s the run escalates with
reason `review_exhausted` (see [`escalations.md`](escalations.md)).

## Run FSM

```
pending → in_progress ─┬─→ done
                       ├─→ escalated         (pause → ferry resume)
                       ├─→ awaiting_handoff    (pause → ferry continue)
                       └─→ aborted             (terminal)
```

`escalated` and `awaiting_handoff` are pauses, not endings. `aborted` is terminal — start a
new run.

## Reading `ferry status <run-id>`

`ferry status` prints:

- the run status (one of the FSM states above);
- the final memory-bank entry path if status is `done`;
- the last failed stage id if status is `aborted` (`FailedStageID`) plus an `ErrorClass`;
- a per-stage table: `stage_id`, `status` (`done`/`failed`), `exit_code`, `duration_ms`,
  `cost_usd`, `error_class`;
- an **Attempts** block for review stages, e.g. `feature_review: 2/3` (current/max).

`feature_review: 3/3` means three consecutive `fail`s with route-back to `feature` — the next
attempt would escalate with `review_exhausted`.

## Crash / Ctrl+C

If Ferry is killed mid-stage, `state.json` stays consistent on disk. `ferry resume <run-id>`
**restarts the current stage from scratch** — it does not resume a partial provider call.
