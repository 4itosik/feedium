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

// interestClientFactory produces an InterestServiceClient for the given endpoint.
// Swappable in tests without opening a real socket.
type interestClientFactory func(endpoint string) (feediumapi.InterestServiceClient, func() error, error)

func defaultInterestClientFactory(endpoint string) (feediumapi.InterestServiceClient, func() error, error) {
	conn, err := transport.Dial(endpoint)
	if err != nil {
		return nil, nil, err
	}
	return feediumapi.NewInterestServiceClient(conn), conn.Close, nil
}

func newInterestCommand(root *cobra.Command, g *globalFlags, factory interestClientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "interest",
		Short:         "Manage Feedium interests",
		Long:          "Create, read, update, and delete Feedium interests.",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("flag: unknown subcommand %q for %q", args[0], cmd.CommandPath())
			}
			return cmd.Help()
		},
	}
	cmd.AddCommand(newInterestListCommand(root, g, factory))
	cmd.AddCommand(newInterestGetCommand(root, g, factory))
	cmd.AddCommand(newInterestCreateCommand(root, g, factory))
	cmd.AddCommand(newInterestUpdateCommand(root, g, factory))
	cmd.AddCommand(newInterestDeleteCommand(root, g, factory))
	return cmd
}

// ── interest list ────────────────────────────────────────────────────────────

func newInterestListCommand(root *cobra.Command, g *globalFlags, factory interestClientFactory) *cobra.Command {
	return &cobra.Command{
		Use:           "list",
		Short:         "List interests",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runInterestList(cmd.Context(), cmd.OutOrStdout(), root, g, factory)
		},
	}
}

func runInterestList(
	ctx context.Context,
	stdout io.Writer,
	root *cobra.Command,
	g *globalFlags,
	factory interestClientFactory,
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

	resp, err := client.V1ListInterests(callCtx, &feediumapi.V1ListInterestsRequest{
		PageSize: int32(settings.PageSize), //nolint:gosec // range-checked upstream
	})
	if err != nil {
		return WrapRPCError(err)
	}
	return render.Write(stdout, settings.Output, resp)
}

// ── interest get ─────────────────────────────────────────────────────────────

func newInterestGetCommand(root *cobra.Command, g *globalFlags, factory interestClientFactory) *cobra.Command {
	return &cobra.Command{
		Use:           "get <id>",
		Short:         "Get an interest by ID",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInterestGet(cmd.Context(), cmd.OutOrStdout(), root, g, factory, args[0])
		},
	}
}

func runInterestGet(
	ctx context.Context,
	stdout io.Writer,
	root *cobra.Command,
	g *globalFlags,
	factory interestClientFactory,
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

	resp, err := client.V1GetInterest(callCtx, &feediumapi.V1GetInterestRequest{Id: id})
	if err != nil {
		return WrapRPCError(err)
	}
	return render.Write(stdout, settings.Output, resp)
}

// ── interest create ──────────────────────────────────────────────────────────

func newInterestCreateCommand(root *cobra.Command, g *globalFlags, factory interestClientFactory) *cobra.Command {
	return &cobra.Command{
		Use:           "create <topic>",
		Short:         "Create an interest with the given topic",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInterestCreate(cmd.Context(), cmd.OutOrStdout(), root, g, factory, args[0])
		},
	}
}

func runInterestCreate(
	ctx context.Context,
	stdout io.Writer,
	root *cobra.Command,
	g *globalFlags,
	factory interestClientFactory,
	topic string,
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

	resp, err := client.V1CreateInterest(callCtx, &feediumapi.V1CreateInterestRequest{Topic: topic})
	if err != nil {
		return WrapRPCError(err)
	}
	return render.Write(stdout, settings.Output, resp)
}

// ── interest update ──────────────────────────────────────────────────────────

func newInterestUpdateCommand(root *cobra.Command, g *globalFlags, factory interestClientFactory) *cobra.Command {
	var topic string

	cmd := &cobra.Command{
		Use:           "update <id>",
		Short:         "Update an interest topic",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInterestUpdate(cmd.Context(), cmd.OutOrStdout(), root, g, factory, args[0], topic)
		},
	}
	cmd.Flags().StringVar(&topic, "topic", "", "new interest topic")
	if err := cmd.MarkFlagRequired("topic"); err != nil {
		panic(err)
	}
	return cmd
}

func runInterestUpdate(
	ctx context.Context,
	stdout io.Writer,
	root *cobra.Command,
	g *globalFlags,
	factory interestClientFactory,
	id, topic string,
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

	resp, err := client.V1UpdateInterest(callCtx, &feediumapi.V1UpdateInterestRequest{Id: id, Topic: topic})
	if err != nil {
		return WrapRPCError(err)
	}
	return render.Write(stdout, settings.Output, resp)
}

// ── interest delete ──────────────────────────────────────────────────────────

func newInterestDeleteCommand(root *cobra.Command, g *globalFlags, factory interestClientFactory) *cobra.Command {
	return &cobra.Command{
		Use:           "delete <id>",
		Short:         "Delete an interest",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInterestDelete(cmd.Context(), cmd.OutOrStdout(), root, g, factory, args[0])
		},
	}
}

func runInterestDelete(
	ctx context.Context,
	stdout io.Writer,
	root *cobra.Command,
	g *globalFlags,
	factory interestClientFactory,
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

	_, err = client.V1DeleteInterest(callCtx, &feediumapi.V1DeleteInterestRequest{Id: id})
	if err != nil {
		return WrapRPCError(err)
	}
	return render.WriteDelete(stdout, settings.Output, id)
}
