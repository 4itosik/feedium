package source

import "github.com/google/wire"

// ProviderSet Wire provider set for source service
//
//nolint:gochecknoglobals // Wire provider sets are intentionally global
var ProviderSet = wire.NewSet(NewSourceService)
