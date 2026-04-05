package connect_test

import (
	"context"
	"errors"
	"testing"

	sourcev1 "feedium/api/source/v1"
	"feedium/internal/app/source"
	sourceconnect "feedium/internal/app/source/adapters/connect"
	"feedium/internal/app/source/mocks"
	"feedium/internal/platform/logger"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"go.uber.org/mock/gomock"
)

func TestHandlerMappings(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := mocks.NewMockRepository(ctrl)
	svc := source.NewService(repo, logger.Init())
	h := sourceconnect.New(svc, logger.Init())
	ctx := context.Background()
	id := uuid.New()

	tests := []struct {
		name string
		call func() error
		want connect.Code
	}{
		{"invalid_uuid", func() error {
			_, err := h.GetSource(ctx, connect.NewRequest(&sourcev1.GetSourceRequest{Id: "bad"}))
			return err
		}, connect.CodeInvalidArgument},
		{"invalid_enum", func() error {
			_, err := h.CreateSource(
				ctx,
				connect.NewRequest(
					&sourcev1.CreateSourceRequest{
						Type: sourcev1.SourceType(99),
						Name: "n",
						Url:  "https://x",
						Config: &sourcev1.SourceConfig{
							Config: &sourcev1.SourceConfig_Rss{Rss: &sourcev1.RssConfig{FeedUrl: "https://f"}},
						},
					},
				),
			)
			return err
		}, connect.CodeInvalidArgument},
		{"not_found", func() error {
			repo.EXPECT().GetByID(ctx, id).Return(nil, source.ErrNotFound)
			_, err := h.GetSource(ctx, connect.NewRequest(&sourcev1.GetSourceRequest{Id: id.String()}))
			return err
		}, connect.CodeNotFound},
		{"internal", func() error {
			repo.EXPECT().Delete(ctx, id).Return(errors.New("boom"))
			_, err := h.DeleteSource(ctx, connect.NewRequest(&sourcev1.DeleteSourceRequest{Id: id.String()}))
			return err
		}, connect.CodeInternal},
		{"list_invalid_enum", func() error {
			_, err := h.ListSources(
				ctx,
				connect.NewRequest(&sourcev1.ListSourcesRequest{TypeFilter: sourcev1.SourceType(42)}),
			)
			return err
		}, connect.CodeInvalidArgument},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			if err == nil {
				t.Fatalf("expected error")
			}
			if c := connect.CodeOf(err); c != tc.want {
				t.Fatalf("got code %v want %v", c, tc.want)
			}
		})
	}
}
