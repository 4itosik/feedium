package connect

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"strings"

	sourcev1 "feedium/api/source/v1"
	sourcev1connect "feedium/api/source/v1/sourcev1connect"
	"feedium/internal/app/source"
	"feedium/internal/app/summary"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Handler struct {
	svc             *source.Service
	log             *slog.Logger
	outboxEventRepo summary.OutboxEventRepository
	sourceQueryRepo summary.SourceQueryRepository
}

func New(svc *source.Service, log *slog.Logger) *Handler { return &Handler{svc: svc, log: log} }

// NewWithProcessing creates a new Handler with dependencies for ProcessSource.
func NewWithProcessing(
	svc *source.Service,
	log *slog.Logger,
	outboxEventRepo summary.OutboxEventRepository,
	sourceQueryRepo summary.SourceQueryRepository,
) *Handler {
	return &Handler{
		svc:             svc,
		log:             log,
		outboxEventRepo: outboxEventRepo,
		sourceQueryRepo: sourceQueryRepo,
	}
}

var _ sourcev1connect.SourceServiceHandler = (*Handler)(nil)

func (h *Handler) CreateSource(
	ctx context.Context,
	req *connect.Request[sourcev1.CreateSourceRequest],
) (*connect.Response[sourcev1.CreateSourceResponse], error) {
	if !isKnownType(req.Msg.GetType()) {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid source type"))
	}
	src, err := h.svc.Create(ctx, fromCreateRequest(req.Msg))
	if err != nil {
		return nil, h.mapError(err)
	}
	return connect.NewResponse(&sourcev1.CreateSourceResponse{Source: toProto(src)}), nil
}

func (h *Handler) GetSource(
	ctx context.Context,
	req *connect.Request[sourcev1.GetSourceRequest],
) (*connect.Response[sourcev1.GetSourceResponse], error) {
	id, err := uuid.Parse(strings.TrimSpace(req.Msg.GetId()))
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	src, err := h.svc.Get(ctx, id)
	if err != nil {
		return nil, h.mapError(err)
	}
	return connect.NewResponse(&sourcev1.GetSourceResponse{Source: toProto(src)}), nil
}

func (h *Handler) UpdateSource(
	ctx context.Context,
	req *connect.Request[sourcev1.UpdateSourceRequest],
) (*connect.Response[sourcev1.UpdateSourceResponse], error) {
	if !isKnownType(req.Msg.GetType()) {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid source type"))
	}
	src := fromUpdateRequest(req.Msg)
	id, err := uuid.Parse(strings.TrimSpace(req.Msg.GetId()))
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	src.ID = id
	updated, err := h.svc.Update(ctx, src)
	if err != nil {
		return nil, h.mapError(err)
	}
	return connect.NewResponse(&sourcev1.UpdateSourceResponse{Source: toProto(updated)}), nil
}

func (h *Handler) DeleteSource(
	ctx context.Context,
	req *connect.Request[sourcev1.DeleteSourceRequest],
) (*connect.Response[sourcev1.DeleteSourceResponse], error) {
	id, err := uuid.Parse(strings.TrimSpace(req.Msg.GetId()))
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	deleteErr := h.svc.Delete(ctx, id)
	if deleteErr != nil {
		return nil, h.mapError(deleteErr)
	}
	return connect.NewResponse(&sourcev1.DeleteSourceResponse{}), nil
}

func (h *Handler) ListSources(
	ctx context.Context,
	req *connect.Request[sourcev1.ListSourcesRequest],
) (*connect.Response[sourcev1.ListSourcesResponse], error) {
	if req.Msg.GetTypeFilter() != sourcev1.SourceType_SOURCE_TYPE_UNSPECIFIED && !isKnownType(req.Msg.GetTypeFilter()) {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid source type filter"))
	}
	srcs, total, err := h.svc.List(
		ctx,
		source.ListFilter{
			Type:     fromProtoType(req.Msg.GetTypeFilter()),
			PageSize: int(req.Msg.GetPageSize()),
			Page:     int(req.Msg.GetPage()),
		},
	)
	if err != nil {
		return nil, h.mapError(err)
	}
	out := make([]*sourcev1.Source, 0, len(srcs))
	for i := range srcs {
		s := srcs[i]
		out = append(out, toProto(&s))
	}
	var totalCount int32
	if total > math.MaxInt32 {
		totalCount = math.MaxInt32
	} else {
		totalCount = int32(total) //nolint:gosec // total is range-checked against MaxInt32 above.
	}
	return connect.NewResponse(&sourcev1.ListSourcesResponse{
		Sources:    out,
		TotalCount: totalCount,
	}), nil
}

func (h *Handler) ProcessSource(
	ctx context.Context,
	req *connect.Request[sourcev1.ProcessSourceRequest],
) (*connect.Response[sourcev1.ProcessSourceResponse], error) {
	// Validate UUID
	id, err := uuid.Parse(strings.TrimSpace(req.Msg.GetId()))
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	// Check if source exists
	if _, err = h.sourceQueryRepo.GetByID(ctx, id); err != nil {
		if errors.Is(err, source.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		// For other errors, return internal error
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to check source"))
	}

	// Create a MANUAL outbox event
	event := &summary.OutboxEvent{
		SourceID:  id,
		PostID:    nil,
		EventType: summary.EventTypeManual,
		Status:    summary.EventStatusPending,
	}

	if err = h.outboxEventRepo.Create(ctx, event); err != nil {
		if h.log != nil {
			h.log.ErrorContext(ctx, "failed to create outbox event", "source_id", id, "error", err)
		}
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to create event"))
	}

	return connect.NewResponse(&sourcev1.ProcessSourceResponse{
		EventId: event.ID.String(),
	}), nil
}

func (h *Handler) mapError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, source.ErrNotFound):
		return connect.NewError(connect.CodeNotFound, err)
	case isValidationErr(err):
		return connect.NewError(connect.CodeInvalidArgument, err)
	default:
		if h.log != nil {
			h.log.Error("source handler error", "error", err)
		}
		return connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
}

func isValidationErr(err error) bool {
	var v source.ValidationError
	return errors.As(err, &v)
}

func fromCreateRequest(req *sourcev1.CreateSourceRequest) *source.Source {
	return &source.Source{
		Type:   fromProtoType(req.GetType()),
		Name:   req.GetName(),
		URL:    req.GetUrl(),
		Config: fromProtoConfig(req.GetType(), req.GetConfig()),
	}
}

func fromUpdateRequest(req *sourcev1.UpdateSourceRequest) *source.Source {
	return &source.Source{
		Type:   fromProtoType(req.GetType()),
		Name:   req.GetName(),
		URL:    req.GetUrl(),
		Config: fromProtoConfig(req.GetType(), req.GetConfig()),
	}
}

func fromProtoType(t sourcev1.SourceType) source.Type {
	switch t {
	case sourcev1.SourceType_SOURCE_TYPE_UNSPECIFIED:
		return ""
	case sourcev1.SourceType_SOURCE_TYPE_TELEGRAM_CHANNEL:
		return source.TypeTelegramChannel
	case sourcev1.SourceType_SOURCE_TYPE_TELEGRAM_GROUP:
		return source.TypeTelegramGroup
	case sourcev1.SourceType_SOURCE_TYPE_RSS:
		return source.TypeRSS
	case sourcev1.SourceType_SOURCE_TYPE_WEB_SCRAPING:
		return source.TypeWebScraping
	default:
		return ""
	}
}

func isKnownType(t sourcev1.SourceType) bool {
	switch t {
	case sourcev1.SourceType_SOURCE_TYPE_UNSPECIFIED,
		sourcev1.SourceType_SOURCE_TYPE_TELEGRAM_CHANNEL,
		sourcev1.SourceType_SOURCE_TYPE_TELEGRAM_GROUP,
		sourcev1.SourceType_SOURCE_TYPE_RSS,
		sourcev1.SourceType_SOURCE_TYPE_WEB_SCRAPING:
		return true
	default:
		return false
	}
}

func fromProtoConfig(t sourcev1.SourceType, cfg *sourcev1.SourceConfig) map[string]any {
	if cfg == nil {
		return nil
	}
	switch v := cfg.GetConfig().(type) {
	case *sourcev1.SourceConfig_TelegramChannel:
		if t != sourcev1.SourceType_SOURCE_TYPE_TELEGRAM_CHANNEL {
			return nil
		}
		return map[string]any{"channel_id": v.TelegramChannel.GetChannelId()}
	case *sourcev1.SourceConfig_TelegramGroup:
		if t != sourcev1.SourceType_SOURCE_TYPE_TELEGRAM_GROUP {
			return nil
		}
		return map[string]any{"group_id": v.TelegramGroup.GetGroupId()}
	case *sourcev1.SourceConfig_Rss:
		if t != sourcev1.SourceType_SOURCE_TYPE_RSS {
			return nil
		}
		return map[string]any{"feed_url": v.Rss.GetFeedUrl()}
	case *sourcev1.SourceConfig_WebScraping:
		if t != sourcev1.SourceType_SOURCE_TYPE_WEB_SCRAPING {
			return nil
		}
		return map[string]any{"selector": v.WebScraping.GetSelector()}
	default:
		return nil
	}
}

func toProto(src *source.Source) *sourcev1.Source {
	if src == nil {
		return nil
	}
	return &sourcev1.Source{
		Id:        src.ID.String(),
		Type:      toProtoType(src.Type),
		Name:      src.Name,
		Url:       src.URL,
		Config:    toProtoConfig(src.Type, src.Config),
		CreatedAt: timestamppb.New(src.CreatedAt),
		UpdatedAt: timestamppb.New(src.UpdatedAt),
	}
}

func toProtoType(t source.Type) sourcev1.SourceType {
	switch t {
	case source.TypeTelegramChannel:
		return sourcev1.SourceType_SOURCE_TYPE_TELEGRAM_CHANNEL
	case source.TypeTelegramGroup:
		return sourcev1.SourceType_SOURCE_TYPE_TELEGRAM_GROUP
	case source.TypeRSS:
		return sourcev1.SourceType_SOURCE_TYPE_RSS
	case source.TypeWebScraping:
		return sourcev1.SourceType_SOURCE_TYPE_WEB_SCRAPING
	default:
		return sourcev1.SourceType_SOURCE_TYPE_UNSPECIFIED
	}
}

func toProtoConfig(t source.Type, cfg map[string]any) *sourcev1.SourceConfig {
	if cfg == nil {
		return nil
	}
	switch t {
	case source.TypeTelegramChannel:
		return &sourcev1.SourceConfig{
			Config: &sourcev1.SourceConfig_TelegramChannel{
				TelegramChannel: &sourcev1.TelegramChannelConfig{ChannelId: str(cfg["channel_id"])},
			},
		}
	case source.TypeTelegramGroup:
		return &sourcev1.SourceConfig{
			Config: &sourcev1.SourceConfig_TelegramGroup{
				TelegramGroup: &sourcev1.TelegramGroupConfig{GroupId: str(cfg["group_id"])},
			},
		}
	case source.TypeRSS:
		return &sourcev1.SourceConfig{
			Config: &sourcev1.SourceConfig_Rss{Rss: &sourcev1.RssConfig{FeedUrl: str(cfg["feed_url"])}},
		}
	case source.TypeWebScraping:
		return &sourcev1.SourceConfig{
			Config: &sourcev1.SourceConfig_WebScraping{
				WebScraping: &sourcev1.WebScrapingConfig{Selector: str(cfg["selector"])},
			},
		}
	default:
		return nil
	}
}

func str(v any) string {
	s, _ := v.(string)
	return s
}
