package prompt

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/4itosik/feedium/internal/biz"
)

func TestNewResolver_ProvidesAllModes(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	r := NewResolver()
	for _, mode := range biz.AllProcessingModes() {
		t.Run(string(mode), func(t *testing.T) {
			body, err := r.For(mode)
			require.NoError(t, err)
			assert.NotEmpty(t, strings.TrimSpace(body), "body for mode %q must be non-empty", mode)
		})
	}
}

func TestNewResolver_BodiesDiffer(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	assert.NotEqual(t, selfContainedBody, cumulativeBody,
		"self_contained.txt and cumulative.txt must have byte-different bodies (CTR-01)")
}

func TestNewResolver_BodiesNonEmpty(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	assert.NotEmpty(t, strings.TrimSpace(selfContainedBody))
	assert.NotEmpty(t, strings.TrimSpace(cumulativeBody))
}

func TestResolver_For_UnknownMode(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	r := &embeddedResolver{bodies: map[biz.ProcessingMode]string{}}
	body, err := r.For("bogus-mode")
	assert.Empty(t, body)
	require.Error(t, err)
	assert.True(t, errors.Is(err, biz.ErrUnknownSummaryPromptMode),
		"unknown mode must surface biz.ErrUnknownSummaryPromptMode (errors.Is)")
}

func TestResolver_Validate_MissingMode(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// Build a resolver missing one of the modes from AllProcessingModes().
	r := &embeddedResolver{
		bodies: map[biz.ProcessingMode]string{
			biz.ProcessingModeSelfContained: "x",
		},
	}
	err := r.validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing prompt for mode")
}

func TestResolver_Validate_EmptyBody(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	r := &embeddedResolver{
		bodies: map[biz.ProcessingMode]string{
			biz.ProcessingModeSelfContained: "x",
			biz.ProcessingModeCumulative:    "  \t\n",
		},
	}
	err := r.validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty prompt body for mode")
}

func TestNewResolver_PanicsOnDriftingState(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// Helper that wraps validate() in the same panic semantics as NewResolver.
	construct := func(bodies map[biz.ProcessingMode]string) {
		r := &embeddedResolver{bodies: bodies}
		if err := r.validate(); err != nil {
			panic(err)
		}
	}

	assert.Panics(t, func() {
		construct(map[biz.ProcessingMode]string{
			biz.ProcessingModeSelfContained: "",
			biz.ProcessingModeCumulative:    "ok",
		})
	}, "empty body must panic")

	assert.Panics(t, func() {
		construct(map[biz.ProcessingMode]string{
			biz.ProcessingModeSelfContained: "ok",
		})
	}, "missing mode must panic")
}

func TestNewResolver_Production_DoesNotPanic(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	assert.NotPanics(t, func() {
		_ = NewResolver()
	})
}
