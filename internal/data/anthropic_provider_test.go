package data_test

import (
	"context"
	"encoding/json"
	"io"
	"maps"
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
)

func anthropicCfg(baseURL string, provider *conf.SummaryLLMProvider) *conf.SummaryLLM {
	provider.Protocol = "anthropic"
	provider.BaseUrl = baseURL
	if provider.GetApiKey() == "" {
		provider.ApiKey = "test-key"
	}
	if provider.GetModel() == "" {
		provider.Model = "claude-haiku-5-5"
	}
	return &conf.SummaryLLM{
		Provider:  "anthropic",
		Timeout:   durationpb.New(2 * time.Second),
		Providers: map[string]*conf.SummaryLLMProvider{"anthropic": provider},
	}
}

func writeMessage(w http.ResponseWriter, msg map[string]any) {
	base := map[string]any{
		"id":            "msg_test",
		"type":          "message",
		"role":          "assistant",
		"model":         "claude-haiku-5-5",
		"stop_sequence": nil,
		"usage":         map[string]any{"input_tokens": 10, "output_tokens": 5},
	}
	maps.Copy(base, msg)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(base)
}

func TestAnthropicProvider_Summarize_SendsMessagesRequest(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	var captured struct {
		path, apiKey, version string
		body                  struct {
			Model     string `json:"model"`
			MaxTokens int    `json:"max_tokens"`
			System    []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"system"`
			Messages []struct {
				Role    string `json:"role"`
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"messages"`
			OutputConfig struct {
				Effort string `json:"effort"`
			} `json:"output_config"`
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.path = r.URL.Path
		captured.apiKey = r.Header.Get("X-Api-Key")
		captured.version = r.Header.Get("Anthropic-Version")
		raw, err := io.ReadAll(r.Body)
		if !assert.NoError(t, err) || !assert.NoError(t, json.Unmarshal(raw, &captured.body)) {
			return
		}
		writeMessage(w, map[string]any{
			"stop_reason": "end_turn",
			"content": []map[string]any{
				{"type": "thinking", "thinking": "", "signature": "sig"},
				{"type": "text", "text": "  summary text  "},
			},
		})
	}))
	defer srv.Close()

	p := newRoutedProvider(t,
		anthropicCfg(srv.URL, &conf.SummaryLLMProvider{Effort: "low"}),
		biz.ProcessingModeCumulative)

	out, err := p.Summarize(context.Background(), "system prompt body", "post1\n\npost2")
	require.NoError(t, err)
	assert.Equal(t, "summary text", out, "text blocks are joined and trimmed; thinking blocks are ignored")

	assert.Equal(t, "/v1/messages", captured.path)
	assert.Equal(t, "test-key", captured.apiKey)
	assert.NotEmpty(t, captured.version, "anthropic-version header must be set")
	assert.Equal(t, "claude-haiku-5-5", captured.body.Model)
	assert.Equal(t, 16000, captured.body.MaxTokens, "default max_tokens")
	require.Len(t, captured.body.System, 1)
	assert.Equal(t, "system prompt body", captured.body.System[0].Text)
	require.Len(t, captured.body.Messages, 1)
	assert.Equal(t, "user", captured.body.Messages[0].Role)
	require.Len(t, captured.body.Messages[0].Content, 1)
	assert.Equal(t, "post1\n\npost2", captured.body.Messages[0].Content[0].Text)
	assert.Equal(t, "low", captured.body.OutputConfig.Effort)
}

func TestAnthropicProvider_Summarize_Errors(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	cases := []struct {
		name    string
		status  int
		message map[string]any
		wantErr string
	}{
		{
			name:   "refusal",
			status: http.StatusOK,
			message: map[string]any{
				"stop_reason":  "refusal",
				"stop_details": map[string]any{"type": "refusal", "category": "cyber", "explanation": "no"},
				"content":      []map[string]any{},
			},
			wantErr: "refused",
		},
		{
			name:   "max_tokens",
			status: http.StatusOK,
			message: map[string]any{
				"stop_reason": "max_tokens",
				"content":     []map[string]any{{"type": "text", "text": "partial"}},
			},
			wantErr: "truncated",
		},
		{
			name:   "empty_text",
			status: http.StatusOK,
			message: map[string]any{
				"stop_reason": "end_turn",
				"content":     []map[string]any{{"type": "text", "text": "   "}},
			},
			wantErr: "empty summary",
		},
		{
			name:    "overloaded_not_retried_by_sdk",
			status:  529,
			wantErr: "llm request",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var hits atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				hits.Add(1)
				if tc.status != http.StatusOK {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(tc.status)
					_, _ = w.Write(
						[]byte(`{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`),
					)
					return
				}
				writeMessage(w, tc.message)
			}))
			defer srv.Close()

			p := newRoutedProvider(
				t,
				anthropicCfg(srv.URL, &conf.SummaryLLMProvider{}),
				biz.ProcessingModeSelfContained,
			)
			out, err := p.Summarize(context.Background(), "sp", "text")
			require.Error(t, err)
			assert.Empty(t, out)
			assert.Contains(t, err.Error(), tc.wantErr)
			assert.Equal(t, int32(1), hits.Load(), "SDK retries must be disabled; biz/ owns retries")
		})
	}
}

func TestAnthropicProvider_Summarize_EmptySystemPrompt_NoRequest(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
	}))
	defer srv.Close()

	p := newRoutedProvider(t, anthropicCfg(srv.URL, &conf.SummaryLLMProvider{}), biz.ProcessingModeSelfContained)
	_, err := p.Summarize(context.Background(), "", "text")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty system prompt")
	assert.Equal(t, int32(0), hits.Load())
}
