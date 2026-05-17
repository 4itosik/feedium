I have enough context. The previous spec attempt failed because (1) it created an import cycle between `internal/biz` and `internal/biz/prompt`, and (2) it inconsistently named the resolver interface. I'll resolve both by declaring the interface in `biz` (Feedium architecture rule: cross-module contracts live in `biz/`), keeping the `prompt` package as a leaf that imports `biz` (but not vice versa), and registering its `ProviderSet` from `cmd/feedium/wire.go`. I'll also explicitly map CHK-03 to a concrete check and harden FM-02 with a boot-time registry assertion.

---
doc_kind: feature
doc_function: spec
purpose: "Feature spec for FT-008: split the single summarization system prompt into two embedded artifacts selected by ProcessingMode, without touching FT-005/FT-007 pipeline behavior. Resolves prior spec review's import-cycle and interface-location blockers."
derived_from:
  - feature.md
  - ../FT-005-ai-summarization/spec.md
  - ../../domain/architecture.md
  - ../../domain/glossary.md
  - ../../engineering/testing-policy.md
status: active
delivery_status: planned
---

# FT-008: Summarization Prompts Split — Spec

## Goal

Replace the single `defaultSystemPrompt` literal in `internal/data/llm_provider.go` with two independent embedded artifacts in `internal/biz/prompt/`, chosen by `biz.ProcessingMode`. After FT-008, self-contained and cumulative prompts evolve in isolation: an edit to one `.txt` file is invisible in the diff and behavior of the other.

This spec is explicitly written against the previous spec-review verdict (run `2026-05-17-FT-008-0519152`):

- The earlier proposal created an `internal/biz` ↔ `internal/biz/prompt` import cycle (resolver wired via `biz.ProviderSet` while `prompt` imported `biz`).
- The earlier proposal contradicted itself on whether the interface was `biz.SummaryPromptResolver` or `prompt.Resolver`.
- CHK-03 was silently re-defined.
- FM-02 ("new mode must produce a build-time or boot-time error") was reduced to a developer-discipline test.

Each is resolved below and called out at its design point.

## Reference

- Brief: `briefs/FT-008-target-prompts.md`
- Feature canonical: `memory-bank/features/FT-008-summarization-prompts-split/feature.md`
- FT-005 spec: `memory-bank/features/FT-005-ai-summarization/spec.md`
- Architecture: `memory-bank/domain/architecture.md` ("межмодульные зависимости — через явно названный интерфейс, определённый в `biz/`")
- Testing policy: `memory-bank/engineering/testing-policy.md`

## Architecture Sketch

### Current state (before FT-008)

```
internal/biz/summary_usecase.go
  summarizeWithLLMRetry(ctx, text)
    → uc.llmProvider.Summarize(ctx, text)              // mode unknown
                                ↓
internal/data/llm_provider.go
  openRouterProvider.Summarize(ctx, text)
    messages = [
      {role: "system", content: defaultSystemPrompt},  // single constant in data/
      {role: "user",   content: text},
    ]
```

The prompt-choice point is a literal in `data/`. The processing mode is known in `biz/` but never reaches the prompt-choice site.

### Target state (after FT-008)

```
internal/biz/prompt/                       ← new sub-package (leaf; imports biz, NOT imported by biz)
  prompt.go            ← interface impl: *embeddedResolver + NewResolver()
  self_contained.txt   ← go:embed body (full text, no formatting)
  cumulative.txt       ← go:embed body
  wire.go              ← prompt.ProviderSet, wired into cmd/feedium/wire.go
  prompt_test.go       ← unit tests

internal/biz/summary.go
  // existing LLMProvider interface — signature CHANGES (gains systemPrompt arg)
  type LLMProvider interface {
      Summarize(ctx context.Context, systemPrompt, text string) (string, error)
  }
  // NEW interface, declared in biz (per architecture rule)
  type SummaryPromptResolver interface {
      For(mode ProcessingMode) (string, error)
  }
  // NEW helper used to enforce FM-02 boot-time
  func AllProcessingModes() []ProcessingMode

internal/biz/summary_usecase.go
  SummaryUsecase{ ..., prompts SummaryPromptResolver }
  NewSummaryUsecase(..., prompts SummaryPromptResolver, ...)
  ProcessPostEvent   → summarizeWithLLMRetry(ctx, ProcessingModeSelfContained, post.Text)
  ProcessSourceEvent → summarizeWithLLMRetry(ctx, ProcessingModeCumulative,    concat)
  summarizeWithLLMRetry(ctx, mode, text):
      sys, err := uc.prompts.For(mode)                 // resolved ONCE, outside retry loop
      …retry loop calls llmProvider.Summarize(ctx, sys, text)

internal/data/llm_provider.go
  // defaultSystemPrompt constant DELETED
  openRouterProvider.Summarize(ctx, systemPrompt, text)
    messages = [
      {role: "system", content: systemPrompt},         // value flows from biz
      {role: "user",   content: text},
    ]

cmd/feedium/wire.go (and wire_gen.go after regeneration)
  wire.Build( …, biz.ProviderSet, data.ProviderSet, prompt.ProviderSet, … )
```

### Layer mapping

| Concern | Layer | Rationale |
| --- | --- | --- |
| Prompt text files (`*.txt`) and their embed binding | `internal/biz/prompt/` | Prompts are domain inputs of summarization, not infrastructure. Architecture rule places domain knowledge under `biz/`. |
| `SummaryPromptResolver` interface | `internal/biz/summary.go` (the `biz` package itself, alongside `LLMProvider`) | Architecture rule: cross-module contracts are declared in `biz/`. This is what resolves the previous-review interface-location contradiction. |
| Resolver implementation `*embeddedResolver` | `internal/biz/prompt/` | Mirrors the `LLMProvider` shape: interface in `biz`, concrete in a sibling/sub-package. |
| Choice of prompt for the current event | `internal/biz/summary_usecase.go` | Domain rule "mode → prompt"; lives in `biz/`. |
| Sending the chosen prompt over the wire | `internal/data/llm_provider.go` | The provider becomes mode-agnostic; it forwards what biz gave it. |

### Why this avoids the import cycle (explicit, from prior review)

- `prompt` imports `biz` (needs `biz.ProcessingMode`, `biz.SummaryPromptResolver`, `biz.AllProcessingModes`).
- `biz` does **NOT** import `prompt`. The interface and `AllProcessingModes` live in `biz`; the resolver implementation is built and bound entirely from outside `biz`.
- `biz.ProviderSet` (in `internal/biz/wire.go`) is **not** modified. The resolver is provided via a separate `prompt.ProviderSet` registered in `cmd/feedium/wire.go`. This mirrors how `data.ProviderSet` registers `NewOpenRouterProvider` and binds `biz.LLMProvider` from a non-`biz` package today (`internal/data/wire.go:18,25`).

## In Scope

- Two embedded `.txt` files: `internal/biz/prompt/self_contained.txt`, `internal/biz/prompt/cumulative.txt`. Each contains the full system-prompt body, no formatting/templating, no concatenation with Go code (CTR-PROMPT-BODY).
- New `biz.SummaryPromptResolver` interface (declared in `internal/biz/summary.go`).
- New `biz.AllProcessingModes() []biz.ProcessingMode` registry function (used by boot-time validation; FM-02).
- New leaf package `internal/biz/prompt/` containing the embedded artifacts, `*embeddedResolver` implementation, `NewResolver()` constructor with boot-time validation, sentinel `ErrUnknownMode`, and `prompt.ProviderSet`.
- Change to `biz.LLMProvider` interface: `Summarize` gains a `systemPrompt string` argument.
- Regeneration of `internal/biz/mock/mock_summary.go` via the existing `go:generate` directive (no new mockgen lines).
- Change to `(*openRouterProvider).Summarize` in `internal/data/llm_provider.go`: uses passed `systemPrompt`; deletes the `defaultSystemPrompt` constant.
- Change to `SummaryUsecase`: new `prompts` field, constructor takes a `SummaryPromptResolver`, `summarizeWithLLMRetry` accepts a `ProcessingMode`, `ProcessPostEvent`/`ProcessSourceEvent` pass their respective mode.
- Wire change in `cmd/feedium/wire.go`: add `prompt.ProviderSet` to `wire.Build(...)`. Regenerate `wire_gen.go`.
- Test-helper updates: `internal/biz/summary_usecase_test.go`, `internal/task/testhelper_test.go` (extra constructor arg).
- New unit tests in `internal/biz/prompt/prompt_test.go` and additions in `internal/biz/summary_usecase_test.go`.
- New unit tests in `internal/data/llm_provider_test.go` (httptest-backed; this file does not exist today and is created as part of FT-008 unit coverage for the new contract).

## Out of Scope (NS-)

- NS-01 Prompts in DB; runtime management via API/CLI/UI.
- NS-02 Versioning of prompts; persisting `prompt_version` in `Summary`.
- NS-03 Hot-reload without rebuild.
- NS-04 A/B experiments, quality metrics, regression-quality monitoring.
- NS-05 Source-variable templating (channel name, window date, source type).
- NS-06 Changes to outbox/retries/cron/Summary Worker lifecycle/lease policy (all FT-007 invariants stay untouched).
- NS-07 Replacing/adding LLM providers, model change, `max_input_chars`/`max_window`/timeout changes.
- NS-08 UI surfaces for prompts.
- NS-09 A third or further mode/artifact.
- NS-10 Content-quality changes: the cumulative prompt is initialized as a byte-for-byte copy of today's `defaultSystemPrompt`; the self-contained prompt is also initialized as a byte-for-byte copy. The two files start identical in content and are made minimally different only to satisfy CTR-01 (see "Initial bodies"). Substantive cumulative rewrite is the next iteration on top of FT-008.
- Concrete wording inside each prompt past the minimum required to make them distinguishable — this is the prompt maintainer's authority; spec only constrains "two files, both non-empty, bytes differ".

## Contracts / APIs

Stable IDs referenced from feature.md: CTR-01..06, REQ-01..06, FM-01..05, EC-01..07, CHK-01..06, EVID-01..06.

### CTR-LLM — `biz.LLMProvider` signature change

File: `internal/biz/summary.go` (single edit, replacing the existing interface block).

Before:
```go
type LLMProvider interface {
    Summarize(ctx context.Context, text string) (string, error)
}
```

After:
```go
// LLMProvider abstracts the chat completion call used by SummaryUsecase.
// The system prompt is an input to the call, chosen upstream in biz/ by
// ProcessingMode (see SummaryPromptResolver). The provider must not branch
// on any notion of mode or prompt identity.
type LLMProvider interface {
    Summarize(ctx context.Context, systemPrompt, text string) (string, error)
}
```

Properties:

- Provider has no mode branching (CTR-06).
- Different `systemPrompt` values lead to different `messages[0].content` on the wire (CTR-03).
- `systemPrompt == ""` is a defensive error inside the provider (`fmt.Errorf("empty system prompt")`). The primary guarantee against empty bodies is boot-time validation in `prompt.NewResolver` (FM-01/FM-02); this is a belt-and-braces check, not the main path.

Mock impact: `internal/biz/mock/mock_summary.go` is regenerated by the existing directive (`mockgen -source=internal/biz/summary.go -destination=internal/biz/mock/mock_summary.go -package=mock`). No new `go:generate` line is added.

### CTR-RESOLVER-IFACE — `biz.SummaryPromptResolver` interface

File: `internal/biz/summary.go` (new block, placed adjacent to `LLMProvider`).

```go
// SummaryPromptResolver returns the system prompt body for a ProcessingMode.
// The mapping is total over AllProcessingModes(); calls with an unknown mode
// return ErrUnknownSummaryPromptMode and an empty body.
type SummaryPromptResolver interface {
    For(mode ProcessingMode) (string, error)
}

// ErrUnknownSummaryPromptMode is returned by SummaryPromptResolver.For when
// called with a ProcessingMode that has no registered prompt artifact.
// SummaryUsecase treats this as a permanent error (no retry).
var ErrUnknownSummaryPromptMode = errors.New("biz: unknown summary prompt mode")
```

Notes:

- The sentinel lives in `biz` so callers (`SummaryUsecase`) can compare with `errors.Is` without importing `prompt`.
- The previously-attempted name `prompt.ErrUnknownMode` is rejected to keep the contract in one package and remove the prior-review interface-location ambiguity.

### CTR-MODES-REGISTRY — `biz.AllProcessingModes`

File: `internal/biz/source.go` (added near the existing `ProcessingMode` constants).

```go
// AllProcessingModes returns every defined ProcessingMode in declaration
// order. It is the registry consulted by SummaryPromptResolver
// implementations at construction time to enforce totality (FM-02).
//
// When a new ProcessingMode constant is introduced, this slice MUST be
// updated in the same change; the boot-time check in
// prompt.NewResolver panics otherwise, surfacing the gap before any
// summarization event is handled.
func AllProcessingModes() []ProcessingMode {
    return []ProcessingMode{
        ProcessingModeSelfContained,
        ProcessingModeCumulative,
    }
}
```

This is the FM-02 strengthening called out by the prior review:

- "New `ProcessingMode` added but Resolver not updated" is caught at process startup (panic in `NewResolver`), not at first event dispatch.
- "New `ProcessingMode` added but `AllProcessingModes()` not updated" surfaces in the unit test that compares `len(prompts) == len(AllProcessingModes())` (CHK-PROMPT-TOTAL below). It cannot regress silently because both `biz.AllProcessingModes()` and the resolver's internal map are consulted at boot.

### CTR-PROMPT — resolver implementation

File: `internal/biz/prompt/prompt.go` (new).

```go
// Package prompt owns the system prompts used by biz.SummaryUsecase.
// It is intentionally a leaf package: it implements biz.SummaryPromptResolver
// and is wired in via cmd/feedium/wire.go to avoid an internal/biz ↔
// internal/biz/prompt import cycle. The package imports biz; biz does not
// import the package.
package prompt

import (
    _ "embed"
    "errors"
    "fmt"
    "strings"

    "github.com/4itosik/feedium/internal/biz"
)

//go:embed self_contained.txt
var selfContainedBody string

//go:embed cumulative.txt
var cumulativeBody string

// embeddedResolver is the production SummaryPromptResolver, backed by the
// two embedded .txt files. Construction is total: any missing or empty
// body, or any biz.ProcessingMode not represented in the internal map,
// panics at NewResolver time (boot-time enforcement of FM-01/FM-02).
type embeddedResolver struct {
    bodies map[biz.ProcessingMode]string
}

// NewResolver returns the production resolver. It panics if any prompt
// body is empty after strings.TrimSpace, or if its internal map does not
// cover every biz.ProcessingMode in biz.AllProcessingModes(). Both
// failures must be loud at process startup, never silent at event time.
func NewResolver() *embeddedResolver {
    r := &embeddedResolver{
        bodies: map[biz.ProcessingMode]string{
            biz.ProcessingModeSelfContained: selfContainedBody,
            biz.ProcessingModeCumulative:    cumulativeBody,
        },
    }
    if err := r.validate(); err != nil {
        panic(fmt.Errorf("prompt.NewResolver: %w", err))
    }
    return r
}

func (r *embeddedResolver) For(mode biz.ProcessingMode) (string, error) {
    body, ok := r.bodies[mode]
    if !ok {
        return "", biz.ErrUnknownSummaryPromptMode
    }
    return body, nil
}

func (r *embeddedResolver) validate() error {
    for _, m := range biz.AllProcessingModes() {
        body, ok := r.bodies[m]
        if !ok {
            return fmt.Errorf("missing prompt for mode %q", m)
        }
        if strings.TrimSpace(body) == "" {
            return fmt.Errorf("empty prompt body for mode %q", m)
        }
    }
    if len(r.bodies) != len(biz.AllProcessingModes()) {
        return errors.New("resolver bodies do not match biz.AllProcessingModes()")
    }
    return nil
}
```

Properties enforced here:

- The embed has one directive per `.txt` file (CTR-01: artifacts are distinct files).
- Bodies pass through `strings.TrimSpace` for emptiness checks only; the body delivered to the LLM is the raw embedded string, **not** trimmed (CTR-03: no transformation between artifact and provider).
- Construction is total over `biz.AllProcessingModes()` (FM-02 boot-time).
- Unknown mode at runtime returns `biz.ErrUnknownSummaryPromptMode` (sentinel in `biz`) — not a panic — so the usecase can treat it as a permanent error and finalize the event.
- The constructor returns the unexported concrete type `*embeddedResolver` so Wire can bind via `wire.Bind(new(biz.SummaryPromptResolver), new(*embeddedResolver))`. This mirrors how `data.NewOpenRouterProvider` returns `*openRouterProvider` and binds to `biz.LLMProvider`.

### CTR-WIRE — Wire registration (no cycle)

File: `internal/biz/prompt/wire.go` (new).

```go
package prompt

import (
    "github.com/google/wire"

    "github.com/4itosik/feedium/internal/biz"
)

// ProviderSet wires the embedded SummaryPromptResolver into the application
// dependency graph. Registered in cmd/feedium/wire.go alongside the other
// ProviderSets so internal/biz never imports this package.
//
//nolint:gochecknoglobals // wire dependency injection
var ProviderSet = wire.NewSet(
    NewResolver,
    wire.Bind(new(biz.SummaryPromptResolver), new(*embeddedResolver)),
)
```

File: `cmd/feedium/wire.go` (edit `wire.Build(...)` only).

```go
wire.Build(
    newServerFromBootstrap,
    newDataFromBootstrap,
    newSummaryConfigFromBootstrap,
    newSummaryLLMFromBootstrap,
    server.ProviderSet,
    data.ProviderSet,
    healthservice.ProviderSet,
    biz.ProviderSet,
    prompt.ProviderSet,             // NEW: introduces SummaryPromptResolver
    sourceservice.ProviderSet,
    postservice.ProviderSet,
    summaryservice.ProviderSet,
    task.ProviderSet,
    wire.Bind(new(sourceservice.Usecase), new(*biz.SourceUsecase)),
    wire.Bind(new(postservice.Usecase),   new(*biz.PostUsecase)),
    wire.Bind(new(summaryservice.Usecase),new(*biz.SummaryUsecase)),
    newApp,
)
```

Files explicitly NOT modified for the resolver wiring:

- `internal/biz/wire.go` — `biz.ProviderSet` remains `wire.NewSet(NewSourceUsecase, NewPostUsecase, NewSummaryUsecase)`. Adding anything from `prompt` here recreates the cycle.
- `internal/data/wire.go` — unchanged. The existing `wire.Bind(new(biz.LLMProvider), new(*openRouterProvider))` continues to work; only the method signature it satisfies changes.

After edits, run `make wire` (or `go generate ./...` depending on Makefile target) so `cmd/feedium/wire_gen.go` is regenerated. The generated `NewSummaryUsecase` call must contain the new `prompts` argument; this is a verification point for CHK-WIRE.

### CTR-USECASE — `SummaryUsecase` change

File: `internal/biz/summary_usecase.go`.

Struct and constructor:

```go
type SummaryUsecase struct {
    summaryRepo SummaryRepo
    outboxRepo  SummaryOutboxRepo
    sourceRepo  SourceRepo
    postRepo    PostRepo
    llmProvider LLMProvider
    prompts     SummaryPromptResolver   // NEW
    txManager   TxManager
    cfg         *conf.Summary
}

func NewSummaryUsecase(
    summaryRepo SummaryRepo,
    outboxRepo SummaryOutboxRepo,
    sourceRepo SourceRepo,
    postRepo PostRepo,
    llmProvider LLMProvider,
    prompts SummaryPromptResolver,    // NEW (placed after llmProvider, before txManager)
    txManager TxManager,
    cfg *conf.Summary,
) *SummaryUsecase
```

Method updates:

- `ProcessPostEvent` (line ~157 today): `text, err := uc.summarizeWithLLMRetry(ctx, ProcessingModeSelfContained, post.Text)`.
- `ProcessSourceEvent` (line ~215 today): `text, err := uc.summarizeWithLLMRetry(ctx, ProcessingModeCumulative, concat)`.
- `summarizeWithLLMRetry` signature and body:

```go
func (uc *SummaryUsecase) summarizeWithLLMRetry(
    ctx context.Context, mode ProcessingMode, text string,
) (string, error) {
    systemPrompt, err := uc.prompts.For(mode)
    if err != nil {
        // Permanent — do NOT enter the retry loop. Caller wraps and finalizes.
        return "", fmt.Errorf("resolve prompt for mode %q: %w", mode, err)
    }
    maxRetries := int(uc.cfg.GetLlm().GetMaxRetries())
    if maxRetries <= 0 {
        maxRetries = defaultLLMMaxRetries
    }
    var lastErr error
    for attempt := range maxRetries {
        result, llmErr := uc.llmProvider.Summarize(ctx, systemPrompt, text)
        if llmErr != nil {
            lastErr = llmErr
            backoff := time.Duration(math.Pow(llmRetryBackoffBase, float64(attempt))) * time.Second
            select {
            case <-time.After(backoff):
            case <-ctx.Done():
                return "", ctx.Err()
            }
            continue
        }
        if strings.TrimSpace(result) == "" {
            lastErr = fmt.Errorf("llm returned empty summary on attempt %d", attempt+1)
            continue
        }
        return result, nil
    }
    return "", fmt.Errorf("all %d attempts failed: %w", maxRetries, lastErr)
}
```

Critical properties:

- `prompts.For(mode)` is called exactly **once per event**, **outside** the retry loop. Two reasons: (1) the body is constant for a given mode, so resolving inside the loop is wasted work; (2) a resolver error must be permanent — wrapping it in retry-with-backoff would mask FM-02.
- Mode is passed in literally from the two call sites, not derived from `Source.Type` inside `summarizeWithLLMRetry`. By FT-005 contract, `ProcessPostEvent` only handles `summarize_post` (always self-contained) and `ProcessSourceEvent` only handles `summarize_source` (always cumulative); the static mapping is a property of the methods, not a runtime decision.
- The retry-loop body is otherwise identical to today (`max_retries`, backoff base, ctx-cancel handling). FT-005/FT-007 invariants (CTR-05 from feature.md) are preserved.

Constructor-callsite churn (downstream of the spec — implementation work):

- `cmd/feedium/wire_gen.go` — regenerated.
- `internal/biz/summary_usecase_test.go` lines 69, 101 (and any others) — update `biz.NewSummaryUsecase(...)` calls to include a stub `SummaryPromptResolver`. A test helper is added (see Testing strategy).
- `internal/task/testhelper_test.go` line 178 — same.

### CTR-PROVIDER — `openRouterProvider.Summarize` change

File: `internal/data/llm_provider.go`.

- Delete the `defaultSystemPrompt` constant (line 21).
- Change signature and body of `Summarize`:

```go
func (p *openRouterProvider) Summarize(
    ctx context.Context, systemPrompt, text string,
) (string, error) {
    if systemPrompt == "" {
        return "", errors.New("empty system prompt")
    }
    reqBody := chatRequest{
        Model: p.model,
        Messages: []chatMessage{
            {Role: "system", Content: systemPrompt},
            {Role: "user",   Content: text},
        },
    }
    // remainder unchanged: marshal, http call, status handling, parse
}
```

- The `_ biz.LLMProvider = (*openRouterProvider)(nil)` interface-assertion line (line 32) stays — compilation will enforce the new signature.

No parallel mode-aware path remains in `data/`; the constant is gone (EC-06).

### CTR-PROMPT-BODY — artifact files

Files: `internal/biz/prompt/self_contained.txt`, `internal/biz/prompt/cumulative.txt`.

- Each file is plain UTF-8 text, full prompt body, no template/format directives, no concatenation hints. Newlines at the end of file are allowed; the embed reads bytes as-is.
- `.txt` extension is deliberate: it signals to a reader and to tooling that the content is not markdown, not Go-template, not configuration.
- Initial bodies in this iteration:
  - `self_contained.txt`: byte-for-byte equal to today's `defaultSystemPrompt` body (the constant on `internal/data/llm_provider.go:21`).
  - `cumulative.txt`: starts from that same body and is modified by **the smallest concrete change** that makes the two files differ (e.g., an explicit one-line preface stating the cumulative context such as `"You are summarizing a multi-author discussion window. "` prepended to the same baseline sentence). The exact wording is the prompt maintainer's call; the spec only mandates "non-empty and `selfContainedBody != cumulativeBody` byte-wise" so that CTR-01 / CHK-01 / EC-01 hold. Anything beyond that minimum is NS-10 and explicitly deferred.
- Trailing whitespace and trailing newline are not normalized by the resolver. What's in the file is what reaches the LLM (CTR-03).

## Data shapes

No DB schema changes. No proto/API changes. No configuration changes.

In-memory shapes introduced by this feature:

| Symbol | Package | Shape | Lifecycle |
| --- | --- | --- | --- |
| `selfContainedBody`, `cumulativeBody` | `internal/biz/prompt` | `string` (read via `//go:embed`) | Initialized at binary load. Immutable. |
| `*embeddedResolver` | `internal/biz/prompt` | struct `{ bodies map[biz.ProcessingMode]string }` | Built once by `NewResolver()` during Wire init at process startup. Goroutine-safe (read-only after construction). |
| `biz.SummaryPromptResolver` | `internal/biz` | interface `{ For(ProcessingMode) (string, error) }` | Resolved by Wire and stored on `*biz.SummaryUsecase`. |
| `biz.ErrUnknownSummaryPromptMode` | `internal/biz` | `error` sentinel | Permanent error; treated as terminal by `SummaryUsecase`. |

`Summary` (`internal/biz/summary.go:30`) — **unchanged**. No `prompt_version` column, no extra field (NS-02).
`SummaryEvent` — **unchanged**.
`conf.Summary*` (proto-generated) — **unchanged**.

## Flow

### Flow 1 — self-contained (per feature.md "Поток 1")

1. Collector saves a new post; FT-004/FT-005 atomically writes a `SummaryEvent(event_type=summarize_post)`.
2. Summary Worker (FT-007) claims the event and calls `SummaryUsecase.ProcessPostEvent`.
3. `ProcessPostEvent` runs TTL gate, idempotency check (existing-summary), fetches the post.
4. Calls `summarizeWithLLMRetry(ctx, biz.ProcessingModeSelfContained, post.Text)`.
5. The usecase calls `uc.prompts.For(biz.ProcessingModeSelfContained)` → `selfContainedBody`.
6. The retry loop calls `uc.llmProvider.Summarize(ctx, selfContainedBody, post.Text)`. The provider sends `messages[0]={role:"system", content:selfContainedBody}` and `messages[1]={role:"user", content:post.Text}`.
7. Result saved as `Summary`; event finalized to `completed`. FT-005 invariants unchanged.

### Flow 2 — cumulative via cron (per feature.md "Поток 2")

Identical to today's FT-007 cron path through `ScheduleNextCumulative` and `ProcessSourceEvent`, except step 4 of `ProcessSourceEvent` now calls `summarizeWithLLMRetry(ctx, biz.ProcessingModeCumulative, concat)`. Resolver returns `cumulativeBody`. Provider sends `cumulativeBody` as the system message.

### Flow 3 — cumulative via manual API (`TriggerSourceSummarize`)

Unchanged event creation. Worker pickup is identical to Flow 2. Cumulative prompt is used (Flow 2 step 4).

### Flow 4 — prompt maintainer changes one prompt

1. Maintainer opens `internal/biz/prompt/self_contained.txt` (or `cumulative.txt`).
2. Edits the file. No code is read.
3. PR diff touches only that file (no `.go` file in the same change is required).
4. The other `.txt` is byte-identical pre/post; the LLM input for the other mode is byte-identical pre/post (CTR-04, CHK-PROMPT-MODE).

## Edge cases / Failure modes

### FM-01 — Missing or empty prompt artifact

- Missing file at build time: `//go:embed` fails to compile. Caught by `go build`. No spec-side measure needed.
- Empty body (or only whitespace) at boot: `NewResolver()` panics with `prompt.NewResolver: empty prompt body for mode "<mode>"`. Process never starts; CI catches it on the first run.
- Empty body is **never** silently substituted with the other mode's body. The resolver's `bodies` map is built per-mode and not cross-referenced.

### FM-02 — Domain/mode drift (build-time-or-boot-time enforcement)

The previous spec made this developer discipline only. This spec strengthens it:

- `biz.AllProcessingModes()` is the canonical registry (CTR-MODES-REGISTRY).
- `NewResolver` consults `AllProcessingModes()` and panics at startup if any registered mode is missing from its `bodies` map (FM-02 path "new mode added, resolver not updated").
- A unit test (CHK-PROMPT-TOTAL) calls `len(NewResolver().bodies)` indirectly by enumerating `AllProcessingModes()` and asserting `For(m)` returns `(non-empty, nil)` for every entry. The test fails the `make test` gate (and CI) if either of:
  - a new `ProcessingMode` constant is declared in `internal/biz/source.go` without being added to `AllProcessingModes()`, **or**
  - a new entry is added to `AllProcessingModes()` without a corresponding prompt file/entry in `prompt.embeddedResolver`.
- Net effect: any drift is loud at the earliest of (a) compile (file-missing), (b) `make test`, (c) `make run` / boot. There is no path by which an event with an uncovered mode reaches the LLM provider in production.
- Unknown mode passed at runtime returns `biz.ErrUnknownSummaryPromptMode`; the usecase wraps it and the event is finalized terminally (permanent), no retry. Test `TestSummaryUsecase_PromptByMode_UnknownMode_IsTerminal` covers this.

### FM-03 — Copy-paste between artifacts (drift to byte-identical)

- Detection by repository structure (two files, each in its own diff) is sufficient.
- A boundary safety net: `TestPromptBodies_Differ` (CHK-PROMPT-DIFFER) asserts `selfContainedBody != cumulativeBody`. With the spec's initial bodies (NS-10), the two files differ by construction; the test guards against accidental copy-back during future maintenance.

### FM-04 — FT-005/FT-007 regression on the callsite refactor

- Functional invariants under test in `internal/biz/summary_usecase_test.go`, `internal/biz/summary_retry_test.go`, `internal/biz/summary_test.go`, `internal/data/summary_outbox_*_test.go`, and `internal/task/*_test.go` (testhelper) must remain green with only mechanical changes (constructor accepts an extra arg; `EXPECT().Summarize(ctx, gomock.Any(), gomock.Any())` for the new third arg in tests that don't care about the prompt body).
- The retry contract (`max_retries`, exponential backoff base 2s, ctx-cancel) is preserved verbatim; only the surrounding function signature changes.

### FM-05 — Provider receives `systemPrompt == ""`

- Should be impossible in production after FM-01 boot validation.
- Provider defensive check returns a plain error before any HTTP call (`errors.New("empty system prompt")`). Unit-tested in `TestOpenRouterProvider_Summarize_EmptySystemPrompt_ReturnsError`.

### EDGE — repeated `For(mode)` calls

- Returns the same `string` from `bodies` map; `string` is immutable in Go. No cache or memoization is needed.

### EDGE — empty `text` (user content)

- Out of scope for FT-008. FT-005 already gates this in `PostUsecase.Create` and `ProcessSourceEvent` (`concat == ""` → `ErrSummarySourceNoPosts`). Unchanged.

### EDGE — context cancellation during prompt resolution

- `prompts.For` does no I/O. Cancellation between resolution and the first retry iteration is handled by the existing `select { case <-ctx.Done(): }` inside the retry loop.

### EDGE — `panic` from `NewResolver` during process startup

- `NewResolver` panics on construction. Wire calls `NewResolver()` once at `wireApp` time. A panic here aborts `main` before kratos starts servers. Acceptable (and intentional — boot-time loudness).
- The `Resolver` is read-only after construction → no panic at request time, no graceful-shutdown interaction.

## Files changed (surface inventory)

| Path | Change | Reason |
| --- | --- | --- |
| `internal/biz/prompt/prompt.go` | **new** | Resolver impl, embed directives, boot-time validation. |
| `internal/biz/prompt/wire.go` | **new** | `prompt.ProviderSet` + bind to `biz.SummaryPromptResolver`. |
| `internal/biz/prompt/self_contained.txt` | **new** | Self-contained prompt body. |
| `internal/biz/prompt/cumulative.txt` | **new** | Cumulative prompt body. |
| `internal/biz/prompt/prompt_test.go` | **new** | Unit tests for resolver (see Testing strategy). |
| `internal/biz/summary.go` | **edit** | `LLMProvider.Summarize` gains `systemPrompt`; new `SummaryPromptResolver` interface; new `ErrUnknownSummaryPromptMode`. |
| `internal/biz/source.go` | **edit** | New `AllProcessingModes()` function. |
| `internal/biz/summary_usecase.go` | **edit** | `prompts` field; constructor arg; `summarizeWithLLMRetry(ctx, mode, text)` signature; both `Process*Event` callers pass their mode. |
| `internal/biz/wire.go` | **unchanged** | Adding to `biz.ProviderSet` recreates the import cycle. |
| `internal/biz/mock/mock_summary.go` | **regenerate** | Mockgen picks up the new `LLMProvider.Summarize` signature; the `SummaryPromptResolver` mock is also generated since the source file is `summary.go` and the existing directive scans the entire file. |
| `internal/biz/summary_usecase_test.go` | **edit** | Constructor accepts resolver stub; new tests `TestSummaryUsecase_PromptByMode_*`. |
| `internal/biz/summary_retry_test.go` | **edit if needed** | Mechanical update if it constructs the usecase directly. |
| `internal/data/llm_provider.go` | **edit** | Delete `defaultSystemPrompt`; new `Summarize` signature; empty-prompt guard. |
| `internal/data/llm_provider_test.go` | **new** | `httptest.Server` covers system-message wiring + empty-prompt guard. |
| `internal/data/wire.go` | **unchanged** | Existing `wire.Bind(biz.LLMProvider, *openRouterProvider)` is signature-agnostic. |
| `internal/task/testhelper_test.go` | **edit** | Pass a resolver into `biz.NewSummaryUsecase(...)`. Use `prompt.NewResolver()` (the real one) to keep integration coverage faithful. |
| `cmd/feedium/wire.go` | **edit** | Add `prompt.ProviderSet` to `wire.Build(...)`. |
| `cmd/feedium/wire_gen.go` | **regenerate** | Generated to reflect new `NewSummaryUsecase` signature and resolver provider. |
| `configs/*.yaml` | **unchanged** | NS-01/NS-03. |
| `migrations/*` | **unchanged** | NS-02. |

`grep -rn "defaultSystemPrompt"` after the change must return zero hits (CHK-NO-DEAD).

## Testing strategy

Per `memory-bank/engineering/testing-policy.md`: TDD for `biz/` (Red → Green → Refactor), tests-after for `data/`, mockgen, AAA, `goleak.VerifyNone(t, goleak.IgnoreCurrent())` in every test that doesn't `t.Parallel()`.

### biz/prompt — unit tests (TDD)

File: `internal/biz/prompt/prompt_test.go`. All in `package prompt` (white-box) so the test can construct an `*embeddedResolver` directly without going through `NewResolver()` for the negative cases.

| Test name | Covers | What it asserts |
| --- | --- | --- |
| `TestNewResolver_ProvidesAllModes` | CHK-PROMPT-TOTAL (FM-02) | For each `biz.AllProcessingModes()`: `r.For(m)` returns `(non-empty, nil)`. Table-driven over the registry. |
| `TestNewResolver_BodiesDiffer` | CHK-PROMPT-DIFFER (FM-03, CTR-01) | `selfContainedBody != cumulativeBody`. |
| `TestNewResolver_BodiesNonEmpty` | FM-01 | `strings.TrimSpace(selfContainedBody) != ""`, ditto cumulative. |
| `TestResolver_For_UnknownMode` | FM-02 runtime path | `(&embeddedResolver{bodies: map[...]string{}}).For("bogus")` returns `("", biz.ErrUnknownSummaryPromptMode)` (use `errors.Is`). |
| `TestNewResolverPanicsOnMissingMode` | FM-02 boot path | Construct `r := &embeddedResolver{bodies: map[biz.ProcessingMode]string{biz.ProcessingModeSelfContained: "x"}}` and assert `r.validate()` returns an error mentioning the missing mode. (Direct call to `validate()` avoids the panic-recover dance for negative paths; the panic itself is one `assert.Panics` test below.) |
| `TestNewResolverPanicsOnEmptyBody` | FM-01 boot path | Build a resolver with one empty body via `validate()` and assert error; for the `panic` wrapping, a single `assert.Panics(t, func(){ ... })` test on a helper that constructs an in-test resolver and forces `validate` to fail. |

The `bodies` field is intentionally unexported; the test file lives in `package prompt` to make direct construction possible. This is the standard go-test idiom for white-box validation tests.

### biz/summary_usecase — unit tests (mockgen)

File: `internal/biz/summary_usecase_test.go` (extended).

#### Test helper

Add a small helper that all existing tests can adopt mechanically:

```go
func newTestUsecase(
    t *testing.T,
    ctrl *gomock.Controller,
    repos repoBundle,
    cfg *conf.Summary,
) *biz.SummaryUsecase {
    t.Helper()
    return biz.NewSummaryUsecase(
        repos.summary, repos.outbox, repos.source, repos.post,
        repos.llm, mock.NewMockSummaryPromptResolver(ctrl), // stub
        repos.tx, cfg,
    )
}
```

Where unused, the stub resolver is set up with `gomock.AnyTimes()`; where the test asserts on the prompt argument, the stub returns a known sentinel string. Existing tests that pass `nil` for repos (e.g., `summary_usecase_test.go:69`) keep the same convention with a `nil` resolver where the path under test doesn't reach `For(...)` (or a no-op stub if it does).

#### New behavioral tests

| Test name | Covers | What it asserts |
| --- | --- | --- |
| `TestSummaryUsecase_PromptByMode_SelfContained` | CHK-02 / EC-02 / SC-02 (REQ-01..03) | Arrange: stub resolver returns `"SENTINEL-SC"` for `ProcessingModeSelfContained`. Mock `LLMProvider.Summarize` with `DoAndReturn` capturing `systemPrompt`. Act: `uc.ProcessPostEvent(ctx, postEvent)`. Assert: captured `systemPrompt == "SENTINEL-SC"`; saved summary persisted. |
| `TestSummaryUsecase_PromptByMode_Cumulative` | CHK-03 / EC-03 / SC-03 (REQ-01..03) | Symmetric for `ProcessSourceEvent` and `ProcessingModeCumulative`. The captured prompt is compared with the sentinel returned by the stub resolver (not a literal copy of the embedded body). This is exactly the structural property that satisfies CHK-04 / EC-04 / SC-04 (isolation): the assertion does not depend on either `.txt` file's content. |
| `TestSummaryUsecase_PromptByMode_Isolation` | CHK-04 / EC-04 / SC-04 (REQ-04) | Single table-driven test: two rows, one for each mode. Each row stubs the resolver to return a unique sentinel for **its** mode and `ErrUnknownSummaryPromptMode` for the other. Asserts that `Process*Event` for mode A captures sentinel A and never queries sentinel B. This directly proves "changing one prompt's body cannot affect the other mode's input". |
| `TestSummaryUsecase_PromptResolveError_IsTerminal` | FM-02 runtime path | Arrange: stub resolver returns `("", biz.ErrUnknownSummaryPromptMode)`. Act: `ProcessPostEvent`. Assert: returns wrapped `ErrUnknownSummaryPromptMode` (`errors.Is`); `LLMProvider.Summarize` was **not** called (gomock strict controller); no `Summary` saved. |
| `TestSummaryUsecase_PromptResolvedOncePerEvent` | CTR-USECASE invariant | Stub resolver `For(...)` is expected `Times(1)` even when LLM retries fire (LLM mock returns transient error twice, success on third). Guards against accidental "resolve per attempt". |

All new tests follow AAA, use `gomock.NewController(t)` + `defer ctrl.Finish()`, and start with `defer goleak.VerifyNone(t, goleak.IgnoreCurrent())`.

#### Existing tests — mechanical updates

All `biz.NewSummaryUsecase(...)` calls take one extra arg (resolver stub). All `EXPECT().Summarize(gomock.Any(), gomock.Any())` calls become `EXPECT().Summarize(gomock.Any(), gomock.Any(), gomock.Any())` (or use `gomock.AssignableToTypeOf("")`). No assertion semantics change.

### data/llm_provider — unit tests

File: `internal/data/llm_provider_test.go` (new). Uses `httptest.NewServer` per testing-policy.

| Test name | Covers | What it asserts |
| --- | --- | --- |
| `TestOpenRouterProvider_Summarize_SendsSystemPrompt` | CTR-PROVIDER / CTR-03 | Server captures the request body; asserts `messages[0].role == "system"`, `messages[0].content == <passed systemPrompt>`, `messages[1].role == "user"`, `messages[1].content == <passed text>`. Two table rows: `"some self-contained prompt body"` and `"some cumulative prompt body"`. |
| `TestOpenRouterProvider_Summarize_EmptySystemPrompt_ReturnsError` | FM-05 | `systemPrompt == ""` → non-nil error before any HTTP request fires (httptest server expects zero hits; assert with a counter). |
| `TestOpenRouterProvider_Summarize_PassesThroughText` | regression | Confirms the existing happy-path test (if any pre-existing) still holds after adding the new arg. |

The test does NOT import the `prompt` package; this is a data-layer unit test of the provider, independent of which biz layer produced the prompt.

### Mapping of CHK-* (feature.md) to tests / artifacts

| Check ID (feature.md) | How this spec satisfies it | Evidence |
| --- | --- | --- |
| `CHK-01` (artifacts exist, non-empty, differ) | (a) `internal/biz/prompt/self_contained.txt` and `cumulative.txt` exist as files in the diff; (b) `TestNewResolver_BodiesNonEmpty` + `TestNewResolver_BodiesDiffer` automate the rest. | `EVID-01`: paths + `git diff --stat` for the two files + the unit test log showing both tests green. |
| `CHK-02` (self-contained → self-contained prompt) | `TestSummaryUsecase_PromptByMode_SelfContained`. The mock captures the system prompt arg passed to `LLMProvider.Summarize` and compares it to the value the resolver returned for `ProcessingModeSelfContained`. | `EVID-02`: `go test -run TestSummaryUsecase_PromptByMode_SelfContained -v ./internal/biz/...` log. |
| `CHK-03` (cumulative → cumulative prompt) | `TestSummaryUsecase_PromptByMode_Cumulative`. **Explicit substitution call-out (prior-review warning 3):** the feature's CHK-03 originally implied a mutation-style probe. This spec replaces that with a captured-arg comparison routed through the resolver itself. Reason: comparing the captured prompt against `resolver.For(mode)` is a strictly stronger structural guarantee than running a mutation harness (it asserts the prompt-flow path end-to-end, not just that "changing X breaks something"). The substitution is recorded here and is the path that the verify stage and evidence collection must follow. | `EVID-03`: `go test -run TestSummaryUsecase_PromptByMode_Cumulative -v ./internal/biz/...` log. |
| `CHK-04` (changing one artifact doesn't affect the other) | (a) `TestSummaryUsecase_PromptByMode_Isolation` proves the runtime path is per-mode at the unit level; (b) the artifact layout (two distinct files, no shared base) provides the static guarantee; (c) at acceptance time, a single commit that modifies only one of the two `.txt` files demonstrates the diff property (`EVID-04` includes both the test log and the demo diff). | `EVID-04`: `go test -run TestSummaryUsecase_PromptByMode_Isolation -v ./internal/biz/...` log + sample one-file diff. |
| `CHK-05` (FT-005/FT-007 regression green) | Existing test files in `internal/biz/`, `internal/data/`, `internal/task/` keep their assertions; only constructor calls and the third `Summarize` arg change. The full `go test ./...` is the proof. | `EVID-05`: `go test ./...` log post-implementation. |
| `CHK-06` (`make build && make test && make lint` green; `defaultSystemPrompt` gone) | `grep -rn "defaultSystemPrompt" .` returns no hits; build/test/lint targets pass. | `EVID-06`: command transcripts. |

The new internal check IDs (`CHK-PROMPT-TOTAL`, `CHK-PROMPT-DIFFER`, `CHK-NO-DEAD`, `CHK-WIRE`) are spec-local refinements; they all roll up into `CHK-01`/`CHK-05`/`CHK-06` for traceability to the feature.

### Goroutine leak detection

Every new test in `internal/biz/prompt/prompt_test.go` and every new/edited test in `internal/biz/summary_usecase_test.go` starts with:

```go
defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
```

except table-driven outer functions that use `t.Parallel()` in subtests (none here).

### Manual / out-of-band

None. All FT-008 acceptance is automatable per the testing policy.

## Configuration, migrations, generated code

- **Config**: no changes. No new keys in `configs/*.yaml`, no new fields in `internal/conf/*.proto`.
- **DB / migrations**: none.
- **Generated code**:
  - `make wire` (or `go generate ./cmd/feedium/...`) regenerates `cmd/feedium/wire_gen.go`. Required after editing `cmd/feedium/wire.go`.
  - `go generate ./internal/biz/...` regenerates `internal/biz/mock/mock_summary.go`. Required after editing `internal/biz/summary.go`.
  - No proto regeneration.

## Open Questions

None remaining at spec stage. The four `OQ-*` items in feature.md are resolved here:

- **OQ-01** (artifact form): plain `.txt` files embedded via `//go:embed` (CTR-PROMPT, CTR-PROMPT-BODY).
- **OQ-02** (layer split): artifacts and resolver impl in `internal/biz/prompt/`; interface in `internal/biz/summary.go`; choice site in `internal/biz/summary_usecase.go`. Mapped onto Feedium's architecture rule that cross-module contracts live in `biz/` (resolves prior-review blocker 2).
- **OQ-03** (validation timing): boot-time via `NewResolver` panic plus build-time via `//go:embed` for missing files (FM-01/FM-02; resolves prior-review warning 4).
- **OQ-04** (initial cumulative body): byte-for-byte copy of today's `defaultSystemPrompt` with the minimal prefix needed for `selfContainedBody != cumulativeBody` (NS-10; see CTR-PROMPT-BODY).
- **OQ-05** (LLM-input fixation in tests): mockgen-based `MockLLMProvider` with `DoAndReturn` capturing the `systemPrompt` arg — same toolchain already used elsewhere in `internal/biz/`. No new infrastructure.
- **OQ-06** (deletion of old constant): single-step deletion in the same change as the introduction of the two artifacts. `git grep defaultSystemPrompt` post-merge must return empty (CHK-06 / EC-06). No transitional period.

## Acceptance Criteria (consolidated, for the next stage)

- [ ] `internal/biz/prompt/self_contained.txt` and `internal/biz/prompt/cumulative.txt` exist, both non-empty, byte contents differ.
- [ ] `internal/biz/prompt/prompt.go` defines `*embeddedResolver` and `NewResolver()` with the boot-time validation described in CTR-PROMPT (panic on empty body, panic on mode missing from `AllProcessingModes()`).
- [ ] `internal/biz/prompt/wire.go` exports `prompt.ProviderSet` with `wire.Bind(new(biz.SummaryPromptResolver), new(*embeddedResolver))`.
- [ ] `internal/biz` does **not** import `internal/biz/prompt` (`go list -deps ./internal/biz/...` shows no edge; `go build ./...` passes).
- [ ] `internal/biz/summary.go` declares `SummaryPromptResolver` and `ErrUnknownSummaryPromptMode`; `LLMProvider.Summarize` has the new `(ctx, systemPrompt, text)` signature.
- [ ] `internal/biz/source.go` exports `AllProcessingModes()` returning all current `ProcessingMode` constants.
- [ ] `internal/biz/summary_usecase.go`: `prompts` field present; constructor takes the resolver; `summarizeWithLLMRetry` accepts `mode`; `ProcessPostEvent` and `ProcessSourceEvent` pass `ProcessingModeSelfContained` and `ProcessingModeCumulative` respectively; resolver `For(mode)` is called exactly once per event, outside the retry loop.
- [ ] `internal/data/llm_provider.go`: `defaultSystemPrompt` removed; `Summarize` uses the passed `systemPrompt`; empty-prompt guard present.
- [ ] `cmd/feedium/wire.go` includes `prompt.ProviderSet`; `cmd/feedium/wire_gen.go` regenerated with the new `NewSummaryUsecase(...)` arity.
- [ ] `internal/biz/mock/mock_summary.go` regenerated; includes `MockSummaryPromptResolver` and the new `Summarize` signature.
- [ ] New tests added per Testing strategy; all green via `go test ./...`.
- [ ] `git grep -n "defaultSystemPrompt"` returns zero hits.
- [ ] `make build && make test && make lint` green locally and in CI.