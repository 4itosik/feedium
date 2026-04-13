package health

import "github.com/google/wire"

// ProviderSet Wire provider set for health service
//
//nolint:gochecknoglobals // Wire provider sets are intentionally global
var ProviderSet = wire.NewSet(NewHealthService)
