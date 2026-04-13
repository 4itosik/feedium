package server

import (
	"log/slog"

	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"

	"github.com/4itosik/feedium/internal/conf"
	healthservice "github.com/4itosik/feedium/internal/service/health"
)

func NewHTTPServer(c *conf.Server, hs *healthservice.HealthService, _ *slog.Logger) *kratoshttp.Server {
	var opts []kratoshttp.ServerOption
	if c.GetHttp().GetAddr() != "" {
		opts = append(opts, kratoshttp.Address(c.GetHttp().GetAddr()))
	}
	if c.GetHttp().GetTimeout() != nil {
		opts = append(opts, kratoshttp.Timeout(c.GetHttp().GetTimeout().AsDuration()))
	}
	srv := kratoshttp.NewServer(opts...)
	srv.Handle("/healthz", healthservice.HTTPHandler(hs))
	return srv
}
