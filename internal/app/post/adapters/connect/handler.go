package connect

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"strings"
	"time"

	postv1 "feedium/api/post/v1"
	"feedium/api/post/v1/postv1connect"
	"feedium/internal/app/post"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Handler struct {
	svc *post.Service
	log *slog.Logger
}

func New(svc *post.Service, log *slog.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

var _ postv1connect.PostServiceHandler = (*Handler)(nil)

func (h *Handler) CreatePost(
	ctx context.Context,
	req *connect.Request[postv1.CreatePostRequest],
) (*connect.Response[postv1.CreatePostResponse], error) {
	sourceID, err := uuid.Parse(strings.TrimSpace(req.Msg.GetSourceId()))
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	publishedAt := timestamppbToTime(req.Msg.GetPublishedAt())
	if publishedAt.IsZero() {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("published_at is required"))
	}

	p := &post.Post{
		SourceID:    sourceID,
		Title:       req.Msg.GetTitle(),
		Content:     req.Msg.GetContent(),
		Author:      req.Msg.GetAuthor(),
		PublishedAt: publishedAt,
	}

	created, err := h.svc.Create(ctx, p)
	if err != nil {
		return nil, h.mapError(err)
	}

	return connect.NewResponse(&postv1.CreatePostResponse{Post: toProto(created)}), nil
}

func (h *Handler) GetPost(
	ctx context.Context,
	req *connect.Request[postv1.GetPostRequest],
) (*connect.Response[postv1.GetPostResponse], error) {
	id, err := uuid.Parse(strings.TrimSpace(req.Msg.GetId()))
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	p, err := h.svc.Get(ctx, id)
	if err != nil {
		return nil, h.mapError(err)
	}

	return connect.NewResponse(&postv1.GetPostResponse{Post: toProto(p)}), nil
}

func (h *Handler) UpdatePost(
	ctx context.Context,
	req *connect.Request[postv1.UpdatePostRequest],
) (*connect.Response[postv1.UpdatePostResponse], error) {
	id, err := uuid.Parse(strings.TrimSpace(req.Msg.GetId()))
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	sourceID, err := uuid.Parse(strings.TrimSpace(req.Msg.GetSourceId()))
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	publishedAt := timestamppbToTime(req.Msg.GetPublishedAt())
	if publishedAt.IsZero() {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("published_at is required"))
	}

	p := &post.Post{
		ID:          id,
		SourceID:    sourceID,
		Title:       req.Msg.GetTitle(),
		Content:     req.Msg.GetContent(),
		Author:      req.Msg.GetAuthor(),
		PublishedAt: publishedAt,
	}

	updated, err := h.svc.Update(ctx, p)
	if err != nil {
		return nil, h.mapError(err)
	}

	return connect.NewResponse(&postv1.UpdatePostResponse{Post: toProto(updated)}), nil
}

func (h *Handler) DeletePost(
	ctx context.Context,
	req *connect.Request[postv1.DeletePostRequest],
) (*connect.Response[postv1.DeletePostResponse], error) {
	id, err := uuid.Parse(strings.TrimSpace(req.Msg.GetId()))
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	if err = h.svc.Delete(ctx, id); err != nil {
		return nil, h.mapError(err)
	}

	return connect.NewResponse(&postv1.DeletePostResponse{}), nil
}

func (h *Handler) ListPosts(
	ctx context.Context,
	req *connect.Request[postv1.ListPostsRequest],
) (*connect.Response[postv1.ListPostsResponse], error) {
	filter := post.ListFilter{
		Page:     int(req.Msg.GetPage()),
		PageSize: int(req.Msg.GetPageSize()),
	}

	if req.Msg.GetPublishedAfter() != nil {
		t := req.Msg.GetPublishedAfter().AsTime()
		filter.PublishedAfter = &t
	}
	if req.Msg.GetPublishedBefore() != nil {
		t := req.Msg.GetPublishedBefore().AsTime()
		filter.PublishedBefore = &t
	}

	posts, total, err := h.svc.List(ctx, filter)
	if err != nil {
		return nil, h.mapError(err)
	}

	out := make([]*postv1.Post, 0, len(posts))
	for i := range posts {
		p := posts[i]
		out = append(out, toProto(&p))
	}

	var totalCount int32
	if total > math.MaxInt32 {
		totalCount = math.MaxInt32
	} else {
		totalCount = int32(total) //nolint:gosec // total is range-checked against MaxInt32 above.
	}

	return connect.NewResponse(&postv1.ListPostsResponse{
		Posts:      out,
		TotalCount: totalCount,
	}), nil
}

func (h *Handler) mapError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, post.ErrNotFound):
		return connect.NewError(connect.CodeNotFound, err)
	case isValidationErr(err):
		return connect.NewError(connect.CodeInvalidArgument, err)
	default:
		if h.log != nil {
			h.log.Error("post handler error", "error", err)
		}
		return connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
}

func isValidationErr(err error) bool {
	var v post.ValidationError
	return errors.As(err, &v)
}

func toProto(p *post.Post) *postv1.Post {
	if p == nil {
		return nil
	}
	return &postv1.Post{
		Id:          p.ID.String(),
		SourceId:    p.SourceID.String(),
		Title:       p.Title,
		Content:     p.Content,
		Author:      p.Author,
		PublishedAt: timestamppb.New(p.PublishedAt),
		CreatedAt:   timestamppb.New(p.CreatedAt),
		UpdatedAt:   timestamppb.New(p.UpdatedAt),
	}
}

func timestamppbToTime(ts *timestamppb.Timestamp) time.Time {
	if ts == nil {
		return time.Time{}
	}
	return ts.AsTime()
}
