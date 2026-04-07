package post

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNormalizePageSizeAndPage(t *testing.T) {
	t.Parallel()

	if got := normalizePageSize(0); got != defaultPageSize {
		t.Fatalf("default page size mismatch: %d", got)
	}
	if got := normalizePageSize(-1); got != defaultPageSize {
		t.Fatalf("negative page size mismatch: %d", got)
	}
	if got := normalizePageSize(maxPageSize + 1); got != maxPageSize {
		t.Fatalf("max page size cap mismatch: %d", got)
	}
	if got := normalizePageSize(10); got != 10 {
		t.Fatalf("normal page size mismatch: %d", got)
	}

	if got := normalizePage(0); got != 1 {
		t.Fatalf("default page mismatch: %d", got)
	}
	if got := normalizePage(-2); got != 1 {
		t.Fatalf("negative page mismatch: %d", got)
	}
	if got := normalizePage(3); got != 3 {
		t.Fatalf("normal page mismatch: %d", got)
	}
}

func TestValidatePost(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()

	tests := []struct {
		name string
		post *Post
		ok   bool
	}{
		{name: "nil_post", post: nil, ok: false},
		{
			name: "valid_post",
			post: &Post{
				SourceID:    uuid.New(),
				Title:       "title",
				Content:     "content",
				PublishedAt: now,
			},
			ok: true,
		},
		{
			name: "nil_source_id",
			post: &Post{
				SourceID:    uuid.Nil,
				Title:       "title",
				Content:     "content",
				PublishedAt: now,
			},
			ok: false,
		},
		{
			name: "empty_title",
			post: &Post{
				SourceID:    uuid.New(),
				Title:       "",
				Content:     "content",
				PublishedAt: now,
			},
			ok: false,
		},
		{
			name: "title_with_only_spaces",
			post: &Post{
				SourceID:    uuid.New(),
				Title:       "   ",
				Content:     "content",
				PublishedAt: now,
			},
			ok: false,
		},
		{
			name: "empty_content",
			post: &Post{
				SourceID:    uuid.New(),
				Title:       "title",
				Content:     "",
				PublishedAt: now,
			},
			ok: false,
		},
		{
			name: "content_with_only_spaces",
			post: &Post{
				SourceID:    uuid.New(),
				Title:       "title",
				Content:     "   ",
				PublishedAt: now,
			},
			ok: false,
		},
		{
			name: "zero_published_at",
			post: &Post{
				SourceID:    uuid.New(),
				Title:       "title",
				Content:     "content",
				PublishedAt: time.Time{},
			},
			ok: false,
		},
	}

	for _, tc := range tests {
		err := validatePost(tc.post)
		if tc.ok && err != nil {
			t.Fatalf("%s: unexpected error: %v", tc.name, err)
		}
		if !tc.ok && err == nil {
			t.Fatalf("%s: expected error", tc.name)
		}
		if !tc.ok {
			var v ValidationError
			if !isValidationError(err, &v) {
				t.Fatalf("%s: expected ValidationError, got %T", tc.name, err)
			}
		}
	}

	valid := &Post{
		SourceID:    uuid.New(),
		Title:       "  title  ",
		Content:     "  content  ",
		PublishedAt: now,
	}
	if err := validatePost(valid); err != nil {
		t.Fatalf("expected valid post, got error: %v", err)
	}
	if valid.Title != "title" || valid.Content != "content" {
		t.Fatalf("expected trimmed fields, got title=%q content=%q", valid.Title, valid.Content)
	}
}

func TestValidationError(t *testing.T) {
	t.Parallel()

	err := validationError("boom")
	if err.Error() != "boom" {
		t.Fatalf("validationError message mismatch: %q", err.Error())
	}
}

func isValidationError(err error, target *ValidationError) bool {
	if err == nil {
		return false
	}
	return errors.As(err, target)
}
