package connect_test

import (
	"context"
	"errors"
	"testing"
	"time"

	postv1 "feedium/api/post/v1"
	"feedium/internal/app/post"
	postconnect "feedium/internal/app/post/adapters/connect"
	"feedium/internal/app/post/mocks"
	"feedium/internal/platform/logger"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestHandlerCreatePost(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := mocks.NewMockRepository(ctrl)
	svc := post.NewService(repo, logger.Init())
	h := postconnect.New(svc, logger.Init())
	ctx := context.Background()
	now := time.Now().UTC()

	t.Run("valid_request", func(t *testing.T) {
		sourceID := uuid.New().String()
		repo.EXPECT().Create(ctx, gomock.Any()).Return(nil)
		resp, err := h.CreatePost(ctx, connect.NewRequest(&postv1.CreatePostRequest{
			SourceId:    sourceID,
			Title:       "title",
			Content:     "content",
			Author:      "author",
			PublishedAt: timestamppb.New(now),
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Msg.GetPost().GetTitle() != "title" {
			t.Fatalf("expected title 'title', got %q", resp.Msg.GetPost().GetTitle())
		}
	})

	t.Run("invalid_source_id", func(t *testing.T) {
		_, err := h.CreatePost(ctx, connect.NewRequest(&postv1.CreatePostRequest{
			SourceId:    "invalid-uuid",
			Title:       "title",
			Content:     "content",
			PublishedAt: timestamppb.New(now),
		}))
		if err == nil {
			t.Fatal("expected error")
		}
		if c := connect.CodeOf(err); c != connect.CodeInvalidArgument {
			t.Fatalf("expected InvalidArgument, got %v", c)
		}
	})

	t.Run("nil_published_at", func(t *testing.T) {
		sourceID := uuid.New().String()
		_, err := h.CreatePost(ctx, connect.NewRequest(&postv1.CreatePostRequest{
			SourceId:    sourceID,
			Title:       "title",
			Content:     "content",
			PublishedAt: nil,
		}))
		if err == nil {
			t.Fatal("expected error")
		}
		if c := connect.CodeOf(err); c != connect.CodeInvalidArgument {
			t.Fatalf("expected InvalidArgument, got %v", c)
		}
	})

	t.Run("service_error", func(t *testing.T) {
		sourceID := uuid.New().String()
		repo.EXPECT().Create(ctx, gomock.Any()).Return(errors.New("db error"))
		_, err := h.CreatePost(ctx, connect.NewRequest(&postv1.CreatePostRequest{
			SourceId:    sourceID,
			Title:       "title",
			Content:     "content",
			PublishedAt: timestamppb.New(now),
		}))
		if err == nil {
			t.Fatal("expected error")
		}
		if c := connect.CodeOf(err); c != connect.CodeInternal {
			t.Fatalf("expected Internal, got %v", c)
		}
	})
}

func TestHandlerGetPost(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := mocks.NewMockRepository(ctrl)
	svc := post.NewService(repo, logger.Init())
	h := postconnect.New(svc, logger.Init())
	ctx := context.Background()
	id := uuid.New()

	t.Run("success", func(t *testing.T) {
		repo.EXPECT().GetByID(ctx, id).Return(&post.Post{
			ID:          id,
			SourceID:    uuid.New(),
			Title:       "test",
			Content:     "content",
			PublishedAt: time.Now().UTC(),
		}, nil)
		resp, err := h.GetPost(ctx, connect.NewRequest(&postv1.GetPostRequest{
			Id: id.String(),
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Msg.GetPost().GetId() != id.String() {
			t.Fatalf("expected id %s, got %s", id.String(), resp.Msg.GetPost().GetId())
		}
	})

	t.Run("invalid_id", func(t *testing.T) {
		_, err := h.GetPost(ctx, connect.NewRequest(&postv1.GetPostRequest{
			Id: "invalid-uuid",
		}))
		if err == nil {
			t.Fatal("expected error")
		}
		if c := connect.CodeOf(err); c != connect.CodeInvalidArgument {
			t.Fatalf("expected InvalidArgument, got %v", c)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		repo.EXPECT().GetByID(ctx, id).Return(nil, post.ErrNotFound)
		_, err := h.GetPost(ctx, connect.NewRequest(&postv1.GetPostRequest{
			Id: id.String(),
		}))
		if err == nil {
			t.Fatal("expected error")
		}
		if c := connect.CodeOf(err); c != connect.CodeNotFound {
			t.Fatalf("expected NotFound, got %v", c)
		}
	})
}

func TestHandlerUpdatePost(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := mocks.NewMockRepository(ctrl)
	svc := post.NewService(repo, logger.Init())
	h := postconnect.New(svc, logger.Init())
	ctx := context.Background()
	id := uuid.New()
	sourceID := uuid.New()
	now := time.Now().UTC()

	t.Run("success", func(t *testing.T) {
		repo.EXPECT().GetByID(ctx, id).Return(&post.Post{ID: id}, nil)
		repo.EXPECT().Update(ctx, gomock.Any()).Return(nil)
		resp, err := h.UpdatePost(ctx, connect.NewRequest(&postv1.UpdatePostRequest{
			Id:          id.String(),
			SourceId:    sourceID.String(),
			Title:       "updated",
			Content:     "updated content",
			PublishedAt: timestamppb.New(now),
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Msg.GetPost().GetTitle() != "updated" {
			t.Fatalf("expected title 'updated', got %q", resp.Msg.GetPost().GetTitle())
		}
	})

	t.Run("invalid_id", func(t *testing.T) {
		_, err := h.UpdatePost(ctx, connect.NewRequest(&postv1.UpdatePostRequest{
			Id:          "invalid-uuid",
			SourceId:    sourceID.String(),
			Title:       "updated",
			Content:     "updated content",
			PublishedAt: timestamppb.New(now),
		}))
		if err == nil {
			t.Fatal("expected error")
		}
		if c := connect.CodeOf(err); c != connect.CodeInvalidArgument {
			t.Fatalf("expected InvalidArgument, got %v", c)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		repo.EXPECT().GetByID(ctx, id).Return(nil, post.ErrNotFound)
		_, err := h.UpdatePost(ctx, connect.NewRequest(&postv1.UpdatePostRequest{
			Id:          id.String(),
			SourceId:    sourceID.String(),
			Title:       "updated",
			Content:     "updated content",
			PublishedAt: timestamppb.New(now),
		}))
		if err == nil {
			t.Fatal("expected error")
		}
		if c := connect.CodeOf(err); c != connect.CodeNotFound {
			t.Fatalf("expected NotFound, got %v", c)
		}
	})
}

func TestHandlerDeletePost(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := mocks.NewMockRepository(ctrl)
	svc := post.NewService(repo, logger.Init())
	h := postconnect.New(svc, logger.Init())
	ctx := context.Background()
	id := uuid.New()

	t.Run("success", func(t *testing.T) {
		repo.EXPECT().Delete(ctx, id).Return(nil)
		_, err := h.DeletePost(ctx, connect.NewRequest(&postv1.DeletePostRequest{
			Id: id.String(),
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("invalid_id", func(t *testing.T) {
		_, err := h.DeletePost(ctx, connect.NewRequest(&postv1.DeletePostRequest{
			Id: "invalid-uuid",
		}))
		if err == nil {
			t.Fatal("expected error")
		}
		if c := connect.CodeOf(err); c != connect.CodeInvalidArgument {
			t.Fatalf("expected InvalidArgument, got %v", c)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		repo.EXPECT().Delete(ctx, id).Return(post.ErrNotFound)
		_, err := h.DeletePost(ctx, connect.NewRequest(&postv1.DeletePostRequest{
			Id: id.String(),
		}))
		if err == nil {
			t.Fatal("expected error")
		}
		if c := connect.CodeOf(err); c != connect.CodeNotFound {
			t.Fatalf("expected NotFound, got %v", c)
		}
	})
}

func TestHandlerListPosts(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := mocks.NewMockRepository(ctrl)
	svc := post.NewService(repo, logger.Init())
	h := postconnect.New(svc, logger.Init())
	ctx := context.Background()
	now := time.Now().UTC()

	t.Run("without_filters", func(t *testing.T) {
		repo.EXPECT().List(ctx, post.ListFilter{PageSize: 50, Page: 1}).Return(nil, 0, nil)
		resp, err := h.ListPosts(ctx, connect.NewRequest(&postv1.ListPostsRequest{}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Msg.GetTotalCount() != 0 {
			t.Fatalf("expected total_count 0, got %d", resp.Msg.GetTotalCount())
		}
	})

	t.Run("with_date_filters", func(t *testing.T) {
		after := now.Add(-24 * time.Hour)
		before := now
		repo.EXPECT().List(ctx, post.ListFilter{
			PublishedAfter:  &after,
			PublishedBefore: &before,
			PageSize:        50,
			Page:            1,
		}).Return([]post.Post{{ID: uuid.New(), Title: "post1"}}, 1, nil)
		resp, err := h.ListPosts(ctx, connect.NewRequest(&postv1.ListPostsRequest{
			PublishedAfter:  timestamppb.New(after),
			PublishedBefore: timestamppb.New(before),
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Msg.GetTotalCount() != 1 {
			t.Fatalf("expected total_count 1, got %d", resp.Msg.GetTotalCount())
		}
		if len(resp.Msg.GetPosts()) != 1 {
			t.Fatalf("expected 1 post, got %d", len(resp.Msg.GetPosts()))
		}
	})
}
