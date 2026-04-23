package data_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/4itosik/feedium/internal/biz"
	"github.com/4itosik/feedium/internal/data"
)

func TestIntegration_OutboxRepo_ClaimOne_EmptyQueue(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	d, cleanup := setupTestData(t)
	defer cleanup()

	repo := data.NewSummaryOutboxRepo(d)

	_, err := repo.ClaimOne(ctx, "worker-1", 5*time.Minute)
	assert.ErrorIs(t, err, biz.ErrNoEventAvailable)
}

func TestIntegration_OutboxRepo_ClaimOne_ConcurrentUniqueness(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	d, cleanup := setupTestData(t)
	defer cleanup()

	client := d.Ent
	repo := data.NewSummaryOutboxRepo(d)

	// Seed N pending events across N sources.
	const n = 20
	wantIDs := make(map[string]struct{}, n)
	for range n {
		sourceID := createTestSource(ctx, t, client)
		event := biz.NewSummaryEvent(biz.SummaryEventTypeSummarizeSource, sourceID, nil)
		saved, err := repo.Save(ctx, event)
		require.NoError(t, err)
		wantIDs[saved.ID] = struct{}{}
	}

	// Run N concurrent claimers.
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		claimed = make(map[string]string) // id -> worker
	)
	for i := range n {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			workerID := uuid.New().String()
			ev, err := repo.ClaimOne(ctx, workerID, 5*time.Minute)
			if err != nil {
				return
			}
			mu.Lock()
			if prev, ok := claimed[ev.ID]; ok {
				t.Errorf("event %s claimed twice (worker=%s prev=%s)", ev.ID, workerID, prev)
			}
			claimed[ev.ID] = workerID
			mu.Unlock()
			_ = idx
		}(i)
	}
	wg.Wait()

	assert.Len(t, claimed, n, "every event claimed exactly once")
	for id := range claimed {
		_, ok := wantIDs[id]
		assert.True(t, ok, "claimed id %s was one of the seeded events", id)
	}
}

func TestIntegration_OutboxRepo_ClaimOne_RecaptureAfterLeaseExpiry(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	d, cleanup := setupTestData(t)
	defer cleanup()

	client := d.Ent
	repo := data.NewSummaryOutboxRepo(d)

	sourceID := createTestSource(ctx, t, client)
	event := biz.NewSummaryEvent(biz.SummaryEventTypeSummarizeSource, sourceID, nil)
	saved, err := repo.Save(ctx, event)
	require.NoError(t, err)

	// First claim with a very short lease, then wait it out.
	firstWorker := "worker-1"
	first, err := repo.ClaimOne(ctx, firstWorker, 50*time.Millisecond)
	require.NoError(t, err)
	assert.Equal(t, saved.ID, first.ID)

	// Before expiry: a second claimer sees nothing.
	_, err = repo.ClaimOne(ctx, "worker-2", 5*time.Minute)
	require.ErrorIs(t, err, biz.ErrNoEventAvailable)

	time.Sleep(150 * time.Millisecond)

	// After expiry: another worker can re-claim the same row.
	second, err := repo.ClaimOne(ctx, "worker-2", 5*time.Minute)
	require.NoError(t, err)
	assert.Equal(t, saved.ID, second.ID)
}

func TestIntegration_OutboxRepo_ExtendLease_LostAfterExpiry(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	d, cleanup := setupTestData(t)
	defer cleanup()

	client := d.Ent
	repo := data.NewSummaryOutboxRepo(d)

	sourceID := createTestSource(ctx, t, client)
	saved, err := repo.Save(ctx, biz.NewSummaryEvent(biz.SummaryEventTypeSummarizeSource, sourceID, nil))
	require.NoError(t, err)

	worker := "worker-1"
	_, err = repo.ClaimOne(ctx, worker, 50*time.Millisecond)
	require.NoError(t, err)

	// While lease is alive, ExtendLease succeeds.
	require.NoError(t, repo.ExtendLease(ctx, saved.ID, worker, 50*time.Millisecond))

	time.Sleep(120 * time.Millisecond)

	// After lease expires, ExtendLease reports ErrLeaseLost.
	err = repo.ExtendLease(ctx, saved.ID, worker, 5*time.Minute)
	assert.ErrorIs(t, err, biz.ErrLeaseLost)
}

func TestIntegration_OutboxRepo_FinalizeWithLease_RejectsStale(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	d, cleanup := setupTestData(t)
	defer cleanup()

	client := d.Ent
	repo := data.NewSummaryOutboxRepo(d)

	sourceID := createTestSource(ctx, t, client)
	saved, err := repo.Save(ctx, biz.NewSummaryEvent(biz.SummaryEventTypeSummarizeSource, sourceID, nil))
	require.NoError(t, err)

	// Claim, then simulate lease loss by waiting past expiry.
	worker := "worker-1"
	_, err = repo.ClaimOne(ctx, worker, 50*time.Millisecond)
	require.NoError(t, err)
	time.Sleep(120 * time.Millisecond)

	// Another worker re-claims.
	_, err = repo.ClaimOne(ctx, "worker-2", 5*time.Minute)
	require.NoError(t, err)

	// Original worker cannot finalize.
	err = repo.FinalizeWithLease(ctx, saved.ID, worker, biz.SummaryEventStatusCompleted, nil, nil)
	assert.ErrorIs(t, err, biz.ErrLeaseLost)
}

func TestIntegration_OutboxRepo_MarkForRetry_ResetsBackToPending(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	d, cleanup := setupTestData(t)
	defer cleanup()

	client := d.Ent
	repo := data.NewSummaryOutboxRepo(d)

	sourceID := createTestSource(ctx, t, client)
	saved, err := repo.Save(ctx, biz.NewSummaryEvent(biz.SummaryEventTypeSummarizeSource, sourceID, nil))
	require.NoError(t, err)

	worker := "worker-1"
	_, err = repo.ClaimOne(ctx, worker, 5*time.Minute)
	require.NoError(t, err)

	retryAt := time.Now().Add(10 * time.Second)
	require.NoError(t, repo.MarkForRetry(ctx, saved.ID, worker, retryAt, "transient boom"))

	fetched, err := repo.Get(ctx, saved.ID)
	require.NoError(t, err)
	assert.Equal(t, biz.SummaryEventStatusPending, fetched.Status)
	require.NotNil(t, fetched.Error)
	assert.Equal(t, "transient boom", *fetched.Error)

	// Before next_attempt_at, a claim must not pick this row.
	_, err = repo.ClaimOne(ctx, "worker-2", 5*time.Minute)
	assert.ErrorIs(t, err, biz.ErrNoEventAvailable)
}

func TestIntegration_OutboxRepo_ListLeaseExpired(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	d, cleanup := setupTestData(t)
	defer cleanup()

	client := d.Ent
	repo := data.NewSummaryOutboxRepo(d)

	sourceID := createTestSource(ctx, t, client)
	saved, err := repo.Save(ctx, biz.NewSummaryEvent(biz.SummaryEventTypeSummarizeSource, sourceID, nil))
	require.NoError(t, err)

	_, err = repo.ClaimOne(ctx, "worker-1", 50*time.Millisecond)
	require.NoError(t, err)

	time.Sleep(150 * time.Millisecond)

	stuck, err := repo.ListLeaseExpired(ctx, 50*time.Millisecond, 10)
	require.NoError(t, err)
	require.Len(t, stuck, 1)
	assert.Equal(t, saved.ID, stuck[0].ID)
}
