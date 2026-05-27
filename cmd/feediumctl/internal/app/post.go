package app

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	feediumapi "github.com/4itosik/feedium/api/feedium"
	"github.com/4itosik/feedium/cmd/feediumctl/internal/render"
	"github.com/4itosik/feedium/cmd/feediumctl/internal/transport"
)

// postClientFactory produces a PostServiceClient for the given endpoint.
// Swappable in tests without opening a real socket.
type postClientFactory func(endpoint string) (feediumapi.PostServiceClient, func() error, error)

func defaultPostClientFactory(endpoint string) (feediumapi.PostServiceClient, func() error, error) {
	conn, err := transport.Dial(endpoint)
	if err != nil {
		return nil, nil, err
	}
	return feediumapi.NewPostServiceClient(conn), conn.Close, nil
}

func newPostCommand(root *cobra.Command, g *globalFlags, factory postClientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "post",
		Short:         "Read posts from sources",
		Long:          "List and get posts from Feedium sources.",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("flag: unknown subcommand %q for %q", args[0], cmd.CommandPath())
			}
			return cmd.Help()
		},
	}
	cmd.AddCommand(newPostListCommand(root, g, factory))
	cmd.AddCommand(newPostGetCommand(root, g, factory))
	return cmd
}

// ── post list ──────────────────────────────────────────────────────────────

func newPostListCommand(root *cobra.Command, g *globalFlags, factory postClientFactory) *cobra.Command {
	var sourceID string

	cmd := &cobra.Command{
		Use:           "list",
		Short:         "List posts",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runPostList(
				cmd.Context(), cmd.OutOrStdout(), root, g, factory,
				sourceID, cmd.Flags().Changed("source-id"),
			)
		},
	}
	cmd.Flags().StringVar(&sourceID, "source-id", "", "filter by source ID")
	return cmd
}

func runPostList(
	ctx context.Context,
	stdout io.Writer,
	root *cobra.Command,
	g *globalFlags,
	factory postClientFactory,
	sourceID string,
	sourceIDSet bool,
) error {
	settings, err := resolveAndValidate(root, g)
	if err != nil {
		return err
	}

	var sourceFilter *string
	if sourceIDSet {
		sourceFilter = &sourceID
	}

	client, closer, err := factory(settings.Endpoint)
	if err != nil {
		return err
	}
	defer func() { _ = closer() }()

	callCtx, cancel := context.WithTimeout(ctx, settings.Timeout)
	defer cancel()

	resp, err := client.V1ListPosts(callCtx, &feediumapi.V1ListPostsRequest{
		PageSize:  int32(settings.PageSize), //nolint:gosec // range-checked upstream
		PageToken: "",
		SourceId:  sourceFilter,
	})
	if err != nil {
		return WrapRPCError(err)
	}
	return render.Write(stdout, settings.Output, resp)
}

// ── post get ───────────────────────────────────────────────────────────────

func newPostGetCommand(root *cobra.Command, g *globalFlags, factory postClientFactory) *cobra.Command {
	return &cobra.Command{
		Use:           "get <id>",
		Short:         "Get a post by ID",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPostGet(cmd.Context(), cmd.OutOrStdout(), root, g, factory, args[0])
		},
	}
}

func runPostGet(
	ctx context.Context,
	stdout io.Writer,
	root *cobra.Command,
	g *globalFlags,
	factory postClientFactory,
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

	resp, err := client.V1GetPost(callCtx, &feediumapi.V1GetPostRequest{Id: id})
	if err != nil {
		return WrapRPCError(err)
	}
	return render.Write(stdout, settings.Output, resp)
}
