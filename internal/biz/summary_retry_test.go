package biz_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/4itosik/feedium/internal/biz"
)

func TestRetryPolicy_CalculateBackoff_Monotonic(t *testing.T) {
	p := biz.RetryPolicy{
		MaxAttempts: 5,
		BackoffBase: 10 * time.Second,
		BackoffMax:  10 * time.Minute,
	}

	var prev time.Duration
	for attempt := 1; attempt <= p.MaxAttempts; attempt++ {
		d := p.CalculateBackoff(attempt)
		assert.GreaterOrEqual(t, d, prev, "attempt %d: backoff must be monotonic", attempt)
		assert.LessOrEqual(t, d, p.BackoffMax, "attempt %d: backoff must be capped", attempt)
		prev = d
	}
}

func TestRetryPolicy_CalculateBackoff_CappedAtMax(t *testing.T) {
	p := biz.RetryPolicy{
		MaxAttempts: 100,
		BackoffBase: 10 * time.Second,
		BackoffMax:  1 * time.Minute,
	}

	// Large attempt number should not exceed BackoffMax and should not overflow.
	d := p.CalculateBackoff(50)
	assert.Equal(t, p.BackoffMax, d)
}

func TestRetryPolicy_CalculateBackoff_FirstAttempt(t *testing.T) {
	p := biz.RetryPolicy{
		MaxAttempts: 5,
		BackoffBase: 10 * time.Second,
		BackoffMax:  10 * time.Minute,
	}

	assert.Equal(t, 10*time.Second, p.CalculateBackoff(1))
	assert.Equal(t, 20*time.Second, p.CalculateBackoff(2))
	assert.Equal(t, 40*time.Second, p.CalculateBackoff(3))
}

func TestRetryPolicy_ShouldRetry_TerminalAfterMax(t *testing.T) {
	p := biz.RetryPolicy{
		MaxAttempts: 3,
		BackoffBase: 1 * time.Second,
		BackoffMax:  1 * time.Minute,
	}

	transient := errors.New("network timeout")

	assert.True(t, p.ShouldRetry(transient, 1))
	assert.True(t, p.ShouldRetry(transient, 2))
	assert.False(t, p.ShouldRetry(transient, 3), "max_attempts reached")
	assert.False(t, p.ShouldRetry(transient, 4), "beyond max_attempts")
}

func TestRetryPolicy_ShouldRetry_PermanentNotRetried(t *testing.T) {
	p := biz.RetryPolicy{
		MaxAttempts: 5,
		BackoffBase: 1 * time.Second,
		BackoffMax:  1 * time.Minute,
	}

	cases := []struct {
		name string
		err  error
	}{
		{"validation", biz.ErrSummaryValidation},
		{"self_contained", biz.ErrSummarizeSelfContainedSrc},
		{"post_not_found", biz.ErrPostNotFound},
		{"source_not_found", biz.ErrSourceNotFound},
		{"context_cancelled", context.Canceled},
		{"context_deadline", context.DeadlineExceeded},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.False(t, p.ShouldRetry(c.err, 1))
		})
	}
}

func TestRetryPolicy_ShouldRetry_NilErrorNotRetried(t *testing.T) {
	p := biz.RetryPolicy{MaxAttempts: 5, BackoffBase: time.Second, BackoffMax: time.Minute}
	assert.False(t, p.ShouldRetry(nil, 1))
}

func TestIsTransientError(t *testing.T) {
	assert.False(t, biz.IsTransientError(nil))
	assert.False(t, biz.IsTransientError(biz.ErrSummaryValidation))
	assert.False(t, biz.IsTransientError(biz.ErrSummarizeSelfContainedSrc))
	assert.False(t, biz.IsTransientError(biz.ErrPostNotFound))
	assert.False(t, biz.IsTransientError(biz.ErrSourceNotFound))
	assert.False(t, biz.IsTransientError(context.Canceled))
	assert.False(t, biz.IsTransientError(context.DeadlineExceeded))
	assert.True(t, biz.IsTransientError(errors.New("rate limited")))
	// Wrapped transient.
	wrapped := errors.New("wrapped: network error")
	assert.True(t, biz.IsTransientError(wrapped))
}
