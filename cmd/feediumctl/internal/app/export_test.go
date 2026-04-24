package app

import (
	"context"

	"github.com/spf13/cobra"
	"google.golang.org/grpc"

	feediumapi "github.com/4itosik/feedium/api/feedium"
)

// ── Source client DI (handoff §2.5, Step 6) ───────────────────────────────

// SourceClientFactory is the exported alias of the unexported sourceClientFactory
// type, allowing tests to inject stub clients.
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

// StubSourceFactory wraps any SourceServiceClient as a factory that returns it
// for every endpoint (no real connection opened).
func StubSourceFactory(stub feediumapi.SourceServiceClient) SourceClientFactory {
	return func(string) (feediumapi.SourceServiceClient, func() error, error) {
		return stub, func() error { return nil }, nil
	}
}

// HealthClientFactory is an alias of the unexported healthClientFactory
// type, exported for tests to inject mock clients.
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

// StubHealthFactory builds a factory that returns a synchronous stub client.
func StubHealthFactory(fn func(context.Context, *feediumapi.V1CheckRequest) (*feediumapi.V1CheckResponse, error)) HealthClientFactory {
	return func(string) (feediumapi.HealthServiceClient, func() error, error) {
		return stubHealthClient{fn: fn}, func() error { return nil }, nil
	}
}

type stubHealthClient struct {
	fn func(context.Context, *feediumapi.V1CheckRequest) (*feediumapi.V1CheckResponse, error)
}

func (s stubHealthClient) V1Check(ctx context.Context, in *feediumapi.V1CheckRequest, _ ...grpc.CallOption) (*feediumapi.V1CheckResponse, error) {
	return s.fn(ctx, in)
}
