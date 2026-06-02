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

// ── Post client DI ─────────────────────────────────────────────────────────

// PostClientFactory is the exported alias of the unexported postClientFactory
// type, allowing tests to inject gomock clients.
type PostClientFactory = postClientFactory

// NewRootCommandWithPost creates a root command whose "post" subcommands
// use the provided factory. Mirrors NewRootCommandWithHealth.
func NewRootCommandWithPost(factory PostClientFactory) *cobra.Command {
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

	cmd.AddCommand(newPostCommand(cmd, &g, factory))
	return cmd
}

// FactoryFromPost wraps any PostServiceClient as a factory that returns it
// for every endpoint (no real connection opened).
func FactoryFromPost(client feediumapi.PostServiceClient) PostClientFactory {
	return func(string) (feediumapi.PostServiceClient, func() error, error) {
		return client, func() error { return nil }, nil
	}
}

// ── Summary client DI ──────────────────────────────────────────────────────

// SummaryClientFactory is the exported alias of the unexported summaryClientFactory
// type, allowing tests to inject gomock clients.
type SummaryClientFactory = summaryClientFactory

// NewRootCommandWithSummary creates a root command whose "summary" subcommands
// use the provided factory. Mirrors NewRootCommandWithHealth.
func NewRootCommandWithSummary(factory SummaryClientFactory) *cobra.Command {
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

	cmd.AddCommand(newSummaryCommand(cmd, &g, factory))
	return cmd
}

// FactoryFromSummary wraps any SummaryServiceClient as a factory that returns it
// for every endpoint (no real connection opened).
func FactoryFromSummary(client feediumapi.SummaryServiceClient) SummaryClientFactory {
	return func(string) (feediumapi.SummaryServiceClient, func() error, error) {
		return client, func() error { return nil }, nil
	}
}

// ── Interest client DI ──────────────────────────────────────────────────────

// InterestClientFactory is the exported alias of the unexported
// interestClientFactory type, allowing tests to inject gomock clients.
type InterestClientFactory = interestClientFactory

// NewRootCommandWithInterest creates a root command whose "interest" subcommands
// use the provided factory.
func NewRootCommandWithInterest(factory InterestClientFactory) *cobra.Command {
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

	cmd.AddCommand(newInterestCommand(cmd, &g, factory))
	return cmd
}

// FactoryFromInterest wraps any InterestServiceClient as a factory that returns
// it for every endpoint (no real connection opened).
func FactoryFromInterest(client feediumapi.InterestServiceClient) InterestClientFactory {
	return func(string) (feediumapi.InterestServiceClient, func() error, error) {
		return client, func() error { return nil }, nil
	}
}

// ── Feed client DI ──────────────────────────────────────────────────────────

// FeedClientFactory is the exported alias of the unexported feedClientFactory
// type, allowing tests to inject gomock clients.
type FeedClientFactory = feedClientFactory

// NewRootCommandWithFeed creates a root command whose "feed" subcommands use the
// provided factory.
func NewRootCommandWithFeed(factory FeedClientFactory) *cobra.Command {
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

	cmd.AddCommand(newFeedCommand(cmd, &g, factory))
	return cmd
}

// FactoryFromFeed wraps any FeedServiceClient as a factory that returns it for
// every endpoint (no real connection opened).
func FactoryFromFeed(client feediumapi.FeedServiceClient) FeedClientFactory {
	return func(string) (feediumapi.FeedServiceClient, func() error, error) {
		return client, func() error { return nil }, nil
	}
}
