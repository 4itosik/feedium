package summary

import (
	"feedium/internal/app/post"
)

// NewOutboxBuilder creates a function that builds OutboxEvent for a post.
// Always creates an IMMEDIATE event; worker will decide how to process based on source type.
func NewOutboxBuilder(_ SourceQueryRepository) func(*post.Post) (any, error) {
	return func(p *post.Post) (any, error) {
		// Always create an IMMEDIATE event for any post
		// The worker will check the source type and decide whether to process it
		return &OutboxEvent{
			SourceID:  p.SourceID,
			PostID:    &p.ID,
			EventType: EventTypeImmediate,
			Status:    EventStatusPending,
		}, nil
	}
}
