package summary

import (
	"github.com/google/wire"
)

var ProviderSet = wire.NewSet(NewService) //nolint:gochecknoglobals // wire dependency injection
