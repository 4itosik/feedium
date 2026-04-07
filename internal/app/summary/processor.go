//go:generate mockgen -source=processor.go -destination=mocks/processor_mock.go -package=mocks

package summary

import (
	"context"

	"feedium/internal/app/post"
)

// Processor processes a list of posts and returns a summary content.
type Processor interface {
	// Process processes a list of posts and returns a summary content.
	// mode determines which prompt templates to use (SelfContained or Cumulative).
	Process(ctx context.Context, mode ProcessingMode, posts []post.Post) (content string, err error)
}
