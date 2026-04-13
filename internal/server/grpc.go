package server

import (
	"log/slog"

	"github.com/go-kratos/kratos/v2/transport/grpc"

	feediumv1 "github.com/4itosik/feedium/api/feedium"
	"github.com/4itosik/feedium/internal/conf"
	healthservice "github.com/4itosik/feedium/internal/service/health"
	sourceservice "github.com/4itosik/feedium/internal/service/source"
)

func NewGRPCServer(
	c *conf.Server,
	hs *healthservice.HealthService,
	ss *sourceservice.SourceService,
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
	return srv
}
