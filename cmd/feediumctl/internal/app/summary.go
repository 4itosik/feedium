package app

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	feediumapi "github.com/4itosik/feedium/api/feedium"
	"github.com/4itosik/feedium/cmd/feediumctl/internal/render"
	"github.com/4itosik/feedium/cmd/feediumctl/internal/transport"
)

// summaryClientFactory produces a SummaryServiceClient for the given endpoint.
// Swappable in tests without opening a real socket.
type summaryClientFactory func(endpoint string) (feediumapi.SummaryServiceClient, func() error, error)

func defaultSummaryClientFactory(endpoint string) (feediumapi.SummaryServiceClient, func() error, error) {
	conn, err := transport.Dial(endpoint)
	if err != nil {
		return nil, nil, err
	}
	return feediumapi.NewSummaryServiceClient(conn), conn.Close, nil
}

func newSummaryCommand(root *cobra.Command, g *globalFlags, factory summaryClientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "summary",
		Short:         "Read summaries",
		Long:          "List and get post/source summaries from Feedium.",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("flag: unknown subcommand %q for %q", args[0], cmd.CommandPath())
			}
			return cmd.Help()
		},
	}
	cmd.AddCommand(newSummaryListCommand(root, g, factory))
	cmd.AddCommand(newSummaryGetCommand(root, g, factory))
	return cmd
}

// ── summary list ───────────────────────────────────────────────────────────

func newSummaryListCommand(root *cobra.Command, g *globalFlags, factory summaryClientFactory) *cobra.Command {
	var postID string
	var sourceID string

	cmd := &cobra.Command{
		Use:           "list",
		Short:         "List summaries by parent",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			postIDSet := cmd.Flags().Changed("post-id")
			sourceIDSet := cmd.Flags().Changed("source-id")
			return runSummaryList(
				cmd.Context(), cmd.OutOrStdout(), root, g, factory,
				postID, postIDSet, sourceID, sourceIDSet,
			)
		},
	}
	cmd.Flags().StringVar(&postID, "post-id", "", "list summaries for a post")
	cmd.Flags().StringVar(&sourceID, "source-id", "", "list summaries for a source")
	return cmd
}

func runSummaryList(
	ctx context.Context,
	stdout io.Writer,
	root *cobra.Command,
	g *globalFlags,
	factory summaryClientFactory,
	postID string,
	postIDSet bool,
	sourceID string,
	sourceIDSet bool,
) error {
	settings, err := resolveAndValidate(root, g)
	if err != nil {
		return err
	}

	// exactly one of --post-id or --source-id must be set
	setCount := 0
	if postIDSet {
		setCount++
	}
	if sourceIDSet {
		setCount++
	}
	if setCount != 1 {
		return errors.New("flag: summary list requires exactly one of --post-id or --source-id")
	}

	client, closer, err := factory(settings.Endpoint)
	if err != nil {
		return err
	}
	defer func() { _ = closer() }()

	callCtx, cancel := context.WithTimeout(ctx, settings.Timeout)
	defer cancel()

	if postIDSet {
		resp, postErr := client.V1ListPostSummaries(callCtx, &feediumapi.V1ListPostSummariesRequest{
			PostId: postID,
		})
		if postErr != nil {
			return WrapRPCError(postErr)
		}
		return render.Write(stdout, settings.Output, resp)
	}

	resp, err := client.V1ListSourceSummaries(callCtx, &feediumapi.V1ListSourceSummariesRequest{
		SourceId: sourceID,
		PageSize: int32(settings.PageSize), //nolint:gosec // range-checked upstream
	})
	if err != nil {
		return WrapRPCError(err)
	}
	return render.Write(stdout, settings.Output, resp)
}

// ── summary get ────────────────────────────────────────────────────────────

func newSummaryGetCommand(root *cobra.Command, g *globalFlags, factory summaryClientFactory) *cobra.Command {
	return &cobra.Command{
		Use:           "get <id>",
		Short:         "Get a summary by ID",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSummaryGet(cmd.Context(), cmd.OutOrStdout(), root, g, factory, args[0])
		},
	}
}

func runSummaryGet(
	ctx context.Context,
	stdout io.Writer,
	root *cobra.Command,
	g *globalFlags,
	factory summaryClientFactory,
	id string,
) error {
	settings, err := resolveAndValidate(root, g)
	if err != nil {
		return err
	}

	client, closer, err := factory(settings.Endpoint)
	if err != nil {
		return err
	}
	defer func() { _ = closer() }()

	callCtx, cancel := context.WithTimeout(ctx, settings.Timeout)
	defer cancel()

	resp, err := client.V1GetSummary(callCtx, &feediumapi.V1GetSummaryRequest{Id: id})
	if err != nil {
		return WrapRPCError(err)
	}
	return render.Write(stdout, settings.Output, resp)
}
