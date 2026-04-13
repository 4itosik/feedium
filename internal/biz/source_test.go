//nolint:testpackage // test needs access to unexported functions and types
package biz

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestValidateSourceConfig_ValidCases(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	tests := []struct {
		name       string
		sourceType SourceType
		config     SourceConfig
	}{
		{
			name:       "valid telegram_channel",
			sourceType: SourceTypeTelegramChannel,
			config:     &TelegramChannelConfig{TgID: 123, Username: "channel"},
		},
		{
			name:       "valid telegram_group",
			sourceType: SourceTypeTelegramGroup,
			config:     &TelegramGroupConfig{TgID: 456, Username: ""},
		},
		{
			name:       "valid rss",
			sourceType: SourceTypeRSS,
			config:     &RSSConfig{FeedURL: "https://example.com/feed"},
		},
		{
			name:       "valid html",
			sourceType: SourceTypeHTML,
			config:     &HTMLConfig{URL: "https://example.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSourceConfig(tt.sourceType, tt.config)
			assert.NoError(t, err)
		})
	}
}

func TestValidateSourceConfig_InvalidCases(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	tests := []struct {
		name       string
		sourceType SourceType
		config     SourceConfig
		wantErr    bool
		errCheck   func(t *testing.T, err error)
	}{
		{
			name:       "telegram_channel without tg_id",
			sourceType: SourceTypeTelegramChannel,
			config:     &TelegramChannelConfig{TgID: 0, Username: "channel"},
			wantErr:    true,
			errCheck: func(t *testing.T, err error) {
				require.ErrorIs(t, err, ErrInvalidConfig)
				assert.Contains(t, err.Error(), "tg_id")
			},
		},
		{
			name:       "telegram_group without tg_id",
			sourceType: SourceTypeTelegramGroup,
			config:     &TelegramGroupConfig{TgID: 0},
			wantErr:    true,
			errCheck: func(t *testing.T, err error) {
				require.ErrorIs(t, err, ErrInvalidConfig)
			},
		},
		{
			name:       "rss without feed_url",
			sourceType: SourceTypeRSS,
			config:     &RSSConfig{FeedURL: ""},
			wantErr:    true,
			errCheck: func(t *testing.T, err error) {
				require.ErrorIs(t, err, ErrInvalidConfig)
				assert.Contains(t, err.Error(), "feed_url")
			},
		},
		{
			name:       "rss with invalid URL",
			sourceType: SourceTypeRSS,
			config:     &RSSConfig{FeedURL: "not-a-url"},
			wantErr:    true,
			errCheck: func(t *testing.T, err error) {
				require.ErrorIs(t, err, ErrInvalidConfig)
				assert.Contains(t, err.Error(), "invalid URL")
			},
		},
		{
			name:       "html without url",
			sourceType: SourceTypeHTML,
			config:     &HTMLConfig{URL: ""},
			wantErr:    true,
			errCheck: func(t *testing.T, err error) {
				require.ErrorIs(t, err, ErrInvalidConfig)
			},
		},
		{
			name:       "html with invalid URL",
			sourceType: SourceTypeHTML,
			config:     &HTMLConfig{URL: "not-a-url"},
			wantErr:    true,
			errCheck: func(t *testing.T, err error) {
				require.ErrorIs(t, err, ErrInvalidConfig)
				assert.Contains(t, err.Error(), "invalid URL")
			},
		},
		{
			name:       "nil config",
			sourceType: SourceTypeRSS,
			config:     nil,
			wantErr:    true,
			errCheck: func(t *testing.T, err error) {
				require.ErrorIs(t, err, ErrInvalidConfig)
			},
		},
		{
			name:       "unknown type",
			sourceType: "unknown",
			config:     &RSSConfig{FeedURL: "https://example.com/feed"},
			wantErr:    true,
			errCheck: func(t *testing.T, err error) {
				assert.ErrorIs(t, err, ErrInvalidSourceType)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSourceConfig(tt.sourceType, tt.config)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errCheck != nil {
					tt.errCheck(t, err)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestProcessingModeForType(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	tests := []struct {
		sourceType   SourceType
		expectedMode ProcessingMode
	}{
		{SourceTypeTelegramChannel, ProcessingModeSelfContained},
		{SourceTypeTelegramGroup, ProcessingModeCumulative},
		{SourceTypeRSS, ProcessingModeSelfContained},
		{SourceTypeHTML, ProcessingModeSelfContained},
	}

	for _, tt := range tests {
		t.Run(string(tt.sourceType), func(t *testing.T) {
			mode := ProcessingModeForType(tt.sourceType)
			assert.Equal(t, tt.expectedMode, mode)
		})
	}
}

func TestValidateSourceType(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	tests := []struct {
		name       string
		sourceType SourceType
		wantErr    bool
	}{
		{
			name:       "valid telegram_channel",
			sourceType: SourceTypeTelegramChannel,
			wantErr:    false,
		},
		{
			name:       "valid telegram_group",
			sourceType: SourceTypeTelegramGroup,
			wantErr:    false,
		},
		{
			name:       "valid rss",
			sourceType: SourceTypeRSS,
			wantErr:    false,
		},
		{
			name:       "valid html",
			sourceType: SourceTypeHTML,
			wantErr:    false,
		},
		{
			name:       "invalid type",
			sourceType: "invalid",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSourceType(tt.sourceType)
			if tt.wantErr {
				assert.ErrorIs(t, err, ErrInvalidSourceType)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
