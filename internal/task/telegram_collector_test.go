package task_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/4itosik/feedium/internal/biz"
	"github.com/4itosik/feedium/internal/data"
	"github.com/4itosik/feedium/internal/task"
)

type fakeSession struct{ msgs []biz.TelegramMessage }

func (f *fakeSession) Run(ctx context.Context, handle func(context.Context, biz.TelegramMessage) error) error {
	for _, m := range f.msgs {
		_ = handle(ctx, m)
	}
	<-ctx.Done()
	return ctx.Err()
}

type fakeSourceLister struct{ items []biz.Source }

func (f *fakeSourceLister) List(_ context.Context, _ biz.ListSourcesFilter) (biz.ListSourcesResult, error) {
	return biz.ListSourcesResult{Items: f.items}, nil
}

type recordingCollector struct {
	mu    sync.Mutex
	calls []struct {
		source biz.Source
		items  []biz.CollectedItem
	}
}

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

	chSrc := biz.Source{
		ID:     "s-chan",
		Type:   biz.SourceTypeTelegramChannel,
		Config: &biz.TelegramChannelConfig{TgID: 111},
	}
	rec := &recordingCollector{}
	session := &fakeSession{msgs: []biz.TelegramMessage{
		{TgID: 111, Item: biz.CollectedItem{ExternalID: "1", Text: "hi"}}, // matches chSrc
		{TgID: 999, Item: biz.CollectedItem{ExternalID: "2", Text: "x"}},  // no source → ignored
	}}
	c := task.NewTelegramCollector(rec, session, &fakeSourceLister{items: []biz.Source{chSrc}}, testLogger())

	require.NoError(t, c.Start(context.Background()))
	gotOneCall := func() bool {
		rec.mu.Lock()
		defer rec.mu.Unlock()
		return len(rec.calls) == 1
	}
	require.Eventually(t, gotOneCall, time.Second, 5*time.Millisecond)
	require.NoError(t, c.Stop(context.Background()))

	require.Equal(t, "s-chan", rec.calls[0].source.ID)
	require.Equal(t, "1", rec.calls[0].items[0].ExternalID)
}

// TestIntegration_TelegramCollector_OutboxByMode proves the per-source-type
// outbox contract (REQ-09) end-to-end against a real PostgreSQL: a Telegram
// channel source (self_contained mode) enqueues a summarize_post event when a
// post is collected, while a Telegram group source (cumulative mode) does NOT
// enqueue a per-post event. It wires the real CollectUsecase (ItemCollector) and
// SourceUsecase (SourceLister) so the routing + persistence + outbox path is the
// production one; only the push session is faked to deliver one message per
// source (DEC-08). This closes the SC-02/SC-03 Telegram outbox gap that the RSS
// integration test could only approximate for the self-contained path.
func TestIntegration_TelegramCollector_OutboxByMode(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// Arrange
	ctx := context.Background()
	d, cleanup := setupTestStack(t)
	defer cleanup()

	sourceUC := biz.NewSourceUsecase(data.NewSourceRepo(d))
	chSrc, err := sourceUC.Create(ctx, biz.SourceTypeTelegramChannel, &biz.TelegramChannelConfig{TgID: 111})
	require.NoError(t, err)
	require.Equal(t, biz.ProcessingModeSelfContained, chSrc.ProcessingMode)
	grpSrc, err := sourceUC.Create(ctx, biz.SourceTypeTelegramGroup, &biz.TelegramGroupConfig{TgID: 222})
	require.NoError(t, err)
	require.Equal(t, biz.ProcessingModeCumulative, grpSrc.ProcessingMode)

	now := time.Now()
	session := &fakeSession{msgs: []biz.TelegramMessage{
		{TgID: 111, Item: biz.CollectedItem{ExternalID: "c1", Text: "channel post", PublishedAt: now}},
		{TgID: 222, Item: biz.CollectedItem{ExternalID: "g1", Text: "group post", PublishedAt: now}},
	}}
	collector := task.NewTelegramCollector(newCollectUsecase(d), session, sourceUC, testLogger())

	// Act
	require.NoError(t, collector.Start(ctx))
	require.Eventually(t, func() bool {
		return countPosts(ctx, t, d, chSrc.ID) >= 1 && countPosts(ctx, t, d, grpSrc.ID) >= 1
	}, 5*time.Second, 20*time.Millisecond, "both posts persisted")
	require.NoError(t, collector.Stop(ctx))

	// Assert — channel (self_contained) enqueues a per-post summarize_post event;
	// group (cumulative) does not (REQ-09).
	outbox := data.NewSummaryOutboxRepo(d)
	chHas, _, err := outbox.HasActiveEvent(ctx, chSrc.ID, biz.SummaryEventTypeSummarizePost)
	require.NoError(t, err)
	assert.True(t, chHas, "channel source enqueues a summarize_post outbox event")

	grpHas, _, err := outbox.HasActiveEvent(ctx, grpSrc.ID, biz.SummaryEventTypeSummarizePost)
	require.NoError(t, err)
	assert.False(t, grpHas, "group source does not enqueue a per-post summarize_post event")
}
