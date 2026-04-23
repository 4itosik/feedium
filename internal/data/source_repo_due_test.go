package data_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/4itosik/feedium/internal/biz"
	"github.com/4itosik/feedium/internal/data"
)

func TestIntegration_SourceRepo_ClaimDueCumulative_NoSource(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	d, cleanup := setupTestData(t)
	defer cleanup()

	repo := data.NewSourceRepo(d)

	_, err := repo.ClaimDueCumulative(ctx)
	require.ErrorIs(t, err, biz.ErrNoSourceDue)
}

func TestIntegration_SourceRepo_ClaimDueCumulative_SkipsSelfContained(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	d, cleanup := setupTestData(t)
	defer cleanup()

	repo := data.NewSourceRepo(d)

	now := time.Now()
	_, err := repo.Save(ctx, biz.Source{
		Type:      biz.SourceTypeRSS,
		Config:    &biz.RSSConfig{FeedURL: "https://example.com/feed"},
		CreatedAt: now,
		UpdatedAt: now,
	})
	require.NoError(t, err)

	_, err = repo.ClaimDueCumulative(ctx)
	assert.ErrorIs(t, err, biz.ErrNoSourceDue)
}

func TestIntegration_SourceRepo_ClaimDueCumulative_PicksDueGroup(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	d, cleanup := setupTestData(t)
	defer cleanup()

	repo := data.NewSourceRepo(d)

	now := time.Now()
	saved, err := repo.Save(ctx, biz.Source{
		Type:      biz.SourceTypeTelegramGroup,
		Config:    &biz.TelegramGroupConfig{TgID: 1234567, Username: "grp"},
		CreatedAt: now,
		UpdatedAt: now,
	})
	require.NoError(t, err)

	picked, err := repo.ClaimDueCumulative(ctx)
	require.NoError(t, err)
	assert.Equal(t, saved.ID, picked.ID)
	assert.Equal(t, biz.ProcessingModeCumulative, picked.ProcessingMode)
}

func TestIntegration_SourceRepo_BumpNextSummaryAt_SkipsInWindow(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	d, cleanup := setupTestData(t)
	defer cleanup()

	repo := data.NewSourceRepo(d)

	now := time.Now()
	saved, err := repo.Save(ctx, biz.Source{
		Type:      biz.SourceTypeTelegramGroup,
		Config:    &biz.TelegramGroupConfig{TgID: 99, Username: "grp2"},
		CreatedAt: now,
		UpdatedAt: now,
	})
	require.NoError(t, err)

	// Due immediately.
	first, err := repo.ClaimDueCumulative(ctx)
	require.NoError(t, err)
	assert.Equal(t, saved.ID, first.ID)

	// Bump to future — not due any more.
	future := time.Now().Add(1 * time.Hour)
	require.NoError(t, repo.BumpNextSummaryAt(ctx, saved.ID, future))

	_, err = repo.ClaimDueCumulative(ctx)
	require.ErrorIs(t, err, biz.ErrNoSourceDue)

	// Bump back to the past — due again.
	require.NoError(t, repo.BumpNextSummaryAt(ctx, saved.ID, time.Now().Add(-time.Minute)))
	second, err := repo.ClaimDueCumulative(ctx)
	require.NoError(t, err)
	assert.Equal(t, saved.ID, second.ID)
}

func TestIntegration_SourceRepo_BumpNextSummaryAt_NotFound(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	d, cleanup := setupTestData(t)
	defer cleanup()

	repo := data.NewSourceRepo(d)

	// Random non-existent UUID.
	const unknownID = "11111111-1111-1111-1111-111111111111"
	err := repo.BumpNextSummaryAt(ctx, unknownID, time.Now())
	assert.ErrorIs(t, err, biz.ErrSourceNotFound)
}
