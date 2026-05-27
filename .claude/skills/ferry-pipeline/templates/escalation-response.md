---
run_id: <copy from request.md — must match>
escalation_id: <copy from request.md — must match>
action: retry   # one of: retry | edit | abort
---

Explain the fix or decision.

- For `action: retry` — this body is fed into the stage's prompt as `{{ .EscalationNotes }}`,
  so give the model the guidance it was missing.
- For `action: edit` — note what you changed by hand; Ferry skips the stage and proceeds.
- For `action: abort` — note why you're stopping; the run ends `aborted`.
