package app

import (
	"context"
	"io"

	"github.com/spf13/cobra"

	feediumapi "github.com/4itosik/feedium/api/feedium"
	"github.com/4itosik/feedium/cmd/feediumctl/internal/render"
	"github.com/4itosik/feedium/cmd/feediumctl/internal/sourcetype"
	"github.com/4itosik/feedium/cmd/feediumctl/internal/transport"
)

// sourceClientFactory produces a SourceServiceClient for the given endpoint.
// Swappable in tests without opening a real socket (handoff §2.5).
type sourceClientFactory func(endpoint string) (feediumapi.SourceServiceClient, func() error, error)

func defaultSourceClientFactory(endpoint string) (feediumapi.SourceServiceClient, func() error, error) {
	conn, err := transport.Dial(endpoint)
	if err != nil {
		return nil, nil, err
	}
	return feediumapi.NewSourceServiceClient(conn), conn.Close, nil
}

// newSourceCommand registers the "source" group with its five leaf commands
// (AC-S6). The factory is injected so tests can substitute a stub.
func newSourceCommand(root *cobra.Command, g *globalFlags, factory sourceClientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "source",
		Short:         "Manage Feedium sources",
		Long:          "Create, read, update, and delete Feedium sources.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(newSourceListCommand(root, g, factory))
	cmd.AddCommand(newSourceGetCommand(root, g, factory))
	cmd.AddCommand(newSourceCreateCommand(root, g, factory))
	cmd.AddCommand(newSourceUpdateCommand(root, g, factory))
	cmd.AddCommand(newSourceDeleteCommand(root, g, factory))
	return cmd
}

// ── source list ────────────────────────────────────────────────────────────

func newSourceListCommand(root *cobra.Command, g *globalFlags, factory sourceClientFactory) *cobra.Command {
	var typeFlag string

	cmd := &cobra.Command{
		Use:           "list",
		Short:         "List sources",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSourceList(
				cmd.Context(), cmd.OutOrStdout(), root, g, factory,
				typeFlag, cmd.Flags().Changed("type"),
			)
		},
	}
	cmd.Flags().StringVar(&typeFlag, "type", "", "filter by SourceType short name (e.g. RSS, TELEGRAM_CHANNEL)")
	return cmd
}

func runSourceList(
	ctx context.Context,
	stdout io.Writer,
	root *cobra.Command,
	g *globalFlags,
	factory sourceClientFactory,
	typeFlag string,
	typeSet bool,
) error {
	settings, err := resolveAndValidate(root, g)
	if err != nil {
		return err
	}

	var typeFilter *feediumapi.SourceType
	if typeSet {
		t, err := sourcetype.LookupEnumFlag(typeFlag)
		if err != nil {
			return err
		}
		typeFilter = t.Enum()
	}

	client, closer, err := factory(settings.Endpoint)
	if err != nil {
		return err
	}
	defer func() { _ = closer() }()

	callCtx, cancel := context.WithTimeout(ctx, settings.Timeout)
	defer cancel()

	resp, err := client.V1ListSources(callCtx, &feediumapi.V1ListSourcesRequest{
		PageSize:  int32(settings.PageSize),
		PageToken: "",
		Type:      typeFilter,
	})
	if err != nil {
		return WrapRPCError(err)
	}
	return render.Write(stdout, settings.Output, resp)
}

// ── source get ─────────────────────────────────────────────────────────────

func newSourceGetCommand(root *cobra.Command, g *globalFlags, factory sourceClientFactory) *cobra.Command {
	return &cobra.Command{
		Use:           "get <id>",
		Short:         "Get a source by ID",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSourceGet(cmd.Context(), cmd.OutOrStdout(), root, g, factory, args[0])
		},
	}
}

func runSourceGet(
	ctx context.Context,
	stdout io.Writer,
	root *cobra.Command,
	g *globalFlags,
	factory sourceClientFactory,
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

	resp, err := client.V1GetSource(callCtx, &feediumapi.V1GetSourceRequest{Id: id})
	if err != nil {
		return WrapRPCError(err)
	}
	return render.Write(stdout, settings.Output, resp)
}

// ── source create ──────────────────────────────────────────────────────────

func newSourceCreateCommand(root *cobra.Command, g *globalFlags, factory sourceClientFactory) *cobra.Command {
	var (
		tgID     int64
		username string
		feedURL  string
		url      string
	)

	cmd := &cobra.Command{
		Use:           "create <type>",
		Short:         "Create a source (type: telegram-channel|telegram-group|rss|html)",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			setFlags := map[string]bool{
				"feed-url": cmd.Flags().Changed("feed-url"),
				"tg-id":    cmd.Flags().Changed("tg-id"),
				"url":      cmd.Flags().Changed("url"),
				"username": cmd.Flags().Changed("username"),
			}
			return runSourceCreate(
				cmd.Context(), cmd.OutOrStdout(), root, g, factory,
				args[0], setFlags,
				sourcetype.Flags{TgID: tgID, Username: username, FeedURL: feedURL, URL: url},
			)
		},
	}
	cmd.Flags().Int64Var(&tgID, "tg-id", 0, "Telegram channel/group numeric ID")
	cmd.Flags().StringVar(&username, "username", "", "Telegram username")
	cmd.Flags().StringVar(&feedURL, "feed-url", "", "RSS feed URL")
	cmd.Flags().StringVar(&url, "url", "", "HTML page URL")
	return cmd
}

func runSourceCreate(
	ctx context.Context,
	stdout io.Writer,
	root *cobra.Command,
	g *globalFlags,
	factory sourceClientFactory,
	typeName string,
	setFlags map[string]bool,
	f sourcetype.Flags,
) error {
	// Validate <type> (EC-E) and flag constraints (EC-I, EC-D) before any RPC.
	protoType, err := sourcetype.Lookup(typeName)
	if err != nil {
		return err
	}
	if err := sourcetype.CheckFlags(typeName, setFlags); err != nil {
		return err
	}

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

	resp, err := client.V1CreateSource(callCtx, &feediumapi.V1CreateSourceRequest{
		Type:   protoType,
		Config: sourcetype.BuildConfig(typeName, f),
	})
	if err != nil {
		return WrapRPCError(err)
	}
	return render.Write(stdout, settings.Output, resp)
}

// ── source update ──────────────────────────────────────────────────────────

func newSourceUpdateCommand(root *cobra.Command, g *globalFlags, factory sourceClientFactory) *cobra.Command {
	var (
		typeFlag string
		tgID     int64
		username string
		feedURL  string
		url      string
	)

	cmd := &cobra.Command{
		Use:           "update <id>",
		Short:         "Update a source",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			setFlags := map[string]bool{
				"feed-url": cmd.Flags().Changed("feed-url"),
				"tg-id":    cmd.Flags().Changed("tg-id"),
				"url":      cmd.Flags().Changed("url"),
				"username": cmd.Flags().Changed("username"),
			}
			return runSourceUpdate(
				cmd.Context(), cmd.OutOrStdout(), root, g, factory,
				args[0], typeFlag, setFlags,
				sourcetype.Flags{TgID: tgID, Username: username, FeedURL: feedURL, URL: url},
			)
		},
	}
	cmd.Flags().StringVar(&typeFlag, "type", "", "source type (telegram-channel|telegram-group|rss|html)")
	if err := cmd.MarkFlagRequired("type"); err != nil {
		panic(err)
	}
	cmd.Flags().Int64Var(&tgID, "tg-id", 0, "Telegram channel/group numeric ID")
	cmd.Flags().StringVar(&username, "username", "", "Telegram username")
	cmd.Flags().StringVar(&feedURL, "feed-url", "", "RSS feed URL")
	cmd.Flags().StringVar(&url, "url", "", "HTML page URL")
	return cmd
}

func runSourceUpdate(
	ctx context.Context,
	stdout io.Writer,
	root *cobra.Command,
	g *globalFlags,
	factory sourceClientFactory,
	id string,
	typeFlag string,
	setFlags map[string]bool,
	f sourcetype.Flags,
) error {
	// Validate --type (EC-G format) and flag constraints (EC-I, EC-D).
	protoType, err := sourcetype.LookupFlag(typeFlag)
	if err != nil {
		return err
	}
	if err := sourcetype.CheckFlags(typeFlag, setFlags); err != nil {
		return err
	}

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

	resp, err := client.V1UpdateSource(callCtx, &feediumapi.V1UpdateSourceRequest{
		Id:     id,
		Type:   protoType,
		Config: sourcetype.BuildConfig(typeFlag, f),
	})
	if err != nil {
		return WrapRPCError(err)
	}
	return render.Write(stdout, settings.Output, resp)
}

// ── source delete ──────────────────────────────────────────────────────────

func newSourceDeleteCommand(root *cobra.Command, g *globalFlags, factory sourceClientFactory) *cobra.Command {
	return &cobra.Command{
		Use:           "delete <id>",
		Short:         "Delete a source",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSourceDelete(cmd.Context(), cmd.OutOrStdout(), root, g, factory, args[0])
		},
	}
}

func runSourceDelete(
	ctx context.Context,
	stdout io.Writer,
	root *cobra.Command,
	g *globalFlags,
	factory sourceClientFactory,
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

	_, err = client.V1DeleteSource(callCtx, &feediumapi.V1DeleteSourceRequest{Id: id})
	if err != nil {
		return WrapRPCError(err)
	}
	return render.WriteDelete(stdout, settings.Output, id)
}
