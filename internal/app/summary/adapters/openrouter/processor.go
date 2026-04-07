//go:generate mockgen -source=processor.go -destination=mocks/llm_client_mock.go -package=mocks LLMClient

// Package openrouter implements the summary.Processor interface using OpenRouter API.
package openrouter

import (
	"context"
	_ "embed"
	"fmt"
	"strings"
	"text/template"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"

	"feedium/internal/app/post"
	"feedium/internal/app/summary"
	"feedium/internal/components/llm"
)

// LLMClient defines the interface for LLM API clients.
// This interface is defined here to avoid circular dependencies and allow mocking in tests.
type LLMClient interface {
	// CreateChatCompletion creates a chat completion with the given messages.
	CreateChatCompletion(
		ctx context.Context,
		model string,
		messages []openai.ChatCompletionMessageParamUnion,
	) (*openai.ChatCompletion, error)
}

//go:embed prompts/self_contained_system.txt
var selfContainedSystemPrompt string

//go:embed prompts/self_contained_user.txt
var selfContainedUserTemplate string

//go:embed prompts/cumulative_system.txt
var cumulativeSystemPrompt string

//go:embed prompts/cumulative_user.txt
var cumulativeUserTemplate string

const (
	// MaxContentLength is the maximum total content length allowed (32,000 characters).
	MaxContentLength = 32000
	// DefaultModel is the default OpenRouter model.
	DefaultModel = "anthropic/claude-haiku-4-5"
	// OpenRouterBaseURL is the OpenRouter API endpoint.
	OpenRouterBaseURL = "https://openrouter.ai/api/v1"
)

// Processor implements summary.Processor interface using OpenRouter.
type Processor struct {
	client LLMClient
	model  string
}

// NewProcessor creates a new OpenRouter processor.
func NewProcessor(apiKey, model string) *Processor {
	if model == "" {
		model = DefaultModel
	}

	openaiClient := openai.NewClient(
		option.WithAPIKey(apiKey),
		option.WithBaseURL(OpenRouterBaseURL),
	)

	return &Processor{
		client: llm.NewOpenRouterClient(openaiClient),
		model:  model,
	}
}

// NewProcessorWithClient creates a processor with a custom client (for testing).
func NewProcessorWithClient(client LLMClient, model string) *Processor {
	if model == "" {
		model = DefaultModel
	}
	return &Processor{
		client: client,
		model:  model,
	}
}

// selfContainedTemplateData is used for self-contained user prompt.
type selfContainedTemplateData struct {
	Title   string
	Content string
}

// cumulativePost is used for cumulative user prompt.
type cumulativePost struct {
	Author      string
	Content     string
	PublishedAt string
}

// cumulativeTemplateData is used for cumulative user prompt.
type cumulativeTemplateData struct {
	Posts []cumulativePost
}

// Process processes posts and returns a summary using OpenRouter API.
func (p *Processor) Process(ctx context.Context, mode summary.ProcessingMode, posts []post.Post) (string, error) {
	// Calculate total content length
	totalLen := 0
	for _, p := range posts {
		totalLen += len(p.Content)
	}

	if totalLen > MaxContentLength {
		return "", summary.ErrContentTooLarge
	}

	// Select prompts based on mode
	var systemPrompt string
	var userPrompt string
	var err error

	switch mode {
	case summary.ModeSelfContained:
		systemPrompt = selfContainedSystemPrompt
		if len(posts) == 0 {
			return "", summary.ErrPostNotFound
		}
		userPrompt, err = renderSelfContainedTemplate(posts[0])
		if err != nil {
			return "", fmt.Errorf("failed to render self-contained template: %w", err)
		}
	case summary.ModeCumulative:
		systemPrompt = cumulativeSystemPrompt
		userPrompt, err = renderCumulativeTemplate(posts)
		if err != nil {
			return "", fmt.Errorf("failed to render cumulative template: %w", err)
		}
	default:
		return "", summary.ErrUnknownSourceType
	}

	// Build messages
	messages := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(systemPrompt),
		openai.UserMessage(userPrompt),
	}

	// Set custom headers via context (OpenRouter specific headers)
	ctx = setOpenRouterHeaders(ctx)

	resp, err := p.client.CreateChatCompletion(ctx, p.model, messages)
	if err != nil {
		return "", fmt.Errorf("openrouter API error: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", summary.ErrEmptyLLMResponse
	}

	content := resp.Choices[0].Message.Content
	if strings.TrimSpace(content) == "" {
		return "", summary.ErrEmptyLLMResponse
	}

	return content, nil
}

// renderSelfContainedTemplate renders the self-contained user template.
func renderSelfContainedTemplate(p post.Post) (string, error) {
	tmpl, err := template.New("self_contained").Parse(selfContainedUserTemplate)
	if err != nil {
		return "", err
	}

	var buf strings.Builder
	data := selfContainedTemplateData{
		Title:   p.Title,
		Content: p.Content,
	}

	if execErr := tmpl.Execute(&buf, data); execErr != nil {
		return "", execErr
	}

	return buf.String(), nil
}

// renderCumulativeTemplate renders the cumulative user template.
func renderCumulativeTemplate(posts []post.Post) (string, error) {
	tmpl, err := template.New("cumulative").Parse(cumulativeUserTemplate)
	if err != nil {
		return "", err
	}

	cumulativePosts := make([]cumulativePost, len(posts))
	for i, p := range posts {
		cumulativePosts[i] = cumulativePost{
			Author:      p.Author,
			Content:     p.Content,
			PublishedAt: p.PublishedAt.UTC().Format("2006-01-02 15:04"),
		}
	}

	var buf strings.Builder
	data := cumulativeTemplateData{
		Posts: cumulativePosts,
	}

	if execErr := tmpl.Execute(&buf, data); execErr != nil {
		return "", execErr
	}

	return buf.String(), nil
}

// openRouterHeadersKey is a custom type for context keys.
type openRouterHeadersKey struct{}

// setOpenRouterHeaders adds OpenRouter-specific headers to the context.
// Note: The openai-go client doesn't directly support custom headers per request,
// so we wrap the HTTP client to inject these headers.
func setOpenRouterHeaders(ctx context.Context) context.Context {
	return context.WithValue(ctx, openRouterHeadersKey{}, map[string]string{
		"HTTP-Referer": "https://feedium.app",
		"X-Title":      "Feedium",
	})
}
