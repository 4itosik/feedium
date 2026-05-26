---
title: "FT-009: Implementation Plan — Telegram Push Collector"
doc_kind: feature
doc_function: derived
purpose: "Execution-план Telegram-остатка FT-009 на push-модели (gotd updates.Manager, DEC-08, утверждено 2026-05-26). Заменяет прежний polling-план. RSS/web уже отгружены и не трогаются; план доводит фичу до MET-01 (4/4 типа)."
derived_from:
  - feature.md
  - spec.md
  - 2026-05-26-telegram-push-design.md
status: active
supersedes: "прежний polling-план (Fetcher/high-water) — устарел после DEC-08"
---

# Telegram Push Collector — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Собирать новые посты из Telegram-каналов (`REQ-02`) и групп (`REQ-03`) в реальном времени через userbot push (gotd `updates.Manager`), прогоняя каждое сообщение через существующий biz-пайплайн (dedup/outbox/text-filter), и достичь `MET-01` (4/4 типа источников).

**Architecture:** Один долгоживущий gotd-коннект отдаёт `UpdateNewChannelMessage`/`UpdateNewMessage`. Каждое сообщение → транспортно-независимый `biz.TelegramMessage` (TgID + `CollectedItem`). `task.TelegramCollector` резолвит источник по `TgID` (индекс поверх `SourceLister`) и персистит через новый biz-seam `CollectUsecase.CollectItems` — что бесплатно даёт `REQ-06`/`REQ-09`/`REQ-10`. Polling, порт `Fetcher` и high-water mark для Telegram **не используются** (см. `2026-05-26-telegram-push-design.md`, DEC-08). RSS/web без изменений.

**Tech Stack:** Go 1.26, `github.com/gotd/td` v0.144.0 (`telegram`, `telegram/updates`, `tg`), kratos `transport.Server`, wire DI, mockgen, testcontainers+goleak.

---

## Слой и файлы

| Файл | Действие | Ответственность |
|---|---|---|
| `internal/conf/conf.proto` | modify | убрать `poll_interval` из `TelegramCollector` (push, не poll) |
| `configs/config.yaml`, `configs/local.example.yaml` | modify | убрать `poll_interval` из `telegram:` |
| `internal/biz/collect.go` | modify | откатить `TelegramFetcher` named-порт; добавить `TelegramMessage`, порт `TelegramSession`; рефактор `CollectSource`→`CollectItems` |
| `internal/biz/mock/mock_collect.go` | regen | без `TelegramFetcher`, с `TelegramSession` |
| `internal/biz/collect_test.go` | modify | тест `CollectItems` |
| `internal/task/telegram_collector.go` | create | `TelegramCollector` (session consumer, resolver, persist dispatch) |
| `internal/task/telegram_collector_test.go` | create | unit (fake session) + integration (testcontainers) |
| `internal/task/wire.go` | modify | `NewTelegramCollector` в ProviderSet |
| `internal/data/telegram_session.go` | create | gotd-backed `biz.TelegramSession` (live, AG-02) |
| `internal/data/wire.go` | modify | `NewTelegramSession` + bind `biz.TelegramSession` |
| `cmd/feedium/wire.go`, `wire_gen.go` | modify | `telegramCollector` в `newApp`/`kratos.Server` + реген |

## Состояние ранее выполненного

- `STEP-01` (proto+yaml): сделано, но добавлен `poll_interval` — **Task 1** его убирает.
- `STEP-02` (`go get gotd/td`): **сделано**, v0.144.0 в go.mod (станет direct после Task 6).
- `STEP-03` (`biz.TelegramFetcher` + мок): **сделано, но устарело** — **Task 2** откатывает.

## Approval Gates

- **AG-01** — перед реген `conf.pb.go` (Task 1) и `wire_gen.go` (Task 7): запускать `make proto`/`make wire`, показывать диффы (утверждено пользователем).
- **AG-02** — перед любым живым коннектом к Telegram (реальная сессия/авторизация): только Task 6 runtime; в тестах fake-сессия. Авто-входа быть не должно.

## Stop Conditions

- `STOP-01`: любой существующий тест (RSS/web/FT-005/FT-007) покраснел → `git` откат текущей задачи, эскалация.
- `STOP-02`: код/тест требует живого входа в Telegram → стоп, оставить fake-путь, эскалация (AG-02).
- `STOP-03`: gotd-типы протекают в `biz`/`task` (наружу seam должны идти только `biz.TelegramMessage`/`biz.CollectedItem`) → стоп, пересмотр seam.

---

## Task 1: Убрать poll_interval из TelegramCollector (push не опрашивает)

**Files:**
- Modify: `internal/conf/conf.proto` (message `TelegramCollector`)
- Modify: `configs/config.yaml`, `configs/local.example.yaml` (`collectors.telegram`)
- Regen: `internal/conf/conf.pb.go`

- [ ] **Step 1: Править proto** — убрать поле `poll_interval`, перенумеровать не нужно (оставить теги 2–5):

```proto
// TelegramCollector configures the userbot push collector (FT-009 REQ-02/REQ-03,
// DEC-08). It carries userbot credentials and the server-side session file
// location (OQ-03, CON-04). No poll interval: messages arrive via push
// (gotd updates.Manager), not polling. Secrets (api_hash, phone) live only in
// the uncommitted config.yaml / env — never in committed examples.
message TelegramCollector {
  int32 api_id = 2;
  string api_hash = 3;
  string phone = 4;
  string session_path = 5;
}
```

- [ ] **Step 2: Править yaml** — убрать `poll_interval` из `collectors.telegram` в обоих файлах. `local.example.yaml`:

```yaml
  telegram:
    api_id: 0                              # int32 app id from my.telegram.org
    api_hash: ""                           # secret — do NOT commit
    phone: ""                              # userbot phone, e.g. +10000000000
    session_path: "/var/lib/feedium/tg.session"
```

и `config.yaml` — те же ключи без `poll_interval`.

- [ ] **Step 3: Реген proto (AG-01)** — `make proto`. Показать `git diff --stat internal/conf/conf.pb.go`.
- [ ] **Step 4: Build** — `make build`. Expected: exit 0; `grep -c GetPollInterval internal/conf/conf.pb.go` не содержит метода у `TelegramCollector` (метод остаётся у `CollectorPoll` — это норм).
- [ ] **Step 5: Commit** — `git add internal/conf configs && git commit -m "feat(ft-009): drop poll_interval from TelegramCollector (push model, DEC-08)"`

---

## Task 2: Откатить устаревший biz.TelegramFetcher named-порт

**Files:**
- Modify: `internal/biz/collect.go`
- Regen: `internal/biz/mock/mock_collect.go`

- [ ] **Step 1: Убрать порт** — вернуть блок к двум портам:

```go
type (
	RSSFetcher interface{ Fetcher }
	WebFetcher interface{ Fetcher }
)
```

- [ ] **Step 2: Реген мока** — `go generate ./internal/biz/...`. Expected: `MockTelegramFetcher` исчез из `mock_collect.go`.
- [ ] **Step 3: Build** — `go build ./...`. Expected: exit 0.
- [ ] **Step 4: Commit** — `git add internal/biz && git commit -m "refactor(ft-009): drop unused TelegramFetcher port (push uses TelegramSession)"`

---

## Task 3: biz — TelegramMessage, TelegramSession port, CollectItems

**Files:**
- Modify: `internal/biz/collect.go`
- Regen: `internal/biz/mock/mock_collect.go`
- Test: `internal/biz/collect_test.go`

- [ ] **Step 1: Failing test для `CollectItems`** — добавить в `collect_test.go` (рядом с существующими `CollectSource`-тестами; использовать те же fakes/mock `PostCreator`):

```go
func TestCollectUsecase_CollectItems_PersistsTextItemsAndSkipsNonText(t *testing.T) {
	creator := mock.NewMockPostCreator(gomock.NewController(t))
	uc := biz.NewCollectUsecase(creator)
	src := biz.Source{ID: "s1", Type: biz.SourceTypeTelegramChannel}

	// non-text item is filtered out before Create (REQ-10); text item persists.
	creator.EXPECT().
		Create(gomock.Any(), "s1", "100", "hello", gomock.Any(), gomock.Any(), gomock.Any()).
		Return(biz.Post{}, nil)

	err := uc.CollectItems(context.Background(), src, []biz.CollectedItem{
		{ExternalID: "99", Text: "   "},               // non-text → dropped
		{ExternalID: "100", Text: "hello"},             // text → Create
	})
	require.NoError(t, err)
}

func TestCollectUsecase_CollectItems_SkipsSourceNotFound(t *testing.T) {
	creator := mock.NewMockPostCreator(gomock.NewController(t))
	uc := biz.NewCollectUsecase(creator)
	creator.EXPECT().Create(gomock.Any(), "s1", "1", "x", gomock.Any(), gomock.Any(), gomock.Any()).
		Return(biz.Post{}, biz.ErrPostSourceNotFound)

	err := uc.CollectItems(context.Background(), biz.Source{ID: "s1"}, []biz.CollectedItem{{ExternalID: "1", Text: "x"}})
	require.NoError(t, err) // ErrPostSourceNotFound is skipped, not fatal (FM-T04)
}
```

> Сверить точные имена mock-конструктора/`gomock` импортов с существующим `collect_test.go` перед написанием (CLAUDE.md: подбирать якоря по факту).

- [ ] **Step 2: Run — ожидаем FAIL** — `go test ./internal/biz/... -run TestCollectUsecase_CollectItems`. Expected: FAIL (`uc.CollectItems undefined`).

- [ ] **Step 3: Рефактор `CollectSource` → выделить `CollectItems`** — в `collect.go`:

```go
// CollectSource fetches material for one source and persists it (REQ-05..10).
// Fetch failure is wrapped so the collector can log and move on (REQ-07, FM-T01).
func (uc *CollectUsecase) CollectSource(ctx context.Context, source Source, fetcher Fetcher) error {
	items, err := fetcher.Fetch(ctx, source)
	if err != nil {
		return fmt.Errorf("fetch source %s: %w", source.ID, err)
	}
	return uc.CollectItems(ctx, source, items)
}

// CollectItems persists already-fetched material for a source: it drops
// non-text items (REQ-10) and saves each text item through PostCreator.Create,
// reusing the single Create path that provides idempotency by
// (source_id, external_id) (REQ-06) and the self-contained-outbox /
// cumulative-no-event behaviour (REQ-09). A per-item ErrPostSourceNotFound is
// skipped (FM-T04); any other Create error aborts the batch and is returned.
// The push Telegram collector calls this directly with one item per message
// (DEC-08); pull collectors reach it via CollectSource.
func (uc *CollectUsecase) CollectItems(ctx context.Context, source Source, items []CollectedItem) error {
	for _, item := range FilterTextItems(items) {
		_, createErr := uc.creator.Create(
			ctx, source.ID, item.ExternalID, item.Text, item.PublishedAt, item.Author, item.Metadata,
		)
		if createErr != nil {
			if errors.Is(createErr, ErrPostSourceNotFound) {
				continue
			}
			return fmt.Errorf("create post for source %s item %s: %w", source.ID, item.ExternalID, createErr)
		}
	}
	return nil
}
```

- [ ] **Step 4: Run — ожидаем PASS** — `go test ./internal/biz/... -run TestCollectUsecase`. Expected: PASS (включая существующие `CollectSource`-тесты — поведение не изменилось).

- [ ] **Step 5: Добавить `TelegramMessage` + порт `TelegramSession`** — в `collect.go` (после `Fetcher`/named-портов):

```go
// TelegramMessage is one new Telegram message yielded by the userbot session
// (FT-009 DEC-08). It is transport-agnostic — no gotd types leak past data/.
// TgID is the channel/chat identifier used to resolve the configured Source
// (matches biz.TelegramChannelConfig/TelegramGroupConfig.TgID).
type TelegramMessage struct {
	TgID int64
	Item CollectedItem
}

// TelegramSession is the live userbot connection port (DEC-08, AG-02). The
// data/ implementation is gotd-backed; it blocks in Run delivering each new
// channel/group message to handle until ctx is cancelled. A handle error is
// logged by the collector and never stops the stream (FM-T01). data/ owns
// connection, auth and FLOOD_WAIT handling; biz/ and task/ stay gotd-free.
type TelegramSession interface {
	Run(ctx context.Context, handle func(context.Context, TelegramMessage) error) error
}
```

- [ ] **Step 6: Реген мока** — `go generate ./internal/biz/...`. Expected: `MockTelegramSession` появился в `mock_collect.go`. `go build ./...` exit 0.

- [ ] **Step 7: Commit** — `git add internal/biz && git commit -m "feat(ft-009): add TelegramSession port + CollectItems seam (push, DEC-08)"`

---

## Task 4: task — TelegramCollector + unit tests (fake session)

**Files:**
- Create: `internal/task/telegram_collector.go`
- Test: `internal/task/telegram_collector_test.go`

- [ ] **Step 1: Failing unit-test (fake session, без Docker)** — `telegram_collector_test.go`:

```go
package task_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/4itosik/feedium/internal/biz"
	"github.com/4itosik/feedium/internal/task"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

// fakeSession delivers a fixed batch of messages once, then blocks until ctx done.
type fakeSession struct{ msgs []biz.TelegramMessage }

func (f *fakeSession) Run(ctx context.Context, handle func(context.Context, biz.TelegramMessage) error) error {
	for _, m := range f.msgs {
		_ = handle(ctx, m)
	}
	<-ctx.Done()
	return ctx.Err()
}

// fakeSourceLister returns configured TG sources for the resolver.
type fakeSourceLister struct{ items []biz.Source }

func (f *fakeSourceLister) List(_ context.Context, _ biz.ListSourcesFilter) (biz.ListSourcesResult, error) {
	return biz.ListSourcesResult{Items: f.items}, nil
}

// recordingCollector records CollectItems calls instead of persisting.
type recordingCollector struct {
	mu    sync.Mutex
	calls []struct {
		source biz.Source
		items  []biz.CollectedItem
	}
}

func (r *recordingCollector) CollectSource(context.Context, biz.Source, biz.Fetcher) error { return nil }
func (r *recordingCollector) CollectItems(_ context.Context, s biz.Source, items []biz.CollectedItem) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, struct {
		source biz.Source
		items  []biz.CollectedItem
	}{s, items})
	return nil
}

func TestTelegramCollector_RoutesMessageToMatchingSource(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	chSrc := biz.Source{ID: "s-chan", Type: biz.SourceTypeTelegramChannel, Config: &biz.TelegramChannelConfig{TgID: 111}}
	rec := &recordingCollector{}
	session := &fakeSession{msgs: []biz.TelegramMessage{
		{TgID: 111, Item: biz.CollectedItem{ExternalID: "1", Text: "hi"}}, // matches chSrc
		{TgID: 999, Item: biz.CollectedItem{ExternalID: "2", Text: "x"}},  // no source → ignored
	}}
	c := task.NewTelegramCollector(rec, session, &fakeSourceLister{items: []biz.Source{chSrc}}, testLogger())

	require.NoError(t, c.Start(context.Background()))
	require.Eventually(t, func() bool { rec.mu.Lock(); defer rec.mu.Unlock(); return len(rec.calls) == 1 }, time.Second, 5*time.Millisecond)
	require.NoError(t, c.Stop(context.Background()))

	require.Equal(t, "s-chan", rec.calls[0].source.ID)
	require.Equal(t, "1", rec.calls[0].items[0].ExternalID)
}
```

> `testLogger()` — взять существующий helper из task-тестов (RSS-тесты его используют); сверить имя.

- [ ] **Step 2: Run — ожидаем FAIL** — `go test ./internal/task/... -run TestTelegramCollector_RoutesMessage`. Expected: FAIL (`task.NewTelegramCollector undefined`).

- [ ] **Step 3: Реализация `TelegramCollector`** — `telegram_collector.go`:

```go
package task

import (
	"context"
	"log/slog"
	"sync"

	"github.com/4itosik/feedium/internal/biz"
)

// ItemCollector is the task-level view of biz.CollectUsecase for push delivery:
// it persists items already obtained (CollectItems), reusing dedup/outbox/text
// filtering. The Telegram collector never lists or fetches — messages arrive via
// push (DEC-08).
type ItemCollector interface {
	CollectItems(ctx context.Context, source biz.Source, items []biz.CollectedItem) error
}

// TelegramCollector is a transport.Server that ingests Telegram posts in real
// time via a userbot push session (FT-009 REQ-02/REQ-03, DEC-08). Start runs the
// session in one goroutine; each new message is routed to its configured source
// by TgID and persisted via ItemCollector.CollectItems. An unknown peer (the
// userbot is in other dialogs) is ignored; a persist error is logged and never
// stops the stream (REQ-07, FM-T01). Stop cancels the session and waits.
type TelegramCollector struct {
	collector ItemCollector
	session   biz.TelegramSession
	sources   SourceLister
	log       *slog.Logger

	cancel context.CancelFunc
	done   chan struct{}
	wg     sync.WaitGroup
}

// NewTelegramCollector constructs the push collector.
func NewTelegramCollector(
	collector ItemCollector,
	session biz.TelegramSession,
	sources SourceLister,
	logger *slog.Logger,
) *TelegramCollector {
	return &TelegramCollector{
		collector: collector,
		session:   session,
		sources:   sources,
		log:       logger,
		done:      make(chan struct{}),
	}
}

// Start launches the push session goroutine. The live gotd connection/auth lives
// in the data/ session impl and is gated by AG-02; tests inject a fake session.
func (c *TelegramCollector) Start(_ context.Context) error {
	runCtx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	c.wg.Go(func() {
		defer close(c.done)
		if err := c.session.Run(runCtx, c.handle); err != nil && runCtx.Err() == nil {
			c.log.Error("telegram collector: session ended", "err", err)
		}
	})
	return nil
}

// Stop cancels the session and waits for the goroutine to drain (graceful).
func (c *TelegramCollector) Stop(_ context.Context) error {
	if c.cancel != nil {
		c.cancel()
	}
	c.wg.Wait()
	return nil
}

// handle routes one message to its configured source and persists it. Unknown
// peer → ignored; persist error → logged, stream continues (FM-T01).
func (c *TelegramCollector) handle(ctx context.Context, msg biz.TelegramMessage) error {
	src, ok, err := c.resolve(ctx, msg.TgID)
	if err != nil {
		c.log.ErrorContext(ctx, "telegram collector: resolve source failed", "tg_id", msg.TgID, "err", err)
		return nil
	}
	if !ok {
		return nil // userbot is a member of a dialog we don't collect
	}
	if cErr := c.collector.CollectItems(ctx, src, []biz.CollectedItem{msg.Item}); cErr != nil {
		c.log.ErrorContext(ctx, "telegram collector: collect failed", "source_id", src.ID, "err", cErr)
	}
	return nil
}

// resolve finds the configured Telegram source whose TgID matches. It lists both
// Telegram source types and matches on config TgID. Listing is per-message —
// source volume is small (CON-03) and this always reflects current sources, so
// added/removed sources are picked up without a restart.
func (c *TelegramCollector) resolve(ctx context.Context, tgID int64) (biz.Source, bool, error) {
	for _, st := range []biz.SourceType{biz.SourceTypeTelegramChannel, biz.SourceTypeTelegramGroup} {
		filter := biz.ListSourcesFilter{Type: st, PageSize: collectorPageSize}
		for {
			res, err := c.sources.List(ctx, filter)
			if err != nil {
				return biz.Source{}, false, err
			}
			for _, s := range res.Items {
				if id, ok := tgIDOf(s); ok && id == tgID {
					return s, true, nil
				}
			}
			if res.NextPageToken == "" {
				break
			}
			filter.PageToken = res.NextPageToken
		}
	}
	return biz.Source{}, false, nil
}

func tgIDOf(s biz.Source) (int64, bool) {
	switch cfg := s.Config.(type) {
	case *biz.TelegramChannelConfig:
		return cfg.TgID, true
	case *biz.TelegramGroupConfig:
		return cfg.TgID, true
	default:
		return 0, false
	}
}
```

- [ ] **Step 4: Run — ожидаем PASS** — `go test ./internal/task/... -run TestTelegramCollector_RoutesMessage -race`. Expected: PASS, без goroutine leak.

- [ ] **Step 5: Commit** — `git add internal/task/telegram_collector.go internal/task/telegram_collector_test.go && git commit -m "feat(ft-009): TelegramCollector push consumer + source resolver"`

---

## Task 5: task — integration tests (outbox по режимам, REQ-09)

**Files:**
- Test: `internal/task/telegram_collector_test.go` (добавить integration-блок; Docker)

Зеркалит существующий `TestIntegration_RSSCollector_PersistsFieldsAndEnqueuesSummary`: тот же testcontainers-bootstrap, реальные `biz.PostUsecase`/`CollectUsecase`/`SummaryOutboxRepo`. Telegram-коллектор конструируется с **fake session** (без живого gotd, AG-02) и реальным `CollectUsecase` (через `CollectItems`).

- [ ] **Step 1: Failing integration-test** — добавить (имена бутстрап-хелперов и `SummaryEventType*` сверить с RSS-тестом):

```go
func TestIntegration_TelegramCollector_ChannelEnqueuesSummary_GroupDoesNot(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	env := setupCollectorIntegration(t) // reuse RSS test's container bootstrap

	chID := env.createSource(t, biz.SourceTypeTelegramChannel, &biz.TelegramChannelConfig{TgID: 111})
	grID := env.createSource(t, biz.SourceTypeTelegramGroup, &biz.TelegramGroupConfig{TgID: 222})

	session := &fakeSession{msgs: []biz.TelegramMessage{
		{TgID: 111, Item: biz.CollectedItem{ExternalID: "c1", Text: "channel post", PublishedAt: time.Now()}},
		{TgID: 222, Item: biz.CollectedItem{ExternalID: "g1", Text: "group post", PublishedAt: time.Now()}},
	}}
	c := task.NewTelegramCollector(env.collectUsecase, session, env.sourceUsecase, testLogger())

	require.NoError(t, c.Start(context.Background()))
	require.Eventually(t, func() bool { return env.postCount(t) == 2 }, 3*time.Second, 20*time.Millisecond)
	require.NoError(t, c.Stop(context.Background()))

	// channel is self_contained → summarize_post event enqueued (REQ-09)
	hasChan, err := env.outbox.HasActiveEvent(context.Background(), chID, biz.SummaryEventTypeSummarizePost)
	require.NoError(t, err)
	require.True(t, hasChan)
	// group is cumulative → no per-post event (REQ-09)
	hasGroup, err := env.outbox.HasActiveEvent(context.Background(), grID, biz.SummaryEventTypeSummarizePost)
	require.NoError(t, err)
	require.False(t, hasGroup)
}
```

> Если RSS-тест использует иную форму bootstrap (не `env`-helper), зеркалить её фактическую структуру, а не выдумывать `setupCollectorIntegration`. Сверить с `internal/task/rss_collector_test.go` перед написанием.

- [ ] **Step 2: Run — ожидаем FAIL→затем PASS** — `go test ./internal/task/... -run TestIntegration_TelegramCollector -race` (Docker). Реализация уже есть (Task 4) — тест должен пройти после правки bootstrap-зеркала. Expected: PASS.

- [ ] **Step 3: Commit** — `git add internal/task/telegram_collector_test.go && git commit -m "test(ft-009): telegram collector outbox-by-mode integration (REQ-09)"`

---

## Task 6: data — gotd-backed TelegramSession (live, AG-02)

**Files:**
- Create: `internal/data/telegram_session.go`
- Modify: `internal/data/wire.go`

Тонкий адаптер: gotd живёт только здесь, наружу — `biz.TelegramMessage`. Не покрывается unit-тестами помимо компиляции (живой коннект — `AG-02`, manual). `STOP-03`: никакие `tg.*`/`telegram.*` типы не должны появляться в `biz`/`task`.

- [ ] **Step 1: Реализация сессии** — `telegram_session.go`:

```go
package data

import (
	"context"
	"strconv"

	"github.com/4itosik/feedium/internal/biz"
	"github.com/4itosik/feedium/internal/conf"
	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/updates"
	"github.com/gotd/td/tg"
)

var _ biz.TelegramSession = (*telegramSession)(nil)

// telegramSession is the gotd-backed userbot push connection (FT-009 DEC-08).
// It registers new-message handlers, runs the updates manager, and translates
// each gotd message into a transport-agnostic biz.TelegramMessage. Connection
// and authorization touch a live account — gated by AG-02; never auto-login.
type telegramSession struct {
	cfg *conf.TelegramCollector
}

// NewTelegramSession constructs the gotd session adapter.
//
//nolint:revive // unexported return type is intentional for Wire injection
func NewTelegramSession(cfg *conf.Collectors) *telegramSession {
	return &telegramSession{cfg: cfg.GetTelegram()}
}

// Run connects the userbot and delivers each new channel/group message to handle
// until ctx is cancelled. The session file at session_path is reused if present;
// first-time login (phone code) is a manual operation (AG-02) and must not be
// automated here.
func (s *telegramSession) Run(ctx context.Context, handle func(context.Context, biz.TelegramMessage) error) error {
	dispatcher := tg.NewUpdateDispatcher()
	mgr := updates.New(updates.Config{Handler: dispatcher})

	emit := func(peerID int64, m *tg.Message, ents tg.Entities) error {
		if m.Out {
			return nil // skip our own outgoing messages
		}
		return handle(ctx, biz.TelegramMessage{
			TgID: peerID,
			Item: biz.CollectedItem{
				ExternalID:  strconv.Itoa(m.ID),
				PublishedAt: timeFromUnix(m.Date),
				Author:      authorFrom(m, ents),
				Text:        m.Message,
				Metadata:    map[string]string{"chat_title": chatTitleFrom(peerID, ents)},
			},
		})
	}

	dispatcher.OnNewChannelMessage(func(c context.Context, e tg.Entities, u *tg.UpdateNewChannelMessage) error {
		m, ok := u.Message.(*tg.Message)
		if !ok {
			return nil
		}
		peer, ok := m.PeerID.(*tg.PeerChannel)
		if !ok {
			return nil
		}
		return emit(peer.ChannelID, m, e)
	})
	dispatcher.OnNewMessage(func(c context.Context, e tg.Entities, u *tg.UpdateNewMessage) error {
		m, ok := u.Message.(*tg.Message)
		if !ok {
			return nil
		}
		peer, ok := m.PeerID.(*tg.PeerChat)
		if !ok {
			return nil // private/peerUser not collected
		}
		return emit(peer.ChatID, m, e)
	})

	client := telegram.NewClient(int(s.cfg.GetApiId()), s.cfg.GetApiHash(), telegram.Options{
		SessionStorage: &session.FileStorage{Path: s.cfg.GetSessionPath()},
		UpdateHandler:  mgr,
	})

	return client.Run(ctx, func(ctx context.Context) error {
		status, err := client.Auth().Status(ctx)
		if err != nil {
			return err
		}
		if !status.Authorized {
			// First-time login needs a phone code — manual, AG-02. Never auto-login.
			return biz.ErrTelegramNotAuthorized
		}
		self, err := client.Self(ctx)
		if err != nil {
			return err
		}
		if err := mgr.Run(ctx, client.API(), self.ID, updates.AuthOptions{}); err != nil {
			return err
		}
		<-ctx.Done()
		return ctx.Err()
	})
}
```

Хелперы `timeFromUnix`/`authorFrom`/`chatTitleFrom` — маленькие чистые функции в этом же файле (Date → `time.Unix(int64(d),0).UTC()`; author из `e.Users[m.FromID...]`; title из `e.Channels[id].Title`/`e.Chats[id].Title`). Sentinel `biz.ErrTelegramNotAuthorized` добавить в `collect.go`.

- [ ] **Step 2: wire** — в `internal/data/wire.go` добавить `NewTelegramSession,` и `wire.Bind(new(biz.TelegramSession), new(*telegramSession)),`.
- [ ] **Step 3: Build + tidy** — `go build ./... && go mod tidy`. Expected: exit 0; `gotd/td` становится direct в go.mod (предупреждение `not used` исчезает).
- [ ] **Step 4: Commit** — `git add internal/data go.mod go.sum && git commit -m "feat(ft-009): gotd-backed TelegramSession (push, AG-02 for live login)"`

---

## Task 7: wire — регистрация коллектора (AG-01)

**Files:**
- Modify: `internal/task/wire.go`, `cmd/feedium/wire.go`
- Regen: `cmd/feedium/wire_gen.go`

- [ ] **Step 1: task ProviderSet** — добавить `NewTelegramCollector,` в `internal/task/wire.go`; обновить комментарий (Telegram больше не «intentionally absent»).
- [ ] **Step 2: bind ItemCollector** — в `cmd/feedium/wire.go` `wire.Build` добавить `wire.Bind(new(task.ItemCollector), new(*biz.CollectUsecase)),`.
- [ ] **Step 3: newApp** — добавить параметр `telegramCollector *task.TelegramCollector` и в `kratos.Server(hs, gs, pool, scheduler, reaper, rssCollector, webCollector, telegramCollector)`.
- [ ] **Step 4: Реген (AG-01)** — `make wire`. Показать `git diff --stat cmd/feedium/wire_gen.go`.
- [ ] **Step 5: Build** — `make build`. Expected: exit 0.
- [ ] **Step 6: Commit** — `git add internal/task/wire.go cmd/feedium && git commit -m "feat(ft-009): wire TelegramCollector into app"`

---

## Task 8: Полный gate

- [ ] **Step 1** — `make build`. Expected: exit 0.
- [ ] **Step 2** — `make test` (Docker для testcontainers). Expected: PASS; существующие RSS/web/FT-005/FT-007 тесты не правились и зелёные (иначе `STOP-01`).
- [ ] **Step 3** — `go test ./internal/task/... ./internal/biz/... -race` (Docker). Expected: PASS, без leak.
- [ ] **Step 4** — `make lint`. Expected: 0 issues.
- [ ] **Step 5: Commit** (если линтер что-то поправил) — `git commit -am "chore(ft-009): lint"`.

---

## Task 9: Docs & status

**Files:**
- Modify: `feature.md`, `spec.md`, `implement.md`, `memory-bank/features/index.md`, `internal/task/telegram_collector.go` (cross-ref)

- [ ] **Step 1: feature.md** — `delivery_status: planned → done`; добавить DEC-08 (push для TG) в раздел решений; пометить `REQ-08` неприменимым к Telegram.
- [ ] **Step 2: spec.md** — отметить, что Telegram использует push (порт `TelegramSession`), polling-описание §Solution относится к RSS/web; сослаться на `2026-05-26-telegram-push-design.md`.
- [ ] **Step 3: index.md** — синхронизировать статус FT-009 (done), добавить недостающие FT при необходимости.
- [ ] **Step 4: cross-ref** — в шапке `telegram_collector.go` ссылка на `feature.md`/design-doc.
- [ ] **Step 5** — `make lint`. Commit: `git add memory-bank internal/task && git commit -m "docs(ft-009): mark delivery done; record push design (DEC-08)"`.

---

## Self-Review (spec coverage)

| Spec / design факт | Покрыто задачей |
|---|---|
| DEC-08 push-модель | Task 3 (port), Task 4 (consumer), Task 6 (gotd impl) |
| REQ-02 канал → пост + событие | Task 4 (routing), Task 5 (outbox channel) |
| REQ-03 группа → cumulative без события | Task 5 (outbox group) |
| REQ-06 idempotency, REQ-10 text-only | Task 3 (`CollectItems` reuse), Task 3 tests |
| REQ-07/FM-T01 изоляция отказов | Task 4 (`handle` логирует, не падает) |
| REQ-09 режимы по источнику | Task 5 (integration) |
| poll_interval удалён (DEC-08) | Task 1 |
| Откат устаревшего `Fetcher`-порта | Task 2 |
| Durability in-memory, без backfill | Task 6 (updates.New с in-mem storage) |
| AG-01 реген | Task 1, Task 7 |
| AG-02 живой коннект | Task 6 (ErrTelegramNotAuthorized вместо авто-входа) |
| MET-01 4/4 типа | Task 7 (регистрация) + Task 8 (gate) |
