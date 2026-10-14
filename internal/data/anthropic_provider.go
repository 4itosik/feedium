package data

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/4itosik/feedium/internal/biz"
	"github.com/4itosik/feedium/internal/conf"
)

// defaultAnthropicMaxTokens is the output ceiling for a non-streaming request.
// It also bounds adaptive thinking, which counts against max_tokens.
const defaultAnthropicMaxTokens = 16000

// anthropicProvider calls the Claude Messages API through the official SDK
// (FT-021 REQ-01).
type anthropicProvider struct {
	client    anthropic.Client
	model     string
	maxTokens int64
	effort    anthropic.OutputConfigEffort
	timeout   time.Duration
	log       *slog.Logger
}

var _ biz.LLMProvider = (*anthropicProvider)(nil)

func newAnthropicProvider(
	cfg *conf.SummaryLLMProvider, timeout time.Duration, logger *slog.Logger,
) (*anthropicProvider, error) {
	if cfg.GetModel() == "" {
		return nil, errors.New("anthropic provider: model is required")
	}
	effort := anthropic.OutputConfigEffort(cfg.GetEffort())
	switch effort {
	case "", anthropic.OutputConfigEffortLow, anthropic.OutputConfigEffortMedium,
		anthropic.OutputConfigEffortHigh, anthropic.OutputConfigEffortXhigh, anthropic.OutputConfigEffortMax:
	default:
		return nil, fmt.Errorf("anthropic provider: unknown effort %q", cfg.GetEffort())
	}
	maxTokens := int64(cfg.GetMaxTokens())
	if maxTokens <= 0 {
		maxTokens = defaultAnthropicMaxTokens
	}

	// SDK retries are disabled: biz/ owns the retry contract (FT-021 CON-01).
	opts := []option.RequestOption{
		option.WithRequestTimeout(timeout),
		option.WithMaxRetries(0),
	}
	if cfg.GetApiKey() != "" {
		opts = append(opts, option.WithAPIKey(cfg.GetApiKey()))
	}
	if cfg.GetBaseUrl() != "" {
		opts = append(opts, option.WithBaseURL(cfg.GetBaseUrl()))
	}

	return &anthropicProvider{
		client:    anthropic.NewClient(opts...),
		model:     cfg.GetModel(),
		maxTokens: maxTokens,
		effort:    effort,
		timeout:   timeout,
		log:       logger,
	}, nil
}

func (p *anthropicProvider) Summarize(ctx context.Context, systemPrompt, text string) (string, error) {
	if systemPrompt == "" {
		return "", errors.New("empty system prompt")
	}

	params := anthropic.MessageNewParams{
		Model:     p.model,
		MaxTokens: p.maxTokens,
		System:    []anthropic.TextBlockParam{{Text: systemPrompt}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(text)),
		},
	}
	if p.effort != "" {
		params.OutputConfig = anthropic.OutputConfigParam{Effort: p.effort}
	}

	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	resp, err := p.client.Messages.New(ctx, params)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("llm request timeout: %w", ctx.Err())
		}
		return "", fmt.Errorf("llm request: %w", err)
	}

	if resp.StopReason == anthropic.StopReasonRefusal {
		p.log.WarnContext(ctx, "anthropic refused summarization",
			slog.String("model", p.model),
			slog.String("category", string(resp.StopDetails.Category)))
		return "", fmt.Errorf("llm refused request: category=%q", resp.StopDetails.Category)
	}
	if resp.StopReason == anthropic.StopReasonMaxTokens {
		return "", fmt.Errorf("llm output truncated at max_tokens=%d", p.maxTokens)
	}

	var sb strings.Builder
	for _, block := range resp.Content {
		if tb, ok := block.AsAny().(anthropic.TextBlock); ok {
			sb.WriteString(tb.Text)
		}
	}
	summaryText := strings.TrimSpace(sb.String())
	if summaryText == "" {
		return "", errors.New("llm returned empty summary")
	}
	return summaryText, nil
}
