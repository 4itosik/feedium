package app_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/4itosik/feedium/cmd/feediumctl/internal/app"
)

func TestFormatRPCError_Codes(t *testing.T) {
	cases := []struct {
		code codes.Code
		msg  string
		want string
	}{
		{codes.Unavailable, "connection refused", "code=Unavailable message=connection refused"},
		{codes.DeadlineExceeded, "ctx timeout", "code=DeadlineExceeded message=ctx timeout"},
		{codes.InvalidArgument, "bad", "code=InvalidArgument message=bad"},
		{codes.Internal, "boom", "code=Internal message=boom"},
	}
	for _, tc := range cases {
		t.Run(tc.code.String(), func(t *testing.T) {
			err := status.Error(tc.code, tc.msg)
			assert.Equal(t, tc.want, app.FormatRPCError(err))
		})
	}
}

func TestFormatRPCError_NonStatus(t *testing.T) {
	err := errors.New("raw text")
	// status.FromError treats non-status errors as codes.Unknown.
	assert.Equal(t, "code=Unknown message=raw text", app.FormatRPCError(err))
}

func TestWrapRPCError_Error(t *testing.T) {
	err := app.WrapRPCError(status.Error(codes.Unavailable, "x"))
	assert.Equal(t, "code=Unavailable message=x", err.Error())
}

func TestWrapRPCError_Nil(t *testing.T) {
	assert.NoError(t, app.WrapRPCError(nil))
}
