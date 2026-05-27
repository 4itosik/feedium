All three review findings are now resolved. Summary of what changed:

**Blocking finding (route_to `feature`) — fixed at the feature level:**
- `feature.md` `DEC-01` (line 53): rewritten from a Plan Ready **blocker** into a resolved decision — FT-006 is strictly additive and does not touch `source.go`; the mutating-command disposition is explicitly deferred to `OQ-02`. This unblocks Plan Ready.
- `feature.md` `OQ-02` (new): a **real** open question hosting the `source create/update/delete` disposition (delete in a follow-up supervised feature vs. accept a mixed read+write tool). The decision is now legitimately owned by `feature.md`.
- `spec.md` `DEC-T2`: now references `feature.md OQ-02` (the correct entry) instead of the false `OQ-01` reference, and notes that `DEC-01` was moved off the Plan Ready blocker.

**Warning — coverage target:** added explicit **>80% line coverage** target ([`testing-policy.md:92`](memory-bank/engineering/testing-policy.md)) to the test-after bullet in `## Tests`, with the measurement command and `EVID-02` binding.

**Warning — conditional fixtures:** resolved the `testdata/*_get/ (if golden tests used)` ambiguity by **dropping** the speculative golden-fixture entries. Verified against the codebase: existing read-command and render tests use inline table-driven assertions; `testdata/` is used only by the `source_delete` mutation (out of FT-006 scope). Added a note in `## Files` stating FT-006 introduces no golden fixtures.

The additive post/summary read delta itself was already correct and grounded — I left it intact. The spec is now internally consistent and its DEC-T2 grounding is true against `feature.md`.

One note: I proceeded with **Defer to follow-up** because the question popup was dismissed without a selection, and that path is the reviewer's endorsed fix plus the lowest-risk option. If you actually want **Delete-in-FT-006** (truly read-only `feediumctl`) or **Accept-mixed-tool** (rewrite `NS-01`), say so and I'll redo `feature.md`/`spec.md` accordingly.