package prompt

import (
	"github.com/google/wire"

	"github.com/4itosik/feedium/internal/biz"
)

// ProviderSet wires the embedded SummaryPromptResolver into the application
// dependency graph. Registered in cmd/feedium/wire.go alongside the other
// ProviderSets so internal/biz never imports this package.
//
//nolint:gochecknoglobals // wire dependency injection
var ProviderSet = wire.NewSet(
	NewResolver,
	wire.Bind(new(biz.SummaryPromptResolver), new(*embeddedResolver)),
)
