package client

import (
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/michaelquigley/df/dd"
)

const DefaultConfigPath = "~/.config/otis/config.yaml"

type Config struct {
	URL        string `dd:"url,+required"`
	Token      string `dd:",+required"`
	TLS        *TLSConfig
	ConfigPath string `dd:"-"`
}

type TLSConfig struct {
	CACert             string `dd:"ca_cert,+omitempty"`
	InsecureSkipVerify bool   `dd:",+omitempty"`
}

func DefaultConfig() *Config {
	return &Config{
		TLS: &TLSConfig{},
	}
}

func LoadConfig(path string) (*Config, error) {
	if path == "" {
		path = DefaultConfigPath
	}
	absPath, err := filepath.Abs(expandPath(path))
	if err != nil {
		return nil, err
	}
	cfg := DefaultConfig()
	if err := dd.BindYAMLFile(cfg, absPath); err != nil {
		return nil, err
	}
	cfg.ConfigPath = absPath
	if err := cfg.Resolve(filepath.Dir(absPath)); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) Resolve(baseDir string) error {
	if c.TLS != nil && c.TLS.CACert != "" {
		c.TLS.CACert = resolvePath(baseDir, c.TLS.CACert)
	}
	return nil
}

func (c *Config) Validate() error {
	if c == nil {
		return errors.New("client config is nil")
	}
	if strings.TrimSpace(c.URL) == "" {
		return errors.New("url is required")
	}
	if _, err := url.ParseRequestURI(c.URL); err != nil {
		return err
	}
	if strings.TrimSpace(c.Token) == "" {
		return errors.New("token is required")
	}
	return nil
}

func resolvePath(baseDir string, path string) string {
	if path == "" {
		return path
	}
	path = expandPath(path)
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(baseDir, path))
}

func expandPath(path string) string {
	if path == "" {
		return path
	}
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			path = home
		}
	} else if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, path[2:])
		}
	}
	return os.ExpandEnv(path)
}
