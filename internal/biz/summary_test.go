//nolint:testpackage // test needs access to unexported functions and types
package biz

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestNewSummaryEvent(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	sourceID := "01961d9c-4f78-7e2e-8c3a-5e7d9a1b2c3d"
	postID := "01961d9c-4f78-7e2e-8c3a-5e7d9a1b2c3e"

	t.Run("self contained", func(t *testing.T) {
		event := NewSummaryEvent(SummaryEventTypeSummarizePost, sourceID, &postID)
		assert.NotEmpty(t, event.ID)
		assert.Equal(t, SummaryEventStatusPending, event.Status)
		assert.Equal(t, SummaryEventTypeSummarizePost, event.EventType)
		assert.Equal(t, sourceID, event.SourceID)
		assert.Equal(t, &postID, event.PostID)
		assert.WithinDuration(t, time.Now(), event.CreatedAt, time.Second)
	})

	t.Run("cumulative", func(t *testing.T) {
		event := NewSummaryEvent(SummaryEventTypeSummarizeSource, sourceID, nil)
		assert.NotEmpty(t, event.ID)
		assert.Equal(t, SummaryEventStatusPending, event.Status)
		assert.Equal(t, SummaryEventTypeSummarizeSource, event.EventType)
		assert.Equal(t, sourceID, event.SourceID)
		assert.Nil(t, event.PostID)
	})

	t.Run("generates unique IDs", func(t *testing.T) {
		event1 := NewSummaryEvent(SummaryEventTypeSummarizePost, sourceID, &postID)
		event2 := NewSummaryEvent(SummaryEventTypeSummarizePost, sourceID, &postID)
		assert.NotEqual(t, event1.ID, event2.ID)
	})
}

func TestValidateSummary(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	tests := []struct {
		name    string
		text    string
		wantErr bool
	}{
		{
			name:    "valid text",
			text:    "This is a valid summary text",
			wantErr: false,
		},
		{
			name:    "empty text",
			text:    "",
			wantErr: true,
		},
		{
			name:    "whitespace only",
			text:    "   \t\n  ",
			wantErr: true,
		},
		{
			name:    "text with leading/trailing whitespace",
			text:    "  valid text  ",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSummary(tt.text)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrSummaryValidation)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSummaryInvariant(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	postID := "01961d9c-4f78-7e2e-8c3a-5e7d9a1b2c3e"

	t.Run("self contained has PostID", func(t *testing.T) {
		summary := Summary{PostID: &postID}
		assert.NotNil(t, summary.PostID)
	})

	t.Run("cumulative has nil PostID", func(t *testing.T) {
		summary := Summary{}
		assert.Nil(t, summary.PostID)
	})
}
