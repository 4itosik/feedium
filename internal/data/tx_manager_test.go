package data_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/4itosik/feedium/internal/biz"
	"github.com/4itosik/feedium/internal/data"
)

func TestIntegration_TxManager_Commit(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	client, cleanup := setupTestDB(t)
	defer cleanup()

	d := &data.Data{Ent: client}
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
	client, cleanup := setupTestDB(t)
	defer cleanup()

	d := &data.Data{Ent: client}
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
