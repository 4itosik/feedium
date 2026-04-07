package connect

import (
	"errors"
	"testing"
	"time"

	"feedium/internal/app/post"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestToProto(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	p := &post.Post{
		ID:          uuid.New(),
		SourceID:    uuid.New(),
		Title:       "title",
		Content:     "content",
		Author:      "author",
		PublishedAt: now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	out := toProto(p)
	if out.GetId() != p.ID.String() {
		t.Fatalf("id mismatch: got %s want %s", out.GetId(), p.ID.String())
	}
	if out.GetSourceId() != p.SourceID.String() {
		t.Fatalf("source_id mismatch: got %s want %s", out.GetSourceId(), p.SourceID.String())
	}
	if out.GetTitle() != p.Title {
		t.Fatalf("title mismatch: got %s want %s", out.GetTitle(), p.Title)
	}
	if out.GetContent() != p.Content {
		t.Fatalf("content mismatch: got %s want %s", out.GetContent(), p.Content)
	}
	if out.GetAuthor() != p.Author {
		t.Fatalf("author mismatch: got %s want %s", out.GetAuthor(), p.Author)
	}
	if out.GetPublishedAt() == nil || !out.GetPublishedAt().AsTime().Equal(p.PublishedAt) {
		t.Fatalf("published_at mismatch")
	}

	if toProto(nil) != nil {
		t.Fatal("nil post must map to nil")
	}
}

func TestMapError(t *testing.T) {
	t.Parallel()

	h := &Handler{}

	if h.mapError(nil) != nil {
		t.Fatal("nil error must stay nil")
	}

	if got := h.mapError(post.ErrNotFound); got == nil {
		t.Fatal("not found must map to connect error")
	} else if c := connect.CodeOf(got); c != connect.CodeNotFound {
		t.Fatalf("expected CodeNotFound, got %v", c)
	}

	if got := h.mapError(post.NewValidationError("bad")); got == nil {
		t.Fatal("validation error must map to connect error")
	} else if c := connect.CodeOf(got); c != connect.CodeInvalidArgument {
		t.Fatalf("expected CodeInvalidArgument, got %v", c)
	}

	if got := h.mapError(errors.New("boom")); got == nil {
		t.Fatal("internal error must map to connect error")
	} else if c := connect.CodeOf(got); c != connect.CodeInternal {
		t.Fatalf("expected CodeInternal, got %v", c)
	}
}

func TestIsValidationErr(t *testing.T) {
	t.Parallel()

	if !isValidationErr(post.NewValidationError("test")) {
		t.Fatal("validation error must be detected")
	}

	if isValidationErr(errors.New("test")) {
		t.Fatal("non-validation error must return false")
	}

	if isValidationErr(nil) {
		t.Fatal("nil must return false")
	}
}

func TestTimestamppbToTime(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	ts := timestamppb.New(now)
	got := timestamppbToTime(ts)
	if !got.Equal(now) {
		t.Fatalf("time mismatch: got %v want %v", got, now)
	}

	if !timestamppbToTime(nil).IsZero() {
		t.Fatal("nil timestamp must return zero time")
	}
}

func TestToProtoConversion(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	p := &post.Post{
		ID:          uuid.New(),
		SourceID:    uuid.New(),
		Title:       "Test Title",
		Content:     "Test Content",
		Author:      "Test Author",
		PublishedAt: now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	protoPost := toProto(p)

	if protoPost.GetId() != p.ID.String() {
		t.Errorf("ID mismatch")
	}
	if protoPost.GetSourceId() != p.SourceID.String() {
		t.Errorf("SourceID mismatch")
	}
	if protoPost.GetTitle() != p.Title {
		t.Errorf("Title mismatch")
	}
	if protoPost.GetContent() != p.Content {
		t.Errorf("Content mismatch")
	}
	if protoPost.GetAuthor() != p.Author {
		t.Errorf("Author mismatch")
	}
	if protoPost.GetPublishedAt() == nil || !protoPost.GetPublishedAt().AsTime().Equal(p.PublishedAt) {
		t.Errorf("PublishedAt mismatch")
	}
	if protoPost.GetCreatedAt() == nil || !protoPost.GetCreatedAt().AsTime().Equal(p.CreatedAt) {
		t.Errorf("CreatedAt mismatch")
	}
	if protoPost.GetUpdatedAt() == nil || !protoPost.GetUpdatedAt().AsTime().Equal(p.UpdatedAt) {
		t.Errorf("UpdatedAt mismatch")
	}
}
