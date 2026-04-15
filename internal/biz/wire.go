package biz

import "github.com/google/wire"

// ProviderSet Wire provider set for biz package
//
//nolint:gochecknoglobals // Wire provider sets are intentionally global
var ProviderSet = wire.NewSet(NewSourceUsecase, NewPostUsecase)
