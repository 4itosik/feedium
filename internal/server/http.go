package server

import (
	"log/slog"

	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"

	feediumv1 "github.com/4itosik/feedium/api/feedium"
	"github.com/4itosik/feedium/internal/conf"
	healthservice "github.com/4itosik/feedium/internal/service/health"
	postservice "github.com/4itosik/feedium/internal/service/post"
	sourceservice "github.com/4itosik/feedium/internal/service/source"
	summaryservice "github.com/4itosik/feedium/internal/service/summary"
)

func NewHTTPServer(
	c *conf.Server,
	hs *healthservice.HealthService,
	ss *sourceservice.SourceService,
	ps *postservice.PostService,
	sms *summaryservice.SummaryService,
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
	feediumv1.RegisterPostServiceHTTPServer(srv, ps)
	feediumv1.RegisterSummaryServiceHTTPServer(srv, sms)
	return srv
}
