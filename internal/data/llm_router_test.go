package data_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/4itosik/feedium/internal/biz"
	"github.com/4itosik/feedium/internal/conf"
	"github.com/4itosik/feedium/internal/data"
)

// fakeLLMServers runs one OpenAI-compatible and one Anthropic endpoint and
// counts the hits on each.
type fakeLLMServers struct {
	openai, anthropic         *httptest.Server
	openaiHits, anthropicHits atomic.Int32
}

func newFakeLLMServers(t *testing.T) *fakeLLMServers {
	t.Helper()
	f := &fakeLLMServers{}
	f.openai = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		f.openaiHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": "from-openai"}}},
		})
	}))
	f.anthropic = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		f.anthropicHits.Add(1)
		writeMessage(w, map[string]any{
			"stop_reason": "end_turn",
			"content":     []map[string]any{{"type": "text", "text": "from-anthropic"}},
		})
	}))
	return f
}

// close must run before goleak.VerifyNone, so callers defer it (t.Cleanup
// would run after the deferred leak check).
func (f *fakeLLMServers) close() {
	f.openai.Close()
	f.anthropic.Close()
}

func (f *fakeLLMServers) cfg(defaultProvider string, modeProviders map[string]string) *conf.SummaryLLM {
	return &conf.SummaryLLM{
		Provider:      defaultProvider,
		ModeProviders: modeProviders,
		Timeout:       durationpb.New(2 * time.Second),
		Providers: map[string]*conf.SummaryLLMProvider{
			"openrouter": {ApiKey: "k", BaseUrl: f.openai.URL, Model: "m"},
			"anthropic":  {Protocol: "anthropic", ApiKey: "k", BaseUrl: f.anthropic.URL, Model: "claude-haiku-5-5"},
		},
	}
}

func summarizeVia(t *testing.T, router interface {
	For(biz.ProcessingMode) (biz.LLMProvider, error)
}, mode biz.ProcessingMode,
) string {
	t.Helper()
	p, err := router.For(mode)
	require.NoError(t, err)
	out, err := p.Summarize(context.Background(), "sp", "text")
	require.NoError(t, err)
	return out
}

func TestSummaryLLMRouter_PerModeProviders(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	f := newFakeLLMServers(t)
	defer f.close()
	router, err := data.NewSummaryLLMRouter(f.cfg("openrouter", map[string]string{
		string(biz.ProcessingModeSelfContained): "anthropic",
	}), discardLogger())
	require.NoError(t, err)

	assert.Equal(t, "from-anthropic", summarizeVia(t, router, biz.ProcessingModeSelfContained),
		"self_contained uses its mode_providers override")
	assert.Equal(t, "from-openai", summarizeVia(t, router, biz.ProcessingModeCumulative),
		"cumulative falls back to summary.llm.provider")
	assert.Equal(t, int32(1), f.anthropicHits.Load())
	assert.Equal(t, int32(1), f.openaiHits.Load())
}

func TestSummaryLLMRouter_DefaultProviderForAllModes(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	f := newFakeLLMServers(t)
	defer f.close()
	router, err := data.NewSummaryLLMRouter(f.cfg("anthropic", nil), discardLogger())
	require.NoError(t, err)

	for _, mode := range biz.AllProcessingModes() {
		assert.Equal(t, "from-anthropic", summarizeVia(t, router, mode), "mode %q", mode)
	}
	assert.Equal(t, int32(0), f.openaiHits.Load())
}

func TestSummaryLLMRouter_InvalidConfig(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	valid := func() *conf.SummaryLLM {
		return &conf.SummaryLLM{
			Provider: "anthropic",
			Providers: map[string]*conf.SummaryLLMProvider{
				"anthropic":  {Protocol: "anthropic", ApiKey: "k", Model: "claude-haiku-5-5"},
				"openrouter": {ApiKey: "k", BaseUrl: "http://x", Model: "m"},
			},
		}
	}

	cases := []struct {
		name    string
		mutate  func(c *conf.SummaryLLM) *conf.SummaryLLM
		wantErr string
	}{
		{
			name:    "nil_config",
			mutate:  func(*conf.SummaryLLM) *conf.SummaryLLM { return nil },
			wantErr: "required",
		},
		{
			name: "unknown_mode_key",
			mutate: func(c *conf.SummaryLLM) *conf.SummaryLLM {
				c.ModeProviders = map[string]string{"cumulativ": "anthropic"}
				return c
			},
			wantErr: "unknown processing mode",
		},
		{
			name: "unknown_provider_name",
			mutate: func(c *conf.SummaryLLM) *conf.SummaryLLM {
				c.ModeProviders = map[string]string{string(biz.ProcessingModeCumulative): "missing"}
				return c
			},
			wantErr: `"missing" not found`,
		},
		{
			name: "no_provider_for_mode",
			mutate: func(c *conf.SummaryLLM) *conf.SummaryLLM {
				c.Provider = ""
				return c
			},
			wantErr: "no provider configured",
		},
		{
			name: "unknown_protocol",
			mutate: func(c *conf.SummaryLLM) *conf.SummaryLLM {
				c.Providers["anthropic"].Protocol = "grpc"
				return c
			},
			wantErr: "unknown protocol",
		},
		{
			name: "unknown_effort",
			mutate: func(c *conf.SummaryLLM) *conf.SummaryLLM {
				c.Providers["anthropic"].Effort = "extreme"
				return c
			},
			wantErr: "unknown effort",
		},
		{
			name: "anthropic_without_model",
			mutate: func(c *conf.SummaryLLM) *conf.SummaryLLM {
				c.Providers["anthropic"].Model = ""
				return c
			},
			wantErr: "model is required",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := data.NewSummaryLLMRouter(tc.mutate(valid()), discardLogger())
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}
