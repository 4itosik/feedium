package summary_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	"feedium/internal/app/post"
	"feedium/internal/app/source"
	"feedium/internal/app/summary"
	"feedium/internal/app/summary/mocks"
)

func TestWorkerProcessNext_SelfContainedHappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	eventID := uuid.New()
	sourceID := uuid.New()
	postID := uuid.New()

	// Setup mocks
	outboxRepo := mocks.NewMockOutboxEventRepository(ctrl)
	summaryRepo := mocks.NewMockRepository(ctrl)
	postRepo := mocks.NewMockPostQueryRepository(ctrl)
	sourceRepo := mocks.NewMockSourceQueryRepository(ctrl)
	processor := mocks.NewMockProcessor(ctrl)

	event := &summary.OutboxEvent{
		ID:       eventID,
		SourceID: sourceID,
		PostID:   &postID,
		Status:   summary.EventStatusPending,
	}

	src := &source.Source{
		ID:   sourceID,
		Type: source.TypeRSS,
	}

	p := &post.Post{
		ID: postID,
	}

	// Expectations
	outboxRepo.EXPECT().
		FetchAndLockPending(ctx).
		Return(event, time.Now(), nil).
		Times(1)

	sourceRepo.EXPECT().
		GetByID(ctx, sourceID).
		Return(src, nil).
		Times(1)

	postRepo.EXPECT().
		GetByID(ctx, postID).
		Return(p, nil).
		Times(1)

	processor.EXPECT().
		Process(gomock.Any(), gomock.Any(), []post.Post{*p}).
		Return("test summary content", nil).
		Times(1)

	summaryRepo.EXPECT().
		Create(ctx, gomock.Any(), []uuid.UUID{postID}).
		Return(nil).
		Times(1)

	outboxRepo.EXPECT().
		UpdateStatus(ctx, eventID, summary.EventStatusCompleted, false).
		Return(nil).
		Times(1)

	// Create worker and run
	worker := summary.NewWorker(outboxRepo, summaryRepo, postRepo, sourceRepo, processor, nil)
	processed, err := worker.ProcessNext(ctx)

	if err != nil {
		t.Fatalf("ProcessNext failed: %v", err)
	}
	if !processed {
		t.Fatal("ProcessNext should return true when event is processed")
	}
}

func TestWorkerProcessNext_CumulativeHappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	eventID := uuid.New()
	sourceID := uuid.New()
	postID1 := uuid.New()
	postID2 := uuid.New()
	postID3 := uuid.New()

	// Setup mocks
	outboxRepo := mocks.NewMockOutboxEventRepository(ctrl)
	summaryRepo := mocks.NewMockRepository(ctrl)
	postRepo := mocks.NewMockPostQueryRepository(ctrl)
	sourceRepo := mocks.NewMockSourceQueryRepository(ctrl)
	processor := mocks.NewMockProcessor(ctrl)

	event := &summary.OutboxEvent{
		ID:       eventID,
		SourceID: sourceID,
		Status:   summary.EventStatusPending,
	}

	src := &source.Source{
		ID:   sourceID,
		Type: source.TypeTelegramGroup,
	}

	posts := []post.Post{
		{ID: postID1},
		{ID: postID2},
		{ID: postID3},
	}

	// Expectations
	lockTime := time.Now()
	outboxRepo.EXPECT().
		FetchAndLockPending(ctx).
		Return(event, lockTime, nil).
		Times(1)

	sourceRepo.EXPECT().
		GetByID(ctx, sourceID).
		Return(src, nil).
		Times(1)

	postRepo.EXPECT().
		FindUnprocessedBySource(ctx, sourceID, gomock.Any()).
		Return(posts, nil).
		Times(1)

	processor.EXPECT().
		Process(gomock.Any(), gomock.Any(), posts).
		Return("cumulative summary", nil).
		Times(1)

	summaryRepo.EXPECT().
		Create(ctx, gomock.Any(), []uuid.UUID{postID1, postID2, postID3}).
		Return(nil).
		Times(1)

	outboxRepo.EXPECT().
		UpdateStatus(ctx, eventID, summary.EventStatusCompleted, false).
		Return(nil).
		Times(1)

	// Create worker and run
	worker := summary.NewWorker(outboxRepo, summaryRepo, postRepo, sourceRepo, processor, nil)
	processed, err := worker.ProcessNext(ctx)

	if err != nil {
		t.Fatalf("ProcessNext failed: %v", err)
	}
	if !processed {
		t.Fatal("ProcessNext should return true when event is processed")
	}
}

func TestWorkerProcessNext_CumulativeNoPosts(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	eventID := uuid.New()
	sourceID := uuid.New()

	// Setup mocks
	outboxRepo := mocks.NewMockOutboxEventRepository(ctrl)
	summaryRepo := mocks.NewMockRepository(ctrl)
	postRepo := mocks.NewMockPostQueryRepository(ctrl)
	sourceRepo := mocks.NewMockSourceQueryRepository(ctrl)
	processor := mocks.NewMockProcessor(ctrl)

	event := &summary.OutboxEvent{
		ID:       eventID,
		SourceID: sourceID,
		Status:   summary.EventStatusPending,
	}

	src := &source.Source{
		ID:   sourceID,
		Type: source.TypeTelegramGroup,
	}

	// Expectations
	lockTime := time.Now()
	outboxRepo.EXPECT().
		FetchAndLockPending(ctx).
		Return(event, lockTime, nil).
		Times(1)

	sourceRepo.EXPECT().
		GetByID(ctx, sourceID).
		Return(src, nil).
		Times(1)

	postRepo.EXPECT().
		FindUnprocessedBySource(ctx, sourceID, gomock.Any()).
		Return([]post.Post{}, nil).
		Times(1)

	outboxRepo.EXPECT().
		UpdateStatus(ctx, eventID, summary.EventStatusCompleted, false).
		Return(nil).
		Times(1)

	// Create worker and run - summary should NOT be created
	worker := summary.NewWorker(outboxRepo, summaryRepo, postRepo, sourceRepo, processor, nil)
	processed, err := worker.ProcessNext(ctx)

	if err != nil {
		t.Fatalf("ProcessNext failed: %v", err)
	}
	if !processed {
		t.Fatal("ProcessNext should return true when event is processed")
	}
}

func TestWorkerProcessNext_PostNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	eventID := uuid.New()
	sourceID := uuid.New()
	postID := uuid.New()

	// Setup mocks
	outboxRepo := mocks.NewMockOutboxEventRepository(ctrl)
	summaryRepo := mocks.NewMockRepository(ctrl)
	postRepo := mocks.NewMockPostQueryRepository(ctrl)
	sourceRepo := mocks.NewMockSourceQueryRepository(ctrl)
	processor := mocks.NewMockProcessor(ctrl)

	event := &summary.OutboxEvent{
		ID:       eventID,
		SourceID: sourceID,
		PostID:   &postID,
		Status:   summary.EventStatusPending,
	}

	src := &source.Source{
		ID:   sourceID,
		Type: source.TypeRSS,
	}

	// Expectations
	outboxRepo.EXPECT().
		FetchAndLockPending(ctx).
		Return(event, time.Now(), nil).
		Times(1)

	sourceRepo.EXPECT().
		GetByID(ctx, sourceID).
		Return(src, nil).
		Times(1)

	postRepo.EXPECT().
		GetByID(ctx, postID).
		Return(nil, summary.ErrPostNotFound).
		Times(1)

	outboxRepo.EXPECT().
		UpdateStatus(ctx, eventID, summary.EventStatusFailed, false).
		Return(nil).
		Times(1)

	// Create worker and run
	worker := summary.NewWorker(outboxRepo, summaryRepo, postRepo, sourceRepo, processor, nil)
	processed, err := worker.ProcessNext(ctx)

	if err != nil {
		t.Fatalf("ProcessNext failed: %v", err)
	}
	if !processed {
		t.Fatal("ProcessNext should return true when event is processed")
	}
}

func TestWorkerProcessNext_ProcessorError_SelfContained(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	eventID := uuid.New()
	sourceID := uuid.New()
	postID := uuid.New()

	// Setup mocks
	outboxRepo := mocks.NewMockOutboxEventRepository(ctrl)
	summaryRepo := mocks.NewMockRepository(ctrl)
	postRepo := mocks.NewMockPostQueryRepository(ctrl)
	sourceRepo := mocks.NewMockSourceQueryRepository(ctrl)
	processor := mocks.NewMockProcessor(ctrl)

	event := &summary.OutboxEvent{
		ID:       eventID,
		SourceID: sourceID,
		PostID:   &postID,
		Status:   summary.EventStatusPending,
	}

	src := &source.Source{
		ID:   sourceID,
		Type: source.TypeRSS,
	}

	p := &post.Post{
		ID: postID,
	}

	// Expectations
	outboxRepo.EXPECT().
		FetchAndLockPending(ctx).
		Return(event, time.Now(), nil).
		Times(1)

	sourceRepo.EXPECT().
		GetByID(ctx, sourceID).
		Return(src, nil).
		Times(1)

	postRepo.EXPECT().
		GetByID(ctx, postID).
		Return(p, nil).
		Times(1)

	processor.EXPECT().
		Process(gomock.Any(), gomock.Any(), []post.Post{*p}).
		Return("", errors.New("processing failed")).
		Times(1)

	// Transient error with retry_count=0 triggers Requeue
	outboxRepo.EXPECT().
		Requeue(ctx, eventID, gomock.Any()).
		Return(nil).
		Times(1)

	// Create worker and run
	worker := summary.NewWorker(outboxRepo, summaryRepo, postRepo, sourceRepo, processor, nil)
	processed, err := worker.ProcessNext(ctx)

	if err != nil {
		t.Fatalf("ProcessNext failed: %v", err)
	}
	if !processed {
		t.Fatal("ProcessNext should return true when event is processed")
	}
}

func TestWorkerProcessNext_UnknownSourceType(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	eventID := uuid.New()
	sourceID := uuid.New()

	// Setup mocks
	outboxRepo := mocks.NewMockOutboxEventRepository(ctrl)
	summaryRepo := mocks.NewMockRepository(ctrl)
	postRepo := mocks.NewMockPostQueryRepository(ctrl)
	sourceRepo := mocks.NewMockSourceQueryRepository(ctrl)
	processor := mocks.NewMockProcessor(ctrl)

	event := &summary.OutboxEvent{
		ID:       eventID,
		SourceID: sourceID,
		Status:   summary.EventStatusPending,
	}

	src := &source.Source{
		ID:   sourceID,
		Type: source.Type("unknown"),
	}

	// Expectations
	outboxRepo.EXPECT().
		FetchAndLockPending(ctx).
		Return(event, time.Now(), nil).
		Times(1)

	sourceRepo.EXPECT().
		GetByID(ctx, sourceID).
		Return(src, nil).
		Times(1)

	outboxRepo.EXPECT().
		UpdateStatus(ctx, eventID, summary.EventStatusFailed, false).
		Return(nil).
		Times(1)

	// Create worker and run
	worker := summary.NewWorker(outboxRepo, summaryRepo, postRepo, sourceRepo, processor, nil)
	processed, err := worker.ProcessNext(ctx)

	if err != nil {
		t.Fatalf("ProcessNext failed: %v", err)
	}
	if !processed {
		t.Fatal("ProcessNext should return true when event is processed")
	}
}

func TestWorkerProcessNext_SourceNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	eventID := uuid.New()
	sourceID := uuid.New()

	// Setup mocks
	outboxRepo := mocks.NewMockOutboxEventRepository(ctrl)
	summaryRepo := mocks.NewMockRepository(ctrl)
	postRepo := mocks.NewMockPostQueryRepository(ctrl)
	sourceRepo := mocks.NewMockSourceQueryRepository(ctrl)
	processor := mocks.NewMockProcessor(ctrl)

	event := &summary.OutboxEvent{
		ID:       eventID,
		SourceID: sourceID,
		Status:   summary.EventStatusPending,
	}

	// Expectations
	outboxRepo.EXPECT().
		FetchAndLockPending(ctx).
		Return(event, time.Now(), nil).
		Times(1)

	sourceRepo.EXPECT().
		GetByID(ctx, sourceID).
		Return(nil, summary.ErrSourceNotFound).
		Times(1)

	outboxRepo.EXPECT().
		UpdateStatus(ctx, eventID, summary.EventStatusFailed, false).
		Return(nil).
		Times(1)

	// Create worker and run
	worker := summary.NewWorker(outboxRepo, summaryRepo, postRepo, sourceRepo, processor, nil)
	processed, err := worker.ProcessNext(ctx)

	if err != nil {
		t.Fatalf("ProcessNext failed: %v", err)
	}
	if !processed {
		t.Fatal("ProcessNext should return true when event is processed")
	}
}

func TestWorkerProcessNext_NoPendingEvents(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	// Setup mocks
	outboxRepo := mocks.NewMockOutboxEventRepository(ctrl)
	summaryRepo := mocks.NewMockRepository(ctrl)
	postRepo := mocks.NewMockPostQueryRepository(ctrl)
	sourceRepo := mocks.NewMockSourceQueryRepository(ctrl)
	processor := mocks.NewMockProcessor(ctrl)

	// Expectations
	outboxRepo.EXPECT().
		FetchAndLockPending(ctx).
		Return(nil, time.Time{}, nil).
		Times(1)

	// Create worker and run
	worker := summary.NewWorker(outboxRepo, summaryRepo, postRepo, sourceRepo, processor, nil)
	processed, err := worker.ProcessNext(ctx)

	if err != nil {
		t.Fatalf("ProcessNext failed: %v", err)
	}
	if processed {
		t.Fatal("ProcessNext should return false when no events are available")
	}
}

func TestWorkerProcessNext_DBConstraintViolation(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	eventID := uuid.New()
	sourceID := uuid.New()
	postID := uuid.New()

	// Setup mocks
	outboxRepo := mocks.NewMockOutboxEventRepository(ctrl)
	summaryRepo := mocks.NewMockRepository(ctrl)
	postRepo := mocks.NewMockPostQueryRepository(ctrl)
	sourceRepo := mocks.NewMockSourceQueryRepository(ctrl)
	processor := mocks.NewMockProcessor(ctrl)

	event := &summary.OutboxEvent{
		ID:       eventID,
		SourceID: sourceID,
		PostID:   &postID,
		Status:   summary.EventStatusPending,
	}

	src := &source.Source{
		ID:   sourceID,
		Type: source.TypeRSS,
	}

	p := &post.Post{
		ID: postID,
	}

	// Expectations
	outboxRepo.EXPECT().
		FetchAndLockPending(ctx).
		Return(event, time.Now(), nil).
		Times(1)

	sourceRepo.EXPECT().
		GetByID(ctx, sourceID).
		Return(src, nil).
		Times(1)

	postRepo.EXPECT().
		GetByID(ctx, postID).
		Return(p, nil).
		Times(1)

	processor.EXPECT().
		Process(gomock.Any(), gomock.Any(), []post.Post{*p}).
		Return("content", nil).
		Times(1)

	summaryRepo.EXPECT().
		Create(ctx, gomock.Any(), []uuid.UUID{postID}).
		Return(errors.New("unique constraint violation")).
		Times(1)

	// Transient error with retry_count=0 triggers Requeue
	outboxRepo.EXPECT().
		Requeue(ctx, eventID, gomock.Any()).
		Return(nil).
		Times(1)

	// Create worker and run
	worker := summary.NewWorker(outboxRepo, summaryRepo, postRepo, sourceRepo, processor, nil)
	processed, err := worker.ProcessNext(ctx)

	if err != nil {
		t.Fatalf("ProcessNext failed: %v", err)
	}
	if !processed {
		t.Fatal("ProcessNext should return true when event is processed")
	}
}

func TestWorkerProcessNext_SkipImmediateForCumulative(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	eventID := uuid.New()
	sourceID := uuid.New()
	postID := uuid.New()

	// Setup mocks
	outboxRepo := mocks.NewMockOutboxEventRepository(ctrl)
	summaryRepo := mocks.NewMockRepository(ctrl)
	postRepo := mocks.NewMockPostQueryRepository(ctrl)
	sourceRepo := mocks.NewMockSourceQueryRepository(ctrl)
	processor := mocks.NewMockProcessor(ctrl)

	event := &summary.OutboxEvent{
		ID:        eventID,
		SourceID:  sourceID,
		PostID:    &postID,
		EventType: summary.EventTypeImmediate,
		Status:    summary.EventStatusPending,
	}

	src := &source.Source{
		ID:   sourceID,
		Type: source.TypeTelegramGroup, // cumulative source
	}

	// Expectations
	outboxRepo.EXPECT().
		FetchAndLockPending(ctx).
		Return(event, time.Now(), nil).
		Times(1)

	sourceRepo.EXPECT().
		GetByID(ctx, sourceID).
		Return(src, nil).
		Times(1)

	// No other repo calls should happen - event should be skipped
	outboxRepo.EXPECT().
		UpdateStatus(ctx, eventID, summary.EventStatusCompleted, false).
		Return(nil).
		Times(1)

	// Create worker and run - should skip processing
	worker := summary.NewWorker(outboxRepo, summaryRepo, postRepo, sourceRepo, processor, nil)
	processed, err := worker.ProcessNext(ctx)

	if err != nil {
		t.Fatalf("ProcessNext failed: %v", err)
	}
	if !processed {
		t.Fatal("ProcessNext should return true when event is processed (even if skipped)")
	}
}
