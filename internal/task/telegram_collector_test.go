package task_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/4itosik/feedium/internal/biz"
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
