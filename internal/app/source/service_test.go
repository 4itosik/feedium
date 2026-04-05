package source_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"feedium/internal/app/source"
	"feedium/internal/app/source/mocks"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"
)

func TestServiceCreate(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := mocks.NewMockRepository(ctrl)
	svc := source.NewService(repo, nil)
	ctx := context.Background()
	now := time.Now().UTC()
	cases := []struct {
		name string
		src  *source.Source
		ok   bool
	}{
		{
			"telegram_channel",
			&source.Source{
				Type:      source.TypeTelegramChannel,
				Name:      "n",
				URL:       "https://x",
				Config:    map[string]any{"channel_id": "1"},
				CreatedAt: now,
				UpdatedAt: now,
			},
			true,
		},
		{
			"telegram_group",
			&source.Source{
				Type:      source.TypeTelegramGroup,
				Name:      "n",
				URL:       "https://x",
				Config:    map[string]any{"group_id": "1"},
				CreatedAt: now,
				UpdatedAt: now,
			},
			true,
		},
		{
			"rss",
			&source.Source{
				Type:      source.TypeRSS,
				Name:      "n",
				URL:       "https://x",
				Config:    map[string]any{"feed_url": "https://feed"},
				CreatedAt: now,
				UpdatedAt: now,
			},
			true,
		},
		{
			"web",
			&source.Source{
				Type:      source.TypeWebScraping,
				Name:      "n",
				URL:       "https://x",
				Config:    map[string]any{"selector": ".post"},
				CreatedAt: now,
				UpdatedAt: now,
			},
			true,
		},
		{
			"invalid_name",
			&source.Source{
				Type:   source.TypeRSS,
				Name:   " ",
				URL:    "https://x",
				Config: map[string]any{"feed_url": "https://feed"},
			},
			false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.ok {
				repo.EXPECT().Create(ctx, tc.src).Return(nil)
			}
			out, err := svc.Create(ctx, tc.src)
			if tc.ok && err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if tc.ok && out != tc.src {
				t.Fatalf("expected same pointer")
			}
			if !tc.ok && err == nil {
				t.Fatalf("expected error")
			}
		})
	}
}

func TestServiceGet(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := mocks.NewMockRepository(ctrl)
	svc := source.NewService(repo, nil)
	ctx := context.Background()
	id := uuid.New()
	want := &source.Source{ID: id}
	repo.EXPECT().GetByID(ctx, id).Return(want, nil)
	got, err := svc.Get(ctx, id)
	if err != nil || got != want {
		t.Fatalf("got %v %v", got, err)
	}
	repo.EXPECT().GetByID(ctx, id).Return(nil, source.ErrNotFound)
	_, err = svc.Get(ctx, id)
	if !errors.Is(err, source.ErrNotFound) {
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
	svc := source.NewService(repo, nil)
	ctx := context.Background()
	id := uuid.New()
	src := &source.Source{
		ID:     id,
		Type:   source.TypeRSS,
		Name:   "n",
		URL:    "https://x",
		Config: map[string]any{"feed_url": "https://feed"},
	}
	repo.EXPECT().GetByID(ctx, id).Return(&source.Source{ID: id}, nil)
	repo.EXPECT().Update(ctx, src).Return(nil)
	if out, err := svc.Update(ctx, src); err != nil || out != src {
		t.Fatalf("unexpected %v %v", out, err)
	}
	repo.EXPECT().GetByID(ctx, id).Return(nil, source.ErrNotFound)
	if _, err := svc.Update(ctx, src); !errors.Is(err, source.ErrNotFound) {
		t.Fatalf("expected not found")
	}
	bad := &source.Source{
		ID:     id,
		Type:   source.TypeRSS,
		Name:   " ",
		URL:    "https://x",
		Config: map[string]any{"feed_url": "https://feed"},
	}
	if _, err := svc.Update(ctx, bad); err == nil {
		t.Fatalf("expected validation error")
	}
	repo.EXPECT().GetByID(ctx, id).Return(&source.Source{ID: id}, nil)
	repo.EXPECT().Update(ctx, src).Return(source.ErrNotFound)
	if _, err := svc.Update(ctx, src); !errors.Is(err, source.ErrNotFound) {
		t.Fatalf("expected race not found")
	}
}

func TestServiceDelete(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := mocks.NewMockRepository(ctrl)
	svc := source.NewService(repo, nil)
	ctx := context.Background()
	id := uuid.New()
	repo.EXPECT().Delete(ctx, id).Return(nil)
	if err := svc.Delete(ctx, id); err != nil {
		t.Fatal(err)
	}
	repo.EXPECT().Delete(ctx, id).Return(source.ErrNotFound)
	if err := svc.Delete(ctx, id); !errors.Is(err, source.ErrNotFound) {
		t.Fatal("expected not found")
	}
}

func TestServiceList(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := mocks.NewMockRepository(ctrl)
	svc := source.NewService(repo, nil)
	ctx := context.Background()
	repo.EXPECT().List(ctx, source.ListFilter{PageSize: 50, Page: 1}).Return(nil, 0, nil)
	if _, _, err := svc.List(ctx, source.ListFilter{}); err != nil {
		t.Fatal(err)
	}
	repo.EXPECT().
		List(ctx, source.ListFilter{Type: source.TypeRSS, PageSize: 100, Page: 2}).
		Return([]source.Source{{}}, 1, nil)
	if _, _, err := svc.List(ctx, source.ListFilter{Type: source.TypeRSS, PageSize: 101, Page: 2}); err != nil {
		t.Fatal(err)
	}
}
