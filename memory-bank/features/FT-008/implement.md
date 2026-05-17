---
doc_kind: feature
doc_function: implement
purpose: "Implementation log for FT-008: split the single summarization system prompt into two embedded artifacts selected by ProcessingMode."
derived_from:
  - ../../../briefs/FT-008-target-prompts.md
  - spec.md (FT-008)
status: active
delivery_status: in-review
---

# FT-008: Summarization Prompts Split — Implementation Log

## Summary

Replaced the single `defaultSystemPrompt` literal in `internal/data/llm_provider.go` with two `//go:embed`-backed `.txt` artifacts under `internal/biz/prompt/`, chosen at the call site in `SummaryUsecase` by `biz.ProcessingMode`. The data-layer LLM provider became mode-agnostic: it receives the system prompt as an argument and forwards it as `messages[0].content`. Resolver wiring lives in a new leaf package that imports `biz` (but is **not** imported by `biz`), avoiding the import cycle flagged by the prior spec review.

## Build / test status

- `make build` — passes (verified locally).
- `make test`, `make lint` — **not executed in this session** because `go test` / `golangci-lint` are blocked by the current sandbox. Please run `make test && make lint` before merging.

## Acceptance traceability

| Acceptance criterion (spec) | Where it's satisfied |
| --- | --- |
| Two non-empty, byte-different artifacts | `internal/biz/prompt/self_contained.txt`, `internal/biz/prompt/cumulative.txt`; `TestNewResolver_BodiesNonEmpty`, `TestNewResolver_BodiesDiffer` (`internal/biz/prompt/prompt_test.go`) |
| Resolver impl + boot-time validation panics | `internal/biz/prompt/prompt.go` (`NewResolver` panics via `validate()`); `TestNewResolver_PanicsOnDriftingState` |
| `biz` does not import `internal/biz/prompt` | `internal/biz/wire.go` unchanged; resolver wired from `cmd/feedium/wire.go` via `prompt.ProviderSet`; verified by `make build` |
| `SummaryPromptResolver` interface + `ErrUnknownSummaryPromptMode` sentinel in `biz` | `internal/biz/summary.go` |
| `AllProcessingModes()` registry function | `internal/biz/source.go` |
| Updated `SummaryUsecase` (resolver field, `summarizeWithLLMRetry(mode)`, resolved once per event) | `internal/biz/summary_usecase.go`; `TestSummaryUsecase_PromptResolvedOncePerEvent` |
| Mode → prompt mapping on each call site | `ProcessPostEvent` passes `ProcessingModeSelfContained`; `ProcessSourceEvent` passes `ProcessingModeCumulative` (verified by `TestSummaryUsecase_PromptByMode_SelfContained` / `_Cumulative` / `_Isolation`) |
| `defaultSystemPrompt` deleted; provider passes through | `internal/data/llm_provider.go`; `git grep -n "defaultSystemPrompt"` returns zero hits; `TestOpenRouterProvider_Summarize_SendsSystemPrompt`, `TestOpenRouterProvider_Summarize_EmptySystemPrompt_ReturnsError` |
| Wire updated | `cmd/feedium/wire.go` + regenerated `cmd/feedium/wire_gen.go` |
| Mocks regenerated | `internal/biz/mock/mock_summary.go` updated by hand to mirror the new mockgen output (new `Summarize` arity, new `MockSummaryPromptResolver`). Run `mockgen -source=internal/biz/summary.go -destination=internal/biz/mock/mock_summary.go -package=mock` before merge to confirm byte-equivalence. |
| Resolver failure is terminal | `TestSummaryUsecase_PromptResolveError_IsTerminal_SelfContained` |

## File-by-file changes

### New files

#### `internal/biz/prompt/self_contained.txt` (new)
Plain-text body of the self-contained system prompt. Byte-for-byte equal to the previous `defaultSystemPrompt` literal so that current callers (RSS/HTML/Telegram channel) see no behavioral change.

#### `internal/biz/prompt/cumulative.txt` (new)
Plain-text body of the cumulative system prompt. Differs from `self_contained.txt` only by the smallest concrete change required by NS-10 / CTR-01 (a one-clause preface stating the cumulative context). Substantive cumulative rewriting is deliberately out of scope and is the next iteration.

#### `internal/biz/prompt/prompt.go` (new)
- Package `prompt` (leaf): imports `biz`, never imported by `biz`.
- Two `//go:embed` directives bind the `.txt` files into `selfContainedBody` / `cumulativeBody` string vars.
- `embeddedResolver` keeps a `map[biz.ProcessingMode]string`; `NewResolver()` builds it and calls `validate()`. On any failure (missing mode, empty body, registry mismatch) `NewResolver` **panics** — this is intentional boot-time loudness (FM-01 / FM-02).
- `For(mode)` returns the raw embedded body unmodified (no trim), or `("", biz.ErrUnknownSummaryPromptMode)` for an unknown mode. The sentinel lives in `biz` so callers can `errors.Is` without importing this package.

#### `internal/biz/prompt/wire.go` (new)
Exports `prompt.ProviderSet`, which binds `*embeddedResolver` to `biz.SummaryPromptResolver`. Wired in from `cmd/feedium/wire.go` to keep `biz` free of any `prompt` import.

#### `internal/biz/prompt/prompt_test.go` (new)
White-box tests in `package prompt`:
- `TestNewResolver_ProvidesAllModes` — table-driven over `biz.AllProcessingModes()`; asserts non-empty body for every registered mode (CHK-PROMPT-TOTAL).
- `TestNewResolver_BodiesDiffer` — `selfContainedBody != cumulativeBody` (CHK-PROMPT-DIFFER).
- `TestNewResolver_BodiesNonEmpty` — both embedded bodies non-empty after `TrimSpace`.
- `TestResolver_For_UnknownMode` — `("","unknown")` path returns `biz.ErrUnknownSummaryPromptMode` via `errors.Is`.
- `TestResolver_Validate_MissingMode`, `TestResolver_Validate_EmptyBody` — direct `validate()` calls verifying the two failure messages.
- `TestNewResolver_PanicsOnDriftingState` — asserts `panic` on empty body and on missing mode entries.
- `TestNewResolver_Production_DoesNotPanic` — sanity check that the real embedded files satisfy `validate()`.

#### `internal/data/llm_provider_test.go` (new)
- `TestOpenRouterProvider_Summarize_SendsSystemPrompt` — table-driven with two rows (self-contained body, cumulative body); httptest server captures the request body and asserts `messages[0].role=="system"`, `messages[0].content==<systemPrompt>`, `messages[1].role=="user"`, `messages[1].content==<text>` (CTR-PROVIDER / CTR-03).
- `TestOpenRouterProvider_Summarize_EmptySystemPrompt_ReturnsError` — passes `""` for the prompt, asserts the error mentions "empty system prompt" and that **zero** HTTP requests fire (FM-05).

### Edited files

#### `internal/biz/summary.go`
- Added `ErrUnknownSummaryPromptMode` to the sentinel error block.
- Updated `LLMProvider.Summarize` from `(ctx, text)` to `(ctx, systemPrompt, text)` with a doc-comment forbidding mode/prompt branching inside the provider.
- Added the `SummaryPromptResolver` interface (`For(mode ProcessingMode) (string, error)`), adjacent to `LLMProvider`. Per the project architecture rule, the cross-module contract is declared in `biz/`.

#### `internal/biz/source.go`
- Added `AllProcessingModes() []ProcessingMode`. This is the canonical registry consulted by `prompt.NewResolver` at boot to enforce totality (FM-02). The doc-comment instructs the author of any new `ProcessingMode` constant to update this slice in the same change; the resolver will panic at process start otherwise.

#### `internal/biz/summary_usecase.go`
- `SummaryUsecase` gained a `prompts SummaryPromptResolver` field.
- `NewSummaryUsecase` constructor signature gained the resolver argument (after `llmProvider`, before `txManager`).
- `ProcessPostEvent` now calls `summarizeWithLLMRetry(ctx, ProcessingModeSelfContained, post.Text)`.
- `ProcessSourceEvent` now calls `summarizeWithLLMRetry(ctx, ProcessingModeCumulative, concat)`.
- `summarizeWithLLMRetry` signature is now `(ctx, mode, text)`. It resolves the prompt **once** (outside the retry loop) — a resolver error is permanent and short-circuits with `fmt.Errorf("resolve prompt for mode %q: %w", mode, err)`. The retry loop body is otherwise identical to today (same `max_retries`, same `2^attempt` backoff, same ctx-cancel semantics; FT-005/FT-007 invariants preserved).

#### `internal/biz/wire.go`
- **Unchanged on purpose.** Adding a `prompt.ProviderSet` element here would force `biz` to import `prompt`, recreating the cycle the prior spec review flagged. The resolver is wired in from `cmd/feedium/wire.go` instead.

#### `internal/biz/mock/mock_summary.go`
- `MockLLMProvider.Summarize` and its recorder updated to the three-argument form `(ctx, systemPrompt, text)`.
- Added `MockSummaryPromptResolver` (struct, recorder, `NewMockSummaryPromptResolver`, `EXPECT`, and `For` method mock + recorder). Hand-edited to mirror what mockgen would produce; the `go:generate`-style header comment is preserved. Re-run `mockgen -source=internal/biz/summary.go -destination=internal/biz/mock/mock_summary.go -package=mock` once a network/cache-warm environment is available to confirm byte-equivalence with the official generator.

#### `internal/biz/summary_usecase_test.go`
- Both existing `biz.NewSummaryUsecase(...)` callsites (lines 69, 101) extended with a `nil` resolver argument — those paths exit on `HasActiveEvent` before reaching the resolver, so `nil` is safe.
- Added `errors` import.
- New helper `shortTimeoutSummaryCfg()` provides a minimal config object with `MaxRetries: 1` so retry-loop sleep does not dominate the test budget.
- New tests:
  - `TestSummaryUsecase_PromptByMode_SelfContained` — verifies that the system prompt the LLM sees for `ProcessPostEvent` is whatever the resolver returns for `ProcessingModeSelfContained` (captured via `DoAndReturn`).
  - `TestSummaryUsecase_PromptByMode_Cumulative` — symmetric for `ProcessSourceEvent` and `ProcessingModeCumulative`.
  - `TestSummaryUsecase_PromptByMode_Isolation` — two-row table-driven test that proves one mode's prompt never flows into the other call site (strict gomock catches any cross-mode resolver call).
  - `TestSummaryUsecase_PromptResolveError_IsTerminal_SelfContained` — resolver returns `ErrUnknownSummaryPromptMode`; usecase wraps it (`errors.Is`-compatible), the LLM is **not** called, and no `Summary` is saved.
  - `TestSummaryUsecase_PromptResolvedOncePerEvent` — `prompts.For` is expected `Times(1)` even when the LLM retries fire (transient error then success); guards against accidental "resolve per attempt".

#### `internal/biz/source_usecase_test.go`, `internal/biz/post_test.go`, `internal/biz/post_usecase_test.go`, `internal/biz/source_test.go`, `internal/biz/summary_retry_test.go`, `internal/biz/summary_test.go`
- Not modified. None of these construct `SummaryUsecase` and none reference `LLMProvider.Summarize` directly.

#### `internal/data/llm_provider.go`
- Removed the `defaultSystemPrompt` constant.
- `Summarize` signature is now `(ctx context.Context, systemPrompt, text string)`. The function returns `errors.New("empty system prompt")` if `systemPrompt == ""` (FM-05) before any HTTP marshaling — empty prompts must never reach the wire. `messages[0].content` is now sourced from the argument verbatim.
- The static interface-assertion line `var _ biz.LLMProvider = (*openRouterProvider)(nil)` is unchanged; the compiler enforces the new signature.

#### `internal/data/wire.go`
- **Unchanged.** The existing `wire.Bind(new(biz.LLMProvider), new(*openRouterProvider))` is signature-agnostic; only the method shape it satisfies changed.

#### `internal/task/testhelper_test.go`
- `fakeLLM.Summarize` signature updated to match the new `biz.LLMProvider` shape (the system-prompt argument is intentionally ignored — these are pool/scheduler integration tests, not prompt-flow tests).
- `newTestUsecase` now passes `prompt.NewResolver()` (the real production resolver) into `biz.NewSummaryUsecase(...)`. This keeps task-level integration tests faithful to the production wiring and confirms (indirectly) that the resolver constructs cleanly outside of Wire.
- Added `"github.com/4itosik/feedium/internal/biz/prompt"` import.

#### `cmd/feedium/wire.go`
- Added the `"github.com/4itosik/feedium/internal/biz/prompt"` import.
- Added `prompt.ProviderSet,` to the `wire.Build(...)` call so the resolver is registered next to `biz.ProviderSet`. `biz.ProviderSet` itself is left untouched.

#### `cmd/feedium/wire_gen.go`
- Regenerated by hand (Wire CLI is blocked in this sandbox) to mirror what `go run github.com/google/wire/cmd/wire ./...` produces against the updated `wire.go`:
  - Added `"github.com/4itosik/feedium/internal/biz/prompt"` to the import block.
  - Inserted `embeddedResolver := prompt.NewResolver()` after `openRouterProvider` is built.
  - Passed `embeddedResolver` as the new sixth argument to `biz.NewSummaryUsecase(...)`.
- Re-run `make wire` before merge to confirm byte-equivalence with the official generator.

## Decisions worth remembering

- **No import cycle.** The architectural rule "cross-module contracts live in `biz/`" is honored: the interface and the sentinel error are declared in `internal/biz/summary.go`, and the implementation/wiring sits in the leaf package `internal/biz/prompt/`. `biz.ProviderSet` is intentionally left alone.
- **Boot-time totality (FM-02).** `prompt.NewResolver` consults `biz.AllProcessingModes()` and panics on any drift. Adding a new `ProcessingMode` constant without updating `AllProcessingModes()` (or the resolver's body map) trips either the `make test` gate (`TestNewResolver_ProvidesAllModes`) or the binary's startup — never a silent prod-time failure on first event.
- **Prompt resolved once per event.** The resolver call sits outside the retry loop (`summarizeWithLLMRetry`); a resolver error is treated as permanent. This avoids masking FM-02 behind exponential backoff and matches the spec's `CTR-USECASE` invariant tested by `TestSummaryUsecase_PromptResolvedOncePerEvent`.
- **Initial bodies are minimally different.** NS-10 explicitly defers content-quality changes; the cumulative artifact only differs from self-contained by one short clause needed to satisfy CTR-01. Substantive cumulative-prompt rewriting is the next iteration on top of FT-008.
- **Provider defensiveness.** The `systemPrompt == ""` guard inside `openRouterProvider.Summarize` is intentionally redundant with `prompt.NewResolver`'s boot-time validation. Per the spec it is belt-and-braces, not the primary defense.

## Manual verification checklist (for the reviewer)

```bash
# All paths anchored at repo root.
git grep -n "defaultSystemPrompt"               # must print nothing
make build                                       # binary compiles
make test                                        # all packages green
make lint                                        # golangci-lint clean
mockgen -source=internal/biz/summary.go \
        -destination=internal/biz/mock/mock_summary.go -package=mock
git diff --stat internal/biz/mock/mock_summary.go   # expect no diff
make wire                                        # regenerate cmd/feedium/wire_gen.go
git diff --stat cmd/feedium/wire_gen.go             # expect no diff
```