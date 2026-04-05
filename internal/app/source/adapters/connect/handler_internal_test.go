package connect

import (
	"errors"
	"testing"
	"time"

	sourcev1 "feedium/api/source/v1"
	"feedium/internal/app/source"

	"github.com/google/uuid"
)

func TestFromProtoType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   sourcev1.SourceType
		want source.Type
	}{
		{name: "unspecified", in: sourcev1.SourceType_SOURCE_TYPE_UNSPECIFIED, want: ""},
		{
			name: "telegram_channel",
			in:   sourcev1.SourceType_SOURCE_TYPE_TELEGRAM_CHANNEL,
			want: source.TypeTelegramChannel,
		},
		{name: "telegram_group", in: sourcev1.SourceType_SOURCE_TYPE_TELEGRAM_GROUP, want: source.TypeTelegramGroup},
		{name: "rss", in: sourcev1.SourceType_SOURCE_TYPE_RSS, want: source.TypeRSS},
		{name: "web_scraping", in: sourcev1.SourceType_SOURCE_TYPE_WEB_SCRAPING, want: source.TypeWebScraping},
		{name: "unknown", in: sourcev1.SourceType(99), want: ""},
	}

	for _, tc := range tests {
		got := fromProtoType(tc.in)
		if got != tc.want {
			t.Fatalf("%s: got %q want %q", tc.name, got, tc.want)
		}
	}
}

func TestIsKnownType(t *testing.T) {
	t.Parallel()

	if !isKnownType(sourcev1.SourceType_SOURCE_TYPE_UNSPECIFIED) {
		t.Fatal("UNSPECIFIED must be known for list filter")
	}
	if isKnownType(sourcev1.SourceType(999)) {
		t.Fatal("unknown enum must be rejected")
	}
}

func TestFromProtoConfig(t *testing.T) {
	t.Parallel()

	if cfg := fromProtoConfig(sourcev1.SourceType_SOURCE_TYPE_RSS, nil); cfg != nil {
		t.Fatal("nil config must stay nil")
	}

	telegramCfg := &sourcev1.SourceConfig{
		Config: &sourcev1.SourceConfig_TelegramChannel{
			TelegramChannel: &sourcev1.TelegramChannelConfig{ChannelId: "c-1"},
		},
	}
	gotTelegram := fromProtoConfig(sourcev1.SourceType_SOURCE_TYPE_TELEGRAM_CHANNEL, telegramCfg)
	if gotTelegram["channel_id"] != "c-1" {
		t.Fatalf("channel_id mismatch: %v", gotTelegram["channel_id"])
	}

	mismatch := fromProtoConfig(sourcev1.SourceType_SOURCE_TYPE_RSS, telegramCfg)
	if mismatch != nil {
		t.Fatal("mismatched oneof must return nil")
	}

	rssCfg := &sourcev1.SourceConfig{
		Config: &sourcev1.SourceConfig_Rss{Rss: &sourcev1.RssConfig{FeedUrl: "https://feed"}},
	}
	gotRSS := fromProtoConfig(sourcev1.SourceType_SOURCE_TYPE_RSS, rssCfg)
	if gotRSS["feed_url"] != "https://feed" {
		t.Fatalf("feed_url mismatch: %v", gotRSS["feed_url"])
	}

	groupCfg := &sourcev1.SourceConfig{
		Config: &sourcev1.SourceConfig_TelegramGroup{
			TelegramGroup: &sourcev1.TelegramGroupConfig{GroupId: "g-1"},
		},
	}
	gotGroup := fromProtoConfig(sourcev1.SourceType_SOURCE_TYPE_TELEGRAM_GROUP, groupCfg)
	if gotGroup["group_id"] != "g-1" {
		t.Fatalf("group_id mismatch: %v", gotGroup["group_id"])
	}

	webCfg := &sourcev1.SourceConfig{
		Config: &sourcev1.SourceConfig_WebScraping{
			WebScraping: &sourcev1.WebScrapingConfig{Selector: ".post"},
		},
	}
	gotWeb := fromProtoConfig(sourcev1.SourceType_SOURCE_TYPE_WEB_SCRAPING, webCfg)
	if gotWeb["selector"] != ".post" {
		t.Fatalf("selector mismatch: %v", gotWeb["selector"])
	}
}

func TestToProtoMapping(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	src := &source.Source{
		ID:        uuid.New(),
		Type:      source.TypeRSS,
		Name:      "rss",
		URL:       "https://example.com",
		Config:    map[string]any{"feed_url": "https://feed"},
		CreatedAt: now,
		UpdatedAt: now,
	}

	out := toProto(src)
	if out.GetId() != src.ID.String() {
		t.Fatalf("id mismatch: got %s want %s", out.GetId(), src.ID)
	}
	if out.GetType() != sourcev1.SourceType_SOURCE_TYPE_RSS {
		t.Fatalf("type mismatch: %v", out.GetType())
	}
	if out.GetConfig().GetRss().GetFeedUrl() != "https://feed" {
		t.Fatalf("config mismatch: %v", out.GetConfig())
	}
}

func TestToProtoTypeAndConfig(t *testing.T) {
	t.Parallel()

	if toProtoType(source.TypeTelegramChannel) != sourcev1.SourceType_SOURCE_TYPE_TELEGRAM_CHANNEL {
		t.Fatal("telegram channel type mismatch")
	}
	if toProtoType(source.TypeTelegramGroup) != sourcev1.SourceType_SOURCE_TYPE_TELEGRAM_GROUP {
		t.Fatal("telegram group type mismatch")
	}
	if toProtoType(source.TypeRSS) != sourcev1.SourceType_SOURCE_TYPE_RSS {
		t.Fatal("rss type mismatch")
	}
	if toProtoType(source.TypeWebScraping) != sourcev1.SourceType_SOURCE_TYPE_WEB_SCRAPING {
		t.Fatal("web scraping type mismatch")
	}
	if toProtoType("other") != sourcev1.SourceType_SOURCE_TYPE_UNSPECIFIED {
		t.Fatal("unknown type must map to unspecified")
	}

	cfg := toProtoConfig(source.TypeTelegramChannel, map[string]any{"channel_id": "c1"})
	if cfg.GetTelegramChannel().GetChannelId() != "c1" {
		t.Fatalf("telegram channel config mismatch: %v", cfg)
	}
	cfg = toProtoConfig(source.TypeTelegramGroup, map[string]any{"group_id": "g1"})
	if cfg.GetTelegramGroup().GetGroupId() != "g1" {
		t.Fatalf("telegram group config mismatch: %v", cfg)
	}
	cfg = toProtoConfig(source.TypeRSS, map[string]any{"feed_url": "https://feed"})
	if cfg.GetRss().GetFeedUrl() != "https://feed" {
		t.Fatalf("rss config mismatch: %v", cfg)
	}
	cfg = toProtoConfig(source.TypeWebScraping, map[string]any{"selector": ".item"})
	if cfg.GetWebScraping().GetSelector() != ".item" {
		t.Fatalf("web config mismatch: %v", cfg)
	}
	if toProtoConfig("other", map[string]any{"x": "y"}) != nil {
		t.Fatal("unknown type config must map to nil")
	}
	if toProtoConfig(source.TypeRSS, nil) != nil {
		t.Fatal("nil config must stay nil")
	}
}

func TestMapErrorAndHelpers(t *testing.T) {
	t.Parallel()

	h := &Handler{}
	if h.mapError(nil) != nil {
		t.Fatal("nil error must stay nil")
	}
	if got := h.mapError(source.ErrNotFound); got == nil {
		t.Fatal("not found must map to connect error")
	}
	if got := h.mapError(source.ValidationError{}); got == nil {
		t.Fatal("validation error must map to connect error")
	}
	if got := h.mapError(errors.New("boom")); got == nil {
		t.Fatal("internal error must map to connect error")
	}
	if !isValidationErr(source.ValidationError{}) {
		t.Fatal("validation helper failed")
	}
	if isValidationErr(errors.New("x")) {
		t.Fatal("non-validation error must be false")
	}
	if str("ok") != "ok" {
		t.Fatal("string helper failed")
	}
	if str(42) != "" {
		t.Fatal("non-string helper must return empty string")
	}
}

func TestFromRequestHelpers(t *testing.T) {
	t.Parallel()

	createReq := &sourcev1.CreateSourceRequest{
		Type: sourcev1.SourceType_SOURCE_TYPE_RSS,
		Name: "n",
		Url:  "https://x",
		Config: &sourcev1.SourceConfig{
			Config: &sourcev1.SourceConfig_Rss{Rss: &sourcev1.RssConfig{FeedUrl: "https://f"}},
		},
	}
	createSrc := fromCreateRequest(createReq)
	if createSrc.Type != source.TypeRSS || createSrc.Config["feed_url"] != "https://f" {
		t.Fatalf("create mapping failed: %+v", createSrc)
	}

	updateReq := &sourcev1.UpdateSourceRequest{
		Type: sourcev1.SourceType_SOURCE_TYPE_WEB_SCRAPING,
		Name: "n2",
		Url:  "https://y",
		Config: &sourcev1.SourceConfig{
			Config: &sourcev1.SourceConfig_WebScraping{
				WebScraping: &sourcev1.WebScrapingConfig{Selector: ".z"},
			},
		},
	}
	updateSrc := fromUpdateRequest(updateReq)
	if updateSrc.Type != source.TypeWebScraping || updateSrc.Config["selector"] != ".z" {
		t.Fatalf("update mapping failed: %+v", updateSrc)
	}
}
