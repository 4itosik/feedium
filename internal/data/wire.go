package data

import (
	"github.com/google/wire"

	"github.com/4itosik/feedium/internal/biz"
	healthservice "github.com/4itosik/feedium/internal/service/health"
)

// ProviderSet Wire provider set for data package
//
//nolint:gochecknoglobals // Wire provider sets are intentionally global
var ProviderSet = wire.NewSet(
	NewData,
	NewHealthRepo,
	NewSourceRepo,
	NewPostRepo,
	wire.Bind(new(healthservice.Pinger), new(*HealthRepo)),
	wire.Bind(new(biz.SourceRepo), new(*sourceRepo)),
	wire.Bind(new(biz.PostRepo), new(*postRepo)),
)
