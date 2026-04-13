package data

import (
	"github.com/google/wire"

	healthservice "github.com/4itosik/feedium/internal/service/health"
)

// ProviderSet Wire provider set for data package
//
//nolint:gochecknoglobals // Wire provider sets are intentionally global
var ProviderSet = wire.NewSet(NewData, NewHealthRepo, wire.Bind(new(healthservice.Pinger), new(*HealthRepo)))
