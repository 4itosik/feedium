package summary

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/4itosik/feedium/api/feedium"
	"github.com/4itosik/feedium/internal/biz"
)

type Usecase interface {
	TriggerSourceSummarize(ctx context.Context, sourceID string) (string, bool, error)
	GetSummaryEvent(ctx context.Context, id string) (*biz.SummaryEvent, error)
	GetSummary(ctx context.Context, id string) (biz.Summary, error)
	ListPostSummaries(ctx context.Context, postID string) ([]biz.Summary, error)
	ListSourceSummaries(ctx context.Context, sourceID string, pageSize int, pageToken string) (biz.ListSummariesResult, error)
}

type SummaryService struct {
	feedium.UnimplementedSummaryServiceServer

	uc Usecase
}

func NewSummaryService(uc Usecase) *SummaryService {
	return &SummaryService{uc: uc}
}

func (s *SummaryService) V1SummarizeSource(ctx context.Context, req *feedium.V1SummarizeSourceRequest) (*feedium.V1SummarizeSourceResponse, error) {
	taskID, existing, err := s.uc.TriggerSourceSummarize(ctx, req.GetSourceId())
	if err != nil {
		return nil, s.mapDomainErrorToStatus(err)
	}

	return &feedium.V1SummarizeSourceResponse{
		TaskId:   taskID,
		Existing: existing,
	}, nil
}

func (s *SummaryService) V1GetSummaryEvent(ctx context.Context, req *feedium.V1GetSummaryEventRequest) (*feedium.V1GetSummaryEventResponse, error) {
	event, err := s.uc.GetSummaryEvent(ctx, req.GetId())
	if err != nil {
		return nil, s.mapDomainErrorToStatus(err)
	}

	return &feedium.V1GetSummaryEventResponse{
		Event: s.mapSummaryEventToProto(event),
	}, nil
}

func (s *SummaryService) V1ListPostSummaries(ctx context.Context, req *feedium.V1ListPostSummariesRequest) (*feedium.V1ListPostSummariesResponse, error) {
	summaries, err := s.uc.ListPostSummaries(ctx, req.GetPostId())
	if err != nil {
		return nil, s.mapDomainErrorToStatus(err)
	}

	protoItems := make([]*feedium.Summary, 0, len(summaries))
	for _, sm := range summaries {
		protoItems = append(protoItems, s.mapSummaryToProto(sm))
	}

	return &feedium.V1ListPostSummariesResponse{Summaries: protoItems}, nil
}

func (s *SummaryService) V1ListSourceSummaries(ctx context.Context, req *feedium.V1ListSourceSummariesRequest) (*feedium.V1ListSourceSummariesResponse, error) {
	result, err := s.uc.ListSourceSummaries(ctx, req.GetSourceId(), int(req.GetPageSize()), req.GetPageToken())
	if err != nil {
		return nil, s.mapDomainErrorToStatus(err)
	}

	protoItems := make([]*feedium.Summary, 0, len(result.Items))
	for _, sm := range result.Items {
		protoItems = append(protoItems, s.mapSummaryToProto(sm))
	}

	return &feedium.V1ListSourceSummariesResponse{
		Summaries:     protoItems,
		NextPageToken: result.NextPageToken,
	}, nil
}

func (s *SummaryService) V1GetSummary(ctx context.Context, req *feedium.V1GetSummaryRequest) (*feedium.V1GetSummaryResponse, error) {
	sm, err := s.uc.GetSummary(ctx, req.GetId())
	if err != nil {
		return nil, s.mapDomainErrorToStatus(err)
	}

	return &feedium.V1GetSummaryResponse{Summary: s.mapSummaryToProto(sm)}, nil
}

func (s *SummaryService) mapSummaryToProto(sm biz.Summary) *feedium.Summary {
	pb := &feedium.Summary{
		Id:        sm.ID,
		SourceId:  sm.SourceID,
		Text:      sm.Text,
		WordCount: int32(sm.WordCount),
		CreatedAt: timestamppb.New(sm.CreatedAt),
	}
	if sm.PostID != nil {
		pb.PostId = sm.PostID
	}
	return pb
}

func (s *SummaryService) mapSummaryEventToProto(event *biz.SummaryEvent) *feedium.SummaryEvent {
	pb := &feedium.SummaryEvent{
		Id:        event.ID,
		SourceId:  event.SourceID,
		EventType: s.mapEventTypeToProto(event.EventType),
		Status:    s.mapEventStatusToProto(event.Status),
		CreatedAt: timestamppb.New(event.CreatedAt),
	}
	if event.PostID != nil {
		pb.PostId = event.PostID
	}
	if event.SummaryID != nil {
		pb.SummaryId = event.SummaryID
	}
	if event.Error != nil {
		pb.Error = event.Error
	}
	if event.ProcessedAt != nil {
		pb.ProcessedAt = timestamppb.New(*event.ProcessedAt)
	}
	return pb
}

func (s *SummaryService) mapEventTypeToProto(t biz.SummaryEventType) feedium.SummaryEventType {
	switch t {
	case biz.SummaryEventTypeSummarizePost:
		return feedium.SummaryEventType_SUMMARY_EVENT_TYPE_SUMMARIZE_POST
	case biz.SummaryEventTypeSummarizeSource:
		return feedium.SummaryEventType_SUMMARY_EVENT_TYPE_SUMMARIZE_SOURCE
	}
	return feedium.SummaryEventType_SUMMARY_EVENT_TYPE_UNSPECIFIED
}

func (s *SummaryService) mapEventStatusToProto(st biz.SummaryEventStatus) feedium.SummaryEventStatus {
	switch st {
	case biz.SummaryEventStatusPending:
		return feedium.SummaryEventStatus_SUMMARY_EVENT_STATUS_PENDING
	case biz.SummaryEventStatusProcessing:
		return feedium.SummaryEventStatus_SUMMARY_EVENT_STATUS_PROCESSING
	case biz.SummaryEventStatusCompleted:
		return feedium.SummaryEventStatus_SUMMARY_EVENT_STATUS_COMPLETED
	case biz.SummaryEventStatusFailed:
		return feedium.SummaryEventStatus_SUMMARY_EVENT_STATUS_FAILED
	case biz.SummaryEventStatusExpired:
		return feedium.SummaryEventStatus_SUMMARY_EVENT_STATUS_EXPIRED
	}
	return feedium.SummaryEventStatus_SUMMARY_EVENT_STATUS_UNSPECIFIED
}

func (s *SummaryService) mapDomainErrorToStatus(err error) error {
	switch {
	case errors.Is(err, biz.ErrSourceNotFound):
		return status.Error(codes.NotFound, feedium.ErrorReason_ERROR_REASON_SOURCE_NOT_FOUND.String())
	case errors.Is(err, biz.ErrSummaryNotFound):
		return status.Error(codes.NotFound, feedium.ErrorReason_ERROR_REASON_SUMMARY_NOT_FOUND.String())
	case errors.Is(err, biz.ErrSummaryEventNotFound):
		return status.Error(codes.NotFound, feedium.ErrorReason_ERROR_REASON_SUMMARY_EVENT_NOT_FOUND.String())
	case errors.Is(err, biz.ErrSummarizeSelfContainedSrc):
		return status.Error(codes.InvalidArgument, feedium.ErrorReason_ERROR_REASON_SUMMARIZE_SELF_CONTAINED_SOURCE.String())
	}
	return status.Error(codes.Internal, "internal error")
}
