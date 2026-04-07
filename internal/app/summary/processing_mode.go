package summary

import (
	"fmt"

	"feedium/internal/app/source"
)

type ProcessingMode string

const (
	ModeSelfContained ProcessingMode = "SELF_CONTAINED"
	ModeCumulative    ProcessingMode = "CUMULATIVE"
)

// ProcessingModeForSourceType returns the processing mode for a given source type.
// Returns error for unknown source types (permanent error).
func ProcessingModeForSourceType(sourceType source.Type) (ProcessingMode, error) {
	switch sourceType {
	case source.TypeTelegramChannel:
		return ModeSelfContained, nil
	case source.TypeRSS:
		return ModeSelfContained, nil
	case source.TypeWebScraping:
		return ModeSelfContained, nil
	case source.TypeTelegramGroup:
		return ModeCumulative, nil
	default:
		return "", fmt.Errorf("unknown source type: %s", sourceType)
	}
}
