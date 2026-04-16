package server

import (
	"log/slog"

	"github.com/go-kratos/kratos/v2/transport/grpc"

	feediumv1 "github.com/4itosik/feedium/api/feedium"
	"github.com/4itosik/feedium/internal/conf"
	healthservice "github.com/4itosik/feedium/internal/service/health"
	postservice "github.com/4itosik/feedium/internal/service/post"
	sourceservice "github.com/4itosik/feedium/internal/service/source"
	summaryservice "github.com/4itosik/feedium/internal/service/summary"
)

func NewGRPCServer(
	c *conf.Server,
	hs *healthservice.HealthService,
	ss *sourceservice.SourceService,
	ps *postservice.PostService,
	sms *summaryservice.Service,
	_ *slog.Logger,
) *grpc.Server {
	var opts []grpc.ServerOption
	if c.GetGrpc().GetAddr() != "" {
		opts = append(opts, grpc.Address(c.GetGrpc().GetAddr()))
	}
	if c.GetGrpc().GetTimeout() != nil {
		opts = append(opts, grpc.Timeout(c.GetGrpc().GetTimeout().AsDuration()))
	}
	srv := grpc.NewServer(opts...)
	feediumv1.RegisterHealthServiceServer(srv, hs)
	feediumv1.RegisterSourceServiceServer(srv, ss)
	feediumv1.RegisterPostServiceServer(srv, ps)
	feediumv1.RegisterSummaryServiceServer(srv, sms)
	return srv
}
