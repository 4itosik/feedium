package data

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/4itosik/feedium/internal/biz"
	entgo "github.com/4itosik/feedium/internal/ent"
	"github.com/4itosik/feedium/internal/ent/summary"
)

type summaryRepo struct {
	data *Data
}

var _ biz.SummaryRepo = (*summaryRepo)(nil)

func NewSummaryRepo(data *Data) *summaryRepo { //nolint:revive // unexported return type for wire DI
	return &summaryRepo{data: data}
}

func (r *summaryRepo) Save(ctx context.Context, s biz.Summary) (biz.Summary, error) {
	client := clientFromContext(ctx, r.data.Ent)

	id, err := uuid.Parse(s.ID)
	if err != nil {
		return biz.Summary{}, fmt.Errorf("invalid summary id: %w", err)
	}
	sourceID, err := uuid.Parse(s.SourceID)
	if err != nil {
		return biz.Summary{}, fmt.Errorf("invalid source id: %w", err)
	}

	builder := client.Summary.Create().
		SetID(id).
		SetSourceID(sourceID).
		SetText(s.Text).
		SetWordCount(s.WordCount).
		SetCreatedAt(s.CreatedAt)

	if s.PostID != nil {
		postID, parseErr := uuid.Parse(*s.PostID)
		if parseErr != nil {
			return biz.Summary{}, fmt.Errorf("invalid post id: %w", parseErr)
		}
		builder.SetPostID(postID)
	}

	entSummary, err := builder.Save(ctx)
	if err != nil {
		return biz.Summary{}, fmt.Errorf("save summary: %w", err)
	}

	return r.mapEntToDomain(entSummary), nil
}

func (r *summaryRepo) Get(ctx context.Context, id string) (biz.Summary, error) {
	client := clientFromContext(ctx, r.data.Ent)

	uid, err := uuid.Parse(id)
	if err != nil {
		return biz.Summary{}, fmt.Errorf("invalid summary id: %w", err)
	}

	entSummary, err := client.Summary.Get(ctx, uid)
	if err != nil {
		if entgo.IsNotFound(err) {
			return biz.Summary{}, biz.ErrSummaryNotFound
		}
		return biz.Summary{}, fmt.Errorf("get summary: %w", err)
	}

	return r.mapEntToDomain(entSummary), nil
}

func (r *summaryRepo) ListByPost(ctx context.Context, postID string) ([]biz.Summary, error) {
	client := clientFromContext(ctx, r.data.Ent)

	uid, err := uuid.Parse(postID)
	if err != nil {
		return nil, fmt.Errorf("invalid post id: %w", err)
	}

	entSummaries, err := client.Summary.Query().
		Where(summary.PostIDEQ(uid)).
		Order(entgo.Desc(summary.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list summaries by post: %w", err)
	}

	result := make([]biz.Summary, 0, len(entSummaries))
	for _, es := range entSummaries {
		result = append(result, r.mapEntToDomain(es))
	}
	return result, nil
}

func (r *summaryRepo) ListBySource(
	ctx context.Context,
	sourceID string,
	pageSize int,
	pageToken string,
) (biz.ListSummariesResult, error) {
	client := clientFromContext(ctx, r.data.Ent)

	uid, err := uuid.Parse(sourceID)
	if err != nil {
		return biz.ListSummariesResult{}, fmt.Errorf("invalid source id: %w", err)
	}

	query := client.Summary.Query().
		Where(summary.SourceIDEQ(uid)).
		Order(entgo.Desc(summary.FieldCreatedAt), entgo.Desc(summary.FieldID))

	if pageToken != "" {
		decoded, decodeErr := decodePageToken(pageToken)
		if decodeErr != nil {
			return biz.ListSummariesResult{}, fmt.Errorf("invalid page token: %w", decodeErr)
		}
		query = query.Where(
			summary.Or(
				summary.CreatedAtLT(decoded.SortValue),
				summary.And(
					summary.CreatedAtEQ(decoded.SortValue),
					summary.IDLT(decoded.ID),
				),
			),
		)
	}

	entSummaries, err := query.Limit(pageSize + 1).All(ctx)
	if err != nil {
		return biz.ListSummariesResult{}, fmt.Errorf("list summaries by source: %w", err)
	}

	result := biz.ListSummariesResult{
		Items:         []biz.Summary{},
		NextPageToken: "",
	}

	returnCount := min(len(entSummaries), pageSize)
	for i := range returnCount {
		result.Items = append(result.Items, r.mapEntToDomain(entSummaries[i]))
	}

	if len(entSummaries) > pageSize {
		last := entSummaries[returnCount-1]
		nextToken, _ := encodePageToken(last.CreatedAt, last.ID)
		result.NextPageToken = nextToken
	}

	return result, nil
}

func (r *summaryRepo) GetLastBySource(ctx context.Context, sourceID string) (*biz.Summary, error) {
	client := clientFromContext(ctx, r.data.Ent)

	uid, err := uuid.Parse(sourceID)
	if err != nil {
		return nil, fmt.Errorf("invalid source id: %w", err)
	}

	entSummary, err := client.Summary.Query().
		Where(summary.SourceIDEQ(uid)).
		Order(entgo.Desc(summary.FieldCreatedAt)).
		First(ctx)
	if err != nil {
		if entgo.IsNotFound(err) {
			return nil, nil //nolint:nilnil // not found is a valid empty result
		}
		return nil, fmt.Errorf("get last summary by source: %w", err)
	}

	s := r.mapEntToDomain(entSummary)
	return &s, nil
}

func (r *summaryRepo) mapEntToDomain(es *entgo.Summary) biz.Summary {
	var postID *string
	if es.PostID != nil {
		pid := es.PostID.String()
		postID = &pid
	}

	return biz.Summary{
		ID:        es.ID.String(),
		PostID:    postID,
		SourceID:  es.SourceID.String(),
		Text:      es.Text,
		WordCount: es.WordCount,
		CreatedAt: es.CreatedAt,
	}
}
