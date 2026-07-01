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

// textCountingScorer is a fake Scorer that returns a deterministic embedding and
// records how many times Embed is called PER input text. This lets the FT-013
// E2E test disambiguate the two kinds of embed calls that occur in the pipeline:
//
//   - POST embeds: keyed by the post's summary text ("a summary"). MET-01/MET-02
//     require ZERO additional post embeds when the summary text is unchanged
//     (the persisted vector is reused, CTR-05/INV-02).
//   - INTEREST embeds: keyed by the interest topic text. These are the interest's
//     own topic encoding (allowed/expected on Create/Update — DEC-04). They are
//     counted separately so they never pollute the post-embed assertion.
type textCountingScorer struct {
	mu     sync.Mutex
	vec    []float32
	byText map[string]int
}

func newTextCountingScorer(vec []float32) *textCountingScorer {
	return &textCountingScorer{vec: vec, byText: map[string]int{}}
}

func (s *textCountingScorer) Embed(_ context.Context, text string) ([]float32, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byText[text]++
	return s.vec, nil
}

func (s *textCountingScorer) Model() string { return "test-model" }

// embedCount returns the number of Embed calls made for the exact input text.
func (s *textCountingScorer) embedCount(text string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.byText[text]
}

// postEmbedCount returns Embed calls for the post summary text. createTestSummary
// persists the summary body as "a summary"; that is the input scoring embeds.
func (s *textCountingScorer) postEmbedCount() int { return s.embedCount("a summary") }

func TestIntegration_FT013_VectorReuse_And_InterestChangeRescore(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	d, cleanup := setupTestStack(t)
	defer cleanup()

	// ── Arrange: one source, one post with a summary, one interest ──────────────
	sourceID := createTestGroupSource(ctx, t, d)
	post := createTestPost(ctx, t, d, sourceID, "post body text")
	createTestSummary(ctx, t, d, sourceID, post.ID) // summary text == "a summary"
	createTestInterest(ctx, t, d, []float32{1, 0})

	cfg := testScoringCfg()
	scorer := newTextCountingScorer([]float32{1, 0})
	uc := newTestScoreUsecase(t, d, cfg, scorer)
	outbox := data.NewScoreOutboxRepo(d)

	currentScore := func() (float64, bool) {
		var value float64
		err := d.DB.QueryRowContext(ctx,
			`SELECT value FROM scores WHERE post_id = $1`, post.ID).Scan(&value)
		if err != nil {
			return 0, false
		}
		return value, true
	}

	postEmbeddingRow := func() biz.PostEmbedding {
		pid := post.ID
		pe, err := data.NewPostEmbeddingRepo(d).Get(ctx, biz.ScoreKey{PostID: &pid})
		require.NoError(t, err)
		return pe
	}

	// latestEventStatus returns the status of the most recent score_event for the
	// post. After repeated enqueues there are several events for the post, so we
	// must look at the newest one rather than an arbitrary row.
	latestEventStatus := func() string {
		var status string
		err := d.DB.QueryRowContext(ctx,
			`SELECT status FROM score_events WHERE post_id = $1 ORDER BY created_at DESC, id DESC LIMIT 1`,
			post.ID).Scan(&status)
		require.NoError(t, err)
		return status
	}

	// driveScoring drains all currently-active events through the real worker pool
	// and waits until none remain pending/processing for the post.
	driveScoring := func() {
		pool := task.NewScoreEventWorkerPool(outbox, uc, cfg, testLogger())
		require.NoError(t, pool.Start(ctx))
		defer func() { require.NoError(t, pool.Stop(context.Background())) }()
		require.Eventually(t, func() bool {
			var active int
			require.NoError(t, d.DB.QueryRowContext(ctx,
				`SELECT count(*) FROM score_events WHERE post_id = $1 AND status IN ('pending','processing')`,
				post.ID).Scan(&active))
			return active == 0 && latestEventStatus() == "completed"
		}, 15*time.Second, 50*time.Millisecond)
	}

	// ── Part 1: Reuse (MET-01, SC-01, CTR-05) ──────────────────────────────────
	// First scoring: enqueue the due post and run it through the worker pool. This
	// must produce a score row, a post_embedding row, and exactly ONE post embed.
	_, err := outbox.EnqueueDuePosts(ctx)
	require.NoError(t, err)
	driveScoring()

	firstScore, ok := currentScore()
	require.True(t, ok, "a score row exists after first scoring")
	assert.InDelta(t, 1.0, firstScore, 1e-9)
	require.Equal(t, 1, scorer.postEmbedCount(), "first scoring embeds the post exactly once")
	row1 := postEmbeddingRow()

	// Second scoring of the SAME post with unchanged summary text. We re-enqueue
	// (the previous event completed, so EnqueueScoredPosts re-queues it) and run
	// the pool again. The persisted vector must be reused: NO new post embed, and
	// an identical score value (CTR-05).
	n, err := outbox.EnqueueScoredPosts(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, n, "completed event no longer blocks re-enqueue")
	driveScoring()

	secondScore, ok := currentScore()
	require.True(t, ok)
	assert.InDelta(t, firstScore, secondScore, 1e-9, "score is identical on reuse (CTR-05)")
	assert.Equal(t, 1, scorer.postEmbedCount(),
		"re-scoring unchanged summary makes 0 additional POST embeds (MET-01)")
	// The persisted vector row is untouched (same model, hash, updated_at).
	row2 := postEmbeddingRow()
	assert.Equal(t, row1.Model, row2.Model)
	assert.Equal(t, row1.InputHash, row2.InputHash)
	assert.True(t, row1.UpdatedAt.Equal(row2.UpdatedAt),
		"post_embedding row is not rewritten when the vector is reused")

	// ── Part 2: Auto re-score on interest change (MET-02, SC-03, REQ-04, INV-04) ─
	// Add a second interest via the real InterestUsecase. Its Create both embeds
	// the NEW topic (an interest embed — expected) and, in the same transaction,
	// EnqueueScoredPosts re-queues the already-scored post (REQ-04).
	postEmbedsBefore := scorer.postEmbedCount()
	interestUC := biz.NewInterestUsecase(
		data.NewInterestRepo(d), scorer, outbox, data.NewTxManager(d),
	)
	_, err = interestUC.Create(ctx, "a brand new interest topic")
	require.NoError(t, err)
	assert.Equal(t, 1, scorer.embedCount("a brand new interest topic"),
		"the interest's own topic is embedded once (expected interest embed)")

	// The interest change must have enqueued exactly one active re-score event for
	// the existing scored post (INV-04: one active event at a time).
	var active int
	require.NoError(t, d.DB.QueryRowContext(ctx,
		`SELECT count(*) FROM score_events WHERE post_id = $1 AND status IN ('pending','processing')`,
		post.ID).Scan(&active))
	require.Equal(t, 1, active, "interest change enqueues one re-score event (REQ-04, INV-04)")

	// Drive the re-score. The post text is unchanged, so the vector is reused: NO
	// new POST embed even though the interest set changed (MET-02).
	driveScoring()

	assert.Equal(t, postEmbedsBefore, scorer.postEmbedCount(),
		"re-scoring after an interest change makes 0 NEW post embeds (MET-02)")

	// Exactly ONE current score row remains for the post (INV-04: single current
	// score, upserted in place).
	var scoreRows int
	require.NoError(t, d.DB.QueryRowContext(ctx,
		`SELECT count(*) FROM scores WHERE post_id = $1`, post.ID).Scan(&scoreRows))
	assert.Equal(t, 1, scoreRows, "exactly one current score row for the post (INV-04)")

	// And the post was genuinely re-scored to completion.
	assert.Equal(t, "completed", latestEventStatus())
}
