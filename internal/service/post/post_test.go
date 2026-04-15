package post_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/4itosik/feedium/api/feedium"
	"github.com/4itosik/feedium/internal/biz"
	"github.com/4itosik/feedium/internal/service/post"
	"github.com/4itosik/feedium/internal/service/post/mock"
)

func makeDomainPost(id, sourceID, externalID, text string) biz.Post {
	return biz.Post{
		ID:          id,
		SourceID:    sourceID,
		Source:      biz.SourceInfo{ID: sourceID, Type: biz.SourceTypeRSS},
		ExternalID:  externalID,
		PublishedAt: time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC),
		Author:      nil,
		Text:        text,
		Metadata:    map[string]string{},
		CreatedAt:   time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC),
	}
}

func TestV1CreatePost(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	sourceID := "01961d9c-4f78-7e2e-8c3a-5e7d9a1b2c3d"
	publishedAt := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)

	t.Run("valid", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		uc := mock.NewMockUsecase(ctrl)
		uc.EXPECT().
			Create(gomock.Any(), sourceID, "ext-1", "text", publishedAt, gomock.Any(), gomock.Any()).
			Return(makeDomainPost("post-1", sourceID, "ext-1", "text"), nil)

		svc := post.NewPostService(uc)
		resp, err := svc.V1CreatePost(ctx, &feedium.V1CreatePostRequest{
			SourceId:    sourceID,
			ExternalId:  "ext-1",
			PublishedAt: timestamppb.New(publishedAt),
			Text:        "text",
		})

		require.NoError(t, err)
		assert.Equal(t, "post-1", resp.GetPost().GetId())
		assert.Equal(t, sourceID, resp.GetPost().GetSource().GetId())
		assert.Equal(t, feedium.SourceType_SOURCE_TYPE_RSS, resp.GetPost().GetSource().GetType())
	})

	t.Run("validation error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		uc := mock.NewMockUsecase(ctrl)
		// Validation fails before repo call, so usecase returns ErrPostInvalidArgument
		uc.EXPECT().
			Create(gomock.Any(), "", "ext-1", "", publishedAt, gomock.Any(), gomock.Any()).
			Return(biz.Post{}, biz.ErrPostInvalidArgument)

		svc := post.NewPostService(uc)
		_, err := svc.V1CreatePost(ctx, &feedium.V1CreatePostRequest{
			SourceId:    "",
			ExternalId:  "ext-1",
			PublishedAt: timestamppb.New(publishedAt),
			Text:        "", // Empty text
		})

		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
		assert.Contains(t, st.Message(), "ERROR_REASON_POST_INVALID_ARGUMENT")
	})

	t.Run("source not found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		uc := mock.NewMockUsecase(ctrl)
		uc.EXPECT().
			Create(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(biz.Post{}, biz.ErrPostSourceNotFound)

		svc := post.NewPostService(uc)
		_, err := svc.V1CreatePost(ctx, &feedium.V1CreatePostRequest{
			SourceId:    "01961d9c-4f78-7e2e-8c3a-5e7d9a1b2c3d",
			ExternalId:  "ext-1",
			PublishedAt: timestamppb.New(publishedAt),
			Text:        "text",
		})

		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.NotFound, st.Code())
		assert.Contains(t, st.Message(), "ERROR_REASON_POST_SOURCE_NOT_FOUND")
	})

	t.Run("idempotent", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// Upsert returns existing post
		uc := mock.NewMockUsecase(ctrl)
		uc.EXPECT().
			Create(gomock.Any(), sourceID, "ext-1", "text", publishedAt, gomock.Any(), gomock.Any()).
			Return(makeDomainPost("existing-id", sourceID, "ext-1", "text"), nil)

		svc := post.NewPostService(uc)
		resp, err := svc.V1CreatePost(ctx, &feedium.V1CreatePostRequest{
			SourceId:    sourceID,
			ExternalId:  "ext-1",
			PublishedAt: timestamppb.New(publishedAt),
			Text:        "text",
		})

		require.NoError(t, err)
		assert.Equal(t, "existing-id", resp.GetPost().GetId())
	})
}

func TestV1UpdatePost(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	postID := "01961d9c-aaaa-7e2e-8c3a-5e7d9a1b2c3d"
	publishedAt := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)

	t.Run("valid", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		uc := mock.NewMockUsecase(ctrl)
		uc.EXPECT().
			Update(gomock.Any(), postID, "ext-1", "new text", publishedAt, gomock.Any(), gomock.Any()).
			Return(makeDomainPost(postID, "source-1", "ext-1", "new text"), nil)

		svc := post.NewPostService(uc)
		resp, err := svc.V1UpdatePost(ctx, &feedium.V1UpdatePostRequest{
			Id:          postID,
			ExternalId:  "ext-1",
			PublishedAt: timestamppb.New(publishedAt),
			Text:        "new text",
		})

		require.NoError(t, err)
		assert.Equal(t, "new text", resp.GetPost().GetText())
	})

	t.Run("not found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		uc := mock.NewMockUsecase(ctrl)
		uc.EXPECT().
			Update(gomock.Any(), postID, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(biz.Post{}, biz.ErrPostNotFound)

		svc := post.NewPostService(uc)
		_, err := svc.V1UpdatePost(ctx, &feedium.V1UpdatePostRequest{
			Id:          postID,
			ExternalId:  "ext-1",
			PublishedAt: timestamppb.New(publishedAt),
			Text:        "text",
		})

		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.NotFound, st.Code())
		assert.Contains(t, st.Message(), "ERROR_REASON_POST_NOT_FOUND")
	})

	t.Run("already exists", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		uc := mock.NewMockUsecase(ctrl)
		uc.EXPECT().
			Update(gomock.Any(), postID, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(biz.Post{}, biz.ErrPostAlreadyExists)

		svc := post.NewPostService(uc)
		_, err := svc.V1UpdatePost(ctx, &feedium.V1UpdatePostRequest{
			Id:          postID,
			ExternalId:  "conflict",
			PublishedAt: timestamppb.New(publishedAt),
			Text:        "text",
		})

		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.AlreadyExists, st.Code())
		assert.Contains(t, st.Message(), "ERROR_REASON_POST_ALREADY_EXISTS")
	})
}

func TestV1DeletePost(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()

	t.Run("valid", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		uc := mock.NewMockUsecase(ctrl)
		uc.EXPECT().Delete(gomock.Any(), "post-1").Return(nil)

		svc := post.NewPostService(uc)
		_, err := svc.V1DeletePost(ctx, &feedium.V1DeletePostRequest{Id: "post-1"})

		require.NoError(t, err)
	})

	t.Run("not found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		uc := mock.NewMockUsecase(ctrl)
		uc.EXPECT().Delete(gomock.Any(), "nonexistent").Return(biz.ErrPostNotFound)

		svc := post.NewPostService(uc)
		_, err := svc.V1DeletePost(ctx, &feedium.V1DeletePostRequest{Id: "nonexistent"})

		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.NotFound, st.Code())
		assert.Contains(t, st.Message(), "ERROR_REASON_POST_NOT_FOUND")
	})
}

func TestV1GetPost(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	sourceID := "01961d9c-4f78-7e2e-8c3a-5e7d9a1b2c3d"

	t.Run("found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		uc := mock.NewMockUsecase(ctrl)
		uc.EXPECT().Get(gomock.Any(), "post-1").Return(makeDomainPost("post-1", sourceID, "ext-1", "text"), nil)

		svc := post.NewPostService(uc)
		resp, err := svc.V1GetPost(ctx, &feedium.V1GetPostRequest{Id: "post-1"})

		require.NoError(t, err)
		assert.Equal(t, "post-1", resp.GetPost().GetId())
		assert.Equal(t, sourceID, resp.GetPost().GetSource().GetId())
		assert.Equal(t, "ext-1", resp.GetPost().GetExternalId())
		assert.Equal(t, "text", resp.GetPost().GetText())
	})

	t.Run("not found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		uc := mock.NewMockUsecase(ctrl)
		uc.EXPECT().Get(gomock.Any(), "nonexistent").Return(biz.Post{}, biz.ErrPostNotFound)

		svc := post.NewPostService(uc)
		_, err := svc.V1GetPost(ctx, &feedium.V1GetPostRequest{Id: "nonexistent"})

		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.NotFound, st.Code())
		assert.Contains(t, st.Message(), "ERROR_REASON_POST_NOT_FOUND")
	})
}

func TestV1ListPosts(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	sourceID := "01961d9c-4f78-7e2e-8c3a-5e7d9a1b2c3d"

	t.Run("valid", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		posts := []biz.Post{
			makeDomainPost("p1", sourceID, "e1", "t1"),
			makeDomainPost("p2", sourceID, "e2", "t2"),
		}

		uc := mock.NewMockUsecase(ctrl)
		uc.EXPECT().List(gomock.Any(), gomock.Any()).Return(biz.ListPostsResult{
			Items:         posts,
			NextPageToken: "next-token",
		}, nil)

		svc := post.NewPostService(uc)
		resp, err := svc.V1ListPosts(ctx, &feedium.V1ListPostsRequest{
			PageSize: 10,
		})

		require.NoError(t, err)
		assert.Len(t, resp.GetItems(), 2)
		assert.Equal(t, "next-token", resp.GetNextPageToken())
	})

	t.Run("invalid order_by", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		uc := mock.NewMockUsecase(ctrl)
		uc.EXPECT().List(gomock.Any(), gomock.Any()).Return(biz.ListPostsResult{}, biz.ErrPostInvalidArgument)

		svc := post.NewPostService(uc)
		_, err := svc.V1ListPosts(ctx, &feedium.V1ListPostsRequest{
			PageSize: 10,
			OrderBy:  feedium.PostSortField_POST_SORT_FIELD_UNSPECIFIED,
		})

		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
	})

	t.Run("page_size clamping", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		uc := mock.NewMockUsecase(ctrl)
		uc.EXPECT().List(gomock.Any(), gomock.Any()).Return(biz.ListPostsResult{}, nil)

		svc := post.NewPostService(uc)
		_, err := svc.V1ListPosts(ctx, &feedium.V1ListPostsRequest{
			PageSize: 0, // Will be clamped to 1 in biz
			OrderBy:  feedium.PostSortField_POST_SORT_FIELD_PUBLISHED_AT,
			OrderDir: feedium.SortDirection_SORT_DIRECTION_DESC,
		})

		require.NoError(t, err)
	})
}
