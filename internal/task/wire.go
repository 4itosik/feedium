package task

import (
	"github.com/google/wire"
)

var ProviderSet = wire.NewSet(NewSummaryWorker, NewCronWorker) //nolint:gochecknoglobals // wire dependency injection
