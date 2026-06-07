package task_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/4itosik/feedium/internal/biz"
	"github.com/4itosik/feedium/internal/conf"
	"github.com/4itosik/feedium/internal/data"
	"github.com/4itosik/feedium/internal/task"
)

// ── scoring test helpers (shared across scoring task tests) ──────────────────

type fakeScorer struct {
	mu        sync.Mutex
	vec       []float32
	failFirst int
	calls     int
}

func (f *fakeScorer) Embed(_ context.Context, _ string) ([]float32, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.calls <= f.failFirst {
		return nil, errors.New("transient 503 from provider")
	}
	return f.vec, nil
}

func (f *fakeScorer) Model() string { return "test-model" }

func testScoringCfg() *conf.Scoring {
	return &conf.Scoring{
		Worker: &conf.ScoringWorker{
			PollInterval:      durationpb.New(20 * time.Millisecond),
			Concurrency:       2,
			LeaseTtl:          durationpb.New(2 * time.Second),
			HeartbeatInterval: durationpb.New(500 * time.Millisecond),
			GracefulTimeout:   durationpb.New(2 * time.Second),
			MaxAttempts:       3,
			BackoffBase:       durationpb.New(100 * time.Millisecond),
			BackoffMax:        durationpb.New(500 * time.Millisecond),
			EventTtl:          durationpb.New(24 * time.Hour),
		},
		Scheduler: &conf.ScoringScheduler{PollInterval: durationpb.New(30 * time.Millisecond)},
		Reaper: &conf.ScoringReaper{
			Interval: durationpb.New(40 * time.Millisecond),
			Grace:    durationpb.New(10 * time.Millisecond),
		},
	}
}

func newTestScoreUsecase(t *testing.T, d *data.Data, cfg *conf.Scoring, scorer biz.Scorer) *biz.ScoreUsecase {
	t.Helper()
	return biz.NewScoreUsecase(
		data.NewScoreRepo(d),
		data.NewInterestRepo(d),
		data.NewSummaryRepo(d),
		data.NewScoreOutboxRepo(d),
		scorer,
		data.NewPostEmbeddingRepo(d),
		cfg,
	)
}

func createTestSummary(ctx context.Context, t *testing.T, d *data.Data, sourceID, postID string) {
	t.Helper()
	pid := postID
	_, err := data.NewSummaryRepo(d).Save(ctx, biz.Summary{
		ID:        uuid.Must(uuid.NewV7()).String(),
		PostID:    &pid,
		SourceID:  sourceID,
		Text:      "a summary",
		WordCount: 2,
		CreatedAt: time.Now(),
	})
	require.NoError(t, err)
}

func createTestInterest(ctx context.Context, t *testing.T, d *data.Data, embedding []float32) {
	t.Helper()
	now := time.Now()
	_, err := data.NewInterestRepo(d).Save(ctx, biz.Interest{
		ID:        uuid.Must(uuid.NewV7()).String(),
		Topic:     "topic-" + uuid.NewString()[:8],
		Embedding: embedding,
		Model:     "test-model",
		CreatedAt: now,
		UpdatedAt: now,
	})
	require.NoError(t, err)
}

func scoreEventStatus(ctx context.Context, t *testing.T, d *data.Data, postID string) string {
	t.Helper()
	var status string
	err := d.DB.QueryRowContext(ctx, `SELECT status FROM score_events WHERE post_id = $1`, postID).Scan(&status)
	require.NoError(t, err)
	return status
}

// ── tests ────────────────────────────────────────────────────────────────────

func TestIntegration_ScoreWorkerPool_HappyPath(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// Arrange
	ctx := context.Background()
	d, cleanup := setupTestStack(t)
	defer cleanup()
	sourceID := createTestGroupSource(ctx, t, d)
	post := createTestPost(ctx, t, d, sourceID, "post body text")
	createTestSummary(ctx, t, d, sourceID, post.ID)
	createTestInterest(ctx, t, d, []float32{1, 0})
	_, err := data.NewScoreOutboxRepo(d).EnqueueDuePosts(ctx)
	require.NoError(t, err)
	cfg := testScoringCfg()
	uc := newTestScoreUsecase(t, d, cfg, &fakeScorer{vec: []float32{1, 0}})
	pool := task.NewScoreEventWorkerPool(data.NewScoreOutboxRepo(d), uc, cfg, testLogger())

	// Act
	require.NoError(t, pool.Start(ctx))
	defer func() { require.NoError(t, pool.Stop(context.Background())) }()

	// Assert
	scoreRepo := data.NewScoreRepo(d)
	require.Eventually(t, func() bool {
		feed, listErr := scoreRepo.ListRanked(ctx, biz.FeedQuery{PageSize: 10})
		return listErr == nil && len(feed.Items) == 1
	}, 15*time.Second, 50*time.Millisecond)

	feed, err := scoreRepo.ListRanked(ctx, biz.FeedQuery{PageSize: 10})
	require.NoError(t, err)
	require.Len(t, feed.Items, 1)
	assert.InDelta(t, 1.0, feed.Items[0].Score, 1e-9)
	assert.Equal(t, "completed", scoreEventStatus(ctx, t, d, post.ID))
}

func TestIntegration_ScoreWorkerPool_TransientThenSuccess(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// Arrange
	ctx := context.Background()
	d, cleanup := setupTestStack(t)
	defer cleanup()
	sourceID := createTestGroupSource(ctx, t, d)
	post := createTestPost(ctx, t, d, sourceID, "post body text")
	createTestSummary(ctx, t, d, sourceID, post.ID)
	createTestInterest(ctx, t, d, []float32{1, 0})
	_, err := data.NewScoreOutboxRepo(d).EnqueueDuePosts(ctx)
	require.NoError(t, err)
	cfg := testScoringCfg()
	uc := newTestScoreUsecase(t, d, cfg, &fakeScorer{vec: []float32{1, 0}, failFirst: 1})
	pool := task.NewScoreEventWorkerPool(data.NewScoreOutboxRepo(d), uc, cfg, testLogger())

	// Act
	require.NoError(t, pool.Start(ctx))
	defer func() { require.NoError(t, pool.Stop(context.Background())) }()

	// Assert
	require.Eventually(t, func() bool {
		return scoreEventStatus(ctx, t, d, post.ID) == "completed"
	}, 15*time.Second, 50*time.Millisecond)
}

func TestIntegration_ScoreWorkerPool_PermanentFailureIsObservable(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// Arrange: a post with no summary forces a permanent ErrScoreValidation.
	ctx := context.Background()
	d, cleanup := setupTestStack(t)
	defer cleanup()
	sourceID := createTestGroupSource(ctx, t, d)
	post := createTestPost(ctx, t, d, sourceID, "post body text")
	createTestSummary(ctx, t, d, sourceID, post.ID)
	// Enqueue while the summary exists, then no interest is needed; permanent
	// failure is induced by deleting the summary so ScorePost finds none.
	_, err := data.NewScoreOutboxRepo(d).EnqueueDuePosts(ctx)
	require.NoError(t, err)
	_, err = d.DB.ExecContext(ctx, `DELETE FROM summaries WHERE post_id = $1`, post.ID)
	require.NoError(t, err)
	cfg := testScoringCfg()
	uc := newTestScoreUsecase(t, d, cfg, &fakeScorer{vec: []float32{1, 0}})
	pool := task.NewScoreEventWorkerPool(data.NewScoreOutboxRepo(d), uc, cfg, testLogger())

	// Act
	require.NoError(t, pool.Start(ctx))
	defer func() { require.NoError(t, pool.Stop(context.Background())) }()

	// Assert
	require.Eventually(t, func() bool {
		return scoreEventStatus(ctx, t, d, post.ID) == "failed"
	}, 15*time.Second, 50*time.Millisecond)
}
