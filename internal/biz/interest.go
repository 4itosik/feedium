package biz

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

//go:generate mockgen -source=interest.go -destination=mock/interest_mock.go -package=mock

// Interest is an instance-global topic of interest (FT-010 CTR-12). Its free-text
// topic is encoded once into a cached embedding used by scoring (DEC-04).
type Interest struct {
	ID        string
	Topic     string
	Embedding []float32
	Model     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ListInterestsFilter defines pagination for listing interests.
type ListInterestsFilter struct {
	PageSize  int
	PageToken string
}

// ListInterestsResult holds a page of interests.
type ListInterestsResult struct {
	Items         []Interest
	NextPageToken string
}

// Sentinel errors for Interest operations.
var (
	ErrInterestNotFound   = errors.New("interest not found")
	ErrInterestValidation = errors.New("interest validation failed")
)

// ValidateInterestTopic rejects an empty topic (REQ-01).
func ValidateInterestTopic(topic string) error {
	if strings.TrimSpace(topic) == "" {
		return fmt.Errorf("%w: topic is empty", ErrInterestValidation)
	}
	return nil
}

// Scorer abstracts the external embeddings provider (CTR-14). It is the biz-side
// port; the implementation lives in data/. Model identifies the embedding model
// so a Score/Interest records which model produced its vector.
type Scorer interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	Model() string
}

// InterestVector is an interest embedding tagged with the model that produced it,
// so scoring never cosines vectors across models (FT-013 INV-01).
type InterestVector struct {
	Embedding []float32
	Model     string
}

// InterestRepo persists interests and exposes their cached embeddings to scoring.
type InterestRepo interface {
	Save(ctx context.Context, interest Interest) (Interest, error)
	Update(ctx context.Context, interest Interest) (Interest, error)
	Delete(ctx context.Context, id string) error
	Get(ctx context.Context, id string) (Interest, error)
	List(ctx context.Context, filter ListInterestsFilter) (ListInterestsResult, error)
	// ListInterestVectors returns every interest embedding with its model (INV-01).
	ListInterestVectors(ctx context.Context) ([]InterestVector, error)
}

// InterestUsecase handles business logic for Interest operations.
type InterestUsecase struct {
	repo   InterestRepo
	scorer Scorer
	outbox ScoreOutboxRepo
	tx     TxManager
}

// NewInterestUsecase creates a new InterestUsecase.
func NewInterestUsecase(repo InterestRepo, scorer Scorer, outbox ScoreOutboxRepo, tx TxManager) *InterestUsecase {
	return &InterestUsecase{repo: repo, scorer: scorer, outbox: outbox, tx: tx}
}

// Create validates the topic, encodes it to an embedding and persists it. The
// paid Embed call happens outside the transaction; the persist and the re-score
// enqueue commit atomically so an interest change always triggers re-scoring of
// existing posts (REQ-04, CTR-03).
func (uc *InterestUsecase) Create(ctx context.Context, topic string) (Interest, error) {
	if err := ValidateInterestTopic(topic); err != nil {
		return Interest{}, err
	}

	embedding, err := uc.scorer.Embed(ctx, topic)
	if err != nil {
		return Interest{}, fmt.Errorf("embed topic: %w", err)
	}

	now := time.Now()
	interest := Interest{
		ID:        uuid.Must(uuid.NewV7()).String(),
		Topic:     topic,
		Embedding: embedding,
		Model:     uc.scorer.Model(),
		CreatedAt: now,
		UpdatedAt: now,
	}

	var saved Interest
	err = uc.tx.InTx(ctx, func(ctx context.Context) error {
		var e error
		if saved, e = uc.repo.Save(ctx, interest); e != nil {
			return e
		}
		if _, e = uc.outbox.EnqueueScoredPosts(ctx); e != nil {
			return fmt.Errorf("enqueue rescore: %w", e)
		}
		return nil
	})
	if err != nil {
		return Interest{}, err
	}
	return saved, nil
}

// Update re-encodes the topic and updates the stored interest in place. The paid
// Embed call happens outside the transaction; the update and the re-score enqueue
// commit atomically (REQ-04, CTR-03).
func (uc *InterestUsecase) Update(ctx context.Context, id, topic string) (Interest, error) {
	if err := ValidateInterestTopic(topic); err != nil {
		return Interest{}, err
	}

	existing, err := uc.repo.Get(ctx, id)
	if err != nil {
		return Interest{}, err
	}

	embedding, err := uc.scorer.Embed(ctx, topic)
	if err != nil {
		return Interest{}, fmt.Errorf("embed topic: %w", err)
	}

	existing.Topic = topic
	existing.Embedding = embedding
	existing.Model = uc.scorer.Model()
	existing.UpdatedAt = time.Now()

	var saved Interest
	err = uc.tx.InTx(ctx, func(ctx context.Context) error {
		var e error
		if saved, e = uc.repo.Update(ctx, existing); e != nil {
			return e
		}
		if _, e = uc.outbox.EnqueueScoredPosts(ctx); e != nil {
			return fmt.Errorf("enqueue rescore: %w", e)
		}
		return nil
	})
	if err != nil {
		return Interest{}, err
	}
	return saved, nil
}

// Delete removes an interest by ID. The delete and the re-score enqueue commit
// atomically so existing posts are re-scored against the remaining interests
// (REQ-04, CTR-03).
func (uc *InterestUsecase) Delete(ctx context.Context, id string) error {
	return uc.tx.InTx(ctx, func(ctx context.Context) error {
		if err := uc.repo.Delete(ctx, id); err != nil {
			return err
		}
		if _, err := uc.outbox.EnqueueScoredPosts(ctx); err != nil {
			return fmt.Errorf("enqueue rescore: %w", err)
		}
		return nil
	})
}

// Get retrieves an interest by ID.
func (uc *InterestUsecase) Get(ctx context.Context, id string) (Interest, error) {
	return uc.repo.Get(ctx, id)
}

// List retrieves interests with clamped pagination.
func (uc *InterestUsecase) List(ctx context.Context, filter ListInterestsFilter) (ListInterestsResult, error) {
	if filter.PageSize < minPageSize {
		filter.PageSize = minPageSize
	}
	if filter.PageSize > maxPageSize {
		filter.PageSize = maxPageSize
	}
	return uc.repo.List(ctx, filter)
}
