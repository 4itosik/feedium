package resolve_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/4itosik/feedium/cmd/feediumctl/internal/config"
	"github.com/4itosik/feedium/cmd/feediumctl/internal/resolve"
)

func ptr[T any](v T) *T {
	p := new(T)
	*p = v
	return p
}

func emptyGetenv(string) string { return "" }

func envFromMap(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestResolve_Priority(t *testing.T) {
	type tc struct {
		name       string
		flags      resolve.FlagSource
		env        map[string]string
		cfg        config.File
		wantEP     string
		wantOutput string
		wantTO     time.Duration
		wantPS     int
	}

	tests := []tc{
		{
			name:       "defaults",
			wantEP:     resolve.DefaultEndpoint,
			wantOutput: resolve.DefaultOutput,
			wantTO:     resolve.DefaultTimeout,
			wantPS:     resolve.DefaultPageSize,
		},
		{
			name: "config layer",
			cfg: config.File{
				Endpoint: ptr("cfg:9000"),
				Output:   ptr("json"),
				Timeout:  ptr(2 * time.Second),
				PageSize: ptr(33),
			},
			wantEP:     "cfg:9000",
			wantOutput: "json",
			wantTO:     2 * time.Second,
			wantPS:     33,
		},
		{
			name: "env overrides config",
			env: map[string]string{
				resolve.EnvEndpoint: "env:1000",
				resolve.EnvOutput:   "yaml",
				resolve.EnvTimeout:  "3s",
				resolve.EnvPageSize: "7",
			},
			cfg: config.File{
				Endpoint: ptr("cfg:9000"),
				Output:   ptr("json"),
				Timeout:  ptr(2 * time.Second),
				PageSize: ptr(33),
			},
			wantEP:     "env:1000",
			wantOutput: "yaml",
			wantTO:     3 * time.Second,
			wantPS:     7,
		},
		{
			name: "flag overrides env and config",
			flags: resolve.FlagSource{
				Endpoint:    "flag:2222",
				EndpointSet: true,
				Output:      "table",
				OutputSet:   true,
				Timeout:     "4s",
				TimeoutSet:  true,
				PageSize:    "9",
				PageSizeSet: true,
			},
			env: map[string]string{
				resolve.EnvEndpoint: "env:1000",
				resolve.EnvOutput:   "yaml",
				resolve.EnvTimeout:  "3s",
				resolve.EnvPageSize: "7",
			},
			cfg: config.File{
				Endpoint: ptr("cfg:9000"),
				Output:   ptr("json"),
				Timeout:  ptr(2 * time.Second),
				PageSize: ptr(33),
			},
			wantEP:     "flag:2222",
			wantOutput: "table",
			wantTO:     4 * time.Second,
			wantPS:     9,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := resolve.Resolve(tt.flags, tt.cfg, envFromMap(tt.env))
			require.NoError(t, err)
			assert.Equal(t, tt.wantEP, s.Endpoint)
			assert.Equal(t, tt.wantOutput, s.Output)
			assert.Equal(t, tt.wantTO, s.Timeout)
			assert.Equal(t, tt.wantPS, s.PageSize)
		})
	}
}

func TestResolve_InvalidValues(t *testing.T) {
	t.Run("invalid flag timeout", func(t *testing.T) {
		_, err := resolve.Resolve(
			resolve.FlagSource{Timeout: "abc", TimeoutSet: true},
			config.File{},
			emptyGetenv,
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `flag: invalid timeout "abc":`)
	})

	t.Run("invalid env timeout", func(t *testing.T) {
		_, err := resolve.Resolve(
			resolve.FlagSource{},
			config.File{},
			envFromMap(map[string]string{resolve.EnvTimeout: "zzz"}),
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `flag: invalid timeout "zzz":`)
	})

	t.Run("invalid flag page-size", func(t *testing.T) {
		_, err := resolve.Resolve(
			resolve.FlagSource{PageSize: "negative", PageSizeSet: true},
			config.File{},
			emptyGetenv,
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `flag: invalid page size "negative":`)
	})

	t.Run("invalid env page-size", func(t *testing.T) {
		_, err := resolve.Resolve(
			resolve.FlagSource{},
			config.File{},
			envFromMap(map[string]string{resolve.EnvPageSize: "NaN"}),
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `flag: invalid page size "NaN":`)
	})
}

func TestValidateOutput(t *testing.T) {
	for _, v := range []string{"table", "json", "yaml"} {
		require.NoError(t, resolve.ValidateOutput(v), v)
	}
	err := resolve.ValidateOutput("xml")
	require.Error(t, err)
	assert.Equal(t, `output: invalid value "xml" (allowed: table,json,yaml)`, err.Error())
}
