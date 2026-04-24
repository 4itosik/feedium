// Package resolve computes effective CLI settings with the priority
// flag > env > config > default (INV-05).
package resolve

import (
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/4itosik/feedium/cmd/feediumctl/internal/config"
)

const (
	DefaultEndpoint = "localhost:9000"
	DefaultOutput   = "table"
	DefaultTimeout  = time.Minute
	DefaultPageSize = 50

	EnvEndpoint = "FEEDIUMCTL_ENDPOINT"
	EnvOutput   = "FEEDIUMCTL_OUTPUT"
	EnvTimeout  = "FEEDIUMCTL_TIMEOUT"
	EnvPageSize = "FEEDIUMCTL_PAGE_SIZE"
)

// FlagSource carries the raw per-flag values together with a flag identifying
// whether the user set each flag explicitly (as reported by cobra).
type FlagSource struct {
	Endpoint    string
	EndpointSet bool

	Output    string
	OutputSet bool

	Timeout    string
	TimeoutSet bool

	PageSize    string
	PageSizeSet bool
}

// Settings are the effective values used by commands.
type Settings struct {
	Endpoint string
	Output   string
	Timeout  time.Duration
	PageSize int
}

// Resolve applies flag > env > config > default for the four parameters.
// Error strings follow NFR-03:
//   - "flag: invalid timeout \"<raw>\": <reason>"
//   - "flag: invalid page size \"<raw>\": <reason>"
//
// Output value validation lives in Validate to keep the "output:" prefix
// exclusive to FR-04 violations (Step 6).
func Resolve(flags FlagSource, cfg config.File, getenv func(string) string) (Settings, error) {
	var s Settings

	s.Endpoint = pickString(flags.Endpoint, flags.EndpointSet, getenv(EnvEndpoint), cfg.Endpoint, DefaultEndpoint)
	s.Output = pickString(flags.Output, flags.OutputSet, getenv(EnvOutput), cfg.Output, DefaultOutput)

	t, err := pickTimeout(flags.Timeout, flags.TimeoutSet, getenv(EnvTimeout), cfg.Timeout)
	if err != nil {
		return Settings{}, err
	}
	s.Timeout = t

	p, err := pickPageSize(flags.PageSize, flags.PageSizeSet, getenv(EnvPageSize), cfg.PageSize)
	if err != nil {
		return Settings{}, err
	}
	s.PageSize = p

	return s, nil
}

func pickString(flagValue string, flagSet bool, envValue string, cfgValue *string, def string) string {
	if flagSet {
		return flagValue
	}
	if envValue != "" {
		return envValue
	}
	if cfgValue != nil {
		return *cfgValue
	}
	return def
}

func pickTimeout(flagValue string, flagSet bool, envValue string, cfgValue *time.Duration) (time.Duration, error) {
	if flagSet {
		d, err := time.ParseDuration(flagValue)
		if err != nil {
			return 0, fmt.Errorf("flag: invalid timeout %q: %s", flagValue, err.Error())
		}
		return d, nil
	}
	if envValue != "" {
		d, err := time.ParseDuration(envValue)
		if err != nil {
			return 0, fmt.Errorf("flag: invalid timeout %q: %s", envValue, err.Error())
		}
		return d, nil
	}
	if cfgValue != nil {
		return *cfgValue, nil
	}
	return DefaultTimeout, nil
}

func pickPageSize(flagValue string, flagSet bool, envValue string, cfgValue *int) (int, error) {
	if flagSet {
		return parsePageSize(flagValue)
	}
	if envValue != "" {
		return parsePageSize(envValue)
	}
	if cfgValue != nil {
		if err := checkPageSizeRange(*cfgValue, strconv.Itoa(*cfgValue)); err != nil {
			return 0, err
		}
		return *cfgValue, nil
	}
	return DefaultPageSize, nil
}

func parsePageSize(raw string) (int, error) {
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("flag: invalid page size %q: %s", raw, err.Error())
	}
	if rangeErr := checkPageSizeRange(n, raw); rangeErr != nil {
		return 0, rangeErr
	}
	return n, nil
}

// checkPageSizeRange guards against int32 truncation on the wire (page_size
// is int32 in the proto). Negative values and values above MaxInt32 are
// rejected locally; the upper bound for valid pages is still validated by
// the server (FR-03).
func checkPageSizeRange(n int, raw string) error {
	if n < 0 {
		return fmt.Errorf("flag: invalid page size %q: must be non-negative", raw)
	}
	if n > math.MaxInt32 {
		return fmt.Errorf("flag: invalid page size %q: out of range", raw)
	}
	return nil
}
