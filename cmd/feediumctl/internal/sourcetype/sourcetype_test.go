package sourcetype_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	feediumapi "github.com/4itosik/feedium/api/feedium"
	"github.com/4itosik/feedium/cmd/feediumctl/internal/sourcetype"
)

// ── Lookup (positional <type>, EC-E) ──────────────────────────────────────

func TestLookup_ValidTypes(t *testing.T) {
	cases := []struct {
		name      string
		protoType feediumapi.SourceType
	}{
		{"telegram-channel", feediumapi.SourceType_SOURCE_TYPE_TELEGRAM_CHANNEL},
		{"telegram-group", feediumapi.SourceType_SOURCE_TYPE_TELEGRAM_GROUP},
		{"rss", feediumapi.SourceType_SOURCE_TYPE_RSS},
		{"html", feediumapi.SourceType_SOURCE_TYPE_HTML},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := sourcetype.Lookup(tc.name)
			require.NoError(t, err)
			assert.Equal(t, tc.protoType, got)
		})
	}
}

func TestLookup_Unknown_EC_E(t *testing.T) {
	_, err := sourcetype.Lookup("ftp")
	require.Error(t, err)
	assert.Equal(t, `flag: unknown source type "ftp" (allowed: html,rss,telegram-channel,telegram-group)`, err.Error())
}

// ── LookupFlag (--type for update, EC-G format) ───────────────────────────

func TestLookupFlag_Valid(t *testing.T) {
	got, err := sourcetype.LookupFlag("rss")
	require.NoError(t, err)
	assert.Equal(t, feediumapi.SourceType_SOURCE_TYPE_RSS, got)
}

func TestLookupFlag_Unknown_EC_G(t *testing.T) {
	_, err := sourcetype.LookupFlag("ftp")
	require.Error(t, err)
	assert.Equal(t, `flag: unknown --type "ftp"`, err.Error())
}

// ── LookupEnumFlag (--type for list, EC-G format) ─────────────────────────

func TestLookupEnumFlag_ValidShortNames(t *testing.T) {
	cases := []struct {
		short     string
		protoType feediumapi.SourceType
	}{
		{"RSS", feediumapi.SourceType_SOURCE_TYPE_RSS},
		{"HTML", feediumapi.SourceType_SOURCE_TYPE_HTML},
		{"TELEGRAM_CHANNEL", feediumapi.SourceType_SOURCE_TYPE_TELEGRAM_CHANNEL},
		{"TELEGRAM_GROUP", feediumapi.SourceType_SOURCE_TYPE_TELEGRAM_GROUP},
		{"UNSPECIFIED", feediumapi.SourceType_SOURCE_TYPE_UNSPECIFIED},
	}
	for _, tc := range cases {
		t.Run(tc.short, func(t *testing.T) {
			got, err := sourcetype.LookupEnumFlag(tc.short)
			require.NoError(t, err)
			assert.Equal(t, tc.protoType, got)
		})
	}
}

func TestLookupEnumFlag_Unknown_EC_G(t *testing.T) {
	_, err := sourcetype.LookupEnumFlag("rss") // lowercase not valid here
	require.Error(t, err)
	assert.Equal(t, `flag: unknown --type "rss"`, err.Error())
}

// ── CheckFlags (EC-I disallowed, EC-D required) ───────────────────────────

func TestCheckFlags_RSS_AllRequired(t *testing.T) {
	err := sourcetype.CheckFlags("rss", map[string]bool{"feed-url": true})
	require.NoError(t, err)
}

func TestCheckFlags_RSS_MissingRequired_EC_D(t *testing.T) {
	err := sourcetype.CheckFlags("rss", map[string]bool{})
	require.Error(t, err)
	assert.Equal(t, `flag: --feed-url is required for type "rss"`, err.Error())
}

func TestCheckFlags_HTML_AllRequired(t *testing.T) {
	err := sourcetype.CheckFlags("html", map[string]bool{"url": true})
	require.NoError(t, err)
}

func TestCheckFlags_HTML_MissingRequired_EC_D(t *testing.T) {
	err := sourcetype.CheckFlags("html", map[string]bool{})
	require.Error(t, err)
	assert.Equal(t, `flag: --url is required for type "html"`, err.Error())
}

func TestCheckFlags_TelegramChannel_AllRequired(t *testing.T) {
	err := sourcetype.CheckFlags("telegram-channel", map[string]bool{"tg-id": true, "username": true})
	require.NoError(t, err)
}

func TestCheckFlags_TelegramChannel_MissingRequired_EC_D(t *testing.T) {
	// tg-id present but username missing; first missing in sorted order is "tg-id" < "username"
	err := sourcetype.CheckFlags("telegram-channel", map[string]bool{"tg-id": true})
	require.Error(t, err)
	assert.Equal(t, `flag: --username is required for type "telegram-channel"`, err.Error())
}

func TestCheckFlags_TelegramGroup_AllRequired(t *testing.T) {
	err := sourcetype.CheckFlags("telegram-group", map[string]bool{"tg-id": true, "username": true})
	require.NoError(t, err)
}

func TestCheckFlags_RSS_DisallowedFlag_EC_I_Create(t *testing.T) {
	// rss does not allow --tg-id; "feed-url" < "tg-id" so feed-url is checked first but allowed
	err := sourcetype.CheckFlags("rss", map[string]bool{"tg-id": true, "feed-url": true})
	require.Error(t, err)
	assert.Equal(t, `flag: --tg-id is not allowed for type "rss"`, err.Error())
}

func TestCheckFlags_RSS_DisallowedFlag_EC_I_Update(t *testing.T) {
	// --username is not allowed for rss
	err := sourcetype.CheckFlags("rss", map[string]bool{"username": true, "feed-url": true})
	require.Error(t, err)
	assert.Equal(t, `flag: --username is not allowed for type "rss"`, err.Error())
}

func TestCheckFlags_EC_I_BeforeEC_D(t *testing.T) {
	// both a disallowed flag and a missing required flag: EC-I wins
	err := sourcetype.CheckFlags("rss", map[string]bool{"tg-id": true})
	require.Error(t, err)
	assert.Equal(t, `flag: --tg-id is not allowed for type "rss"`, err.Error())
}

// ── BuildConfig ───────────────────────────────────────────────────────────

func TestBuildConfig_TelegramChannel(t *testing.T) {
	cfg := sourcetype.BuildConfig("telegram-channel", sourcetype.Flags{TgID: 42, Username: "foo"})
	require.NotNil(t, cfg)
	tc, ok := cfg.GetConfig().(*feediumapi.SourceConfig_TelegramChannel)
	require.True(t, ok, "expected TelegramChannel oneof")
	assert.Equal(t, int64(42), tc.TelegramChannel.GetTgId())
	assert.Equal(t, "foo", tc.TelegramChannel.GetUsername())
}

func TestBuildConfig_TelegramGroup(t *testing.T) {
	cfg := sourcetype.BuildConfig("telegram-group", sourcetype.Flags{TgID: 7, Username: "bar"})
	require.NotNil(t, cfg)
	tg, ok := cfg.GetConfig().(*feediumapi.SourceConfig_TelegramGroup)
	require.True(t, ok, "expected TelegramGroup oneof")
	assert.Equal(t, int64(7), tg.TelegramGroup.GetTgId())
	assert.Equal(t, "bar", tg.TelegramGroup.GetUsername())
}

func TestBuildConfig_RSS(t *testing.T) {
	cfg := sourcetype.BuildConfig("rss", sourcetype.Flags{FeedURL: "https://example.com/feed"})
	require.NotNil(t, cfg)
	r, ok := cfg.GetConfig().(*feediumapi.SourceConfig_Rss)
	require.True(t, ok, "expected RSS oneof")
	assert.Equal(t, "https://example.com/feed", r.Rss.GetFeedUrl())
}

func TestBuildConfig_HTML(t *testing.T) {
	cfg := sourcetype.BuildConfig("html", sourcetype.Flags{URL: "https://example.com"})
	require.NotNil(t, cfg)
	h, ok := cfg.GetConfig().(*feediumapi.SourceConfig_Html)
	require.True(t, ok, "expected HTML oneof")
	assert.Equal(t, "https://example.com", h.Html.GetUrl())
}
