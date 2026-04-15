//nolint:testpackage // test needs access to unexported functions and types
package biz

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestValidateCreatePost(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	validSourceID := "01961d9c-4f78-7e2e-8c3a-5e7d9a1b2c3d"
	validPublishedAt := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		sourceID    string
		externalID  string
		text        string
		publishedAt time.Time
		wantErr     bool
		errContains string
	}{
		{
			name:        "empty sourceID",
			sourceID:    "",
			externalID:  "ext-1",
			text:        "hello",
			publishedAt: validPublishedAt,
			wantErr:     true,
			errContains: "source_id",
		},
		{
			name:        "invalid UUID sourceID",
			sourceID:    "not-a-uuid",
			externalID:  "ext-1",
			text:        "hello",
			publishedAt: validPublishedAt,
			wantErr:     true,
			errContains: "source_id",
		},
		{
			name:        "empty externalID",
			sourceID:    validSourceID,
			externalID:  "",
			text:        "hello",
			publishedAt: validPublishedAt,
			wantErr:     true,
			errContains: "external_id",
		},
		{
			name:        "empty text",
			sourceID:    validSourceID,
			externalID:  "ext-1",
			text:        "",
			publishedAt: validPublishedAt,
			wantErr:     true,
			errContains: "text",
		},
		{
			name:        "whitespace-only text",
			sourceID:    validSourceID,
			externalID:  "ext-1",
			text:        "   ",
			publishedAt: validPublishedAt,
			wantErr:     true,
			errContains: "text",
		},
		{
			name:        "zero publishedAt",
			sourceID:    validSourceID,
			externalID:  "ext-1",
			text:        "hello",
			publishedAt: time.Time{},
			wantErr:     true,
			errContains: "published_at",
		},
		{
			name:        "valid",
			sourceID:    validSourceID,
			externalID:  "ext-1",
			text:        "hello",
			publishedAt: validPublishedAt,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCreatePost(tt.sourceID, tt.externalID, tt.text, tt.publishedAt)
			if tt.wantErr {
				require.Error(t, err)
				require.ErrorIs(t, err, ErrPostInvalidArgument)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateUpdatePost(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	validPublishedAt := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		externalID  string
		text        string
		publishedAt time.Time
		wantErr     bool
		errContains string
	}{
		{
			name:        "empty externalID",
			externalID:  "",
			text:        "hello",
			publishedAt: validPublishedAt,
			wantErr:     true,
			errContains: "external_id",
		},
		{
			name:        "empty text",
			externalID:  "ext-1",
			text:        "",
			publishedAt: validPublishedAt,
			wantErr:     true,
			errContains: "text",
		},
		{
			name:        "valid",
			externalID:  "ext-1",
			text:        "hello",
			publishedAt: validPublishedAt,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateUpdatePost(tt.externalID, tt.text, tt.publishedAt)
			if tt.wantErr {
				require.Error(t, err)
				require.ErrorIs(t, err, ErrPostInvalidArgument)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateListPostsFilter(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	validSourceID := "01961d9c-4f78-7e2e-8c3a-5e7d9a1b2c3d"

	tests := []struct {
		name    string
		filter  ListPostsFilter
		wantErr bool
	}{
		{
			name: "valid",
			filter: ListPostsFilter{
				OrderBy:  SortByPublishedAt,
				OrderDir: SortDesc,
			},
			wantErr: false,
		},
		{
			name: "valid with sourceID",
			filter: ListPostsFilter{
				SourceID: validSourceID,
				OrderBy:  SortByCreatedAt,
				OrderDir: SortAsc,
			},
			wantErr: false,
		},
		{
			name: "invalid OrderBy zero",
			filter: ListPostsFilter{
				OrderBy:  SortField(0),
				OrderDir: SortDesc,
			},
			wantErr: true,
		},
		{
			name: "invalid OrderBy unknown",
			filter: ListPostsFilter{
				OrderBy:  SortField(99),
				OrderDir: SortDesc,
			},
			wantErr: true,
		},
		{
			name: "invalid OrderDir zero",
			filter: ListPostsFilter{
				OrderBy:  SortByPublishedAt,
				OrderDir: SortDirection(0),
			},
			wantErr: true,
		},
		{
			name: "invalid OrderDir unknown",
			filter: ListPostsFilter{
				OrderBy:  SortByPublishedAt,
				OrderDir: SortDirection(99),
			},
			wantErr: true,
		},
		{
			name: "invalid SourceID",
			filter: ListPostsFilter{
				SourceID: "not-a-uuid",
				OrderBy:  SortByPublishedAt,
				OrderDir: SortDesc,
			},
			wantErr: true,
		},
		{
			name: "empty SourceID is valid",
			filter: ListPostsFilter{
				SourceID: "",
				OrderBy:  SortByPublishedAt,
				OrderDir: SortDesc,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateListPostsFilter(tt.filter)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrPostInvalidArgument)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
