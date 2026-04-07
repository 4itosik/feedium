// Package connect provides Connect RPC handlers for the summary API.
package connect

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	summaryv1 "feedium/api/summary/v1"
	"feedium/api/summary/v1/summaryv1connect"
	"feedium/internal/app/summary"
)

// Handler implements the SummaryService Connect handlers.
type Handler struct {
	repo summary.Repository
	log  *slog.Logger
}

// New creates a new Handler instance.
func New(repo summary.Repository, log *slog.Logger) *Handler {
	return &Handler{repo: repo, log: log}
}

var _ summaryv1connect.SummaryServiceHandler = (*Handler)(nil)

// GetSummaryByPost retrieves a summary by its associated post ID.
func (h *Handler) GetSummaryByPost(
	ctx context.Context,
	req *connect.Request[summaryv1.GetSummaryByPostRequest],
) (*connect.Response[summaryv1.GetSummaryByPostResponse], error) {
	postID, err := uuid.Parse(strings.TrimSpace(req.Msg.GetPostId()))
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	s, postIDs, err := h.repo.GetByPostID(ctx, postID)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to get summary by post", "post_id", postID, "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}

	if s == nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("summary not found"))
	}

	return connect.NewResponse(&summaryv1.GetSummaryByPostResponse{
		Summary: toProto(s, postIDs),
	}), nil
}

// ListSummariesBySource retrieves summaries, optionally filtered by source.
func (h *Handler) ListSummariesBySource(
	ctx context.Context,
	req *connect.Request[summaryv1.ListSummariesBySourceRequest],
) (*connect.Response[summaryv1.ListSummariesBySourceResponse], error) {
	// Validate limit
	limit := req.Msg.GetLimit()
	if limit <= 0 || limit > 100 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("limit must be between 1 and 100"))
	}

	// Parse optional source ID
	var sourceID *uuid.UUID
	if req.Msg.GetSourceId() != "" {
		id, err := uuid.Parse(strings.TrimSpace(req.Msg.GetSourceId()))
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		sourceID = &id
	}

	results, err := h.repo.ListSummaries(ctx, sourceID, int(limit))
	if err != nil {
		h.log.ErrorContext(ctx, "failed to list summaries", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}

	summaries := make([]*summaryv1.Summary, len(results))
	for i, r := range results {
		summaries[i] = toProto(&r.Summary, r.PostIDs)
	}

	return connect.NewResponse(&summaryv1.ListSummariesBySourceResponse{
		Summaries: summaries,
	}), nil
}

// toProto converts a domain Summary to its protobuf representation.
func toProto(s *summary.Summary, postIDs []uuid.UUID) *summaryv1.Summary {
	if s == nil {
		return nil
	}

	pbPostIDs := make([]string, len(postIDs))
	for i, id := range postIDs {
		pbPostIDs[i] = id.String()
	}

	return &summaryv1.Summary{
		Id:        s.ID.String(),
		SourceId:  s.SourceID.String(),
		EventId:   s.EventID.String(),
		Content:   s.Content,
		CreatedAt: timestamppb.New(s.CreatedAt),
		PostIds:   pbPostIDs,
	}
}
