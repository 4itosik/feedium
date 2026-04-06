package summary_test

import (
	"testing"
	"time"

	"feedium/internal/app/post"
	"feedium/internal/app/summary"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewOutboxBuilder_CreatesEventForAnySource(t *testing.T) {
	sourceID := uuid.New()
	builder := summary.NewOutboxBuilder(nil)
	postID := uuid.New()
	publishedAt := time.Now()

	event, err := builder(&post.Post{
		ID:          postID,
		SourceID:    sourceID,
		Title:       "Test",
		Content:     "Content",
		PublishedAt: publishedAt,
	})

	require.NoError(t, err)
	require.NotNil(t, event)

	outboxEvent := event.(*summary.OutboxEvent)
	assert.Equal(t, sourceID, outboxEvent.SourceID)
	assert.Equal(t, &postID, outboxEvent.PostID)
	assert.Equal(t, summary.EventTypeImmediate, outboxEvent.EventType)
	assert.Equal(t, summary.EventStatusPending, outboxEvent.Status)
}

