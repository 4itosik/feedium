package interest

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/4itosik/feedium/api/feedium"
	"github.com/4itosik/feedium/internal/biz"
)

// Usecase defines the Interest business logic operations consumed by the service.
type Usecase interface {
	Create(ctx context.Context, topic string) (biz.Interest, error)
	Update(ctx context.Context, id, topic string) (biz.Interest, error)
	Delete(ctx context.Context, id string) error
	Get(ctx context.Context, id string) (biz.Interest, error)
	List(ctx context.Context, filter biz.ListInterestsFilter) (biz.ListInterestsResult, error)
}

// InterestService implements feedium.InterestServiceServer.
//
//nolint:revive // exported type name Interest+Service is not a stutter
type InterestService struct {
	feedium.UnimplementedInterestServiceServer

	uc Usecase
}

// NewInterestService creates a new InterestService.
func NewInterestService(uc Usecase) *InterestService {
	return &InterestService{uc: uc}
}

func (s *InterestService) V1CreateInterest(
	ctx context.Context,
	req *feedium.V1CreateInterestRequest,
) (*feedium.V1CreateInterestResponse, error) {
	created, err := s.uc.Create(ctx, req.GetTopic())
	if err != nil {
		return nil, s.mapDomainErrorToStatus(err)
	}
	return &feedium.V1CreateInterestResponse{Interest: mapDomainToProto(created)}, nil
}

func (s *InterestService) V1UpdateInterest(
	ctx context.Context,
	req *feedium.V1UpdateInterestRequest,
) (*feedium.V1UpdateInterestResponse, error) {
	updated, err := s.uc.Update(ctx, req.GetId(), req.GetTopic())
	if err != nil {
		return nil, s.mapDomainErrorToStatus(err)
	}
	return &feedium.V1UpdateInterestResponse{Interest: mapDomainToProto(updated)}, nil
}

func (s *InterestService) V1DeleteInterest(
	ctx context.Context,
	req *feedium.V1DeleteInterestRequest,
) (*feedium.V1DeleteInterestResponse, error) {
	if err := s.uc.Delete(ctx, req.GetId()); err != nil {
		return nil, s.mapDomainErrorToStatus(err)
	}
	return &feedium.V1DeleteInterestResponse{}, nil
}

func (s *InterestService) V1GetInterest(
	ctx context.Context,
	req *feedium.V1GetInterestRequest,
) (*feedium.V1GetInterestResponse, error) {
	got, err := s.uc.Get(ctx, req.GetId())
	if err != nil {
		return nil, s.mapDomainErrorToStatus(err)
	}
	return &feedium.V1GetInterestResponse{Interest: mapDomainToProto(got)}, nil
}

func (s *InterestService) V1ListInterests(
	ctx context.Context,
	req *feedium.V1ListInterestsRequest,
) (*feedium.V1ListInterestsResponse, error) {
	result, err := s.uc.List(ctx, biz.ListInterestsFilter{
		PageSize:  int(req.GetPageSize()),
		PageToken: req.GetPageToken(),
	})
	if err != nil {
		return nil, s.mapDomainErrorToStatus(err)
	}

	items := make([]*feedium.Interest, 0, len(result.Items))
	for _, in := range result.Items {
		items = append(items, mapDomainToProto(in))
	}
	return &feedium.V1ListInterestsResponse{
		Items:         items,
		NextPageToken: result.NextPageToken,
	}, nil
}

func mapDomainToProto(in biz.Interest) *feedium.Interest {
	return &feedium.Interest{
		Id:        in.ID,
		Topic:     in.Topic,
		Model:     in.Model,
		CreatedAt: timestamppb.New(in.CreatedAt),
		UpdatedAt: timestamppb.New(in.UpdatedAt),
	}
}

func (s *InterestService) mapDomainErrorToStatus(err error) error {
	switch {
	case errors.Is(err, biz.ErrInterestNotFound):
		return status.Error(codes.NotFound, feedium.ErrorReason_ERROR_REASON_INTEREST_NOT_FOUND.String())
	case errors.Is(err, biz.ErrInterestValidation):
		return status.Error(codes.InvalidArgument, feedium.ErrorReason_ERROR_REASON_INTEREST_INVALID_ARGUMENT.String())
	}
	return status.Error(codes.Internal, "internal error")
}
