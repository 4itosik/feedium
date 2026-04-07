// Package llm provides implementations for interacting with LLM APIs.
package llm

import (
	"context"

	"github.com/openai/openai-go"
)

// OpenRouterClient wraps openai.Client to provide a concrete implementation.
type OpenRouterClient struct {
	client openai.Client
}

// NewOpenRouterClient creates a new OpenRouterClient wrapper.
func NewOpenRouterClient(client openai.Client) *OpenRouterClient {
	return &OpenRouterClient{client: client}
}

// CreateChatCompletion creates a chat completion with the given messages.
func (c *OpenRouterClient) CreateChatCompletion(
	ctx context.Context,
	model string,
	messages []openai.ChatCompletionMessageParamUnion,
) (*openai.ChatCompletion, error) {
	req := openai.ChatCompletionNewParams{
		Model:    model,
		Messages: messages,
	}
	return c.client.Chat.Completions.New(ctx, req)
}
