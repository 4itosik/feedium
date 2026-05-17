package data_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/4itosik/feedium/internal/conf"
	"github.com/4itosik/feedium/internal/data"
)

type summarizer interface {
	Summarize(ctx context.Context, systemPrompt, text string) (string, error)
}

func newProvider(t *testing.T, baseURL string) summarizer {
	t.Helper()
	cfg := &conf.SummaryLLM{
		Provider: "openrouter",
		Timeout:  durationpb.New(2 * time.Second),
		Providers: map[string]*conf.SummaryLLMProvider{
			"openrouter": {
				ApiKey:  "test-key",
				BaseUrl: baseURL,
				Model:   "test-model",
			},
		},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
	p, err := data.NewOpenRouterProvider(cfg, logger)
	require.NoError(t, err)
	return p
}

func TestOpenRouterProvider_Summarize_SendsSystemPrompt(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	cases := []struct {
		name         string
		systemPrompt string
		userText     string
	}{
		{
			name:         "self_contained_body",
			systemPrompt: "self-contained system prompt body",
			userText:     "post text",
		},
		{
			name:         "cumulative_body",
			systemPrompt: "cumulative system prompt body",
			userText:     "post1\n\npost2",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var captured struct {
				role0    string
				content0 string
				role1    string
				content1 string
				calls    int32
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&captured.calls, 1)
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				var req struct {
					Model    string `json:"model"`
					Messages []struct {
						Role    string `json:"role"`
						Content string `json:"content"`
					} `json:"messages"`
				}
				require.NoError(t, json.Unmarshal(body, &req))
				require.Len(t, req.Messages, 2)
				captured.role0 = req.Messages[0].Role
				captured.content0 = req.Messages[0].Content
				captured.role1 = req.Messages[1].Role
				captured.content1 = req.Messages[1].Content

				resp := map[string]any{
					"choices": []map[string]any{
						{"message": map[string]any{"content": "ok"}},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(resp)
			}))
			defer srv.Close()

			p := newProvider(t, srv.URL)
			out, err := p.Summarize(context.Background(), tc.systemPrompt, tc.userText)
			require.NoError(t, err)
			assert.Equal(t, "ok", out)
			assert.Equal(t, "system", captured.role0)
			assert.Equal(t, tc.systemPrompt, captured.content0,
				"messages[0].content must equal the passed system prompt verbatim")
			assert.Equal(t, "user", captured.role1)
			assert.Equal(t, tc.userText, captured.content1)
		})
	}
}

func TestOpenRouterProvider_Summarize_EmptySystemPrompt_ReturnsError(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
	}))
	defer srv.Close()

	p := newProvider(t, srv.URL)
	out, err := p.Summarize(context.Background(), "", "anything")
	require.Error(t, err)
	assert.Empty(t, out)
	assert.True(t, strings.Contains(err.Error(), "empty system prompt"),
		"error must explicitly mention empty system prompt")
	assert.Equal(t, int32(0), atomic.LoadInt32(&hits),
		"no HTTP request must fire when systemPrompt is empty")
}
