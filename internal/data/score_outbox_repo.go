package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/4itosik/feedium/internal/biz"
)

// Columns kept in sync with migration 20260602100200_create_score_events_table.sql.
const scoreEventColumns = `id, post_id, status, error, attempt_count, created_at, processed_at`

const scoreEventColumnsQualified = `
    score_events.id, score_events.post_id, score_events.status, score_events.error,
    score_events.attempt_count, score_events.created_at, score_events.processed_at
`

// enqueueScanLimit bounds how many due posts a single enqueue pass considers.
const enqueueScanLimit = 500

// scoreEventInsertQuery inserts a pending score_event, idempotent via ON CONFLICT
// DO NOTHING on the partial unique index over active statuses (INV-04). Shared by
// EnqueueDuePosts and EnqueueScoredPosts to prevent drift.
const scoreEventInsertQuery = `
INSERT INTO score_events (id, post_id, status, attempt_count, created_at)
VALUES ($1, $2, 'pending', 0, now())
ON CONFLICT (post_id) WHERE status IN ('pending', 'processing') DO NOTHING
`

type scoreOutboxRepo struct {
	data *Data
}

var _ biz.ScoreOutboxRepo = (*scoreOutboxRepo)(nil)

// NewScoreOutboxRepo creates a new scoring outbox repository.
//
//nolint:revive // unexported return type for wire DI
func NewScoreOutboxRepo(data *Data) *scoreOutboxRepo {
	return &scoreOutboxRepo{data: data}
}

// EnqueueDuePosts inserts a pending score_event for each summarized-unscored post
// without an active event. Idempotent via ON CONFLICT DO NOTHING on the partial
// unique index (INV-04). Returns the number of events actually enqueued.
func (r *scoreOutboxRepo) EnqueueDuePosts(ctx context.Context) (int, error) {
	ex := sqlExecerFromContext(ctx, r.data.DB)

	postIDs, err := r.dueCandidatePostIDs(ctx, ex)
	if err != nil {
		return 0, err
	}

	enqueued := 0
	for _, pid := range postIDs {
		res, insErr := ex.ExecContext(ctx, scoreEventInsertQuery, uuid.Must(uuid.NewV7()), pid)
		if insErr != nil {
			return enqueued, fmt.Errorf("insert score event: %w", insErr)
		}
		affected, affErr := res.RowsAffected()
		if affErr != nil {
			return enqueued, fmt.Errorf("insert rows affected: %w", affErr)
		}
		enqueued += int(affected)
	}
	return enqueued, nil
}

// EnqueueScoredPosts inserts a pending score_event for each already-scored post
// without an active event — the re-scoring path on interest change (FT-013 CTR-03).
// Idempotent via ON CONFLICT DO NOTHING on the partial unique index (INV-04).
// Returns the number of events actually enqueued.
func (r *scoreOutboxRepo) EnqueueScoredPosts(ctx context.Context) (int, error) {
	ex := sqlExecerFromContext(ctx, r.data.DB)

	postIDs, err := r.scoredCandidatePostIDs(ctx, ex)
	if err != nil {
		return 0, err
	}

	enqueued := 0
	for _, pid := range postIDs {
		res, insErr := ex.ExecContext(ctx, scoreEventInsertQuery, uuid.Must(uuid.NewV7()), pid)
		if insErr != nil {
			return enqueued, fmt.Errorf("insert rescore event: %w", insErr)
		}
		affected, affErr := res.RowsAffected()
		if affErr != nil {
			return enqueued, fmt.Errorf("rescore rows affected: %w", affErr)
		}
		enqueued += int(affected)
	}
	return enqueued, nil
}

// scoredCandidatePostIDs returns post ids that already have a score and no active
// score_event — the posts eligible for re-score enqueue.
func (r *scoreOutboxRepo) scoredCandidatePostIDs(ctx context.Context, ex sqlExecer) ([]uuid.UUID, error) {
	const candidateQuery = `
SELECT sc.post_id
FROM scores sc
WHERE NOT EXISTS (
    SELECT 1 FROM score_events se
    WHERE se.post_id = sc.post_id AND se.status IN ('pending', 'processing')
)
LIMIT $1
`
	rows, err := ex.QueryContext(ctx, candidateQuery, enqueueScanLimit)
	if err != nil {
		return nil, fmt.Errorf("query scored posts: %w", err)
	}
	defer rows.Close()

	var postIDs []uuid.UUID
	for rows.Next() {
		var pid uuid.UUID
		if scanErr := rows.Scan(&pid); scanErr != nil {
			return nil, fmt.Errorf("scan scored post: %w", scanErr)
		}
		postIDs = append(postIDs, pid)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, fmt.Errorf("rows scored posts: %w", rowsErr)
	}
	return postIDs, nil
}

// dueCandidatePostIDs returns post ids that have a summary, no score, and no
// active score_event — the posts eligible for enqueue.
func (r *scoreOutboxRepo) dueCandidatePostIDs(ctx context.Context, ex sqlExecer) ([]uuid.UUID, error) {
	const candidateQuery = `
SELECT DISTINCT su.post_id
FROM summaries su
WHERE su.post_id IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM scores sc WHERE sc.post_id = su.post_id)
  AND NOT EXISTS (
      SELECT 1 FROM score_events se
      WHERE se.post_id = su.post_id AND se.status IN ('pending', 'processing')
  )
LIMIT $1
`
	rows, err := ex.QueryContext(ctx, candidateQuery, enqueueScanLimit)
	if err != nil {
		return nil, fmt.Errorf("query due posts: %w", err)
	}
	defer rows.Close()

	var postIDs []uuid.UUID
	for rows.Next() {
		var pid uuid.UUID
		if scanErr := rows.Scan(&pid); scanErr != nil {
			return nil, fmt.Errorf("scan due post: %w", scanErr)
		}
		postIDs = append(postIDs, pid)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, fmt.Errorf("rows due posts: %w", rowsErr)
	}
	return postIDs, nil
}

// ClaimOne captures one eligible event under FOR UPDATE SKIP LOCKED.
func (r *scoreOutboxRepo) ClaimOne(
	ctx context.Context,
	workerID string,
	leaseTTL time.Duration,
) (biz.ScoreEvent, error) {
	ex := sqlExecerFromContext(ctx, r.data.DB)

	query := `
WITH claimed AS (
    SELECT id FROM score_events
    WHERE
        (status = 'pending' AND (next_attempt_at IS NULL OR next_attempt_at <= now()))
        OR (status = 'processing' AND locked_until < now())
    ORDER BY COALESCE(next_attempt_at, created_at) ASC
    LIMIT 1
    FOR UPDATE SKIP LOCKED
)
UPDATE score_events
SET
    status = 'processing',
    locked_until = now() + ($1::bigint || ' microseconds')::interval,
    locked_by = $2,
    attempt_count = attempt_count + 1
FROM claimed
WHERE score_events.id = claimed.id
RETURNING ` + scoreEventColumnsQualified

	row := ex.QueryRowContext(ctx, query, leaseTTL.Microseconds(), workerID)
	event, err := scanScoreEvent(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return biz.ScoreEvent{}, biz.ErrNoScoreEventAvailable
		}
		return biz.ScoreEvent{}, fmt.Errorf("claim score event: %w", err)
	}
	return event, nil
}

func (r *scoreOutboxRepo) ExtendLease(
	ctx context.Context,
	eventID, workerID string,
	leaseTTL time.Duration,
) error {
	ex := sqlExecerFromContext(ctx, r.data.DB)

	eid, err := uuid.Parse(eventID)
	if err != nil {
		return fmt.Errorf("invalid event id: %w", err)
	}

	query := `
UPDATE score_events
SET locked_until = now() + ($1::bigint || ' microseconds')::interval
WHERE id = $2 AND locked_by = $3 AND status = 'processing' AND locked_until > now()
`
	res, err := ex.ExecContext(ctx, query, leaseTTL.Microseconds(), eid, workerID)
	if err != nil {
		return fmt.Errorf("extend lease: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("extend lease rows affected: %w", err)
	}
	if affected == 0 {
		return biz.ErrLeaseLost
	}
	return nil
}

func (r *scoreOutboxRepo) FinalizeWithLease(
	ctx context.Context,
	eventID, workerID string,
	status biz.ScoreEventStatus,
	errText *string,
) error {
	ex := sqlExecerFromContext(ctx, r.data.DB)

	eid, err := uuid.Parse(eventID)
	if err != nil {
		return fmt.Errorf("invalid event id: %w", err)
	}

	var errorArg any
	if errText != nil {
		errorArg = *errText
	}

	query := `
UPDATE score_events
SET
    status = $1,
    error = COALESCE($2::text, error),
    processed_at = now(),
    locked_until = NULL,
    locked_by = NULL
WHERE id = $3 AND locked_by = $4 AND status = 'processing' AND locked_until > now()
`
	res, err := ex.ExecContext(ctx, query, string(status), errorArg, eid, workerID)
	if err != nil {
		return fmt.Errorf("finalize with lease: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("finalize rows affected: %w", err)
	}
	if affected == 0 {
		return biz.ErrLeaseLost
	}
	return nil
}

func (r *scoreOutboxRepo) MarkForRetry(
	ctx context.Context,
	eventID, workerID string,
	retryAt time.Time,
	errText string,
) error {
	ex := sqlExecerFromContext(ctx, r.data.DB)

	eid, err := uuid.Parse(eventID)
	if err != nil {
		return fmt.Errorf("invalid event id: %w", err)
	}

	query := `
UPDATE score_events
SET
    status = 'pending',
    next_attempt_at = $1,
    error = $2,
    locked_until = NULL,
    locked_by = NULL
WHERE id = $3 AND locked_by = $4 AND status = 'processing' AND locked_until > now()
`
	res, err := ex.ExecContext(ctx, query, retryAt, errText, eid, workerID)
	if err != nil {
		return fmt.Errorf("mark for retry: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("mark for retry rows affected: %w", err)
	}
	if affected == 0 {
		return biz.ErrLeaseLost
	}
	return nil
}

func (r *scoreOutboxRepo) FailExpired(
	ctx context.Context,
	eventID string,
	maxAttempts int,
	grace time.Duration,
	errText string,
) (bool, error) {
	ex := sqlExecerFromContext(ctx, r.data.DB)

	eid, err := uuid.Parse(eventID)
	if err != nil {
		return false, fmt.Errorf("invalid event id: %w", err)
	}

	query := `
UPDATE score_events
SET
    status = 'failed',
    error = $1,
    processed_at = now(),
    locked_until = NULL,
    locked_by = NULL
WHERE id = $2
  AND status = 'processing'
  AND attempt_count >= $3
  AND locked_until < now() - ($4::bigint || ' microseconds')::interval
`
	res, err := ex.ExecContext(ctx, query, errText, eid, maxAttempts, grace.Microseconds())
	if err != nil {
		return false, fmt.Errorf("fail expired: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("fail expired rows affected: %w", err)
	}
	return affected > 0, nil
}

func (r *scoreOutboxRepo) ListLeaseExpired(
	ctx context.Context,
	grace time.Duration,
	limit int,
) ([]biz.ScoreEvent, error) {
	ex := sqlExecerFromContext(ctx, r.data.DB)

	query := `
SELECT ` + scoreEventColumns + `
FROM score_events
WHERE status = 'processing'
  AND locked_until < now() - ($1::bigint || ' microseconds')::interval
ORDER BY locked_until ASC
LIMIT $2
`
	rows, err := ex.QueryContext(ctx, query, grace.Microseconds(), limit)
	if err != nil {
		return nil, fmt.Errorf("list lease expired: %w", err)
	}
	defer rows.Close()

	var events []biz.ScoreEvent
	for rows.Next() {
		ev, scanErr := scanScoreEvent(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan lease expired: %w", scanErr)
		}
		events = append(events, ev)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, fmt.Errorf("rows lease expired: %w", rowsErr)
	}
	return events, nil
}

func scanScoreEvent(row rowScanner) (biz.ScoreEvent, error) {
	var (
		id           uuid.UUID
		postID       uuid.UUID
		status       string
		errText      sql.NullString
		attemptCount int
		createdAt    time.Time
		processedAt  sql.NullTime
	)
	if err := row.Scan(&id, &postID, &status, &errText, &attemptCount, &createdAt, &processedAt); err != nil {
		return biz.ScoreEvent{}, err
	}

	ev := biz.ScoreEvent{
		ID:           id.String(),
		PostID:       postID.String(),
		Status:       biz.ScoreEventStatus(status),
		AttemptCount: attemptCount,
		CreatedAt:    createdAt,
	}
	if errText.Valid {
		s := errText.String
		ev.Error = &s
	}
	if processedAt.Valid {
		t := processedAt.Time
		ev.ProcessedAt = &t
	}
	return ev, nil
}
