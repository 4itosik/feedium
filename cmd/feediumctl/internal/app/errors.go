package app

import (
	"errors"
	"fmt"
	"strings"

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

// FormatError classifies an error for stderr output per NFR-03:
//   - RPC errors already shaped by WrapRPCError ("code=… message=…") pass through;
//   - errors already prefixed with one of the allowed local prefixes pass through;
//   - anything else (cobra built-in errors, unknown sources) is remapped to
//     "flag: <reason>" so the closed list of prefixes is preserved.
//
// The error string is also trimmed of trailing whitespace/newlines to keep the
// single-line invariant.
func FormatError(err error) string {
	if err == nil {
		return ""
	}
	// allowedLocalPrefixes enumerates the closed list permitted by NFR-03.
	allowedLocalPrefixes := [...]string{"config: ", "flag: ", "output: ", "endpoint: "}
	s := strings.TrimRight(err.Error(), "\r\n\t ")
	if strings.HasPrefix(s, "code=") {
		return s
	}
	for _, p := range allowedLocalPrefixes {
		if strings.HasPrefix(s, p) {
			return s
		}
	}
	return "flag: " + s
}
