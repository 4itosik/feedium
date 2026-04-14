package source

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/4itosik/feedium/api/feedium"
	"github.com/4itosik/feedium/internal/biz"
)

// Usecase defines the interface for Source business logic operations.
// This interface is defined here (where it's used) rather than in the biz package.
type Usecase interface {
	Create(ctx context.Context, sourceType biz.SourceType, config biz.SourceConfig) (biz.Source, error)
	Update(ctx context.Context, id string, sourceType biz.SourceType, config biz.SourceConfig) (biz.Source, error)
	Delete(ctx context.Context, id string) error
	Get(ctx context.Context, id string) (biz.Source, error)
	List(ctx context.Context, filter biz.ListSourcesFilter) (biz.ListSourcesResult, error)
}

// SourceService implements the feedium.SourceServiceServer.
//
//nolint:revive // exported type name Source+Service is not a stutter
type SourceService struct {
	feedium.UnimplementedSourceServiceServer

	uc Usecase
}

// NewSourceService creates a new SourceService.
func NewSourceService(uc Usecase) *SourceService {
	return &SourceService{uc: uc}
}

// V1CreateSource creates a new source.
func (s *SourceService) V1CreateSource(
	ctx context.Context,
	req *feedium.V1CreateSourceRequest,
) (*feedium.V1CreateSourceResponse, error) {
	// Map proto type to domain type
	sourceType := s.mapProtoTypeToDomain(req.GetType())

	// Map proto config to domain config
	domainConfig, err := s.mapProtoConfigToDomain(sourceType, req.GetConfig())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, feedium.ErrorReason_ERROR_REASON_SOURCE_INVALID_CONFIG.String())
	}

	// Call usecase
	source, err := s.uc.Create(ctx, sourceType, domainConfig)
	if err != nil {
		return nil, s.mapDomainErrorToStatus(err)
	}

	// Map domain to proto
	protoSource, err := s.mapDomainToProto(source)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to map response")
	}

	return &feedium.V1CreateSourceResponse{Source: protoSource}, nil
}

// V1UpdateSource updates an existing source.
func (s *SourceService) V1UpdateSource(
	ctx context.Context,
	req *feedium.V1UpdateSourceRequest,
) (*feedium.V1UpdateSourceResponse, error) {
	// Map proto type to domain type
	sourceType := s.mapProtoTypeToDomain(req.GetType())

	// Map proto config to domain config
	domainConfig, err := s.mapProtoConfigToDomain(sourceType, req.GetConfig())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, feedium.ErrorReason_ERROR_REASON_SOURCE_INVALID_CONFIG.String())
	}

	// Call usecase
	source, err := s.uc.Update(ctx, req.GetId(), sourceType, domainConfig)
	if err != nil {
		return nil, s.mapDomainErrorToStatus(err)
	}

	// Map domain to proto
	protoSource, err := s.mapDomainToProto(source)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to map response")
	}

	return &feedium.V1UpdateSourceResponse{Source: protoSource}, nil
}

// V1DeleteSource deletes a source.
func (s *SourceService) V1DeleteSource(
	ctx context.Context,
	req *feedium.V1DeleteSourceRequest,
) (*feedium.V1DeleteSourceResponse, error) {
	err := s.uc.Delete(ctx, req.GetId())
	if err != nil {
		return nil, s.mapDomainErrorToStatus(err)
	}

	return &feedium.V1DeleteSourceResponse{}, nil
}

// V1GetSource retrieves a source by ID.
func (s *SourceService) V1GetSource(
	ctx context.Context,
	req *feedium.V1GetSourceRequest,
) (*feedium.V1GetSourceResponse, error) {
	source, err := s.uc.Get(ctx, req.GetId())
	if err != nil {
		return nil, s.mapDomainErrorToStatus(err)
	}

	// Map domain to proto
	protoSource, err := s.mapDomainToProto(source)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to map response")
	}

	return &feedium.V1GetSourceResponse{Source: protoSource}, nil
}

// V1ListSources lists sources with pagination and filtering.
func (s *SourceService) V1ListSources(
	ctx context.Context,
	req *feedium.V1ListSourcesRequest,
) (*feedium.V1ListSourcesResponse, error) {
	filter := biz.ListSourcesFilter{
		PageSize:  int(req.GetPageSize()),
		PageToken: req.GetPageToken(),
	}

	// Map proto type to domain type if provided
	if reqType := req.GetType(); reqType != feedium.SourceType_SOURCE_TYPE_UNSPECIFIED {
		filter.Type = s.mapProtoTypeToDomain(reqType)
	}

	result, err := s.uc.List(ctx, filter)
	if err != nil {
		return nil, s.mapDomainErrorToStatus(err)
	}

	// Map domain sources to proto
	protoSources := make([]*feedium.Source, 0, len(result.Items))
	for _, source := range result.Items {
		protoSource, mapErr := s.mapDomainToProto(source)
		if mapErr != nil {
			return nil, status.Error(codes.Internal, "failed to map response")
		}
		protoSources = append(protoSources, protoSource)
	}

	return &feedium.V1ListSourcesResponse{
		Items:         protoSources,
		NextPageToken: result.NextPageToken,
	}, nil
}

// mapProtoConfigToDomain maps proto config oneof to domain config.
func (s *SourceService) mapProtoConfigToDomain(
	sourceType biz.SourceType,
	protoConfig *feedium.SourceConfig,
) (biz.SourceConfig, error) {
	if protoConfig == nil {
		return nil, errors.New("config is nil")
	}

	switch sourceType {
	case biz.SourceTypeTelegramChannel:
		tc, ok := protoConfig.GetConfig().(*feedium.SourceConfig_TelegramChannel)
		if !ok {
			return nil, errors.New("expected telegram_channel config")
		}
		return &biz.TelegramChannelConfig{
			TgID:     tc.TelegramChannel.GetTgId(),
			Username: tc.TelegramChannel.GetUsername(),
		}, nil

	case biz.SourceTypeTelegramGroup:
		tg, ok := protoConfig.GetConfig().(*feedium.SourceConfig_TelegramGroup)
		if !ok {
			return nil, errors.New("expected telegram_group config")
		}
		return &biz.TelegramGroupConfig{
			TgID:     tg.TelegramGroup.GetTgId(),
			Username: tg.TelegramGroup.GetUsername(),
		}, nil

	case biz.SourceTypeRSS:
		rc, ok := protoConfig.GetConfig().(*feedium.SourceConfig_Rss)
		if !ok {
			return nil, errors.New("expected rss config")
		}
		return &biz.RSSConfig{
			FeedURL: rc.Rss.GetFeedUrl(),
		}, nil

	case biz.SourceTypeHTML:
		hc, ok := protoConfig.GetConfig().(*feedium.SourceConfig_Html)
		if !ok {
			return nil, errors.New("expected html config")
		}
		return &biz.HTMLConfig{
			URL: hc.Html.GetUrl(),
		}, nil
	}

	return nil, fmt.Errorf("unknown source type: %s", sourceType)
}

// mapDomainToProto maps domain Source to proto Source.
func (s *SourceService) mapDomainToProto(source biz.Source) (*feedium.Source, error) {
	protoType := s.mapDomainTypeToProto(source.Type)
	protoMode := s.mapDomainModeToProto(source.ProcessingMode)

	protoConfig, err := s.mapDomainConfigToProto(source.Type, source.Config)
	if err != nil {
		return nil, fmt.Errorf("map config: %w", err)
	}

	return &feedium.Source{
		Id:             source.ID,
		Type:           protoType,
		ProcessingMode: protoMode,
		Config:         protoConfig,
		CreatedAt:      timestamppb.New(source.CreatedAt),
		UpdatedAt:      timestamppb.New(source.UpdatedAt),
	}, nil
}

// mapDomainConfigToProto maps domain config to proto config oneof.
func (s *SourceService) mapDomainConfigToProto(
	sourceType biz.SourceType,
	domainConfig biz.SourceConfig,
) (*feedium.SourceConfig, error) {
	protoConfig := &feedium.SourceConfig{}

	switch sourceType {
	case biz.SourceTypeTelegramChannel:
		tc, ok := domainConfig.(*biz.TelegramChannelConfig)
		if !ok {
			return nil, errors.New("expected TelegramChannelConfig")
		}
		protoConfig.Config = &feedium.SourceConfig_TelegramChannel{
			TelegramChannel: &feedium.TelegramChannelConfig{
				TgId:     tc.TgID,
				Username: tc.Username,
			},
		}

	case biz.SourceTypeTelegramGroup:
		tg, ok := domainConfig.(*biz.TelegramGroupConfig)
		if !ok {
			return nil, errors.New("expected TelegramGroupConfig")
		}
		protoConfig.Config = &feedium.SourceConfig_TelegramGroup{
			TelegramGroup: &feedium.TelegramGroupConfig{
				TgId:     tg.TgID,
				Username: tg.Username,
			},
		}

	case biz.SourceTypeRSS:
		rc, ok := domainConfig.(*biz.RSSConfig)
		if !ok {
			return nil, errors.New("expected RSSConfig")
		}
		protoConfig.Config = &feedium.SourceConfig_Rss{
			Rss: &feedium.RSSConfig{
				FeedUrl: rc.FeedURL,
			},
		}

	case biz.SourceTypeHTML:
		hc, ok := domainConfig.(*biz.HTMLConfig)
		if !ok {
			return nil, errors.New("expected HTMLConfig")
		}
		protoConfig.Config = &feedium.SourceConfig_Html{
			Html: &feedium.HTMLConfig{
				Url: hc.URL,
			},
		}

	default:
		return nil, fmt.Errorf("unknown source type: %s", sourceType)
	}

	return protoConfig, nil
}

// mapDomainTypeToProto maps domain SourceType to proto SourceType.
func (s *SourceService) mapDomainTypeToProto(sourceType biz.SourceType) feedium.SourceType {
	switch sourceType {
	case biz.SourceTypeTelegramChannel:
		return feedium.SourceType_SOURCE_TYPE_TELEGRAM_CHANNEL
	case biz.SourceTypeTelegramGroup:
		return feedium.SourceType_SOURCE_TYPE_TELEGRAM_GROUP
	case biz.SourceTypeRSS:
		return feedium.SourceType_SOURCE_TYPE_RSS
	case biz.SourceTypeHTML:
		return feedium.SourceType_SOURCE_TYPE_HTML
	}
	return feedium.SourceType_SOURCE_TYPE_UNSPECIFIED
}

// mapDomainModeToProto maps domain ProcessingMode to proto ProcessingMode.
func (s *SourceService) mapDomainModeToProto(mode biz.ProcessingMode) feedium.ProcessingMode {
	switch mode {
	case biz.ProcessingModeSelfContained:
		return feedium.ProcessingMode_PROCESSING_MODE_SELF_CONTAINED
	case biz.ProcessingModeCumulative:
		return feedium.ProcessingMode_PROCESSING_MODE_CUMULATIVE
	}
	return feedium.ProcessingMode_PROCESSING_MODE_UNSPECIFIED
}

// mapProtoTypeToProto maps proto SourceType to domain SourceType.
func (s *SourceService) mapProtoTypeToDomain(protoType feedium.SourceType) biz.SourceType {
	switch protoType {
	case feedium.SourceType_SOURCE_TYPE_TELEGRAM_CHANNEL:
		return biz.SourceTypeTelegramChannel
	case feedium.SourceType_SOURCE_TYPE_TELEGRAM_GROUP:
		return biz.SourceTypeTelegramGroup
	case feedium.SourceType_SOURCE_TYPE_RSS:
		return biz.SourceTypeRSS
	case feedium.SourceType_SOURCE_TYPE_HTML:
		return biz.SourceTypeHTML
	case feedium.SourceType_SOURCE_TYPE_UNSPECIFIED:
		return biz.SourceType("")
	}
	return biz.SourceType("")
}

// mapDomainErrorToStatus maps domain errors to gRPC status errors.
func (s *SourceService) mapDomainErrorToStatus(err error) error {
	switch {
	case errors.Is(err, biz.ErrSourceNotFound):
		return status.Error(codes.NotFound, feedium.ErrorReason_ERROR_REASON_SOURCE_NOT_FOUND.String())
	case errors.Is(err, biz.ErrInvalidSourceType):
		return status.Error(codes.InvalidArgument, feedium.ErrorReason_ERROR_REASON_SOURCE_INVALID_TYPE.String())
	case errors.Is(err, biz.ErrInvalidConfig):
		return status.Error(codes.InvalidArgument, feedium.ErrorReason_ERROR_REASON_SOURCE_INVALID_CONFIG.String())
	case errors.Is(err, biz.ErrTypeImmutable):
		return status.Error(codes.InvalidArgument, feedium.ErrorReason_ERROR_REASON_SOURCE_TYPE_IMMUTABLE.String())
	}
	return status.Error(codes.Internal, "internal error")
}
