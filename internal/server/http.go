package server

import (
	"log/slog"

	"github.com/go-kratos/kratos/v2/transport/http"

	"github.com/4itosik/feedium/internal/conf"
)

func NewHTTPServer(c *conf.Server, _ *slog.Logger) *http.Server {
	var opts []http.ServerOption
	if c.GetHttp().GetAddr() != "" {
		opts = append(opts, http.Address(c.GetHttp().GetAddr()))
	}
	if c.GetHttp().GetTimeout() != nil {
		opts = append(opts, http.Timeout(c.GetHttp().GetTimeout().AsDuration()))
	}
	srv := http.NewServer(opts...)
	return srv
}
