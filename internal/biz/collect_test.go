package biz_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"github.com/4itosik/feedium/internal/biz"
	"github.com/4itosik/feedium/internal/biz/mock"
)

func ptr(s string) *string { return &s }

func TestFilterTextItems(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	tests := []struct {
		name      string
		items     []biz.CollectedItem
		wantTexts []string
	}{
		{
			name:      "empty input",
			items:     nil,
			wantTexts: []string{},
		},
		{
			name: "all text kept",
			items: []biz.CollectedItem{
				{ExternalID: "a", Text: "hello"},
				{ExternalID: "b", Text: "world"},
			},
			wantTexts: []string{"hello", "world"},
		},
		{
			name: "all non-text dropped",
			items: []biz.CollectedItem{
				{ExternalID: "a", Text: ""},
				{ExternalID: "b", Text: "   \n\t"},
			},
			wantTexts: []string{},
		},
		{
			name: "mixed set keeps only text",
			items: []biz.CollectedItem{
				{ExternalID: "a", Text: "kept"},
				{ExternalID: "b", Text: ""},
				{ExternalID: "c", Text: "also kept"},
			},
			wantTexts: []string{"kept", "also kept"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			input := tt.items

			// Act
			got := biz.FilterTextItems(input)

			// Assert
			texts := make([]string, 0, len(got))
			for _, it := range got {
				texts = append(texts, it.Text)
			}
			assert.Equal(t, tt.wantTexts, texts)
		})
	}
}

func TestFilterTextItems_DoesNotMutateInput(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// Arrange
	input := []biz.CollectedItem{
		{ExternalID: "a", Text: "keep"},
		{ExternalID: "b", Text: ""},
	}

	// Act
	_ = biz.FilterTextItems(input)

	// Assert
	assert.Len(t, input, 2)
	assert.Equal(t, "b", input[1].ExternalID)
}

func collectTestSource() biz.Source {
	return biz.Source{
		ID:             "01961d9c-4f78-7e2e-8c3a-5e7d9a1b2c3d",
		Type:           biz.SourceTypeRSS,
		ProcessingMode: biz.ProcessingModeSelfContained,
		Config:         &biz.RSSConfig{FeedURL: "https://example.com/feed"},
	}
}

func TestCollectUsecase_CollectSource(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	src := collectTestSource()
	publishedAt := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)

	t.Run("persists each text item with mapped fields", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		fetcher := mock.NewMockFetcher(ctrl)
		creator := mock.NewMockPostCreator(ctrl)

		fetcher.EXPECT().Fetch(gomock.Any(), src).Return([]biz.CollectedItem{
			{
				ExternalID:  "ext-1",
				Text:        "first body",
				Author:      ptr("Alice"),
				PublishedAt: publishedAt,
				Metadata:    map[string]string{"title": "First"},
			},
			{ExternalID: "ext-2", Text: "second body", PublishedAt: publishedAt},
		}, nil)

		creator.EXPECT().
			Create(gomock.Any(), src.ID, "ext-1", "first body", publishedAt, ptr("Alice"), map[string]string{"title": "First"}).
			Return(biz.Post{ID: "p1"}, nil)
		creator.EXPECT().
			Create(gomock.Any(), src.ID, "ext-2", "second body", publishedAt, gomock.Nil(), gomock.Nil()).
			Return(biz.Post{ID: "p2"}, nil)

		uc := biz.NewCollectUsecase(creator)

		// Act
		err := uc.CollectSource(context.Background(), src, fetcher)

		// Assert
		require.NoError(t, err)
	})

	t.Run("empty fetch persists nothing and is not an error", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		fetcher := mock.NewMockFetcher(ctrl)
		creator := mock.NewMockPostCreator(ctrl)

		// A source with no new material returns an empty slice; Create must not
		// be called and the cycle must succeed.
		fetcher.EXPECT().Fetch(gomock.Any(), src).Return(nil, nil)

		uc := biz.NewCollectUsecase(creator)

		// Act
		err := uc.CollectSource(context.Background(), src, fetcher)

		// Assert
		require.NoError(t, err)
	})

	t.Run("non-text item never reaches Create", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		fetcher := mock.NewMockFetcher(ctrl)
		creator := mock.NewMockPostCreator(ctrl)

		fetcher.EXPECT().Fetch(gomock.Any(), src).Return([]biz.CollectedItem{
			{ExternalID: "img-1", Text: ""},
			{ExternalID: "ext-1", Text: "real text", PublishedAt: publishedAt},
		}, nil)

		creator.EXPECT().
			Create(gomock.Any(), src.ID, "ext-1", "real text", publishedAt, gomock.Nil(), gomock.Nil()).
			Return(biz.Post{ID: "p1"}, nil)

		uc := biz.NewCollectUsecase(creator)

		// Act
		err := uc.CollectSource(context.Background(), src, fetcher)

		// Assert
		require.NoError(t, err)
	})

	t.Run("duplicate item returned by Create is not fatal", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		fetcher := mock.NewMockFetcher(ctrl)
		creator := mock.NewMockPostCreator(ctrl)

		fetcher.EXPECT().Fetch(gomock.Any(), src).Return([]biz.CollectedItem{
			{ExternalID: "ext-1", Text: "already collected", PublishedAt: publishedAt},
		}, nil)

		// Create resolves an existing post (upsert no-op) and returns no error.
		creator.EXPECT().
			Create(gomock.Any(), src.ID, "ext-1", "already collected", publishedAt, gomock.Nil(), gomock.Nil()).
			Return(biz.Post{ID: "existing"}, nil)

		uc := biz.NewCollectUsecase(creator)

		// Act
		err := uc.CollectSource(context.Background(), src, fetcher)

		// Assert
		require.NoError(t, err)
	})

	t.Run("fetch error is wrapped and Create is never called", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		fetcher := mock.NewMockFetcher(ctrl)
		creator := mock.NewMockPostCreator(ctrl)

		fetchErr := errors.New("connection refused")
		fetcher.EXPECT().Fetch(gomock.Any(), src).Return(nil, fetchErr)

		uc := biz.NewCollectUsecase(creator)

		// Act
		err := uc.CollectSource(context.Background(), src, fetcher)

		// Assert
		require.Error(t, err)
		assert.ErrorIs(t, err, fetchErr)
	})

	t.Run("ErrPostSourceNotFound on one item is skipped, others persist", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		fetcher := mock.NewMockFetcher(ctrl)
		creator := mock.NewMockPostCreator(ctrl)

		fetcher.EXPECT().Fetch(gomock.Any(), src).Return([]biz.CollectedItem{
			{ExternalID: "gone", Text: "source removed", PublishedAt: publishedAt},
			{ExternalID: "ext-2", Text: "still here", PublishedAt: publishedAt},
		}, nil)

		creator.EXPECT().
			Create(gomock.Any(), src.ID, "gone", "source removed", publishedAt, gomock.Nil(), gomock.Nil()).
			Return(biz.Post{}, biz.ErrPostSourceNotFound)
		creator.EXPECT().
			Create(gomock.Any(), src.ID, "ext-2", "still here", publishedAt, gomock.Nil(), gomock.Nil()).
			Return(biz.Post{ID: "p2"}, nil)

		uc := biz.NewCollectUsecase(creator)

		// Act
		err := uc.CollectSource(context.Background(), src, fetcher)

		// Assert
		require.NoError(t, err)
	})

	t.Run("unexpected Create error aborts the cycle", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		fetcher := mock.NewMockFetcher(ctrl)
		creator := mock.NewMockPostCreator(ctrl)

		createErr := errors.New("db down")
		fetcher.EXPECT().Fetch(gomock.Any(), src).Return([]biz.CollectedItem{
			{ExternalID: "ext-1", Text: "body", PublishedAt: publishedAt},
		}, nil)
		creator.EXPECT().
			Create(gomock.Any(), src.ID, "ext-1", "body", publishedAt, gomock.Nil(), gomock.Nil()).
			Return(biz.Post{}, createErr)

		uc := biz.NewCollectUsecase(creator)

		// Act
		err := uc.CollectSource(context.Background(), src, fetcher)

		// Assert
		require.Error(t, err)
		assert.ErrorIs(t, err, createErr)
	})
}

func TestCollectUsecase_CollectItems(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	src := collectTestSource()
	publishedAt := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)

	t.Run("non-text item filtered, text item persisted (push path)", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		creator := mock.NewMockPostCreator(ctrl)

		items := []biz.CollectedItem{
			{ExternalID: "img-1", Text: "   \n\t"},
			{
				ExternalID:  "ext-1",
				Text:        "message body",
				Author:      ptr("Bob"),
				PublishedAt: publishedAt,
				Metadata:    map[string]string{"title": "Msg"},
			},
		}

		creator.EXPECT().
			Create(gomock.Any(), src.ID, "ext-1", "message body", publishedAt, ptr("Bob"), map[string]string{"title": "Msg"}).
			Return(biz.Post{ID: "p1"}, nil)

		uc := biz.NewCollectUsecase(creator)

		// Act
		err := uc.CollectItems(context.Background(), src, items)

		// Assert
		require.NoError(t, err)
	})

	t.Run("ErrPostSourceNotFound on an item is skipped, no error returned", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		creator := mock.NewMockPostCreator(ctrl)

		items := []biz.CollectedItem{
			{ExternalID: "gone", Text: "source removed", PublishedAt: publishedAt},
		}

		creator.EXPECT().
			Create(gomock.Any(), src.ID, "gone", "source removed", publishedAt, gomock.Nil(), gomock.Nil()).
			Return(biz.Post{}, biz.ErrPostSourceNotFound)

		uc := biz.NewCollectUsecase(creator)

		// Act
		err := uc.CollectItems(context.Background(), src, items)

		// Assert
		require.NoError(t, err)
	})

	t.Run("unexpected Create error aborts the batch and is returned", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		creator := mock.NewMockPostCreator(ctrl)

		createErr := errors.New("db down")
		items := []biz.CollectedItem{
			{ExternalID: "ext-1", Text: "first", PublishedAt: publishedAt},
		}

		creator.EXPECT().
			Create(gomock.Any(), src.ID, "ext-1", "first", publishedAt, gomock.Nil(), gomock.Nil()).
			Return(biz.Post{}, createErr)

		uc := biz.NewCollectUsecase(creator)

		// Act
		err := uc.CollectItems(context.Background(), src, items)

		// Assert
		require.Error(t, err)
		assert.ErrorIs(t, err, createErr)
	})
}
