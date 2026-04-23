package biz

import (
	"context"
	"errors"
	"time"
)

// RetryPolicy describes how transient failures should be retried.
const backoffGrowthFactor = 2

type RetryPolicy struct {
	MaxAttempts int
	BackoffBase time.Duration
	BackoffMax  time.Duration
}

// CalculateBackoff returns the backoff duration for the given attempt number
// (1-based). The policy is exponential with base p.BackoffBase and cap p.BackoffMax:
//
//	attempt=1 -> base
//	attempt=2 -> 2*base
//	attempt=3 -> 4*base
//	...
//	never exceeds BackoffMax.
func (p RetryPolicy) CalculateBackoff(attempt int) time.Duration {
	if attempt <= 0 {
		return p.BackoffBase
	}
	d := p.BackoffBase
	for i := 1; i < attempt; i++ {
		next := d * backoffGrowthFactor
		if next < d || next > p.BackoffMax {
			return p.BackoffMax
		}
		d = next
	}
	if d > p.BackoffMax {
		return p.BackoffMax
	}
	return d
}

// ShouldRetry decides whether a failing attempt should be retried. It returns
// false when the error is not transient or when max_attempts has been exhausted.
func (p RetryPolicy) ShouldRetry(err error, attempt int) bool {
	if err == nil {
		return false
	}
	if !IsTransientError(err) {
		return false
	}
	return attempt < p.MaxAttempts
}

// IsTransientError classifies errors into transient (retryable) vs permanent.
// Context cancellation/deadline are treated as permanent to avoid further retries
// once the owning context is gone. Business sentinels (ErrSummaryValidation,
// ErrSummarizeSelfContainedSrc) are permanent. All other errors are assumed
// transient.
func IsTransientError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if errors.Is(err, ErrSummaryValidation) ||
		errors.Is(err, ErrSummarizeSelfContainedSrc) ||
		errors.Is(err, ErrPostNotFound) ||
		errors.Is(err, ErrSourceNotFound) {
		return false
	}
	return true
}
