# Custom prompts

Ferry ships builtin prompts for all 8 stages. Projects override them per stage. As operator
you mostly need to know **how to inspect and override** prompts, and how to debug them.

## Resolution order (project > builtin)

Resolving prompt `<name>`:

1. `<project>/.ferry/prompts/<name>.md.tmpl` — project override.
2. builtin embedded in the Ferry binary.

First match wins. If neither exists, the run fails with `ErrPromptNotFound`.

Override files in `.ferry/prompts/` are tracked by git (gitignore covers only `runs/`) — by
design, so prompt changes go through code review.

## Builtin prompt names (one per stage)

`brief_review`, `feature`, `feature_review`, `spec`, `spec_review`, `implement`,
`code_review`, `verify`.

To start from a builtin as a template, copy the corresponding `internal/prompt/builtin/<name>.md.tmpl`
from the Ferry source into `.ferry/prompts/<name>.md.tmpl` and edit. (On MVP the `implement`
builtin is a placeholder — override it for production use.)

## Template context (`PromptContext`)

Prompts are Go `text/template` + [sprig], with a few Ferry file helpers. Key fields:

| Path | What |
| --- | --- |
| `.Run.ID`, `.Run.Status`, `.Run.BriefSlug`, `.Run.BriefPath`, `.Run.StartedAt` | run metadata |
| `.Stage.ID`, `.Stage.AttemptNum`, `.Stage.AllowedRoutes` | current stage |
| `.Artifacts.Brief` / `.Feature` / `.Spec` / `.Implement` | upstream artifacts (when produced) |
| `.Project.Root`, `.Project.MemoryBankDir` | absolute project paths |
| `.EscalationNotes` | body of the last escalation `response.md` (empty if none) |
| `.ReviewFindings`, `.ReviewNotes` | findings/narrative from the last review of this stage |

Each `.ReviewFindings` entry has `.Severity` (`blocking`/`warning`/`info`), `.RouteTo`,
`.Summary`.

Ferry file helpers (paths are relative to project root, sandboxed): `include "path"`,
`excerpt "path" N`, `list "glob"`, `readDir "dir"`, `readDirRecursive "dir"`. The sprig
functions `env`, `expandenv`, `getHostByName` are disabled (no process secrets in prompts).

## Inspect a resolved prompt without calling the provider

```sh
ferry render-prompt <run-id> <stage>
```

Resolves the prompt with that run's context and prints the result — no provider call. Run-id
comes **first**, then the stage id (see [`cli-reference.md`](cli-reference.md)).

## Per-stage model / effort

Beyond run-wide `ferry start --model/--effort`, `.ferry/config.yaml` can set models per stage:
a `models` block with `models.default` and `models.stages.<stage-id>` (model + effort).
Precedence: code default → config global → config per-stage → CLI override.
