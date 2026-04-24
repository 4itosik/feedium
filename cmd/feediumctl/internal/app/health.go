package app

import (
	"context"
	"io"

	"github.com/spf13/cobra"

	feediumapi "github.com/4itosik/feedium/api/feedium"
	"github.com/4itosik/feedium/cmd/feediumctl/internal/render"
	"github.com/4itosik/feedium/cmd/feediumctl/internal/transport"
)

// healthClientFactory produces a HealthServiceClient given an endpoint and a
// "close" function. It is swappable so tests can inject a mock client without
// opening a socket.
type healthClientFactory func(endpoint string) (feediumapi.HealthServiceClient, func() error, error)

func defaultHealthClientFactory(endpoint string) (feediumapi.HealthServiceClient, func() error, error) {
	conn, err := transport.Dial(endpoint)
	if err != nil {
		return nil, nil, err
	}
	return feediumapi.NewHealthServiceClient(conn), conn.Close, nil
}

func newHealthCommand(root *cobra.Command, g *globalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "health",
		Short:         "Check the Feedium server health",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runHealth(cmd.Context(), cmd.OutOrStdout(), root, g, defaultHealthClientFactory)
		},
	}
	return cmd
}

func runHealth(
	ctx context.Context,
	stdout io.Writer,
	root *cobra.Command,
	g *globalFlags,
	factory healthClientFactory,
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

	resp, err := client.V1Check(callCtx, &feediumapi.V1CheckRequest{})
	if err != nil {
		return WrapRPCError(err)
	}
	return render.Write(stdout, settings.Output, resp)
}
