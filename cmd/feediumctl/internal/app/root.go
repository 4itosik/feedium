package app

import (
	"github.com/spf13/cobra"
)

type globalFlags struct {
	endpoint string
	output   string
	timeout  string
	pageSize string
	config   string
}

func NewRootCommand() *cobra.Command {
	var g globalFlags

	cmd := &cobra.Command{
		Use:           "feediumctl",
		Short:         "CLI client for Feedium gRPC services",
		Long:          "feediumctl is a typed CLI client for the Feedium gRPC API.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.PersistentFlags().StringVar(&g.endpoint, "endpoint", "", "gRPC endpoint (host:port)")
	cmd.PersistentFlags().StringVarP(&g.output, "output", "o", "", "output format: table|json|yaml")
	cmd.PersistentFlags().StringVar(&g.timeout, "timeout", "", "per-RPC timeout (Go duration, e.g. 30s, 1m)")
	cmd.PersistentFlags().StringVar(&g.pageSize, "page-size", "", "page size for list requests")
	cmd.PersistentFlags().StringVar(&g.config, "config", "", "path to YAML config file")

	cmd.AddCommand(newHealthCommand(cmd, &g))
	cmd.AddCommand(newSourceCommand(cmd, &g, defaultSourceClientFactory))

	return cmd
}
