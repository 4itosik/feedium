package data_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/4itosik/feedium/internal/biz"
	"github.com/4itosik/feedium/internal/conf"
	"github.com/4itosik/feedium/internal/data"
)

func embeddingConfig(baseURL string) *conf.ScoringEmbedding {
	return &conf.ScoringEmbedding{
		Timeout:  durationpb.New(0),
		Provider: "openai",
		Providers: map[string]*conf.ScoringEmbeddingProvider{
			"openai": {ApiKey: "k", BaseUrl: baseURL, Model: "text-embedding-3-small"},
		},
	}
}

func TestEmbeddingProvider_Embed_Success(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// Arrange
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0.1,0.2,0.3]}]}`))
	}))
	defer srv.Close()
	provider, err := data.NewEmbeddingProvider(embeddingConfig(srv.URL), slog.Default())
	require.NoError(t, err)

	// Act
	vec, err := provider.Embed(context.Background(), "hello")

	// Assert
	require.NoError(t, err)
	assert.Equal(t, []float32{0.1, 0.2, 0.3}, vec)
	assert.Equal(t, "text-embedding-3-small", provider.Model())
}

func TestEmbeddingProvider_Embed_ServerError(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// Arrange
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	provider, err := data.NewEmbeddingProvider(embeddingConfig(srv.URL), slog.Default())
	require.NoError(t, err)

	// Act
	_, err = provider.Embed(context.Background(), "hello")

	// Assert
	require.Error(t, err)
	assert.True(t, biz.IsTransientScoreError(err), "5xx must be transient")
}

func TestEmbeddingProvider_Embed_Malformed(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// Arrange
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()
	provider, err := data.NewEmbeddingProvider(embeddingConfig(srv.URL), slog.Default())
	require.NoError(t, err)

	// Act
	_, err = provider.Embed(context.Background(), "hello")

	// Assert
	require.Error(t, err)
	assert.ErrorIs(t, err, biz.ErrScoreValidation, "empty vector is a permanent validation error")
}
