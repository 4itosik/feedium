package server

import "github.com/google/wire"

//nolint:gochecknoglobals // Wire provider set requires global variable
var ProviderSet = wire.NewSet(
	NewHTTPServer,
	NewGRPCServer,
)
