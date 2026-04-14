//nolint:testpackage // test needs access to unexported types
package source

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/4itosik/feedium/api/feedium"
	"github.com/4itosik/feedium/internal/biz"
	"github.com/4itosik/feedium/internal/service/source/mock"
)

func testSource(id string, sourceType biz.SourceType, config biz.SourceConfig) biz.Source {
	mode := biz.ProcessingModeForType(sourceType)
	return biz.Source{
		ID:             id,
		Type:           sourceType,
		ProcessingMode: mode,
		Config:         config,
		CreatedAt:      time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC),
		UpdatedAt:      time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC),
	}
}

func TestNewSourceService(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockUC := mock.NewMockUsecase(ctrl)
	svc := NewSourceService(mockUC)

	assert.NotNil(t, svc)
}

func TestV1CreateSource(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	t.Run("create RSS source successfully", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockUC := mock.NewMockUsecase(ctrl)
		mockUC.EXPECT().
			Create(gomock.Any(), biz.SourceTypeRSS, gomock.Any()).
			Return(testSource("rss-001", biz.SourceTypeRSS, &biz.RSSConfig{FeedURL: "https://example.com/feed"}), nil)

		svc := NewSourceService(mockUC)

		req := &feedium.V1CreateSourceRequest{
			Type: feedium.SourceType_SOURCE_TYPE_RSS,
			Config: &feedium.SourceConfig{
				Config: &feedium.SourceConfig_Rss{
					Rss: &feedium.RSSConfig{FeedUrl: "https://example.com/feed"},
				},
			},
		}

		resp, err := svc.V1CreateSource(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, "rss-001", resp.GetSource().GetId())
		assert.Equal(t, feedium.SourceType_SOURCE_TYPE_RSS, resp.GetSource().GetType())
	})

	t.Run("invalid config", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockUC := mock.NewMockUsecase(ctrl)
		svc := NewSourceService(mockUC)

		req := &feedium.V1CreateSourceRequest{
			Type:   feedium.SourceType_SOURCE_TYPE_RSS,
			Config: nil,
		}

		resp, err := svc.V1CreateSource(context.Background(), req)

		assert.Nil(t, resp)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
	})

	t.Run("usecase returns error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockUC := mock.NewMockUsecase(ctrl)
		mockUC.EXPECT().
			Create(gomock.Any(), biz.SourceTypeRSS, gomock.Any()).
			Return(biz.Source{}, biz.ErrInvalidConfig)

		svc := NewSourceService(mockUC)

		req := &feedium.V1CreateSourceRequest{
			Type: feedium.SourceType_SOURCE_TYPE_RSS,
			Config: &feedium.SourceConfig{
				Config: &feedium.SourceConfig_Rss{
					Rss: &feedium.RSSConfig{FeedUrl: "https://example.com/feed"},
				},
			},
		}

		resp, err := svc.V1CreateSource(context.Background(), req)

		assert.Nil(t, resp)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
	})
}

func TestV1UpdateSource(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	t.Run("update source successfully", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockUC := mock.NewMockUsecase(ctrl)
		mockUC.EXPECT().
			Update(gomock.Any(), "rss-001", biz.SourceTypeRSS, gomock.Any()).
			Return(testSource("rss-001", biz.SourceTypeRSS, &biz.RSSConfig{FeedURL: "https://updated.com/feed"}), nil)

		svc := NewSourceService(mockUC)

		req := &feedium.V1UpdateSourceRequest{
			Id:   "rss-001",
			Type: feedium.SourceType_SOURCE_TYPE_RSS,
			Config: &feedium.SourceConfig{
				Config: &feedium.SourceConfig_Rss{
					Rss: &feedium.RSSConfig{FeedUrl: "https://updated.com/feed"},
				},
			},
		}

		resp, err := svc.V1UpdateSource(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, "rss-001", resp.GetSource().GetId())
	})

	t.Run("source not found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockUC := mock.NewMockUsecase(ctrl)
		mockUC.EXPECT().
			Update(gomock.Any(), "nonexistent", biz.SourceTypeRSS, gomock.Any()).
			Return(biz.Source{}, biz.ErrSourceNotFound)

		svc := NewSourceService(mockUC)

		req := &feedium.V1UpdateSourceRequest{
			Id:   "nonexistent",
			Type: feedium.SourceType_SOURCE_TYPE_RSS,
			Config: &feedium.SourceConfig{
				Config: &feedium.SourceConfig_Rss{
					Rss: &feedium.RSSConfig{FeedUrl: "https://example.com/feed"},
				},
			},
		}

		resp, err := svc.V1UpdateSource(context.Background(), req)

		assert.Nil(t, resp)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.NotFound, st.Code())
	})
}

func TestV1DeleteSource(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	t.Run("delete source successfully", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockUC := mock.NewMockUsecase(ctrl)
		mockUC.EXPECT().
			Delete(gomock.Any(), "rss-001").
			Return(nil)

		svc := NewSourceService(mockUC)

		req := &feedium.V1DeleteSourceRequest{Id: "rss-001"}

		resp, err := svc.V1DeleteSource(context.Background(), req)

		require.NoError(t, err)
		assert.NotNil(t, resp)
	})

	t.Run("source not found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockUC := mock.NewMockUsecase(ctrl)
		mockUC.EXPECT().
			Delete(gomock.Any(), "nonexistent").
			Return(biz.ErrSourceNotFound)

		svc := NewSourceService(mockUC)

		req := &feedium.V1DeleteSourceRequest{Id: "nonexistent"}

		resp, err := svc.V1DeleteSource(context.Background(), req)

		assert.Nil(t, resp)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.NotFound, st.Code())
	})
}

func TestV1GetSource(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	t.Run("get source successfully", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockUC := mock.NewMockUsecase(ctrl)
		mockUC.EXPECT().
			Get(gomock.Any(), "rss-001").
			Return(testSource("rss-001", biz.SourceTypeRSS, &biz.RSSConfig{FeedURL: "https://example.com/feed"}), nil)

		svc := NewSourceService(mockUC)

		req := &feedium.V1GetSourceRequest{Id: "rss-001"}

		resp, err := svc.V1GetSource(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, "rss-001", resp.GetSource().GetId())
	})

	t.Run("source not found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockUC := mock.NewMockUsecase(ctrl)
		mockUC.EXPECT().
			Get(gomock.Any(), "nonexistent").
			Return(biz.Source{}, biz.ErrSourceNotFound)

		svc := NewSourceService(mockUC)

		req := &feedium.V1GetSourceRequest{Id: "nonexistent"}

		resp, err := svc.V1GetSource(context.Background(), req)

		assert.Nil(t, resp)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.NotFound, st.Code())
	})
}

func TestV1ListSources(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	t.Run("list sources successfully", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		sources := []biz.Source{
			testSource("rss-001", biz.SourceTypeRSS, &biz.RSSConfig{FeedURL: "https://example1.com/feed"}),
			testSource("rss-002", biz.SourceTypeRSS, &biz.RSSConfig{FeedURL: "https://example2.com/feed"}),
		}

		mockUC := mock.NewMockUsecase(ctrl)
		mockUC.EXPECT().
			List(gomock.Any(), gomock.Any()).
			Return(biz.ListSourcesResult{
				Items:         sources,
				NextPageToken: "next-token",
			}, nil)

		svc := NewSourceService(mockUC)

		req := &feedium.V1ListSourcesRequest{PageSize: 10}

		resp, err := svc.V1ListSources(context.Background(), req)

		require.NoError(t, err)
		assert.Len(t, resp.GetItems(), 2)
		assert.Equal(t, "next-token", resp.GetNextPageToken())
	})

	t.Run("list empty results", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockUC := mock.NewMockUsecase(ctrl)
		mockUC.EXPECT().
			List(gomock.Any(), gomock.Any()).
			Return(biz.ListSourcesResult{Items: []biz.Source{}}, nil)

		svc := NewSourceService(mockUC)

		req := &feedium.V1ListSourcesRequest{PageSize: 10}

		resp, err := svc.V1ListSources(context.Background(), req)

		require.NoError(t, err)
		assert.Empty(t, resp.GetItems())
	})
}

func TestMapProtoConfigToDomain(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockUC := mock.NewMockUsecase(ctrl)
	svc := NewSourceService(mockUC)

	t.Run("convert RSS config", func(t *testing.T) {
		protoConfig := &feedium.SourceConfig{
			Config: &feedium.SourceConfig_Rss{
				Rss: &feedium.RSSConfig{FeedUrl: "https://example.com/feed"},
			},
		}

		domainConfig, err := svc.mapProtoConfigToDomain(biz.SourceTypeRSS, protoConfig)

		require.NoError(t, err)
		rcfg, ok := domainConfig.(*biz.RSSConfig)
		require.True(t, ok)
		assert.Equal(t, "https://example.com/feed", rcfg.FeedURL)
	})

	t.Run("nil config", func(t *testing.T) {
		_, err := svc.mapProtoConfigToDomain(biz.SourceTypeRSS, nil)
		assert.Error(t, err)
	})

	t.Run("unknown source type", func(t *testing.T) {
		protoConfig := &feedium.SourceConfig{
			Config: &feedium.SourceConfig_Rss{
				Rss: &feedium.RSSConfig{FeedUrl: "https://example.com/feed"},
			},
		}

		_, err := svc.mapProtoConfigToDomain(biz.SourceType("unknown"), protoConfig)
		assert.Error(t, err)
	})
}

func TestMapDomainToProto(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockUC := mock.NewMockUsecase(ctrl)
	svc := NewSourceService(mockUC)

	t.Run("convert RSS source", func(t *testing.T) {
		source := testSource("rss-001", biz.SourceTypeRSS, &biz.RSSConfig{FeedURL: "https://example.com/feed"})

		protoSource, err := svc.mapDomainToProto(source)

		require.NoError(t, err)
		assert.Equal(t, "rss-001", protoSource.GetId())
		assert.Equal(t, feedium.SourceType_SOURCE_TYPE_RSS, protoSource.GetType())
	})

	t.Run("with invalid config type", func(t *testing.T) {
		source := biz.Source{
			ID:             "test-001",
			Type:           biz.SourceTypeRSS,
			ProcessingMode: biz.ProcessingModeSelfContained,
			Config:         &biz.HTMLConfig{URL: "https://example.com"},
			CreatedAt:      time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC),
			UpdatedAt:      time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC),
		}

		protoSource, err := svc.mapDomainToProto(source)

		require.Error(t, err)
		assert.Nil(t, protoSource)
	})
}

func TestMapDomainConfigToProto(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockUC := mock.NewMockUsecase(ctrl)
	svc := NewSourceService(mockUC)

	t.Run("convert RSS config", func(t *testing.T) {
		config := &biz.RSSConfig{FeedURL: "https://example.com/feed"}

		protoConfig, err := svc.mapDomainConfigToProto(biz.SourceTypeRSS, config)

		require.NoError(t, err)
		rss := protoConfig.GetRss()
		assert.NotNil(t, rss)
	})

	t.Run("mismatched config type", func(t *testing.T) {
		config := &biz.HTMLConfig{URL: "https://example.com"}

		_, err := svc.mapDomainConfigToProto(biz.SourceTypeRSS, config)

		assert.Error(t, err)
	})
}

func TestMapDomainTypeToProto(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockUC := mock.NewMockUsecase(ctrl)
	svc := NewSourceService(mockUC)

	result := svc.mapDomainTypeToProto(biz.SourceTypeRSS)
	assert.Equal(t, feedium.SourceType_SOURCE_TYPE_RSS, result)
}

func TestMapDomainModeToProto(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockUC := mock.NewMockUsecase(ctrl)
	svc := NewSourceService(mockUC)

	result := svc.mapDomainModeToProto(biz.ProcessingModeSelfContained)
	assert.Equal(t, feedium.ProcessingMode_PROCESSING_MODE_SELF_CONTAINED, result)
}

func TestMapProtoTypeToDomain(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockUC := mock.NewMockUsecase(ctrl)
	svc := NewSourceService(mockUC)

	result := svc.mapProtoTypeToDomain(feedium.SourceType_SOURCE_TYPE_RSS)
	assert.Equal(t, biz.SourceTypeRSS, result)
}

func TestMapDomainErrorToStatus(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockUC := mock.NewMockUsecase(ctrl)
	svc := NewSourceService(mockUC)

	tests := []struct {
		name         string
		err          error
		expectedCode codes.Code
	}{
		{
			name:         "source not found",
			err:          biz.ErrSourceNotFound,
			expectedCode: codes.NotFound,
		},
		{
			name:         "invalid source type",
			err:          biz.ErrInvalidSourceType,
			expectedCode: codes.InvalidArgument,
		},
		{
			name:         "invalid config",
			err:          biz.ErrInvalidConfig,
			expectedCode: codes.InvalidArgument,
		},
		{
			name:         "type immutable",
			err:          biz.ErrTypeImmutable,
			expectedCode: codes.InvalidArgument,
		},
		{
			name:         "unknown error",
			err:          errors.New("unknown"),
			expectedCode: codes.Internal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			statusErr := svc.mapDomainErrorToStatus(tt.err)
			st, ok := status.FromError(statusErr)
			require.True(t, ok)
			assert.Equal(t, tt.expectedCode, st.Code())
		})
	}
}

func TestV1CreateSourceAllTypes(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	t.Run("create Telegram channel", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockUC := mock.NewMockUsecase(ctrl)
		mockUC.EXPECT().
			Create(gomock.Any(), biz.SourceTypeTelegramChannel, gomock.Any()).
			Return(testSource("tg-001", biz.SourceTypeTelegramChannel, &biz.TelegramChannelConfig{TgID: 123, Username: "ch"}), nil)

		svc := NewSourceService(mockUC)

		req := &feedium.V1CreateSourceRequest{
			Type: feedium.SourceType_SOURCE_TYPE_TELEGRAM_CHANNEL,
			Config: &feedium.SourceConfig{
				Config: &feedium.SourceConfig_TelegramChannel{
					TelegramChannel: &feedium.TelegramChannelConfig{TgId: 123, Username: "ch"},
				},
			},
		}

		resp, err := svc.V1CreateSource(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, feedium.ProcessingMode_PROCESSING_MODE_SELF_CONTAINED, resp.GetSource().GetProcessingMode())
	})

	t.Run("create Telegram group", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockUC := mock.NewMockUsecase(ctrl)
		mockUC.EXPECT().
			Create(gomock.Any(), biz.SourceTypeTelegramGroup, gomock.Any()).
			Return(testSource("tg-002", biz.SourceTypeTelegramGroup, &biz.TelegramGroupConfig{TgID: 456, Username: "grp"}), nil)

		svc := NewSourceService(mockUC)

		req := &feedium.V1CreateSourceRequest{
			Type: feedium.SourceType_SOURCE_TYPE_TELEGRAM_GROUP,
			Config: &feedium.SourceConfig{
				Config: &feedium.SourceConfig_TelegramGroup{
					TelegramGroup: &feedium.TelegramGroupConfig{TgId: 456, Username: "grp"},
				},
			},
		}

		resp, err := svc.V1CreateSource(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, feedium.ProcessingMode_PROCESSING_MODE_CUMULATIVE, resp.GetSource().GetProcessingMode())
	})

	t.Run("create HTML source", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockUC := mock.NewMockUsecase(ctrl)
		mockUC.EXPECT().
			Create(gomock.Any(), biz.SourceTypeHTML, gomock.Any()).
			Return(testSource("html-001", biz.SourceTypeHTML, &biz.HTMLConfig{URL: "https://example.com"}), nil)

		svc := NewSourceService(mockUC)

		req := &feedium.V1CreateSourceRequest{
			Type: feedium.SourceType_SOURCE_TYPE_HTML,
			Config: &feedium.SourceConfig{
				Config: &feedium.SourceConfig_Html{
					Html: &feedium.HTMLConfig{Url: "https://example.com"},
				},
			},
		}

		resp, err := svc.V1CreateSource(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, feedium.SourceType_SOURCE_TYPE_HTML, resp.GetSource().GetType())
	})
}

func TestV1ListSourcesWithFilter(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	t.Run("with type filter", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		sources := []biz.Source{
			testSource("tg-001", biz.SourceTypeTelegramChannel, &biz.TelegramChannelConfig{TgID: 123, Username: "ch"}),
		}

		mockUC := mock.NewMockUsecase(ctrl)
		mockUC.EXPECT().
			List(gomock.Any(), gomock.Any()).
			Return(biz.ListSourcesResult{Items: sources}, nil)

		svc := NewSourceService(mockUC)

		sourceType := feedium.SourceType_SOURCE_TYPE_TELEGRAM_CHANNEL
		req := &feedium.V1ListSourcesRequest{Type: &sourceType, PageSize: 10}

		resp, err := svc.V1ListSources(context.Background(), req)

		require.NoError(t, err)
		assert.Len(t, resp.GetItems(), 1)
	})
}

func TestMapDomainTypeToProtoAllTypes(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockUC := mock.NewMockUsecase(ctrl)
	svc := NewSourceService(mockUC)

	tests := []struct {
		name     string
		input    biz.SourceType
		expected feedium.SourceType
	}{
		{"telegram_channel", biz.SourceTypeTelegramChannel, feedium.SourceType_SOURCE_TYPE_TELEGRAM_CHANNEL},
		{"telegram_group", biz.SourceTypeTelegramGroup, feedium.SourceType_SOURCE_TYPE_TELEGRAM_GROUP},
		{"rss", biz.SourceTypeRSS, feedium.SourceType_SOURCE_TYPE_RSS},
		{"html", biz.SourceTypeHTML, feedium.SourceType_SOURCE_TYPE_HTML},
		{"unknown", biz.SourceType("unknown"), feedium.SourceType_SOURCE_TYPE_UNSPECIFIED},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := svc.mapDomainTypeToProto(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMapDomainModeToProtoAllModes(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockUC := mock.NewMockUsecase(ctrl)
	svc := NewSourceService(mockUC)

	tests := []struct {
		name     string
		input    biz.ProcessingMode
		expected feedium.ProcessingMode
	}{
		{"self_contained", biz.ProcessingModeSelfContained, feedium.ProcessingMode_PROCESSING_MODE_SELF_CONTAINED},
		{"cumulative", biz.ProcessingModeCumulative, feedium.ProcessingMode_PROCESSING_MODE_CUMULATIVE},
		{"unknown", biz.ProcessingMode("unknown"), feedium.ProcessingMode_PROCESSING_MODE_UNSPECIFIED},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := svc.mapDomainModeToProto(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMapProtoTypeToDomainAllTypes(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockUC := mock.NewMockUsecase(ctrl)
	svc := NewSourceService(mockUC)

	tests := []struct {
		name     string
		input    feedium.SourceType
		expected biz.SourceType
	}{
		{"telegram_channel", feedium.SourceType_SOURCE_TYPE_TELEGRAM_CHANNEL, biz.SourceTypeTelegramChannel},
		{"telegram_group", feedium.SourceType_SOURCE_TYPE_TELEGRAM_GROUP, biz.SourceTypeTelegramGroup},
		{"rss", feedium.SourceType_SOURCE_TYPE_RSS, biz.SourceTypeRSS},
		{"html", feedium.SourceType_SOURCE_TYPE_HTML, biz.SourceTypeHTML},
		{"unspecified", feedium.SourceType_SOURCE_TYPE_UNSPECIFIED, biz.SourceType("")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := svc.mapProtoTypeToDomain(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMapProtoConfigToDomainAllTypes(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockUC := mock.NewMockUsecase(ctrl)
	svc := NewSourceService(mockUC)

	t.Run("telegram_channel", func(t *testing.T) {
		protoConfig := &feedium.SourceConfig{
			Config: &feedium.SourceConfig_TelegramChannel{
				TelegramChannel: &feedium.TelegramChannelConfig{TgId: 123, Username: "ch"},
			},
		}
		domainConfig, err := svc.mapProtoConfigToDomain(biz.SourceTypeTelegramChannel, protoConfig)
		require.NoError(t, err)
		tc, ok := domainConfig.(*biz.TelegramChannelConfig)
		require.True(t, ok)
		assert.Equal(t, int64(123), tc.TgID)
	})

	t.Run("telegram_group", func(t *testing.T) {
		protoConfig := &feedium.SourceConfig{
			Config: &feedium.SourceConfig_TelegramGroup{
				TelegramGroup: &feedium.TelegramGroupConfig{TgId: 456, Username: "grp"},
			},
		}
		domainConfig, err := svc.mapProtoConfigToDomain(biz.SourceTypeTelegramGroup, protoConfig)
		require.NoError(t, err)
		tg, ok := domainConfig.(*biz.TelegramGroupConfig)
		require.True(t, ok)
		assert.Equal(t, int64(456), tg.TgID)
	})

	t.Run("html", func(t *testing.T) {
		protoConfig := &feedium.SourceConfig{
			Config: &feedium.SourceConfig_Html{
				Html: &feedium.HTMLConfig{Url: "https://example.com"},
			},
		}
		domainConfig, err := svc.mapProtoConfigToDomain(biz.SourceTypeHTML, protoConfig)
		require.NoError(t, err)
		hc, ok := domainConfig.(*biz.HTMLConfig)
		require.True(t, ok)
		assert.Equal(t, "https://example.com", hc.URL)
	})

	t.Run("mismatched config", func(t *testing.T) {
		protoConfig := &feedium.SourceConfig{
			Config: &feedium.SourceConfig_Html{
				Html: &feedium.HTMLConfig{Url: "https://example.com"},
			},
		}
		_, err := svc.mapProtoConfigToDomain(biz.SourceTypeRSS, protoConfig)
		assert.Error(t, err)
	})
}

func TestMapDomainConfigToProtoAllTypes(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockUC := mock.NewMockUsecase(ctrl)
	svc := NewSourceService(mockUC)

	t.Run("telegram_channel", func(t *testing.T) {
		config := &biz.TelegramChannelConfig{TgID: 123, Username: "ch"}
		protoConfig, err := svc.mapDomainConfigToProto(biz.SourceTypeTelegramChannel, config)
		require.NoError(t, err)
		tc := protoConfig.GetTelegramChannel()
		assert.NotNil(t, tc)
		assert.Equal(t, int64(123), tc.GetTgId())
	})

	t.Run("telegram_group", func(t *testing.T) {
		config := &biz.TelegramGroupConfig{TgID: 456, Username: "grp"}
		protoConfig, err := svc.mapDomainConfigToProto(biz.SourceTypeTelegramGroup, config)
		require.NoError(t, err)
		tg := protoConfig.GetTelegramGroup()
		assert.NotNil(t, tg)
		assert.Equal(t, int64(456), tg.GetTgId())
	})

	t.Run("html", func(t *testing.T) {
		config := &biz.HTMLConfig{URL: "https://example.com"}
		protoConfig, err := svc.mapDomainConfigToProto(biz.SourceTypeHTML, config)
		require.NoError(t, err)
		html := protoConfig.GetHtml()
		assert.NotNil(t, html)
		assert.Equal(t, "https://example.com", html.GetUrl())
	})

	t.Run("mismatched config", func(t *testing.T) {
		config := &biz.HTMLConfig{URL: "https://example.com"}
		_, err := svc.mapDomainConfigToProto(biz.SourceTypeRSS, config)
		assert.Error(t, err)
	})

	t.Run("unknown source type", func(t *testing.T) {
		config := &biz.RSSConfig{FeedURL: "https://example.com/feed"}
		_, err := svc.mapDomainConfigToProto(biz.SourceType("unknown"), config)
		assert.Error(t, err)
	})
}
