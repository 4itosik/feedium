---
name: ferry-pipeline
description: Use when operating the Ferry pipeline CLI in a project — a task needs to be run or resumed through ferry, a brief must be written for ferry, a ferry run is escalated or awaiting handoff, or ferry status / start / resume / continue come up. Triggers on "ferry", ".ferry/", "прогони задачу через ferry".
---

# Operating the Ferry pipeline

Ferry is a local CLI that drives a structured **brief** through an 8-stage development
pipeline, pausing via file-based **escalations** and **handoffs**, and writing the final
artifacts to `memory-bank/features/<slug>/`. Your job here is the **operator** role: take a
task, run it through Ferry, and make the operator decisions — not to write code Ferry asks
for on its own stages.

This skill is self-contained: everything you need is here and in `references/`. Don't rely on
any repo-only docs at runtime — you operate Ferry in arbitrary projects that won't have them.

## Mental model

**Pipeline (8 stages, hard-wired in MVP):**

```
brief_review → feature → feature_review → spec → spec_review → implement → code_review → verify
   (review)   (prompt)    (review)      (prompt) (review)     (impl)      (review)     (review)
```

- **prompt** stages produce an artifact (`feature.md`, `spec.md`, `implement.md`).
- **review** stages judge an artifact `pass`/`fail`; on `fail` they may route the run
  **back** to an earlier stage. Allowed routes and the exhaustion rule are in
  [`references/lifecycle-and-status.md`](references/lifecycle-and-status.md).

**Run FSM:**

```
pending → in_progress ─┬─→ done              (artifacts written to memory-bank)
                       ├─→ escalated          (needs a human answer → resume)
                       ├─→ awaiting_handoff    (needs manual work → continue)
                       └─→ aborted             (terminal; start a new run)
```

`escalated` and `awaiting_handoff` are **pauses**, not endings.

## Preflight

Before anything, from the **project root** (Ferry expects `.ferry/` in the current dir):

1. `ferry --version` — confirm the binary is available. If not, it must be built/installed
   (clone + `make build`); without it you cannot operate.
2. Confirm `.ferry/` exists. If not, the project hasn't run `ferry init` — see
   [`references/cli-reference.md`](references/cli-reference.md) (`ferry init`). A
   `memory-bank/` directory must also already exist; Ferry does not create it.
3. Always run Ferry from the project root. From a subdirectory it won't find `.ferry/`.

## Operate loop

1. **Author/validate the brief.** A brief is a markdown file with required frontmatter
   (`slug`). Validate without running: `ferry validate-brief <brief.md>`. Format, slug
   rules, and common errors: [`references/brief-format.md`](references/brief-format.md).
   Template: [`templates/brief.md`](templates/brief.md).
2. **Start the run.** `ferry start <brief.md>` → capture the printed **run-id**
   (`YYYY-MM-DD-<slug>-<hash>`). A repeated `start` creates a **new** run, never reuses one.
3. **Monitor.** `ferry status <run-id>` and branch on the status (router below). Re-run
   `status` as needed. Reading the status table and attempts:
   [`references/lifecycle-and-status.md`](references/lifecycle-and-status.md).

## Decision router

| `ferry status` says | What it means | What you do |
| --- | --- | --- |
| `done` | Pipeline finished | Report the memory-bank entry path (`memory-bank/features/<slug>/`). Done. |
| `escalated` | Ferry needs a human answer | Resolve via [`references/escalations.md`](references/escalations.md): read `request.md`, write `response.md` (`action`: retry/edit/abort), then `ferry resume <run-id>`. |
| `awaiting_handoff` | A stage needs manual work | Resolve via [`references/handoff.md`](references/handoff.md): read `prompt.md`, do the work, write `response.md`, then `ferry continue <run-id>`. |
| `aborted` | Terminal failure | Read `FailedStageID`/`ErrorClass` from status; inspect `.ferry/runs/<run-id>/runtime/<stage>/`; fix brief/prompts/env and start a **new** run. The old run stays as forensics. |
| `in_progress`/`pending` | Still running | Wait and re-run `ferry status`. |

To customize stage prompts (project overrides of builtins), see
[`references/custom-prompts.md`](references/custom-prompts.md).

## Hard rules

- **`resume` ≠ `continue`.** `ferry resume` is for `escalated` (applies `response.md` action);
  `ferry continue` is for `awaiting_handoff` (applies handoff `response.md` + checks the
  working tree). Using the wrong one errors out — don't guess, match it to the status.
- **A brief is edited by a human/operator, not silently rewritten by you mid-run.**
  `brief_review` escalates immediately on ambiguity, by design — fix the brief, then resume.
- **A repeated `ferry start` makes a NEW run.** Never assume it resumes; use `resume`/`continue`.
- **Run from the project root**, always.
- After a crash/Ctrl+C, `ferry resume <run-id>` **restarts the current stage from scratch**
  (it does not resume a partial provider call).
