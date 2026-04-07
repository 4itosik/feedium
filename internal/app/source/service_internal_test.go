package source

import (
	"errors"
	"testing"
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

func TestValidateSourceAndConfigEdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  *Source
		ok   bool
	}{
		{name: "nil_source", src: nil, ok: false},
		{
			name: "name_spaces",
			src: &Source{
				Type:   TypeRSS,
				Name:   "   ",
				URL:    "https://x",
				Config: map[string]any{"feed_url": "https://feed"},
			},
			ok: false,
		},
		{
			name: "name_too_long",
			src: &Source{
				Type:   TypeRSS,
				Name:   string(make([]byte, 256)),
				URL:    "https://x",
				Config: map[string]any{"feed_url": "https://feed"},
			},
			ok: false,
		},
		{
			name: "invalid_url_scheme",
			src:  &Source{Type: TypeRSS, Name: "n", URL: "ftp://x", Config: map[string]any{"feed_url": "https://feed"}},
			ok:   false,
		},
		{
			name: "missing_type",
			src:  &Source{Type: "", Name: "n", URL: "https://x", Config: map[string]any{"feed_url": "https://feed"}},
			ok:   false,
		},
		{name: "nil_config", src: &Source{Type: TypeRSS, Name: "n", URL: "https://x", Config: nil}, ok: false},
		{
			name: "telegram_channel_spaces",
			src: &Source{
				Type:   TypeTelegramChannel,
				Name:   "n",
				URL:    "https://x",
				Config: map[string]any{"channel_id": "   "},
			},
			ok: false,
		},
		{
			name: "telegram_group_spaces",
			src: &Source{
				Type:   TypeTelegramGroup,
				Name:   "n",
				URL:    "https://x",
				Config: map[string]any{"group_id": "   "},
			},
			ok: false,
		},
		{
			name: "rss_invalid_feed_url",
			src:  &Source{Type: TypeRSS, Name: "n", URL: "https://x", Config: map[string]any{"feed_url": "not_url"}},
			ok:   false,
		},
		{
			name: "web_selector_spaces",
			src: &Source{
				Type:   TypeWebScraping,
				Name:   "n",
				URL:    "https://x",
				Config: map[string]any{"selector": "   "},
			},
			ok: false,
		},
		{
			name: "unknown_type",
			src:  &Source{Type: Type("x"), Name: "n", URL: "https://x", Config: map[string]any{"k": "v"}},
			ok:   false,
		},
		{
			name: "valid_rss",
			src: &Source{
				Type:   TypeRSS,
				Name:   "  n  ",
				URL:    "  https://x  ",
				Config: map[string]any{"feed_url": "https://feed"},
			},
			ok: true,
		},
	}

	for _, tc := range tests {
		err := validateSource(tc.src)
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

	valid := &Source{
		Type:   TypeRSS,
		Name:   "  name  ",
		URL:    "  https://example.com/path  ",
		Config: map[string]any{"feed_url": "https://feed"},
	}
	if err := validateSource(valid); err != nil {
		t.Fatalf("expected valid source, got error: %v", err)
	}
	if valid.Name != "name" || valid.URL != "https://example.com/path" {
		t.Fatalf("expected trimmed fields, got name=%q url=%q", valid.Name, valid.URL)
	}
}

func TestValidationErrorAndHTTPURL(t *testing.T) {
	t.Parallel()

	err := validationError("boom")
	if err.Error() != "boom" {
		t.Fatalf("validationError message mismatch: %q", err.Error())
	}

	if !isHTTPURL("https://example.com") {
		t.Fatal("https URL must be valid")
	}
	if !isHTTPURL("http://example.com") {
		t.Fatal("http URL must be valid")
	}
	if isHTTPURL("mailto:test@example.com") {
		t.Fatal("mailto URL must be invalid")
	}
	if isHTTPURL("https:///missing-host") {
		t.Fatal("URL without host must be invalid")
	}
}

func isValidationError(err error, target *ValidationError) bool {
	if err == nil {
		return false
	}
	return errors.As(err, target)
}
