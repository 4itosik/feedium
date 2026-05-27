# Brief format

A **brief** is the only required input to the pipeline: a markdown file with required
frontmatter and a free-form body. Quality of output tracks the detail of the brief — a thin
brief means more escalations at `brief_review`.

## Required frontmatter

Minimum: a `slug`.

```markdown
---
slug: FT-042
---

# What's needed

One or two paragraphs describing the task.
```

## Frontmatter fields

| Field | Required? | Purpose |
| --- | --- | --- |
| `slug` | **yes** | Unique feature id. Format `FT-NNN` (3–4 digits) or `FT-NNN-<kebab-name>`. |
| `title` | recommended | Human-readable name; reaches the prompt. |
| `description` | no | One line of context. |
| others | no | Arbitrary YAML; available to prompts as `.Brief`. |

The `slug` determines the output directory: Ferry writes artifacts to
`memory-bank/features/<slug>/`. Use the full `FT-NNN-<kebab-name>` form when the project's
`memory-bank/features/` already follows that naming convention.

## Validate before running

```sh
ferry validate-brief <path>          # prints the run-id that start would assign
ferry validate-brief <path> --json   # machine-readable
```

It checks: file exists and is readable; frontmatter is valid YAML; `slug` is present,
non-empty, and matches `FT-NNN[-kebab]`; the body is non-empty. It does **not** start a run.

## Common errors

- **`slug missing` / `slug invalid format`** — add `slug: FT-042`. Allowed: `FT-NNN` or
  `FT-NNN-<kebab-name>`.
- **`unclosed frontmatter`** — the closing `---` after the YAML block is missing.
- **`bad YAML`** — usually indentation, or a colon in a value that needs quoting.

A copyable skeleton: [`../templates/brief.md`](../templates/brief.md).
