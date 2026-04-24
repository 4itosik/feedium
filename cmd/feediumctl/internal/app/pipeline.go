package app

import (
	"github.com/spf13/cobra"

	"github.com/4itosik/feedium/cmd/feediumctl/internal/resolve"
)

// resolveSettings loads the YAML config and applies the flag > env > config >
// default priority for all four parameters (INV-05).
func resolveSettings(root *cobra.Command, g *globalFlags) (resolve.Settings, error) {
	cfg, err := loadConfig(g.config)
	if err != nil {
		return resolve.Settings{}, err
	}
	flags := flagSourceFrom(root, g)
	return resolve.Resolve(flags, cfg, osGetenv)
}

// resolveAndValidate is resolveSettings + output-format validation in one call.
// All source commands use this instead of duplicating the two-step pattern from
// runHealth (OI-2 from handoff).
func resolveAndValidate(root *cobra.Command, g *globalFlags) (resolve.Settings, error) {
	settings, err := resolveSettings(root, g)
	if err != nil {
		return resolve.Settings{}, err
	}
	if err := resolve.ValidateOutput(settings.Output); err != nil {
		return resolve.Settings{}, err
	}
	return settings, nil
}
