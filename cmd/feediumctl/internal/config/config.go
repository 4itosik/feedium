// Package config loads the feediumctl YAML configuration file.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"sigs.k8s.io/yaml"
)

const EnvConfigPath = "FEEDIUMCTL_CONFIG"

// File holds the parsed configuration values from the YAML file. Each field is
// a pointer so callers can distinguish "unset" from "set to zero value".
type File struct {
	Endpoint *string
	Output   *string
	Timeout  *time.Duration
	PageSize *int
}

// Path describes where the config was sourced from.
type Path struct {
	Value    string
	Explicit bool // true if from --config or FEEDIUMCTL_CONFIG
}

// ResolvePath picks the config path:
//
//	--config (flagValue, if non-empty) > env FEEDIUMCTL_CONFIG > ~/.feediumctl.yaml.
//
// Returns ok=false only for the default case when $HOME cannot be resolved.
func ResolvePath(flagValue string, getenv func(string) string, userHomeDir func() (string, error)) (Path, bool) {
	if flagValue != "" {
		return Path{Value: flagValue, Explicit: true}, true
	}
	if v := getenv(EnvConfigPath); v != "" {
		return Path{Value: v, Explicit: true}, true
	}
	home, err := userHomeDir()
	if err != nil || home == "" {
		return Path{}, false
	}
	return Path{Value: filepath.Join(home, ".feediumctl.yaml"), Explicit: false}, true
}

// Load reads and parses the YAML config at p. Error messages follow the
// pattern "config: <path>: <reason>" (NFR-03, FR-06).
//
// Behaviour:
//   - explicit path, file absent -> error "config: <path>: not found"
//   - default path, file absent  -> empty File, no error (silent fallback)
//   - unparseable YAML or unknown key -> error "config: <path>: <reason>"
func Load(p Path) (File, error) {
	data, err := os.ReadFile(p.Value)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			if p.Explicit {
				return File{}, fmt.Errorf("config: %s: not found", p.Value)
			}
			return File{}, nil
		}
		return File{}, fmt.Errorf("config: %s: %s", p.Value, err.Error())
	}
	return parse(p.Value, data)
}

// LoadDefault is a convenience wrapper for callers that do not want to resolve
// the path themselves. If no explicit path is supplied and $HOME is missing,
// the returned File is empty with no error.
func LoadDefault(flagValue string) (File, error) {
	p, ok := ResolvePath(flagValue, os.Getenv, os.UserHomeDir)
	if !ok {
		return File{}, nil
	}
	return Load(p)
}

type rawFile struct {
	Endpoint *string `json:"endpoint,omitempty"`
	Output   *string `json:"output,omitempty"`
	Timeout  *string `json:"timeout,omitempty"`
	PageSize *int    `json:"page_size,omitempty"`
}

func parse(path string, data []byte) (File, error) {
	var raw rawFile
	if err := yaml.UnmarshalStrict(data, &raw); err != nil {
		return File{}, fmt.Errorf("config: %s: %s", path, err.Error())
	}

	var out File
	if raw.Endpoint != nil {
		e := *raw.Endpoint
		out.Endpoint = &e
	}
	if raw.Output != nil {
		o := *raw.Output
		out.Output = &o
	}
	if raw.PageSize != nil {
		p := *raw.PageSize
		out.PageSize = &p
	}
	if raw.Timeout != nil {
		d, err := time.ParseDuration(*raw.Timeout)
		if err != nil {
			return File{}, fmt.Errorf("config: %s: invalid timeout %q: %s", path, *raw.Timeout, err.Error())
		}
		out.Timeout = &d
	}
	return out, nil
}
