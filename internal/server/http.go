package server

import (
	"log/slog"

	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"

	feediumv1 "github.com/4itosik/feedium/api/feedium"
	"github.com/4itosik/feedium/internal/conf"
	healthservice "github.com/4itosik/feedium/internal/service/health"
	sourceservice "github.com/4itosik/feedium/internal/service/source"
)

func NewHTTPServer(
	c *conf.Server,
	hs *healthservice.HealthService,
	ss *sourceservice.SourceService,
	_ *slog.Logger,
) *kratoshttp.Server {
	var opts []kratoshttp.ServerOption
	if c.GetHttp().GetAddr() != "" {
		opts = append(opts, kratoshttp.Address(c.GetHttp().GetAddr()))
	}
	if c.GetHttp().GetTimeout() != nil {
		opts = append(opts, kratoshttp.Timeout(c.GetHttp().GetTimeout().AsDuration()))
	}
	srv := kratoshttp.NewServer(opts...)
	srv.Handle("/healthz", healthservice.HTTPHandler(hs))
	feediumv1.RegisterSourceServiceHTTPServer(srv, ss)
	return srv
}
