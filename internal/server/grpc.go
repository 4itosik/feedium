package server

import (
	"log/slog"

	"github.com/go-kratos/kratos/v2/transport/grpc"

	"github.com/4itosik/feedium/internal/conf"
)

func NewGRPCServer(c *conf.Server, _ *slog.Logger) *grpc.Server {
	var opts []grpc.ServerOption
	if c.GetGrpc().GetAddr() != "" {
		opts = append(opts, grpc.Address(c.GetGrpc().GetAddr()))
	}
	if c.GetGrpc().GetTimeout() != nil {
		opts = append(opts, grpc.Timeout(c.GetGrpc().GetTimeout().AsDuration()))
	}
	srv := grpc.NewServer(opts...)
	return srv
}
