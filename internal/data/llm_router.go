package data

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/4itosik/feedium/internal/biz"
	"github.com/4itosik/feedium/internal/conf"
)

const defaultLLMTimeout = 30 * time.Second

// Wire protocols a summary.llm provider entry can speak (FT-021).
const (
	llmProtocolOpenAI    = "openai"
	llmProtocolAnthropic = "anthropic"
)

// summaryLLMRouter maps each ProcessingMode to the LLMProvider configured for
// it: summary.llm.mode_providers[mode], falling back to summary.llm.provider
// (FT-021 REQ-02). All clients are built up front, so a broken config fails
// at startup rather than on the first event.
type summaryLLMRouter struct {
	byMode map[biz.ProcessingMode]biz.LLMProvider
}

var _ biz.SummaryLLMResolver = (*summaryLLMRouter)(nil)

// NewSummaryLLMRouter builds one client per provider referenced by any mode.
//
//nolint:revive // unexported return type for wire DI
func NewSummaryLLMRouter(cfg *conf.SummaryLLM, logger *slog.Logger) (*summaryLLMRouter, error) {
	if cfg == nil {
		return nil, errors.New("summary.llm configuration is required")
	}

	timeout := cfg.GetTimeout().AsDuration()
	if timeout <= 0 {
		timeout = defaultLLMTimeout
	}

	modes := biz.AllProcessingModes()
	known := make(map[string]bool, len(modes))
	for _, mode := range modes {
		known[string(mode)] = true
	}
	for mode := range cfg.GetModeProviders() {
		if !known[mode] {
			return nil, fmt.Errorf("summary.llm.mode_providers: unknown processing mode %q", mode)
		}
	}

	clients := make(map[string]biz.LLMProvider)
	byMode := make(map[biz.ProcessingMode]biz.LLMProvider, len(modes))
	for _, mode := range modes {
		name := cfg.GetModeProviders()[string(mode)]
		if name == "" {
			name = cfg.GetProvider()
		}
		if name == "" {
			return nil, fmt.Errorf("summary.llm: no provider configured for mode %q", mode)
		}
		client, ok := clients[name]
		if !ok {
			providerCfg, found := cfg.GetProviders()[name]
			if !found {
				return nil, fmt.Errorf("llm provider %q not found in configuration", name)
			}
			var err error
			client, err = newLLMProvider(providerCfg, timeout, logger)
			if err != nil {
				return nil, fmt.Errorf("llm provider %q: %w", name, err)
			}
			clients[name] = client
		}
		byMode[mode] = client
		logger.Info("summary llm provider selected",
			slog.String("mode", string(mode)),
			slog.String("provider", name),
			slog.String("model", cfg.GetProviders()[name].GetModel()))
	}

	return &summaryLLMRouter{byMode: byMode}, nil
}

// For returns the provider configured for mode.
func (r *summaryLLMRouter) For(mode biz.ProcessingMode) (biz.LLMProvider, error) {
	p, ok := r.byMode[mode]
	if !ok {
		return nil, fmt.Errorf("no llm provider for processing mode %q", mode)
	}
	return p, nil
}

func newLLMProvider(
	cfg *conf.SummaryLLMProvider, timeout time.Duration, logger *slog.Logger,
) (biz.LLMProvider, error) {
	switch cfg.GetProtocol() {
	case "", llmProtocolOpenAI:
		return newOpenRouterProvider(cfg, timeout, logger), nil
	case llmProtocolAnthropic:
		return newAnthropicProvider(cfg, timeout, logger)
	default:
		return nil, fmt.Errorf("unknown protocol %q (want %q or %q)",
			cfg.GetProtocol(), llmProtocolOpenAI, llmProtocolAnthropic)
	}
}
