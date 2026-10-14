package data

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/4itosik/feedium/internal/biz"
	"github.com/4itosik/feedium/internal/conf"
)

// openRouterProvider speaks the OpenAI-compatible /chat/completions protocol
// (summary.llm protocol "openai", e.g. OpenRouter).
type openRouterProvider struct {
	apiKey     string
	baseURL    string
	model      string
	httpClient *http.Client
	timeout    time.Duration
	log        *slog.Logger
}

var _ biz.LLMProvider = (*openRouterProvider)(nil)

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func newOpenRouterProvider(
	cfg *conf.SummaryLLMProvider, timeout time.Duration, logger *slog.Logger,
) *openRouterProvider {
	return &openRouterProvider{
		apiKey:  cfg.GetApiKey(),
		baseURL: cfg.GetBaseUrl(),
		model:   cfg.GetModel(),
		httpClient: &http.Client{
			Timeout: timeout,
		},
		timeout: timeout,
		log:     logger,
	}
}

func (p *openRouterProvider) Summarize(ctx context.Context, systemPrompt, text string) (string, error) {
	if systemPrompt == "" {
		return "", errors.New("empty system prompt")
	}
	reqBody := chatRequest{
		Model: p.model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: text},
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("llm request timeout: %w", ctx.Err())
		}
		return "", fmt.Errorf("llm request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("llm api error: status=%d body=%s", resp.StatusCode, string(respBody))
	}

	var chatResp chatResponse
	if unmarshalErr := json.Unmarshal(respBody, &chatResp); unmarshalErr != nil {
		return "", fmt.Errorf("unmarshal response: %w", unmarshalErr)
	}

	if len(chatResp.Choices) == 0 {
		return "", errors.New("llm returned empty choices")
	}

	summaryText := strings.TrimSpace(chatResp.Choices[0].Message.Content)
	if summaryText == "" {
		return "", errors.New("llm returned empty summary")
	}

	return summaryText, nil
}
