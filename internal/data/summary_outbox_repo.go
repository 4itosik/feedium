package data

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/4itosik/feedium/internal/biz"
	entgo "github.com/4itosik/feedium/internal/ent"
	"github.com/4itosik/feedium/internal/ent/summaryevent"
)

type summaryOutboxRepo struct {
	data *Data
}

var _ biz.SummaryOutboxRepo = (*summaryOutboxRepo)(nil)

func NewSummaryOutboxRepo(data *Data) *summaryOutboxRepo {
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

func (r *summaryOutboxRepo) ListPending(ctx context.Context, limit int) ([]biz.SummaryEvent, error) {
	client := clientFromContext(ctx, r.data.Ent)

	entEvents, err := client.SummaryEvent.Query().
		Where(summaryevent.StatusEQ(string(biz.SummaryEventStatusPending))).
		Order(entgo.Asc(summaryevent.FieldCreatedAt)).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list pending events: %w", err)
	}

	result := make([]biz.SummaryEvent, 0, len(entEvents))
	for _, ee := range entEvents {
		result = append(result, r.mapEntToDomain(ee))
	}
	return result, nil
}

func (r *summaryOutboxRepo) UpdateStatus(ctx context.Context, id string, status biz.SummaryEventStatus, summaryID *string, errText *string) error {
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

	if status == biz.SummaryEventStatusCompleted || status == biz.SummaryEventStatusFailed || status == biz.SummaryEventStatusExpired {
		now := time.Now()
		builder.SetProcessedAt(now)
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

func (r *summaryOutboxRepo) HasActiveEvent(ctx context.Context, sourceID string, eventType biz.SummaryEventType) (bool, *biz.SummaryEvent, error) {
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
		ID:          ee.ID.String(),
		PostID:      postID,
		SourceID:    ee.SourceID.String(),
		EventType:   biz.SummaryEventType(ee.EventType),
		Status:      biz.SummaryEventStatus(ee.Status),
		SummaryID:   summaryID,
		Error:       errText,
		CreatedAt:   ee.CreatedAt,
		ProcessedAt: processedAt,
	}
}
