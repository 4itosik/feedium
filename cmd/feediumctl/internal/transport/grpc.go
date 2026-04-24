// Package transport contains the gRPC dial factory used by feediumctl
// commands (FR-07).
package transport

import (
	"fmt"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Dial opens a plaintext gRPC connection to endpoint. No metadata-based
// authentication is attached (MVP, CON-03).
//
// The function performs a tiny syntactic sanity check on the endpoint before
// handing it to grpc.NewClient so that obviously-malformed strings are
// reported with the "endpoint:" prefix (NFR-03, EC-C). Full validation is
// delegated to gRPC itself, where failures surface as status errors through
// the standard RPC formatter (Step 7).
func Dial(endpoint string) (*grpc.ClientConn, error) {
	if err := validateEndpoint(endpoint); err != nil {
		return nil, err
	}
	conn, err := grpc.NewClient(endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("endpoint: %s", err.Error())
	}
	return conn, nil
}

func validateEndpoint(endpoint string) error {
	trimmed := strings.TrimSpace(endpoint)
	if trimmed == "" {
		return fmt.Errorf("endpoint: empty")
	}
	if strings.ContainsAny(trimmed, " \t\r\n") {
		return fmt.Errorf("endpoint: whitespace not allowed in %q", endpoint)
	}
	return nil
}
