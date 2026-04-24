// Package sourcetype maps CLI source-type names to proto enums and validates
// per-type flag combinations (SR-03, SR-04, EC-D, EC-E, EC-G, EC-I).
package sourcetype

import (
	"fmt"
	"sort"
	"strings"

	feediumapi "github.com/4itosik/feedium/api/feedium"
)

// entry describes one CLI source type.
type entry struct {
	protoType     feediumapi.SourceType
	requiredFlags []string // sorted lexicographically
	allowedFlags  map[string]bool
}

// registry maps the CLI dash-case type name to its descriptor.
//
//nolint:gochecknoglobals // read-only lookup table
var registry = map[string]entry{
	"telegram-channel": {
		protoType:     feediumapi.SourceType_SOURCE_TYPE_TELEGRAM_CHANNEL,
		requiredFlags: []string{"tg-id", "username"},
		allowedFlags:  map[string]bool{"tg-id": true, "username": true},
	},
	"telegram-group": {
		protoType:     feediumapi.SourceType_SOURCE_TYPE_TELEGRAM_GROUP,
		requiredFlags: []string{"tg-id", "username"},
		allowedFlags:  map[string]bool{"tg-id": true, "username": true},
	},
	"rss": {
		protoType:     feediumapi.SourceType_SOURCE_TYPE_RSS,
		requiredFlags: []string{"feed-url"},
		allowedFlags:  map[string]bool{"feed-url": true},
	},
	"html": {
		protoType:     feediumapi.SourceType_SOURCE_TYPE_HTML,
		requiredFlags: []string{"url"},
		allowedFlags:  map[string]bool{"url": true},
	},
}

// allTypeNames is the stable sorted list of CLI type names (for EC-E messages).
//
//nolint:gochecknoglobals // derived once from read-only registry
var allTypeNames = func() string {
	names := make([]string, 0, len(registry))
	for k := range registry {
		names = append(names, k)
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}()

// allSourceFlags is the complete sorted list of source-related flag names.
// Used to iterate flags in a deterministic order for EC-I checks.
//
//nolint:gochecknoglobals // read-only ordering
var allSourceFlags = []string{"feed-url", "tg-id", "url", "username"}

// Lookup validates a positional <type> argument (create) and returns the proto
// enum. Error format matches EC-E.
func Lookup(name string) (feediumapi.SourceType, error) {
	e, ok := registry[name]
	if !ok {
		return feediumapi.SourceType_SOURCE_TYPE_UNSPECIFIED,
			fmt.Errorf("flag: unknown source type %q (allowed: %s)", name, allTypeNames)
	}
	return e.protoType, nil
}

// LookupFlag validates a CLI dash-case --type flag value (update) and returns
// the proto enum. Error format matches EC-G.
func LookupFlag(name string) (feediumapi.SourceType, error) {
	e, ok := registry[name]
	if !ok {
		return feediumapi.SourceType_SOURCE_TYPE_UNSPECIFIED,
			fmt.Errorf("flag: unknown --type %q", name)
	}
	return e.protoType, nil
}

// enumShortRegistry maps enum short names (without SOURCE_TYPE_ prefix) to
// SourceType, used for the list --type flag (EC-G).
//
//nolint:gochecknoglobals // read-only lookup table
var enumShortRegistry = map[string]feediumapi.SourceType{
	"UNSPECIFIED":      feediumapi.SourceType_SOURCE_TYPE_UNSPECIFIED,
	"TELEGRAM_CHANNEL": feediumapi.SourceType_SOURCE_TYPE_TELEGRAM_CHANNEL,
	"TELEGRAM_GROUP":   feediumapi.SourceType_SOURCE_TYPE_TELEGRAM_GROUP,
	"RSS":              feediumapi.SourceType_SOURCE_TYPE_RSS,
	"HTML":             feediumapi.SourceType_SOURCE_TYPE_HTML,
}

// LookupEnumFlag validates a proto-short-name --type flag value (list) and
// returns the proto enum. Error format matches EC-G.
func LookupEnumFlag(name string) (feediumapi.SourceType, error) {
	t, ok := enumShortRegistry[name]
	if !ok {
		return feediumapi.SourceType_SOURCE_TYPE_UNSPECIFIED,
			fmt.Errorf("flag: unknown --type %q", name)
	}
	return t, nil
}

// CheckFlags validates EC-I (disallowed flags) then EC-D (required missing).
// setFlags maps flag name → true if the user explicitly set it.
// typeName must already be a valid registry key.
func CheckFlags(typeName string, setFlags map[string]bool) error {
	e := registry[typeName]

	// EC-I: any set flag that is not in allowedFlags is an error.
	// Iterate in lexicographic order so messages are deterministic (OQ-S2).
	for _, name := range allSourceFlags {
		if setFlags[name] && !e.allowedFlags[name] {
			return fmt.Errorf("flag: --%s is not allowed for type %q", name, typeName)
		}
	}

	// EC-D: required flags must be set.
	// requiredFlags is already sorted lexicographically per registry definition.
	for _, name := range e.requiredFlags {
		if !setFlags[name] {
			return fmt.Errorf("flag: --%s is required for type %q", name, typeName)
		}
	}

	return nil
}

// Flags holds the raw values of the four source-related CLI flags.
type Flags struct {
	TgID     int64
	Username string
	FeedURL  string
	URL      string
}

// BuildConfig constructs the SourceConfig oneof for the given type.
// typeName must already be a valid registry key.
func BuildConfig(typeName string, f Flags) *feediumapi.SourceConfig {
	switch typeName {
	case "telegram-channel":
		return &feediumapi.SourceConfig{
			Config: &feediumapi.SourceConfig_TelegramChannel{
				TelegramChannel: &feediumapi.TelegramChannelConfig{
					TgId:     f.TgID,
					Username: f.Username,
				},
			},
		}
	case "telegram-group":
		return &feediumapi.SourceConfig{
			Config: &feediumapi.SourceConfig_TelegramGroup{
				TelegramGroup: &feediumapi.TelegramGroupConfig{
					TgId:     f.TgID,
					Username: f.Username,
				},
			},
		}
	case "rss":
		return &feediumapi.SourceConfig{
			Config: &feediumapi.SourceConfig_Rss{
				Rss: &feediumapi.RSSConfig{
					FeedUrl: f.FeedURL,
				},
			},
		}
	case "html":
		return &feediumapi.SourceConfig{
			Config: &feediumapi.SourceConfig_Html{
				Html: &feediumapi.HTMLConfig{
					Url: f.URL,
				},
			},
		}
	default:
		return nil
	}
}
