package feed

import "github.com/google/wire"

// ProviderSet Wire provider set for feed service.
//
//nolint:gochecknoglobals // Wire provider sets are intentionally global
var ProviderSet = wire.NewSet(NewFeedService)
