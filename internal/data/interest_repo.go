package data

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/4itosik/feedium/internal/biz"
	entgo "github.com/4itosik/feedium/internal/ent"
	"github.com/4itosik/feedium/internal/ent/interest"
)

type interestRepo struct {
	data *Data
}

var _ biz.InterestRepo = (*interestRepo)(nil)

// NewInterestRepo creates a new interest repository.
//
//nolint:revive // unexported return type for wire DI
func NewInterestRepo(data *Data) *interestRepo {
	return &interestRepo{data: data}
}

func (r *interestRepo) Save(ctx context.Context, in biz.Interest) (biz.Interest, error) {
	id, err := uuid.Parse(in.ID)
	if err != nil {
		return biz.Interest{}, fmt.Errorf("invalid interest id: %w", err)
	}

	entInterest, err := r.data.Ent.Interest.Create().
		SetID(id).
		SetTopic(in.Topic).
		SetEmbedding(in.Embedding).
		SetModel(in.Model).
		SetCreatedAt(in.CreatedAt).
		SetUpdatedAt(in.UpdatedAt).
		Save(ctx)
	if err != nil {
		return biz.Interest{}, fmt.Errorf("save interest: %w", err)
	}
	return mapInterestToDomain(entInterest), nil
}

func (r *interestRepo) Update(ctx context.Context, in biz.Interest) (biz.Interest, error) {
	id, err := uuid.Parse(in.ID)
	if err != nil {
		return biz.Interest{}, fmt.Errorf("invalid interest id: %w", err)
	}

	entInterest, err := r.data.Ent.Interest.UpdateOneID(id).
		SetTopic(in.Topic).
		SetEmbedding(in.Embedding).
		SetModel(in.Model).
		SetUpdatedAt(in.UpdatedAt).
		Save(ctx)
	if err != nil {
		if entgo.IsNotFound(err) {
			return biz.Interest{}, biz.ErrInterestNotFound
		}
		return biz.Interest{}, fmt.Errorf("update interest: %w", err)
	}
	return mapInterestToDomain(entInterest), nil
}

func (r *interestRepo) Delete(ctx context.Context, id string) error {
	uid, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid interest id: %w", err)
	}

	if delErr := r.data.Ent.Interest.DeleteOneID(uid).Exec(ctx); delErr != nil {
		if entgo.IsNotFound(delErr) {
			return biz.ErrInterestNotFound
		}
		return fmt.Errorf("delete interest: %w", delErr)
	}
	return nil
}

func (r *interestRepo) Get(ctx context.Context, id string) (biz.Interest, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return biz.Interest{}, fmt.Errorf("invalid interest id: %w", err)
	}

	entInterest, err := r.data.Ent.Interest.Get(ctx, uid)
	if err != nil {
		if entgo.IsNotFound(err) {
			return biz.Interest{}, biz.ErrInterestNotFound
		}
		return biz.Interest{}, fmt.Errorf("get interest: %w", err)
	}
	return mapInterestToDomain(entInterest), nil
}

func (r *interestRepo) List(ctx context.Context, filter biz.ListInterestsFilter) (biz.ListInterestsResult, error) {
	query := r.data.Ent.Interest.Query().
		Order(entgo.Asc(interest.FieldCreatedAt), entgo.Asc(interest.FieldID))

	if filter.PageToken != "" {
		decoded, err := decodePageToken(filter.PageToken)
		if err != nil {
			return biz.ListInterestsResult{}, fmt.Errorf("%w: invalid page token", biz.ErrInterestValidation)
		}
		query = query.Where(
			interest.Or(
				interest.CreatedAtGT(decoded.SortValue),
				interest.And(
					interest.CreatedAtEQ(decoded.SortValue),
					interest.IDGT(decoded.ID),
				),
			),
		)
	}

	entInterests, err := query.Limit(filter.PageSize + 1).All(ctx)
	if err != nil {
		return biz.ListInterestsResult{}, fmt.Errorf("list interests: %w", err)
	}

	result := biz.ListInterestsResult{Items: []biz.Interest{}}
	returnCount := min(len(entInterests), filter.PageSize)
	for i := range returnCount {
		result.Items = append(result.Items, mapInterestToDomain(entInterests[i]))
	}
	if len(entInterests) > filter.PageSize {
		last := entInterests[returnCount-1]
		token, tokenErr := encodePageToken(last.CreatedAt, last.ID)
		if tokenErr != nil {
			return biz.ListInterestsResult{}, fmt.Errorf("encode page token: %w", tokenErr)
		}
		result.NextPageToken = token
	}
	return result, nil
}

// ListEmbeddings returns the cached embedding of every interest (DEC-04).
func (r *interestRepo) ListEmbeddings(ctx context.Context) ([][]float32, error) {
	entInterests, err := r.data.Ent.Interest.Query().All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list interest embeddings: %w", err)
	}
	embeddings := make([][]float32, 0, len(entInterests))
	for _, ei := range entInterests {
		embeddings = append(embeddings, ei.Embedding)
	}
	return embeddings, nil
}

// ListInterestVectors returns each interest embedding tagged with its model (INV-01).
func (r *interestRepo) ListInterestVectors(ctx context.Context) ([]biz.InterestVector, error) {
	entInterests, err := r.data.Ent.Interest.Query().All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list interest vectors: %w", err)
	}
	vectors := make([]biz.InterestVector, 0, len(entInterests))
	for _, ei := range entInterests {
		vectors = append(vectors, biz.InterestVector{Embedding: ei.Embedding, Model: ei.Model})
	}
	return vectors, nil
}

func mapInterestToDomain(ei *entgo.Interest) biz.Interest {
	return biz.Interest{
		ID:        ei.ID.String(),
		Topic:     ei.Topic,
		Embedding: ei.Embedding,
		Model:     ei.Model,
		CreatedAt: ei.CreatedAt,
		UpdatedAt: ei.UpdatedAt,
	}
}
