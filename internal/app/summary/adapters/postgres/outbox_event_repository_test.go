package postgres_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"feedium/internal/app/summary"
	postgres "feedium/internal/app/summary/adapters/postgres"

	mocket "github.com/Selvatico/go-mocket"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func newMockDB(t *testing.T) *gorm.DB {
	t.Helper()

	mocket.Catcher.Register()
	mocket.Catcher.Logging = false
	mocket.Catcher.Reset()

	sqlDB, err := sql.Open(mocket.DriverName, "connection_string")
	require.NoError(t, err, "sql.Open mock failed")
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	db, err := gorm.Open(gormpostgres.New(gormpostgres.Config{Conn: sqlDB}), &gorm.Config{})
	require.NoError(t, err, "gorm.Open mock failed")

	return db
}

func TestOutboxEventRepository_UpdateStatus_WithRetry(t *testing.T) {
	db := newMockDB(t)
	repo := postgres.NewOutboxEventRepository(db)
	ctx := context.Background()
	id := uuid.New()

	// Expect update with retry_count increment
	mocket.Catcher.Reset().NewMock().
		WithQuery(`UPDATE "outbox_events"`).
		WithQuery(`retry_count`).
		WithRowsNum(1)

	err := repo.UpdateStatus(ctx, id, summary.EventStatusCompleted, true)
	assert.NoError(t, err)
}

func TestOutboxEventRepository_UpdateStatus_WithoutRetry(t *testing.T) {
	db := newMockDB(t)
	repo := postgres.NewOutboxEventRepository(db)
	ctx := context.Background()
	id := uuid.New()

	// Expect update without retry_count increment
	mocket.Catcher.Reset().NewMock().
		WithQuery(`UPDATE "outbox_events"`).
		WithRowsNum(1)

	err := repo.UpdateStatus(ctx, id, summary.EventStatusFailed, false)
	assert.NoError(t, err)
}

func TestOutboxEventRepository_Requeue(t *testing.T) {
	db := newMockDB(t)
	repo := postgres.NewOutboxEventRepository(db)
	ctx := context.Background()
	id := uuid.New()
	scheduledAt := time.Now().Add(2 * time.Minute)

	// Expect UPDATE with status=PENDING, retry_count+1, and scheduled_at
	mocket.Catcher.Reset().NewMock().
		WithQuery(`UPDATE "outbox_events"`).
		WithQuery(`status`).
		WithQuery(`retry_count`).
		WithQuery(`scheduled_at`).
		WithRowsNum(1)

	err := repo.Requeue(ctx, id, scheduledAt)
	assert.NoError(t, err)
}

func TestOutboxEventRepository_Create(t *testing.T) {
	db := newMockDB(t)
	repo := postgres.NewOutboxEventRepository(db)
	ctx := context.Background()

	event := &summary.OutboxEvent{
		SourceID:  uuid.New(),
		EventType: summary.EventTypeImmediate,
		Status:    summary.EventStatusPending,
	}

	mocket.Catcher.Reset().NewMock().
		WithQuery(`INSERT INTO "outbox_events"`).
		WithRowsNum(1)

	err := repo.Create(ctx, event)
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, event.ID, "ID should be generated")
}

func TestOutboxEventRepository_CreateScheduledForType(t *testing.T) {
	// Note: go-mocket has limitations with complex raw SQL queries like INSERT ... SELECT
	// This would require integration testing with a real database
	t.Skip("requires integration test with real database - go-mocket doesn't support complex INSERT ... SELECT")
}
