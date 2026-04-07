package post_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"feedium/internal/app/post"
	"feedium/internal/app/post/mocks"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"
)

func TestServiceCreate(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := mocks.NewMockRepository(ctrl)
	svc := post.NewService(repo, nil)
	ctx := context.Background()
	now := time.Now().UTC()

	cases := []struct {
		name string
		p    *post.Post
		ok   bool
	}{
		{
			"valid_post",
			&post.Post{
				SourceID:    uuid.New(),
				Title:       "title",
				Content:     "content",
				Author:      "author",
				PublishedAt: now,
			},
			true,
		},
		{
			"empty_title",
			&post.Post{
				SourceID:    uuid.New(),
				Title:       "",
				Content:     "content",
				PublishedAt: now,
			},
			false,
		},
		{
			"empty_content",
			&post.Post{
				SourceID:    uuid.New(),
				Title:       "title",
				Content:     "",
				PublishedAt: now,
			},
			false,
		},
		{
			"nil_source_id",
			&post.Post{
				SourceID:    uuid.Nil,
				Title:       "title",
				Content:     "content",
				PublishedAt: now,
			},
			false,
		},
		{
			"zero_published_at",
			&post.Post{
				SourceID:    uuid.New(),
				Title:       "title",
				Content:     "content",
				PublishedAt: time.Time{},
			},
			false,
		},
		{
			"nil_post",
			nil,
			false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.ok {
				repo.EXPECT().Create(ctx, tc.p).Return(nil)
			}
			out, err := svc.Create(ctx, tc.p)
			if tc.ok && err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if tc.ok && out != tc.p {
				t.Fatalf("expected same pointer")
			}
			if !tc.ok && err == nil {
				t.Fatalf("expected error")
			}
		})
	}
}

func TestServiceCreateRepoError(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := mocks.NewMockRepository(ctrl)
	svc := post.NewService(repo, nil)
	ctx := context.Background()
	now := time.Now().UTC()

	p := &post.Post{
		SourceID:    uuid.New(),
		Title:       "title",
		Content:     "content",
		PublishedAt: now,
	}
	repo.EXPECT().Create(ctx, p).Return(errors.New("db error"))
	_, err := svc.Create(ctx, p)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestServiceGet(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := mocks.NewMockRepository(ctrl)
	svc := post.NewService(repo, nil)
	ctx := context.Background()
	id := uuid.New()
	want := &post.Post{ID: id, Title: "test"}

	repo.EXPECT().GetByID(ctx, id).Return(want, nil)
	got, err := svc.Get(ctx, id)
	if err != nil || got != want {
		t.Fatalf("got %v %v", got, err)
	}

	repo.EXPECT().GetByID(ctx, id).Return(nil, post.ErrNotFound)
	_, err = svc.Get(ctx, id)
	if !errors.Is(err, post.ErrNotFound) {
		t.Fatalf("expected not found")
	}

	internal := errors.New("boom")
	repo.EXPECT().GetByID(ctx, id).Return(nil, internal)
	_, err = svc.Get(ctx, id)
	if !errors.Is(err, internal) {
		t.Fatalf("expected passthrough")
	}
}

func TestServiceUpdate(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := mocks.NewMockRepository(ctrl)
	svc := post.NewService(repo, nil)
	ctx := context.Background()
	id := uuid.New()
	sourceID := uuid.New()
	p := &post.Post{
		ID:          id,
		SourceID:    sourceID,
		Title:       "title",
		Content:     "content",
		PublishedAt: time.Now().UTC(),
	}

	repo.EXPECT().GetByID(ctx, id).Return(&post.Post{ID: id}, nil)
	repo.EXPECT().Update(ctx, p).Return(nil)
	if out, err := svc.Update(ctx, p); err != nil || out != p {
		t.Fatalf("unexpected %v %v", out, err)
	}

	repo.EXPECT().GetByID(ctx, id).Return(nil, post.ErrNotFound)
	if _, err := svc.Update(ctx, p); !errors.Is(err, post.ErrNotFound) {
		t.Fatalf("expected not found")
	}

	bad := &post.Post{
		ID:          id,
		SourceID:    sourceID,
		Title:       " ",
		Content:     "content",
		PublishedAt: time.Now().UTC(),
	}
	if _, err := svc.Update(ctx, bad); err == nil {
		t.Fatalf("expected validation error")
	}

	repo.EXPECT().GetByID(ctx, id).Return(&post.Post{ID: id}, nil)
	repo.EXPECT().Update(ctx, p).Return(post.ErrNotFound)
	if _, err := svc.Update(ctx, p); !errors.Is(err, post.ErrNotFound) {
		t.Fatalf("expected race not found")
	}
}

func TestServiceDelete(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := mocks.NewMockRepository(ctrl)
	svc := post.NewService(repo, nil)
	ctx := context.Background()
	id := uuid.New()

	repo.EXPECT().Delete(ctx, id).Return(nil)
	if err := svc.Delete(ctx, id); err != nil {
		t.Fatal(err)
	}

	repo.EXPECT().Delete(ctx, id).Return(post.ErrNotFound)
	if err := svc.Delete(ctx, id); !errors.Is(err, post.ErrNotFound) {
		t.Fatal("expected not found")
	}
}

func TestServiceList(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := mocks.NewMockRepository(ctrl)
	svc := post.NewService(repo, nil)
	ctx := context.Background()

	repo.EXPECT().List(ctx, post.ListFilter{PageSize: 50, Page: 1}).Return(nil, 0, nil)
	if _, _, err := svc.List(ctx, post.ListFilter{}); err != nil {
		t.Fatal(err)
	}

	repo.EXPECT().
		List(ctx, post.ListFilter{PageSize: 100, Page: 2}).
		Return([]post.Post{{}}, 1, nil)
	if _, _, err := svc.List(ctx, post.ListFilter{PageSize: 101, Page: 2}); err != nil {
		t.Fatal(err)
	}

	publishedAfter := time.Now().UTC().Add(-24 * time.Hour)
	publishedBefore := time.Now().UTC()
	repo.EXPECT().
		List(ctx, post.ListFilter{
			PublishedAfter:  &publishedAfter,
			PublishedBefore: &publishedBefore,
			PageSize:        50,
			Page:            1,
		}).
		Return([]post.Post{{}, {}}, 2, nil)
	if _, _, err := svc.List(ctx, post.ListFilter{
		PublishedAfter:  &publishedAfter,
		PublishedBefore: &publishedBefore,
	}); err != nil {
		t.Fatal(err)
	}
}
