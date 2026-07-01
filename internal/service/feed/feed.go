package feed

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/4itosik/feedium/api/feedium"
	"github.com/4itosik/feedium/internal/biz"
)

// Usecase defines the ranked-feed business logic consumed by the service.
type Usecase interface {
	RankedFeed(ctx context.Context, query biz.FeedQuery) (biz.FeedResult, error)
}

// ReactionUsecase defines the reaction business logic consumed by the service.
type ReactionUsecase interface {
	React(ctx context.Context, postID, signal string) error
}

// FeedService implements feedium.FeedServiceServer.
//
//nolint:revive // exported type name Feed+Service is not a stutter
type FeedService struct {
	feedium.UnimplementedFeedServiceServer

	uc  Usecase
	ruc ReactionUsecase
}

// NewFeedService creates a new FeedService.
func NewFeedService(uc Usecase, ruc ReactionUsecase) *FeedService {
	return &FeedService{uc: uc, ruc: ruc}
}

func (s *FeedService) V1ListFeed(
	ctx context.Context,
	req *feedium.V1ListFeedRequest,
) (*feedium.V1ListFeedResponse, error) {
	query := biz.FeedQuery{
		MinScore:  req.GetMinScore(),
		PageSize:  int(req.GetPageSize()),
		PageToken: req.GetPageToken(),
	}
	if req.GetFrom() != nil {
		from := req.GetFrom().AsTime()
		query.From = &from
	}
	if req.GetTo() != nil {
		to := req.GetTo().AsTime()
		query.To = &to
	}

	result, err := s.uc.RankedFeed(ctx, query)
	if err != nil {
		return nil, s.mapDomainErrorToStatus(err)
	}

	items := make([]*feedium.FeedItem, 0, len(result.Items))
	for _, item := range result.Items {
		var author string
		if item.Post.Author != nil {
			author = *item.Post.Author
		}
		items = append(items, &feedium.FeedItem{
			PostId:      item.Post.ID,
			SourceId:    item.Post.SourceID,
			Author:      author,
			Text:        item.Post.Text,
			PublishedAt: timestamppb.New(item.Post.PublishedAt),
			Score:       item.Score,
			Url:         item.URL,
			Reaction:    item.Reaction,
			Kind:        mapFeedItemKind(item.Kind),
		})
	}
	return &feedium.V1ListFeedResponse{
		Items:         items,
		NextPageToken: result.NextPageToken,
	}, nil
}

func (s *FeedService) V1ReactToPost(
	ctx context.Context,
	req *feedium.V1ReactToPostRequest,
) (*feedium.V1ReactToPostResponse, error) {
	if err := s.ruc.React(ctx, req.GetPostId(), req.GetSignal()); err != nil {
		if errors.Is(err, biz.ErrInvalidSignal) {
			return nil, status.Error(codes.InvalidArgument, "signal must be 'like' or 'dislike'")
		}
		return nil, status.Error(codes.Internal, "internal error")
	}
	return &feedium.V1ReactToPostResponse{}, nil
}

// mapFeedItemKind maps the domain feed-item kind to the proto enum (FT-020 CTR-02).
func mapFeedItemKind(kind biz.FeedItemKind) feedium.FeedItemKind {
	switch kind {
	case biz.FeedItemKindPost:
		return feedium.FeedItemKind_FEED_ITEM_KIND_POST
	case biz.FeedItemKindDigest:
		return feedium.FeedItemKind_FEED_ITEM_KIND_DIGEST
	default:
		return feedium.FeedItemKind_FEED_ITEM_KIND_UNSPECIFIED
	}
}

func (s *FeedService) mapDomainErrorToStatus(err error) error {
	if errors.Is(err, biz.ErrInvalidThreshold) {
		return status.Error(
			codes.InvalidArgument,
			feedium.ErrorReason_ERROR_REASON_INVALID_THRESHOLD.String(),
		)
	}
	return status.Error(codes.Internal, "internal error")
}
