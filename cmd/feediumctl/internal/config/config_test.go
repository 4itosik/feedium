package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/4itosik/feedium/cmd/feediumctl/internal/config"
)

func TestResolvePath(t *testing.T) {
	t.Run("flag wins", func(t *testing.T) {
		p, ok := config.ResolvePath("/etc/feedium.yaml",
			func(string) string { return "/env/path" },
			func() (string, error) { return "/home/u", nil })
		require.True(t, ok)
		assert.Equal(t, "/etc/feedium.yaml", p.Value)
		assert.True(t, p.Explicit)
	})

	t.Run("env used when flag empty", func(t *testing.T) {
		p, ok := config.ResolvePath("",
			func(k string) string {
				if k == config.EnvConfigPath {
					return "/env/path.yaml"
				}
				return ""
			},
			func() (string, error) { return "/home/u", nil })
		require.True(t, ok)
		assert.Equal(t, "/env/path.yaml", p.Value)
		assert.True(t, p.Explicit)
	})

	t.Run("default uses $HOME", func(t *testing.T) {
		p, ok := config.ResolvePath("",
			func(string) string { return "" },
			func() (string, error) { return "/home/u", nil })
		require.True(t, ok)
		assert.Equal(t, filepath.Join("/home/u", ".feediumctl.yaml"), p.Value)
		assert.False(t, p.Explicit)
	})

	t.Run("HOME unavailable => no path", func(t *testing.T) {
		_, ok := config.ResolvePath("",
			func(string) string { return "" },
			func() (string, error) { return "", errors.New("no home") })
		assert.False(t, ok)
	})
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()

	t.Run("default path missing is silent", func(t *testing.T) {
		missing := filepath.Join(dir, ".feediumctl.yaml")
		f, err := config.Load(config.Path{Value: missing, Explicit: false})
		require.NoError(t, err)
		assert.Equal(t, config.File{}, f)
	})

	t.Run("explicit path missing is error", func(t *testing.T) {
		missing := filepath.Join(dir, "explicit.yaml")
		_, err := config.Load(config.Path{Value: missing, Explicit: true})
		require.Error(t, err)
		assert.Equal(t, "config: "+missing+": not found", err.Error())
	})

	t.Run("invalid YAML is error", func(t *testing.T) {
		path := filepath.Join(dir, "bad.yaml")
		require.NoError(t, os.WriteFile(path, []byte(":::not yaml:::"), 0o600))
		_, err := config.Load(config.Path{Value: path, Explicit: false})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "config: "+path+": ")
	})

	t.Run("unknown key is error", func(t *testing.T) {
		path := filepath.Join(dir, "unknown.yaml")
		require.NoError(t, os.WriteFile(path, []byte("weird_key: 1\n"), 0o600))
		_, err := config.Load(config.Path{Value: path, Explicit: false})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "config: "+path+": ")
	})

	t.Run("full valid file", func(t *testing.T) {
		path := filepath.Join(dir, "full.yaml")
		body := `endpoint: prod:9000
output: json
timeout: 30s
page_size: 10
`
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
		f, err := config.Load(config.Path{Value: path, Explicit: true})
		require.NoError(t, err)
		require.NotNil(t, f.Endpoint)
		assert.Equal(t, "prod:9000", *f.Endpoint)
		require.NotNil(t, f.Output)
		assert.Equal(t, "json", *f.Output)
		require.NotNil(t, f.Timeout)
		assert.Equal(t, 30*time.Second, *f.Timeout)
		require.NotNil(t, f.PageSize)
		assert.Equal(t, 10, *f.PageSize)
	})

	t.Run("partial valid file", func(t *testing.T) {
		path := filepath.Join(dir, "partial.yaml")
		require.NoError(t, os.WriteFile(path, []byte("endpoint: only.host:1\n"), 0o600))
		f, err := config.Load(config.Path{Value: path, Explicit: false})
		require.NoError(t, err)
		require.NotNil(t, f.Endpoint)
		assert.Equal(t, "only.host:1", *f.Endpoint)
		assert.Nil(t, f.Output)
		assert.Nil(t, f.Timeout)
		assert.Nil(t, f.PageSize)
	})

	t.Run("invalid timeout value", func(t *testing.T) {
		path := filepath.Join(dir, "bad-timeout.yaml")
		require.NoError(t, os.WriteFile(path, []byte("timeout: nope\n"), 0o600))
		_, err := config.Load(config.Path{Value: path, Explicit: false})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "config: "+path+": invalid timeout \"nope\":")
	})
}
