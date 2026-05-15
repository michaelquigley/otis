package config

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/michaelquigley/df/dd"
)

// ProfileConfig is a BoK-root shared pass profile.
type ProfileConfig struct {
	Passes     []*Pass
	ConfigPath string `dd:"-"`
}

type profileFile struct {
	Passes []*Pass
	Extra  map[string]any `dd:",+extra"`
}

// LoadProfile reads one shared pass profile from disk.
func LoadProfile(path string) (*ProfileConfig, error) {
	absPath, err := filepath.Abs(expandPath(path))
	if err != nil {
		return nil, err
	}
	file := &profileFile{}
	if err := dd.BindYAMLFile(file, absPath); err != nil {
		return nil, err
	}
	if err := rejectProfileOnlyFields(absPath, file.Extra); err != nil {
		return nil, err
	}
	cfg := &ProfileConfig{
		Passes:     file.Passes,
		ConfigPath: absPath,
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Validate checks that profile passes are complete and non-conflicting.
func (c *ProfileConfig) Validate() error {
	seen := map[string]struct{}{}
	for i, pass := range c.Passes {
		if pass == nil {
			return fmt.Errorf("%s: passes[%d] is required", c.ConfigPath, i)
		}
		pass.Source = c.ConfigPath
		if _, ok := seen[pass.Name]; ok {
			return fmt.Errorf("%s: duplicate pass %q", c.ConfigPath, pass.Name)
		}
		seen[pass.Name] = struct{}{}
		if err := validateProfilePass(pass, c.ConfigPath); err != nil {
			return err
		}
	}
	return nil
}

func rejectProfileOnlyFields(path string, extra map[string]any) error {
	for _, key := range []string{"include_configs", "disable", "project"} {
		if _, ok := extra[key]; ok {
			return fmt.Errorf("%s: shared profile cannot contain %s", path, key)
		}
	}
	for key := range extra {
		if strings.TrimSpace(key) != "" {
			return fmt.Errorf("%s: unknown top-level profile field %q", path, key)
		}
	}
	return nil
}
