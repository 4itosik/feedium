package app

import (
	"github.com/spf13/cobra"

	feediumapi "github.com/4itosik/feedium/api/feedium"
)

// ── Source client DI (handoff §2.5, Step 6) ───────────────────────────────

// SourceClientFactory is the exported alias of the unexported sourceClientFactory
// type, allowing tests to inject gomock clients.
type SourceClientFactory = sourceClientFactory

// NewRootCommandWithSource creates a root command whose "source" subcommands
// use the provided factory. Mirrors NewRootCommandWithHealth.
func NewRootCommandWithSource(factory SourceClientFactory) *cobra.Command {
	var g globalFlags
	cmd := &cobra.Command{
		Use:           "feediumctl",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.PersistentFlags().StringVar(&g.endpoint, "endpoint", "", "")
	cmd.PersistentFlags().StringVarP(&g.output, "output", "o", "", "")
	cmd.PersistentFlags().StringVar(&g.timeout, "timeout", "", "")
	cmd.PersistentFlags().StringVar(&g.pageSize, "page-size", "", "")
	cmd.PersistentFlags().StringVar(&g.config, "config", "", "")

	cmd.AddCommand(newSourceCommand(cmd, &g, factory))
	return cmd
}

// FactoryFromSource wraps any SourceServiceClient as a factory that returns it
// for every endpoint (no real connection opened).
func FactoryFromSource(client feediumapi.SourceServiceClient) SourceClientFactory {
	return func(string) (feediumapi.SourceServiceClient, func() error, error) {
		return client, func() error { return nil }, nil
	}
}

// HealthClientFactory is an alias of the unexported healthClientFactory type,
// exported for tests to inject gomock clients.
type HealthClientFactory = healthClientFactory

// NewRootCommandWithHealth builds a root command whose "health" subcommand
// uses the provided factory, letting tests exercise the full resolve→RPC→
// render pipeline without opening a socket.
func NewRootCommandWithHealth(factory HealthClientFactory) *cobra.Command {
	var g globalFlags
	cmd := &cobra.Command{
		Use:           "feediumctl",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.PersistentFlags().StringVar(&g.endpoint, "endpoint", "", "")
	cmd.PersistentFlags().StringVarP(&g.output, "output", "o", "", "")
	cmd.PersistentFlags().StringVar(&g.timeout, "timeout", "", "")
	cmd.PersistentFlags().StringVar(&g.pageSize, "page-size", "", "")
	cmd.PersistentFlags().StringVar(&g.config, "config", "", "")

	h := &cobra.Command{
		Use:           "health",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return runHealth(c.Context(), c.OutOrStdout(), cmd, &g, factory)
		},
	}
	cmd.AddCommand(h)
	return cmd
}

// FactoryFromHealth wraps any HealthServiceClient as a factory that returns it
// for every endpoint (no real connection opened).
func FactoryFromHealth(client feediumapi.HealthServiceClient) HealthClientFactory {
	return func(string) (feediumapi.HealthServiceClient, func() error, error) {
		return client, func() error { return nil }, nil
	}
}
