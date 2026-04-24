package app

import (
	"errors"
	"fmt"

	"google.golang.org/grpc/status"
)

// FormatRPCError renders a gRPC error per NFR-03:
//
//	"code=<CodeName> message=<msg>"
//
// If err is not a gRPC status error, the code defaults to Unknown.
func FormatRPCError(err error) string {
	if err == nil {
		return ""
	}
	st, _ := status.FromError(err)
	return fmt.Sprintf("code=%s message=%s", st.Code().String(), st.Message())
}

// rpcError wraps a gRPC error so its Error() already matches NFR-03.
type rpcError struct{ wrapped error }

func (e *rpcError) Error() string { return FormatRPCError(e.wrapped) }
func (e *rpcError) Unwrap() error { return e.wrapped }

// WrapRPCError tags a gRPC error so the main loop prints it in the
// canonical "code=… message=…" form.
func WrapRPCError(err error) error {
	if err == nil {
		return nil
	}
	var already *rpcError
	if errors.As(err, &already) {
		return err
	}
	return &rpcError{wrapped: err}
}
