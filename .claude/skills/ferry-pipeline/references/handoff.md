# Implementation handoff

The `implement` stage is the only one that can be **human-in-the-loop** instead of automated.
When configured that way, the run pauses at `implement` with status `awaiting_handoff`, and a
human (or you, acting as the implementer) does the work, then runs `ferry continue`.

Use it when: the stage needs IDE/internal tools with no CLI; the task is too sensitive (DB
migration, secrets, security fix); or you want to write the code yourself but still record
`implement.md` in memory-bank.

## How Ferry enters handoff

On reaching `implement`, Ferry:

1. resolves the implement prompt;
2. creates `.ferry/runs/<run-id>/handoffs/001-implement/` with:
   - `prompt.md` — the full resolved prompt,
   - `expected.md` — instructions for the human (what to produce and where),
   - `meta.json` — run metadata;
3. sets the run to `awaiting_handoff` and exits.

The handoff folder path appears in `ferry status <run-id>`.

## What to do

1. Read `prompt.md` in the handoff folder.
2. Do the work — edit project files per `spec.md ## Files`, write code, run tests.
3. Create `response.md` in the same folder. For `implement` this is a **textual description**
   of what you did — it becomes the final `implement.md` in memory-bank. Skeleton:
   [`../templates/handoff-response.md`](../templates/handoff-response.md).
4. Run `ferry continue <run-id>`.

## Path whitelist & `unexpected_paths`

Code handoff checks the working tree: Ferry diffs your changes (`git diff`) against the
whitelist in `spec.md`'s `## Files` section.

- All changes inside the whitelist → `ferry continue` accepts.
- Files **outside** the whitelist → the run escalates with `reason=unexpected_paths`. This is
  not fatal: confirm via `response.md` that the changes are needed and proceed, **or** bring
  the working tree back in line with `## Files`, **or** update `spec.md` by hand before
  `ferry continue`.

## Changed your mind

There is no "cancel handoff" command in MVP. Either write an honest `response.md` and
`ferry continue` (the run proceeds with what exists), or `ferry abort <run-id>` (the run ends
`aborted`, nothing written to memory-bank).
