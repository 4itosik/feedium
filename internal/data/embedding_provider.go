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
	"time"

	"github.com/4itosik/feedium/internal/biz"
	"github.com/4itosik/feedium/internal/conf"
)

const defaultEmbeddingTimeout = 30 * time.Second

type embeddingProvider struct {
	apiKey     string
	baseURL    string
	model      string
	httpClient *http.Client
	timeout    time.Duration
	log        *slog.Logger
}

var _ biz.Scorer = (*embeddingProvider)(nil)

type embeddingRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

type embeddingResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

// NewEmbeddingProvider builds the OpenAI-compatible embeddings client (CTR-14).
//
//nolint:revive // unexported return type for wire DI
func NewEmbeddingProvider(cfg *conf.ScoringEmbedding, logger *slog.Logger) (*embeddingProvider, error) {
	if cfg == nil {
		return nil, errors.New("scoring.embedding configuration is required")
	}
	providerName := cfg.GetProvider()
	providerCfg, ok := cfg.GetProviders()[providerName]
	if !ok {
		return nil, fmt.Errorf("embedding provider %q not found in configuration", providerName)
	}

	timeout := cfg.GetTimeout().AsDuration()
	if timeout <= 0 {
		timeout = defaultEmbeddingTimeout
	}

	return &embeddingProvider{
		apiKey:     providerCfg.GetApiKey(),
		baseURL:    providerCfg.GetBaseUrl(),
		model:      providerCfg.GetModel(),
		httpClient: &http.Client{Timeout: timeout},
		timeout:    timeout,
		log:        logger,
	}, nil
}

// Model returns the embedding model name recorded on each score/interest.
func (p *embeddingProvider) Model() string {
	return p.model
}

// Embed returns the embedding vector for text. A timeout/5xx surfaces as a
// transient error (FM-01); an empty vector is a permanent malformed-response
// error (FM-08, classified by biz.IsTransientScoreError as permanent only when
// wrapped in ErrScoreValidation upstream — here it is reported plainly and the
// worker treats provider errors as transient unless validation says otherwise).
func (p *embeddingProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	body, err := json.Marshal(embeddingRequest{Model: p.model, Input: text})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("embedding request timeout: %w", ctx.Err())
		}
		return nil, fmt.Errorf("embedding request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embedding api error: status=%d body=%s", resp.StatusCode, string(respBody))
	}

	var embResp embeddingResponse
	if unmarshalErr := json.Unmarshal(respBody, &embResp); unmarshalErr != nil {
		return nil, fmt.Errorf("unmarshal response: %w", unmarshalErr)
	}
	if len(embResp.Data) == 0 || len(embResp.Data[0].Embedding) == 0 {
		return nil, fmt.Errorf("%w: embedding response has no vector", biz.ErrScoreValidation)
	}
	return embResp.Data[0].Embedding, nil
}
