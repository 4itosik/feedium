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

const defaultSystemPrompt = `You are a concise summarizer. Summarize the provided text in the same language as the original. Preserve key facts, names, numbers, and conclusions. Produce a concise summary.`

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

func NewOpenRouterProvider(cfg *conf.SummaryLLM, logger *slog.Logger) (*openRouterProvider, error) {
	if cfg == nil {
		return nil, errors.New("summary.llm configuration is required")
	}
	providerName := cfg.GetProvider()
	providerCfg, ok := cfg.GetProviders()[providerName]
	if !ok {
		return nil, fmt.Errorf("llm provider %q not found in configuration", providerName)
	}

	timeout := cfg.GetTimeout().AsDuration()
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &openRouterProvider{
		apiKey:  providerCfg.GetApiKey(),
		baseURL: providerCfg.GetBaseUrl(),
		model:   providerCfg.GetModel(),
		httpClient: &http.Client{
			Timeout: timeout,
		},
		timeout: timeout,
		log:     logger,
	}, nil
}

func (p *openRouterProvider) Summarize(ctx context.Context, text string) (string, error) {
	reqBody := chatRequest{
		Model: p.model,
		Messages: []chatMessage{
			{Role: "system", Content: defaultSystemPrompt},
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
