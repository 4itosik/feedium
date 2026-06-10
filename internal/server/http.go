package server

import (
	"log/slog"

	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"

	feediumv1 "github.com/4itosik/feedium/api/feedium"
	"github.com/4itosik/feedium/internal/conf"
	feedservice "github.com/4itosik/feedium/internal/service/feed"
	healthservice "github.com/4itosik/feedium/internal/service/health"
	interestservice "github.com/4itosik/feedium/internal/service/interest"
	postservice "github.com/4itosik/feedium/internal/service/post"
	sourceservice "github.com/4itosik/feedium/internal/service/source"
	summaryservice "github.com/4itosik/feedium/internal/service/summary"
	"github.com/4itosik/feedium/ui"
)

func NewHTTPServer(
	c *conf.Server,
	hs *healthservice.HealthService,
	ss *sourceservice.SourceService,
	ps *postservice.PostService,
	sms *summaryservice.Service,
	is *interestservice.InterestService,
	fs *feedservice.FeedService,
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
	// API routes registered first so SPA handler does not shadow them.
	srv.Handle("/healthz", healthservice.HTTPHandler(hs))
	feediumv1.RegisterSourceServiceHTTPServer(srv, ss)
	feediumv1.RegisterPostServiceHTTPServer(srv, ps)
	feediumv1.RegisterSummaryServiceHTTPServer(srv, sms)
	feediumv1.RegisterInterestServiceHTTPServer(srv, is)
	feediumv1.RegisterFeedServiceHTTPServer(srv, fs)
	// SPA fallback: serves ui/dist for all non-API paths (gorilla/mux matches in registration order).
	srv.HandlePrefix("/", NewSPAHandler(ui.Assets()))
	return srv
}
