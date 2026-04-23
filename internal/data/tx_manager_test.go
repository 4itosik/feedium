package data_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/4itosik/feedium/internal/biz"
	"github.com/4itosik/feedium/internal/data"
)

func TestIntegration_TxManager_Commit(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	d, cleanup := setupTestData(t)
	defer cleanup()

	txMgr := data.NewTxManager(d)
	sourceRepo := data.NewSourceRepo(d)
	outboxRepo := data.NewSummaryOutboxRepo(d)

	source, err := sourceRepo.Save(ctx, biz.Source{
		Type:      biz.SourceTypeTelegramGroup,
		Config:    &biz.TelegramGroupConfig{TgID: 111222333, Username: "tx_commit_channel"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	})
	require.NoError(t, err)

	event := biz.NewSummaryEvent(biz.SummaryEventTypeSummarizeSource, source.ID, nil)

	var savedEvent biz.SummaryEvent

	err = txMgr.InTx(ctx, func(txCtx context.Context) error {
		savedEvent, err = outboxRepo.Save(txCtx, event)
		return err
	})
	require.NoError(t, err)

	fetched, err := outboxRepo.Get(ctx, savedEvent.ID)
	require.NoError(t, err)
	assert.Equal(t, savedEvent.ID, fetched.ID)
	assert.Equal(t, biz.SummaryEventStatusPending, fetched.Status)
	assert.Equal(t, biz.SummaryEventTypeSummarizeSource, fetched.EventType)
	assert.Equal(t, source.ID, fetched.SourceID)
}

func TestIntegration_TxManager_Rollback(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	d, cleanup := setupTestData(t)
	defer cleanup()

	txMgr := data.NewTxManager(d)
	sourceRepo := data.NewSourceRepo(d)
	outboxRepo := data.NewSummaryOutboxRepo(d)

	source, err := sourceRepo.Save(ctx, biz.Source{
		Type:      biz.SourceTypeTelegramGroup,
		Config:    &biz.TelegramGroupConfig{TgID: 444555666, Username: "tx_rollback_channel"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	})
	require.NoError(t, err)

	event := biz.NewSummaryEvent(biz.SummaryEventTypeSummarizeSource, source.ID, nil)
	rollbackErr := errors.New("intentional rollback")

	err = txMgr.InTx(ctx, func(txCtx context.Context) error {
		_, saveErr := outboxRepo.Save(txCtx, event)
		if saveErr != nil {
			return saveErr
		}
		return rollbackErr
	})
	require.ErrorIs(t, err, rollbackErr)

	_, err = outboxRepo.Get(ctx, event.ID)
	assert.ErrorIs(t, err, biz.ErrSummaryEventNotFound)
}

func TestIntegration_TxManager_Rollback_PostAndOutbox(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	d, cleanup := setupTestData(t)
	defer cleanup()

	txMgr := data.NewTxManager(d)
	sourceRepo := data.NewSourceRepo(d)
	postRepo := data.NewPostRepo(d)
	outboxRepo := data.NewSummaryOutboxRepo(d)

	source, err := sourceRepo.Save(ctx, biz.Source{
		Type:      biz.SourceTypeTelegramChannel,
		Config:    &biz.TelegramChannelConfig{TgID: 777888999, Username: "tx_post_rollback_channel"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	})
	require.NoError(t, err)

	now := time.Now()
	post := biz.Post{
		ID:          uuid.Must(uuid.NewV7()).String(),
		SourceID:    source.ID,
		ExternalID:  "ext-tx-rollback-001",
		PublishedAt: now,
		Text:        "post body for rollback test",
		Metadata:    map[string]string{},
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	event := biz.NewSummaryEvent(biz.SummaryEventTypeSummarizePost, source.ID, &post.ID)
	rollbackErr := errors.New("intentional rollback")

	err = txMgr.InTx(ctx, func(txCtx context.Context) error {
		if _, _, saveErr := postRepo.Save(txCtx, post); saveErr != nil {
			return saveErr
		}
		if _, saveErr := outboxRepo.Save(txCtx, event); saveErr != nil {
			return saveErr
		}
		return rollbackErr
	})
	require.ErrorIs(t, err, rollbackErr)

	// Both post and outbox event must be absent — atomicity guarantee (INV-1).
	_, getErr := postRepo.Get(ctx, post.ID)
	require.ErrorIs(t, getErr, biz.ErrPostNotFound, "post must not be in DB after rollback")

	_, getErr = outboxRepo.Get(ctx, event.ID)
	assert.ErrorIs(t, getErr, biz.ErrSummaryEventNotFound, "outbox event must not be in DB after rollback")
}
