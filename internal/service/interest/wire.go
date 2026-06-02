package interest

import "github.com/google/wire"

// ProviderSet Wire provider set for interest service.
//
//nolint:gochecknoglobals // Wire provider sets are intentionally global
var ProviderSet = wire.NewSet(NewInterestService)
