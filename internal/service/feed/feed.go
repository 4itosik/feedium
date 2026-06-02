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

// FeedService implements feedium.FeedServiceServer.
//
//nolint:revive // exported type name Feed+Service is not a stutter
type FeedService struct {
	feedium.UnimplementedFeedServiceServer

	uc Usecase
}

// NewFeedService creates a new FeedService.
func NewFeedService(uc Usecase) *FeedService {
	return &FeedService{uc: uc}
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
		})
	}
	return &feedium.V1ListFeedResponse{
		Items:         items,
		NextPageToken: result.NextPageToken,
	}, nil
}

func (s *FeedService) mapDomainErrorToStatus(err error) error {
	if errors.Is(err, biz.ErrInvalidThreshold) {
		return status.Error(codes.InvalidArgument, feedium.ErrorReason_ERROR_REASON_INVALID_THRESHOLD.String())
	}
	return status.Error(codes.Internal, "internal error")
}
