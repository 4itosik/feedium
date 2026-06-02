package app

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/timestamppb"

	feediumapi "github.com/4itosik/feedium/api/feedium"
	"github.com/4itosik/feedium/cmd/feediumctl/internal/render"
	"github.com/4itosik/feedium/cmd/feediumctl/internal/transport"
)

// feedClientFactory produces a FeedServiceClient for the given endpoint.
// Swappable in tests without opening a real socket.
type feedClientFactory func(endpoint string) (feediumapi.FeedServiceClient, func() error, error)

func defaultFeedClientFactory(endpoint string) (feediumapi.FeedServiceClient, func() error, error) {
	conn, err := transport.Dial(endpoint)
	if err != nil {
		return nil, nil, err
	}
	return feediumapi.NewFeedServiceClient(conn), conn.Close, nil
}

func newFeedCommand(root *cobra.Command, g *globalFlags, factory feedClientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "feed",
		Short:         "Read the ranked feed",
		Long:          "List posts ordered by relevance score with time and threshold filters.",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("flag: unknown subcommand %q for %q", args[0], cmd.CommandPath())
			}
			return cmd.Help()
		},
	}
	cmd.AddCommand(newFeedListCommand(root, g, factory))
	return cmd
}

// feedListFlags collects the ranked-feed filters parsed before the RPC.
type feedListFlags struct {
	from     string
	to       string
	minScore float64
}

func newFeedListCommand(root *cobra.Command, g *globalFlags, factory feedClientFactory) *cobra.Command {
	var f feedListFlags

	cmd := &cobra.Command{
		Use:           "list",
		Short:         "List the ranked feed",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runFeedList(cmd.Context(), cmd.OutOrStdout(), root, g, factory, f)
		},
	}
	cmd.Flags().StringVar(&f.from, "from", "", "lower bound on published_at (RFC3339)")
	cmd.Flags().StringVar(&f.to, "to", "", "upper bound on published_at (RFC3339)")
	cmd.Flags().Float64Var(&f.minScore, "min-score", 0, "minimum relevance score in [0,1]")
	return cmd
}

func runFeedList(
	ctx context.Context,
	stdout io.Writer,
	root *cobra.Command,
	g *globalFlags,
	factory feedClientFactory,
	f feedListFlags,
) error {
	settings, err := resolveAndValidate(root, g)
	if err != nil {
		return err
	}

	req := &feediumapi.V1ListFeedRequest{
		MinScore: f.minScore,
		PageSize: int32(settings.PageSize), //nolint:gosec // range-checked upstream
	}
	if f.from != "" {
		from, parseErr := time.Parse(time.RFC3339, f.from)
		if parseErr != nil {
			return fmt.Errorf("flag: invalid --from timestamp: %w", parseErr)
		}
		req.From = timestamppb.New(from)
	}
	if f.to != "" {
		to, parseErr := time.Parse(time.RFC3339, f.to)
		if parseErr != nil {
			return fmt.Errorf("flag: invalid --to timestamp: %w", parseErr)
		}
		req.To = timestamppb.New(to)
	}

	client, closer, err := factory(settings.Endpoint)
	if err != nil {
		return err
	}
	defer func() { _ = closer() }()

	callCtx, cancel := context.WithTimeout(ctx, settings.Timeout)
	defer cancel()

	resp, err := client.V1ListFeed(callCtx, req)
	if err != nil {
		return WrapRPCError(err)
	}
	return render.Write(stdout, settings.Output, resp)
}
