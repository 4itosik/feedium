package task

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/4itosik/feedium/internal/biz"
	"github.com/4itosik/feedium/internal/conf"
)

// These are white-box tests (package task) for the unexported collection
// helpers. They are deterministic — no time.Sleep, no real clock — which is
// what the SC-08 cadence and FM-T02 retry-recovery scenarios require and what
// the black-box integration tests in package task_test cannot reach.

// fakeCollector returns a scripted error per call and counts invocations.
type fakeCollector struct {
	mu      sync.Mutex
	results []error
	calls   int
}

func (f *fakeCollector) CollectSource(_ context.Context, _ biz.Source, _ biz.Fetcher) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	i := f.calls
	f.calls++
	if i < len(f.results) {
		return f.results[i]
	}
	return nil
}

func (f *fakeCollector) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// fakeSourceLister returns scripted pages/errors, one per List call.
type fakeSourceLister struct {
	mu    sync.Mutex
	pages []biz.ListSourcesResult
	errs  []error
	calls int
}

func (f *fakeSourceLister) List(_ context.Context, _ biz.ListSourcesFilter) (biz.ListSourcesResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	i := f.calls
	f.calls++
	if i < len(f.errs) && f.errs[i] != nil {
		return biz.ListSourcesResult{}, f.errs[i]
	}
	if i < len(f.pages) {
		return f.pages[i], nil
	}
	return biz.ListSourcesResult{}, nil
}

func (f *fakeSourceLister) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// errCountHandler counts error-level slog records.
type errCountHandler struct {
	mu sync.Mutex
	n  int
}

func (h *errCountHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *errCountHandler) Handle(_ context.Context, r slog.Record) error {
	if r.Level >= slog.LevelError {
		h.mu.Lock()
		h.n++
		h.mu.Unlock()
	}
	return nil
}

func (h *errCountHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *errCountHandler) WithGroup(string) slog.Handler      { return h }

func (h *errCountHandler) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.n
}

// TestRSSCollector_StartUsesConfiguredPollInterval proves the RSS collector
// waits exactly the interval taken from config (SC-08) — not a hardcoded value
// or the default. A fake `after` captures the duration the loop requests.
func TestRSSCollector_StartUsesConfiguredPollInterval(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// Arrange
	const configured = 7 * time.Second // differs from defaultCollectorPoll (5m)
	gotInterval := make(chan time.Duration, 8)
	release := make(chan time.Time, 8)

	c := NewRSSCollector(
		&fakeCollector{},
		nil, // fetcher unused: no sources are listed
		&fakeSourceLister{},
		&conf.Collectors{Rss: &conf.CollectorPoll{PollInterval: durationpb.New(configured)}},
		slog.New(&errCountHandler{}),
	)
	c.after = func(d time.Duration) <-chan time.Time {
		gotInterval <- d
		return release
	}

	// Act
	require.NoError(t, c.Start(context.Background()))
	first := <-gotInterval
	release <- time.Now() // let one more cycle run
	second := <-gotInterval
	require.NoError(t, c.Stop(context.Background()))

	// Assert
	assert.Equal(t, configured, first, "first wait must use the configured interval, not the default")
	assert.Equal(t, configured, second, "cadence keeps using the configured interval")
}

// TestWebCollector_StartUsesConfiguredPollInterval is the web counterpart of
// the test above: it guards against the web collector reading the wrong config
// field (SC-08).
func TestWebCollector_StartUsesConfiguredPollInterval(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// Arrange
	const configured = 11 * time.Second
	gotInterval := make(chan time.Duration, 8)
	release := make(chan time.Time, 8)

	c := NewWebCollector(
		&fakeCollector{},
		nil,
		&fakeSourceLister{},
		&conf.Collectors{Web: &conf.CollectorPoll{PollInterval: durationpb.New(configured)}},
		slog.New(&errCountHandler{}),
	)
	c.after = func(d time.Duration) <-chan time.Time {
		gotInterval <- d
		return release
	}

	// Act
	require.NoError(t, c.Start(context.Background()))
	first := <-gotInterval
	require.NoError(t, c.Stop(context.Background()))

	// Assert
	assert.Equal(t, configured, first)
}

// TestWebCollector_collectWithRetry covers single-retry-then-skip (FM-T02),
// including the retry-recovery path a missing retry would silently break, and
// the cancelled-context guard.
func TestWebCollector_collectWithRetry(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	transient := errors.New("transient extraction failure")
	persistent := errors.New("markup changed")

	t.Run("first attempt fails, retry succeeds", func(t *testing.T) {
		// Arrange
		fc := &fakeCollector{results: []error{transient, nil}}
		c := NewWebCollector(fc, nil, &fakeSourceLister{}, &conf.Collectors{}, slog.New(&errCountHandler{}))

		// Act
		err := c.collectWithRetry(context.Background(), biz.Source{ID: "s1"})

		// Assert
		require.NoError(t, err)
		assert.Equal(t, 2, fc.callCount(), "exactly one retry after the first failure")
	})

	t.Run("both attempts fail returns the error", func(t *testing.T) {
		// Arrange
		fc := &fakeCollector{results: []error{transient, persistent}}
		c := NewWebCollector(fc, nil, &fakeSourceLister{}, &conf.Collectors{}, slog.New(&errCountHandler{}))

		// Act
		err := c.collectWithRetry(context.Background(), biz.Source{ID: "s1"})

		// Assert
		require.Error(t, err)
		require.ErrorIs(t, err, persistent)
		assert.Equal(t, 2, fc.callCount())
	})

	t.Run("cancelled context skips the retry", func(t *testing.T) {
		// Arrange
		fc := &fakeCollector{results: []error{transient, nil}}
		c := NewWebCollector(fc, nil, &fakeSourceLister{}, &conf.Collectors{}, slog.New(&errCountHandler{}))
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		// Act
		err := c.collectWithRetry(ctx, biz.Source{ID: "s1"})

		// Assert
		require.Error(t, err)
		require.ErrorIs(t, err, transient)
		assert.Equal(t, 1, fc.callCount(), "no retry once the context is cancelled (graceful shutdown)")
	})
}

// TestCollectAllOfType_PagesThroughResults proves the listing loop follows
// NextPageToken across pages (otherwise a paginated source set is silently
// half-collected).
func TestCollectAllOfType_PagesThroughResults(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// Arrange
	lister := &fakeSourceLister{pages: []biz.ListSourcesResult{
		{Items: []biz.Source{{ID: "s1"}}, NextPageToken: "page-2"},
		{Items: []biz.Source{{ID: "s2"}}, NextPageToken: ""},
	}}
	var collected []string

	// Act
	collectAllOfType(context.Background(), slog.New(&errCountHandler{}), "test", biz.SourceTypeRSS, lister,
		func(src biz.Source) error {
			collected = append(collected, src.ID)
			return nil
		})

	// Assert
	assert.Equal(t, []string{"s1", "s2"}, collected)
	assert.Equal(t, 2, lister.callCount(), "second page is requested")
}

// TestCollectAllOfType_ListFailureIsObservable proves a listing error ends the
// tick with an observable error log (REQ-07) and never invokes collection.
func TestCollectAllOfType_ListFailureIsObservable(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// Arrange
	lister := &fakeSourceLister{errs: []error{errors.New("list failed")}}
	rec := &errCountHandler{}
	collectCalls := 0

	// Act
	collectAllOfType(context.Background(), slog.New(rec), "test", biz.SourceTypeRSS, lister,
		func(biz.Source) error {
			collectCalls++
			return nil
		})

	// Assert
	assert.Equal(t, 0, collectCalls, "no source is collected when listing fails")
	assert.Positive(t, rec.count(), "listing failure is logged")
}

// itemCollectorStub records CollectItems calls and can return a scripted error,
// so the Telegram push resilience branches can be exercised deterministically.
type itemCollectorStub struct {
	mu    sync.Mutex
	err   error
	calls int
}

func (s *itemCollectorStub) CollectItems(_ context.Context, _ biz.Source, _ []biz.CollectedItem) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	return s.err
}

func (s *itemCollectorStub) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// TestTelegramCollector_handle_ResolveErrorIsLoggedAndStreamContinues proves a
// source-listing failure during resolve is logged and swallowed (handle returns
// nil) so the push stream survives, and no persistence is attempted (REQ-07,
// FM-T01). Without this, a transient List error would silently drop the message
// with no regression guard.
func TestTelegramCollector_handle_ResolveErrorIsLoggedAndStreamContinues(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// Arrange
	lister := &fakeSourceLister{errs: []error{errors.New("list down")}}
	rec := &errCountHandler{}
	collector := &itemCollectorStub{}
	c := NewTelegramCollector(collector, nil, lister, slog.New(rec))

	// Act
	err := c.handle(context.Background(), biz.TelegramMessage{TgID: 1, Item: biz.CollectedItem{Text: "x"}})

	// Assert
	require.NoError(t, err, "a resolve error never breaks the push stream")
	assert.Positive(t, rec.count(), "resolve failure is logged")
	assert.Zero(t, collector.callCount(), "no persistence when the source cannot be resolved")
}

// TestTelegramCollector_handle_PersistErrorIsLoggedAndStreamContinues proves a
// CollectItems failure is logged and swallowed (handle returns nil) so a persist
// error never stops the push stream (REQ-07, FM-T01).
func TestTelegramCollector_handle_PersistErrorIsLoggedAndStreamContinues(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// Arrange
	src := biz.Source{ID: "s1", Type: biz.SourceTypeTelegramChannel, Config: &biz.TelegramChannelConfig{TgID: 7}}
	lister := &fakeSourceLister{pages: []biz.ListSourcesResult{{Items: []biz.Source{src}}}}
	rec := &errCountHandler{}
	collector := &itemCollectorStub{err: errors.New("db down")}
	c := NewTelegramCollector(collector, nil, lister, slog.New(rec))

	// Act
	err := c.handle(
		context.Background(),
		biz.TelegramMessage{TgID: 7, Item: biz.CollectedItem{ExternalID: "1", Text: "hi"}},
	)

	// Assert
	require.NoError(t, err, "a persist error never breaks the push stream")
	assert.Equal(t, 1, collector.callCount(), "persistence was attempted on the resolved source")
	assert.Positive(t, rec.count(), "persist failure is logged")
}
