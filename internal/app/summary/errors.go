package summary

import "errors"

var (
	// ErrPostNotFound is returned when a post cannot be found (permanent error, no retry increment).
	ErrPostNotFound = errors.New("post not found")

	// ErrUnknownSourceType is returned when source type is not recognized (permanent error, no retry increment).
	ErrUnknownSourceType = errors.New("unknown source type")

	// ErrSourceNotFound is returned when a source cannot be found (permanent error, no retry increment).
	ErrSourceNotFound = errors.New("source not found")
)

// IsPermanentError returns true if the error is a permanent error that should not increment retry_count.
func IsPermanentError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, ErrPostNotFound) ||
		errors.Is(err, ErrUnknownSourceType) ||
		errors.Is(err, ErrSourceNotFound)
}
