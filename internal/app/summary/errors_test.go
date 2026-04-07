package summary_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"feedium/internal/app/source"
	"feedium/internal/app/summary"
)

func TestIsPermanentError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "post not found",
			err:      summary.ErrPostNotFound,
			expected: true,
		},
		{
			name:     "unknown source type",
			err:      summary.ErrUnknownSourceType,
			expected: true,
		},
		{
			name:     "source not found",
			err:      summary.ErrSourceNotFound,
			expected: true,
		},
		{
			name:     "content too large",
			err:      summary.ErrContentTooLarge,
			expected: true,
		},
		{
			name:     "empty LLM response (transient)",
			err:      summary.ErrEmptyLLMResponse,
			expected: false,
		},
		{
			name:     "wrapped permanent error",
			err:      assert.AnError,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := summary.IsPermanentError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProcessingModeForSourceType(t *testing.T) {
	tests := []struct {
		name         string
		sourceType   source.Type
		expectedMode summary.ProcessingMode
		expectedErr  error
	}{
		{
			name:         "RSS type",
			sourceType:   source.TypeRSS,
			expectedMode: summary.ModeSelfContained,
			expectedErr:  nil,
		},
		{
			name:         "Telegram channel type",
			sourceType:   source.TypeTelegramChannel,
			expectedMode: summary.ModeSelfContained,
			expectedErr:  nil,
		},
		{
			name:         "Telegram group type (cumulative)",
			sourceType:   source.TypeTelegramGroup,
			expectedMode: summary.ModeCumulative,
			expectedErr:  nil,
		},
		{
			name:         "Web scraping type",
			sourceType:   source.TypeWebScraping,
			expectedMode: summary.ModeSelfContained,
			expectedErr:  nil,
		},
		{
			name:         "unknown type",
			sourceType:   "unknown",
			expectedMode: "",
			expectedErr:  nil, // any error is acceptable
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mode, err := summary.ProcessingModeForSourceType(tt.sourceType)
			if tt.name == "unknown type" {
				require.Error(t, err)
				return
			}
			if tt.expectedErr != nil {
				require.ErrorIs(t, err, tt.expectedErr)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedMode, mode)
			}
		})
	}
}
