package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/4itosik/feedium/internal/biz"
	entgo "github.com/4itosik/feedium/internal/ent"
	"github.com/4itosik/feedium/internal/ent/summaryevent"
)

// claim-time SQL fragments. Columns kept in sync with migrations
// 20260415100000_create_summaries_and_events.sql and 20260423100000_summary_events_lease.sql.
const summaryEventColumns = `
    id, post_id, source_id, event_type, status, summary_id, error,
    created_at, processed_at, attempt_count
`

const summaryEventColumnsQualified = `
    summary_events.id, summary_events.post_id, summary_events.source_id,
    summary_events.event_type, summary_events.status, summary_events.summary_id,
    summary_events.error, summary_events.created_at, summary_events.processed_at,
    summary_events.attempt_count
`

type summaryOutboxRepo struct {
	data *Data
}

var _ biz.SummaryOutboxRepo = (*summaryOutboxRepo)(nil)

func NewSummaryOutboxRepo(data *Data) *summaryOutboxRepo { //nolint:revive // unexported return type for wire DI
	return &summaryOutboxRepo{data: data}
}

func (r *summaryOutboxRepo) Save(ctx context.Context, event biz.SummaryEvent) (biz.SummaryEvent, error) {
	client := clientFromContext(ctx, r.data.Ent)

	id, err := uuid.Parse(event.ID)
	if err != nil {
		return biz.SummaryEvent{}, fmt.Errorf("invalid event id: %w", err)
	}
	sourceID, err := uuid.Parse(event.SourceID)
	if err != nil {
		return biz.SummaryEvent{}, fmt.Errorf("invalid source id: %w", err)
	}

	builder := client.SummaryEvent.Create().
		SetID(id).
		SetSourceID(sourceID).
		SetEventType(string(event.EventType)).
		SetStatus(string(event.Status)).
		SetCreatedAt(event.CreatedAt)

	if event.PostID != nil {
		postID, parseErr := uuid.Parse(*event.PostID)
		if parseErr != nil {
			return biz.SummaryEvent{}, fmt.Errorf("invalid post id: %w", parseErr)
		}
		builder.SetPostID(postID)
	}

	entEvent, err := builder.Save(ctx)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			return biz.SummaryEvent{}, biz.ErrSummaryAlreadyProcessing
		}
		return biz.SummaryEvent{}, fmt.Errorf("save summary event: %w", err)
	}

	return r.mapEntToDomain(entEvent), nil
}

func (r *summaryOutboxRepo) Get(ctx context.Context, id string) (biz.SummaryEvent, error) {
	client := clientFromContext(ctx, r.data.Ent)

	uid, err := uuid.Parse(id)
	if err != nil {
		return biz.SummaryEvent{}, fmt.Errorf("invalid event id: %w", err)
	}

	entEvent, err := client.SummaryEvent.Get(ctx, uid)
	if err != nil {
		if entgo.IsNotFound(err) {
			return biz.SummaryEvent{}, biz.ErrSummaryEventNotFound
		}
		return biz.SummaryEvent{}, fmt.Errorf("get summary event: %w", err)
	}

	return r.mapEntToDomain(entEvent), nil
}

// ClaimOne captures one eligible event (pending+due, or processing with expired lease)
// under FOR UPDATE SKIP LOCKED, flipping it to processing under the caller's lease.
func (r *summaryOutboxRepo) ClaimOne(
	ctx context.Context,
	workerID string,
	leaseTTL time.Duration,
) (biz.SummaryEvent, error) {
	ex := sqlExecerFromContext(ctx, r.data.DB)

	query := `
WITH claimed AS (
    SELECT id FROM summary_events
    WHERE
        (status = 'pending' AND (next_attempt_at IS NULL OR next_attempt_at <= now()))
        OR (status = 'processing' AND locked_until < now())
    ORDER BY COALESCE(next_attempt_at, created_at) ASC
    LIMIT 1
    FOR UPDATE SKIP LOCKED
)
UPDATE summary_events
SET
    status = 'processing',
    locked_until = now() + ($1::bigint || ' microseconds')::interval,
    locked_by = $2,
    attempt_count = attempt_count + 1
FROM claimed
WHERE summary_events.id = claimed.id
RETURNING ` + summaryEventColumnsQualified

	row := ex.QueryRowContext(ctx, query, leaseTTL.Microseconds(), workerID)

	event, err := scanSummaryEvent(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return biz.SummaryEvent{}, biz.ErrNoEventAvailable
		}
		return biz.SummaryEvent{}, fmt.Errorf("claim event: %w", err)
	}
	return event, nil
}

// ExtendLease refreshes locked_until for a live lease owned by workerID.
// If the lease has already expired or been reassigned, returns ErrLeaseLost.
func (r *summaryOutboxRepo) ExtendLease(
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
UPDATE summary_events
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

// FinalizeWithLease writes a terminal status only if the worker still owns the lease.
func (r *summaryOutboxRepo) FinalizeWithLease(
	ctx context.Context,
	eventID, workerID string,
	status biz.SummaryEventStatus,
	summaryID *string,
	errText *string,
) error {
	ex := sqlExecerFromContext(ctx, r.data.DB)

	eid, err := uuid.Parse(eventID)
	if err != nil {
		return fmt.Errorf("invalid event id: %w", err)
	}

	var summaryArg any
	if summaryID != nil {
		sid, parseErr := uuid.Parse(*summaryID)
		if parseErr != nil {
			return fmt.Errorf("invalid summary id: %w", parseErr)
		}
		summaryArg = sid
	}

	var errorArg any
	if errText != nil {
		errorArg = *errText
	}

	query := `
UPDATE summary_events
SET
    status = $1,
    summary_id = COALESCE($2::uuid, summary_id),
    error = COALESCE($3::text, error),
    processed_at = now(),
    locked_until = NULL,
    locked_by = NULL
WHERE id = $4 AND locked_by = $5 AND status = 'processing' AND locked_until > now()
`
	res, err := ex.ExecContext(ctx, query, string(status), summaryArg, errorArg, eid, workerID)
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

func (r *summaryOutboxRepo) UpdateStatus(
	ctx context.Context,
	id string,
	status biz.SummaryEventStatus,
	summaryID *string,
	errText *string,
) error {
	client := clientFromContext(ctx, r.data.Ent)

	uid, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid event id: %w", err)
	}

	builder := client.SummaryEvent.UpdateOneID(uid).
		SetStatus(string(status))

	if summaryID != nil {
		sid, parseErr := uuid.Parse(*summaryID)
		if parseErr != nil {
			return fmt.Errorf("invalid summary id: %w", parseErr)
		}
		builder.SetSummaryID(sid)
	}

	if errText != nil {
		builder.SetError(*errText)
	}

	if status == biz.SummaryEventStatusCompleted || status == biz.SummaryEventStatusFailed ||
		status == biz.SummaryEventStatusExpired {
		now := time.Now()
		builder.SetProcessedAt(now)
		builder.ClearLockedUntil()
		builder.ClearLockedBy()
	}

	_, err = builder.Save(ctx)
	if err != nil {
		if entgo.IsNotFound(err) {
			return biz.ErrSummaryEventNotFound
		}
		return fmt.Errorf("update summary event status: %w", err)
	}

	return nil
}

// MarkForRetry resets a processing event back to pending for another attempt.
func (r *summaryOutboxRepo) MarkForRetry(
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
UPDATE summary_events
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

// ListLeaseExpired returns processing events whose lease has been expired for
// at least `grace`. Used by the reaper to detect stuck leases.
func (r *summaryOutboxRepo) ListLeaseExpired(
	ctx context.Context,
	grace time.Duration,
	limit int,
) ([]biz.SummaryEvent, error) {
	ex := sqlExecerFromContext(ctx, r.data.DB)

	query := `
SELECT ` + summaryEventColumns + `
FROM summary_events
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

	var events []biz.SummaryEvent
	for rows.Next() {
		ev, scanErr := scanSummaryEvent(rows)
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

func (r *summaryOutboxRepo) HasActiveEvent(
	ctx context.Context,
	sourceID string,
	eventType biz.SummaryEventType,
) (bool, *biz.SummaryEvent, error) {
	client := clientFromContext(ctx, r.data.Ent)

	uid, err := uuid.Parse(sourceID)
	if err != nil {
		return false, nil, fmt.Errorf("invalid source id: %w", err)
	}

	entEvent, err := client.SummaryEvent.Query().
		Where(
			summaryevent.SourceIDEQ(uid),
			summaryevent.EventTypeEQ(string(eventType)),
			summaryevent.StatusIn(string(biz.SummaryEventStatusPending), string(biz.SummaryEventStatusProcessing)),
		).
		Only(ctx)
	if err != nil {
		if entgo.IsNotFound(err) {
			return false, nil, nil
		}
		return false, nil, fmt.Errorf("check active event: %w", err)
	}

	event := r.mapEntToDomain(entEvent)
	return true, &event, nil
}

func (r *summaryOutboxRepo) mapEntToDomain(ee *entgo.SummaryEvent) biz.SummaryEvent {
	var postID *string
	if ee.PostID != nil {
		pid := ee.PostID.String()
		postID = &pid
	}
	var summaryID *string
	if ee.SummaryID != nil {
		sid := ee.SummaryID.String()
		summaryID = &sid
	}
	var errText *string
	if ee.Error != nil {
		errText = ee.Error
	}
	var processedAt *time.Time
	if ee.ProcessedAt != nil {
		processedAt = ee.ProcessedAt
	}

	return biz.SummaryEvent{
		ID:           ee.ID.String(),
		PostID:       postID,
		SourceID:     ee.SourceID.String(),
		EventType:    biz.SummaryEventType(ee.EventType),
		Status:       biz.SummaryEventStatus(ee.Status),
		SummaryID:    summaryID,
		Error:        errText,
		CreatedAt:    ee.CreatedAt,
		ProcessedAt:  processedAt,
		AttemptCount: ee.AttemptCount,
	}
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanSummaryEvent(row rowScanner) (biz.SummaryEvent, error) {
	var (
		id           uuid.UUID
		postID       uuid.NullUUID
		sourceID     uuid.UUID
		eventType    string
		status       string
		summaryID    uuid.NullUUID
		errText      sql.NullString
		createdAt    time.Time
		processedAt  sql.NullTime
		attemptCount int
	)

	if err := row.Scan(
		&id, &postID, &sourceID, &eventType, &status, &summaryID, &errText,
		&createdAt, &processedAt, &attemptCount,
	); err != nil {
		return biz.SummaryEvent{}, err
	}

	ev := biz.SummaryEvent{
		ID:        id.String(),
		SourceID:  sourceID.String(),
		EventType: biz.SummaryEventType(eventType),
		Status:    biz.SummaryEventStatus(status),
		CreatedAt: createdAt,
	}
	if postID.Valid {
		s := postID.UUID.String()
		ev.PostID = &s
	}
	if summaryID.Valid {
		s := summaryID.UUID.String()
		ev.SummaryID = &s
	}
	if errText.Valid {
		s := errText.String
		ev.Error = &s
	}
	if processedAt.Valid {
		t := processedAt.Time
		ev.ProcessedAt = &t
	}
	ev.AttemptCount = attemptCount

	return ev, nil
}
