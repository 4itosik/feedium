package openrouter_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/openai/openai-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"feedium/internal/app/post"
	"feedium/internal/app/summary"
	"feedium/internal/app/summary/adapters/openrouter"
	"feedium/internal/app/summary/adapters/openrouter/mocks"
)

func TestProcessor_Process_SelfContained_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mocks.NewMockLLMClient(ctrl)
	processor := openrouter.NewProcessorWithClient(mockClient, "test-model")

	posts := []post.Post{
		{
			ID:      uuid.New(),
			Title:   "Test Article",
			Content: "This is test content",
		},
	}

	expectedSummary := "Generated summary"
	mockClient.EXPECT().
		CreateChatCompletion(gomock.Any(), "test-model", gomock.Any()).
		Return(&openai.ChatCompletion{
			Choices: []openai.ChatCompletionChoice{
				{
					Message: openai.ChatCompletionMessage{
						Content: expectedSummary,
					},
				},
			},
		}, nil)

	result, err := processor.Process(context.Background(), summary.ModeSelfContained, posts)

	require.NoError(t, err)
	assert.Equal(t, expectedSummary, result)
}

func TestProcessor_Process_Cumulative_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mocks.NewMockLLMClient(ctrl)
	processor := openrouter.NewProcessorWithClient(mockClient, "test-model")

	posts := []post.Post{
		{
			ID:          uuid.New(),
			Author:      "Alice",
			Content:     "Hello!",
			PublishedAt: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		},
		{
			ID:          uuid.New(),
			Author:      "Bob",
			Content:     "Hi Alice!",
			PublishedAt: time.Date(2024, 1, 1, 12, 5, 0, 0, time.UTC),
		},
	}

	expectedSummary := "Conversation summary"
	mockClient.EXPECT().
		CreateChatCompletion(gomock.Any(), "test-model", gomock.Any()).
		Return(&openai.ChatCompletion{
			Choices: []openai.ChatCompletionChoice{
				{
					Message: openai.ChatCompletionMessage{
						Content: expectedSummary,
					},
				},
			},
		}, nil)

	result, err := processor.Process(context.Background(), summary.ModeCumulative, posts)

	require.NoError(t, err)
	assert.Equal(t, expectedSummary, result)
}

func TestProcessor_Process_APIError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mocks.NewMockLLMClient(ctrl)
	processor := openrouter.NewProcessorWithClient(mockClient, "test-model")

	posts := []post.Post{
		{
			ID:      uuid.New(),
			Title:   "Test",
			Content: "Content",
		},
	}

	mockClient.EXPECT().
		CreateChatCompletion(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, errors.New("API error"))

	_, err := processor.Process(context.Background(), summary.ModeSelfContained, posts)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "openrouter API error")
}

func TestProcessor_Process_EmptyChoices(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mocks.NewMockLLMClient(ctrl)
	processor := openrouter.NewProcessorWithClient(mockClient, "test-model")

	posts := []post.Post{
		{
			ID:      uuid.New(),
			Title:   "Test",
			Content: "Content",
		},
	}

	mockClient.EXPECT().
		CreateChatCompletion(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&openai.ChatCompletion{
			Choices: []openai.ChatCompletionChoice{},
		}, nil)

	_, err := processor.Process(context.Background(), summary.ModeSelfContained, posts)

	require.ErrorIs(t, err, summary.ErrEmptyLLMResponse)
}

func TestProcessor_Process_EmptyContent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mocks.NewMockLLMClient(ctrl)
	processor := openrouter.NewProcessorWithClient(mockClient, "test-model")

	posts := []post.Post{
		{
			ID:      uuid.New(),
			Title:   "Test",
			Content: "Content",
		},
	}

	mockClient.EXPECT().
		CreateChatCompletion(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&openai.ChatCompletion{
			Choices: []openai.ChatCompletionChoice{
				{
					Message: openai.ChatCompletionMessage{
						Content: "   ",
					},
				},
			},
		}, nil)

	_, err := processor.Process(context.Background(), summary.ModeSelfContained, posts)

	require.ErrorIs(t, err, summary.ErrEmptyLLMResponse)
}

func TestProcessor_Process_WhitespaceContent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mocks.NewMockLLMClient(ctrl)
	processor := openrouter.NewProcessorWithClient(mockClient, "test-model")

	posts := []post.Post{
		{
			ID:      uuid.New(),
			Title:   "Test",
			Content: "Content",
		},
	}

	mockClient.EXPECT().
		CreateChatCompletion(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&openai.ChatCompletion{
			Choices: []openai.ChatCompletionChoice{
				{
					Message: openai.ChatCompletionMessage{
						Content: "\n\t  \n",
					},
				},
			},
		}, nil)

	_, err := processor.Process(context.Background(), summary.ModeSelfContained, posts)

	require.ErrorIs(t, err, summary.ErrEmptyLLMResponse)
}

func TestProcessor_Process_ContentTooLarge(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mocks.NewMockLLMClient(ctrl)
	processor := openrouter.NewProcessorWithClient(mockClient, "test-model")

	posts := []post.Post{
		{
			ID:      uuid.New(),
			Content: strings.Repeat("a", 32001),
		},
	}

	// No API call expected for content too large

	_, err := processor.Process(context.Background(), summary.ModeSelfContained, posts)

	require.ErrorIs(t, err, summary.ErrContentTooLarge)
}

func TestProcessor_Process_ContentExactlyAtLimit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mocks.NewMockLLMClient(ctrl)
	processor := openrouter.NewProcessorWithClient(mockClient, "test-model")

	posts := []post.Post{
		{
			ID:      uuid.New(),
			Content: strings.Repeat("a", 32000),
		},
	}

	mockClient.EXPECT().
		CreateChatCompletion(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&openai.ChatCompletion{
			Choices: []openai.ChatCompletionChoice{
				{
					Message: openai.ChatCompletionMessage{
						Content: "Summary",
					},
				},
			},
		}, nil).
		Times(1)

	_, err := processor.Process(context.Background(), summary.ModeSelfContained, posts)

	require.NoError(t, err)
}

func TestProcessor_Process_EmptyPostsSelfContained(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mocks.NewMockLLMClient(ctrl)
	processor := openrouter.NewProcessorWithClient(mockClient, "test-model")

	// No API call expected

	_, err := processor.Process(context.Background(), summary.ModeSelfContained, []post.Post{})

	require.ErrorIs(t, err, summary.ErrPostNotFound)
}

func TestProcessor_Process_UnknownMode(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mocks.NewMockLLMClient(ctrl)
	processor := openrouter.NewProcessorWithClient(mockClient, "test-model")

	posts := []post.Post{
		{
			ID:      uuid.New(),
			Content: "Content",
		},
	}

	// No API call expected for unknown mode

	_, err := processor.Process(context.Background(), summary.ProcessingMode("UNKNOWN"), posts)

	require.ErrorIs(t, err, summary.ErrUnknownSourceType)
}

func TestProcessor_Process_SelfContainedWithTitle(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mocks.NewMockLLMClient(ctrl)
	processor := openrouter.NewProcessorWithClient(mockClient, "test-model")

	posts := []post.Post{
		{
			ID:      uuid.New(),
			Title:   "My Great Article",
			Content: "Content of the article",
		},
	}

	mockClient.EXPECT().
		CreateChatCompletion(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&openai.ChatCompletion{
			Choices: []openai.ChatCompletionChoice{
				{
					Message: openai.ChatCompletionMessage{
						Content: "Article summary",
					},
				},
			},
		}, nil)

	result, err := processor.Process(context.Background(), summary.ModeSelfContained, posts)

	require.NoError(t, err)
	assert.Equal(t, "Article summary", result)
}

func TestProcessor_Process_CumulativeWithMultipleAuthors(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mocks.NewMockLLMClient(ctrl)
	processor := openrouter.NewProcessorWithClient(mockClient, "test-model")

	posts := []post.Post{
		{
			ID:          uuid.New(),
			Author:      "Alice",
			Content:     "First message",
			PublishedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
		},
		{
			ID:          uuid.New(),
			Author:      "Bob",
			Content:     "Second message",
			PublishedAt: time.Date(2024, 1, 1, 10, 5, 0, 0, time.UTC),
		},
		{
			ID:          uuid.New(),
			Author:      "Charlie",
			Content:     "Third message",
			PublishedAt: time.Date(2024, 1, 1, 10, 10, 0, 0, time.UTC),
		},
	}

	mockClient.EXPECT().
		CreateChatCompletion(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&openai.ChatCompletion{
			Choices: []openai.ChatCompletionChoice{
				{
					Message: openai.ChatCompletionMessage{
						Content: "Multi-author summary",
					},
				},
			},
		}, nil)

	result, err := processor.Process(context.Background(), summary.ModeCumulative, posts)

	require.NoError(t, err)
	assert.Equal(t, "Multi-author summary", result)
}
