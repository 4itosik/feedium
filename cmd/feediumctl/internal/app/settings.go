package app

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/4itosik/feedium/cmd/feediumctl/internal/config"
	"github.com/4itosik/feedium/cmd/feediumctl/internal/resolve"
)

// osGetenv is a package-level indirection to keep tests side-effect-free.
var osGetenv = os.Getenv

func loadConfig(flagValue string) (config.File, error) {
	p, ok := config.ResolvePath(flagValue, osGetenv, os.UserHomeDir)
	if !ok {
		return config.File{}, nil
	}
	return config.Load(p)
}

func flagSourceFrom(root *cobra.Command, g *globalFlags) resolve.FlagSource {
	flag := func(name string) bool {
		f := root.PersistentFlags().Lookup(name)
		return f != nil && f.Changed
	}
	return resolve.FlagSource{
		Endpoint:    g.endpoint,
		EndpointSet: flag("endpoint"),
		Output:      g.output,
		OutputSet:   flag("output"),
		Timeout:     g.timeout,
		TimeoutSet:  flag("timeout"),
		PageSize:    g.pageSize,
		PageSizeSet: flag("page-size"),
	}
}
