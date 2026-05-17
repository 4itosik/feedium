// Package prompt owns the system prompts used by biz.SummaryUsecase.
//
// It is intentionally a leaf package: it implements biz.SummaryPromptResolver
// and is wired in via cmd/feedium/wire.go to avoid an internal/biz <->
// internal/biz/prompt import cycle. The package imports biz; biz does not
// import the package.
package prompt

import (
	_ "embed"
	"errors"
	"fmt"
	"strings"

	"github.com/4itosik/feedium/internal/biz"
)

//go:embed self_contained.txt
var selfContainedBody string

//go:embed cumulative.txt
var cumulativeBody string

// embeddedResolver is the production SummaryPromptResolver, backed by the
// two embedded .txt files. Construction is total: any missing or empty
// body, or any biz.ProcessingMode not represented in the internal map,
// panics at NewResolver time (boot-time enforcement of FM-01/FM-02).
type embeddedResolver struct {
	bodies map[biz.ProcessingMode]string
}

// NewResolver returns the production resolver. It panics if any prompt
// body is empty after strings.TrimSpace, or if its internal map does not
// cover every biz.ProcessingMode in biz.AllProcessingModes(). Both
// failures must be loud at process startup, never silent at event time.
//
//nolint:revive // unexported return type for wire DI
func NewResolver() *embeddedResolver {
	r := &embeddedResolver{
		bodies: map[biz.ProcessingMode]string{
			biz.ProcessingModeSelfContained: selfContainedBody,
			biz.ProcessingModeCumulative:    cumulativeBody,
		},
	}
	if err := r.validate(); err != nil {
		panic(fmt.Errorf("prompt.NewResolver: %w", err))
	}
	return r
}

// For returns the embedded body for the given mode. The raw embedded
// string is returned without trimming; an unknown mode produces an empty
// body and biz.ErrUnknownSummaryPromptMode.
func (r *embeddedResolver) For(mode biz.ProcessingMode) (string, error) {
	body, ok := r.bodies[mode]
	if !ok {
		return "", biz.ErrUnknownSummaryPromptMode
	}
	return body, nil
}

func (r *embeddedResolver) validate() error {
	for _, m := range biz.AllProcessingModes() {
		body, ok := r.bodies[m]
		if !ok {
			return fmt.Errorf("missing prompt for mode %q", m)
		}
		if strings.TrimSpace(body) == "" {
			return fmt.Errorf("empty prompt body for mode %q", m)
		}
	}
	if len(r.bodies) != len(biz.AllProcessingModes()) {
		return errors.New("resolver bodies do not match biz.AllProcessingModes()")
	}
	return nil
}
