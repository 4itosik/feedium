# Escalations

An escalation is an asynchronous, file-based dialog: Ferry hit a Definition-of-Done failure
and won't continue without a human. The run goes to status `escalated` and Ferry exits. You
read the request, write a response, and run `ferry resume`.

## When it triggers

| `reason` | Stage | Meaning |
| --- | --- | --- |
| `brief_ambiguous` | `brief_review` | Brief is unclear or contradictory. |
| `review_exhausted` | any review | 3 attempts, still `fail`. |
| `missing_implement_doc` | `verify` | `implement` produced no `implement.md`. |
| `stage_failed` | any prompt stage | Provider failed or returned an empty artifact. |
| `unexpected_paths` | `implement` | Working tree went outside the `## Files` whitelist (see [`handoff.md`](handoff.md)). |
| `timeout` | any | Stage exceeded its timeout. |

(Exact reason strings can vary by version; the workflow below is the same regardless.)

## Folder layout

```
.ferry/runs/<run-id>/escalations/001-feature-stage_failed/
├── request.md      # written by Ferry: what happened, what's needed
├── meta.json       # metadata (read-only)
└── response.md     # ← you create this
```

The folder name is `NNN-<stage>-<reason>`. A run has at most **one** active escalation. Find
its path with:

```sh
ferry escalation <run-id>
```

## Reading `request.md`

Frontmatter (`run_id`, `escalation_id`, `stage`, `reason`, `attempts`, `max_attempts`,
`error_class`) plus a body: **Context** (why), **What was tried** (last error text),
**Files to review** (where to look), **Review findings** (if from a review stage, with
`route_to` and severity), **Suggested response**.

## Writing `response.md`

```markdown
---
run_id: 2026-05-15-FT-042-3a9c12
escalation_id: 001-feature-stage_failed
action: retry
---

Free-form markdown explaining the fix / decision.
```

- `run_id` and `escalation_id` **must match** `request.md` (guards against applying a stale
  answer).
- `action` is one of:
  - `retry` — Ferry deletes the stage artifact (if any) and **re-runs the stage from
    scratch**. The body is passed into the prompt as `{{ .EscalationNotes }}`.
  - `edit` — Ferry **skips the stage** and moves on. Use when you manually fixed the artifact
    (e.g. `feature.md`) and want the run to proceed with your edits.
  - `abort` — Ferry ends the run as `aborted`. Nothing written to memory-bank.

Skeleton: [`../templates/escalation-response.md`](../templates/escalation-response.md).

## Resume

```sh
ferry resume <run-id>
```

Ferry validates the frontmatter (matching `run_id`/`escalation_id`), applies the action, and
returns the run to `in_progress`. (`ferry resume` is also what you use after a crash.)
