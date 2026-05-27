# Ferry CLI reference

Run everything from the **project root** (Ferry expects `.ferry/` in the current directory).
Global flags: `-h, --help` (per command), `-v, --version`.

| Command | Signature | Purpose | Key flags |
| --- | --- | --- | --- |
| `init` | `ferry init` | Scaffold the per-project `.ferry/` tree (idempotent; existing files are skipped). | `--dir <path>` — project root to scaffold (default: cwd) |
| `validate-brief` | `ferry validate-brief <path>` | Validate a brief without starting a run; prints the run-id that `start` would assign. | `--json` — machine-readable output |
| `start` | `ferry start <brief.md>` | Start a **new** run for the brief; prints the run-id. | `--model <id>` — override model for all stages (e.g. `claude-opus-4-5`); `--effort <level>` — override effort for all stages (`low\|medium\|high\|xhigh\|max`) |
| `status` | `ferry status <run-id>` | Show run status, per-stage table, and review attempts. | — |
| `escalation` | `ferry escalation <run-id>` | Print the path to the active escalation folder. | — |
| `resume` | `ferry resume <run-id>` | Re-enter a run after an **escalation** (applies `response.md` action) or after a crash (restarts the current stage). | — |
| `continue` | `ferry continue <run-id>` | Resume a run from **awaiting_handoff** (applies the handoff `response.md` and checks the working tree). | — |
| `abort` | `ferry abort <run-id>` | Mark a run `aborted` and release any stale implementation lock. Terminal; nothing is written to memory-bank. | — |
| `list` | `ferry list` | Show all runs in the current project. | — |
| `render-prompt` | `ferry render-prompt <run-id> <stage>` | Render a stage's resolved prompt **without** invoking the provider. For debugging prompt templates. | — |
| `completion` | `ferry completion <shell>` | Generate a shell autocompletion script. | — |

Notes:
- `render-prompt` takes the run-id **first**, then the stage id (both positional).
- `start` always creates a new run. To continue an existing one use `resume`/`continue`.
- Stage ids accepted by `render-prompt`: see [`lifecycle-and-status.md`](lifecycle-and-status.md).
- `--model`/`--effort` on `start` are run-wide overrides; finer per-stage configuration lives
  in `.ferry/config.yaml` — see [`custom-prompts.md`](custom-prompts.md).
