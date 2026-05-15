package config

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/michaelquigley/otis/internal/state"
)

const (
	DefaultConfigPath           = "./otis.yaml"
	DefaultStateDir             = "/var/otis/state"
	DefaultPerFileBytes         = 8192
	DefaultTotalScopeBytes      = 262144
	DefaultGlobalConcurrencyCap = 6
)

// GlobalConfig is the operator-owned Otis configuration.
type GlobalConfig struct {
	Bok                  *BokConfig
	Storage              *StorageConfig
	Prompt               *PromptConfig
	API                  *APIConfig `dd:"api"`
	Notification         *NotificationConfig
	Reviewers            map[string]*ReviewerConfig
	Windows              map[string]*WindowConfig
	GlobalConcurrencyCap int
	Projects             []*GlobalProjectConfig
	ConfigPath           string `dd:"-"`
}

type BokConfig struct {
	Path string `dd:",+required"`
}

type StorageConfig struct {
	StateDir string
}

type PromptConfig struct {
	PerFileBytes    int
	TotalScopeBytes int
}

type APIConfig struct {
	Listen string
	TLS    *TLSConfig `dd:"tls"`
}

type TLSConfig struct {
	Cert string
	Key  string
}

func (c *APIConfig) TLSConfigured() bool {
	return c != nil &&
		c.TLS != nil &&
		strings.TrimSpace(c.TLS.Cert) != "" &&
		strings.TrimSpace(c.TLS.Key) != ""
}

type NotificationConfig struct {
	Mattermost    *MattermostConfig
	ReportBaseURL string `dd:"report_base_url"`
}

type MattermostConfig struct {
	URL      string
	TokenEnv string
}

type ReviewerConfig struct {
	Binary         string
	DefaultModel   string
	ConcurrencyCap int
	Window         string
	DryRun         bool `dd:",+omitempty"`
	OutputPath     string
}

type WindowConfig struct {
	Hours string
}

type GlobalProjectConfig struct {
	Name   string `dd:",+required"`
	Path   string `dd:",+required"`
	Config string
}

// DefaultGlobalConfig returns the operator config defaults.
func DefaultGlobalConfig() *GlobalConfig {
	return &GlobalConfig{
		Bok: &BokConfig{
			Path: "/var/otis/bok",
		},
		Storage: &StorageConfig{
			StateDir: DefaultStateDir,
		},
		Prompt: &PromptConfig{
			PerFileBytes:    DefaultPerFileBytes,
			TotalScopeBytes: DefaultTotalScopeBytes,
		},
		API: &APIConfig{
			Listen: "127.0.0.1:8443",
			TLS:    &TLSConfig{},
		},
		Notification: &NotificationConfig{
			Mattermost: &MattermostConfig{
				TokenEnv: "MATTERMOST_TOKEN",
			},
		},
		Reviewers: map[string]*ReviewerConfig{
			"codex": {
				Binary:         "codex",
				DefaultModel:   "gpt-5.4",
				ConcurrencyCap: 2,
				Window:         "overnight",
			},
			"claude-code": {
				Binary:         "claude",
				DefaultModel:   "opus-4.7",
				ConcurrencyCap: 2,
				Window:         "overnight",
			},
			"pi": {
				Binary:         "pi",
				DefaultModel:   "ollama/llama3.3",
				ConcurrencyCap: 4,
				Window:         "anytime",
			},
		},
		Windows: map[string]*WindowConfig{
			"overnight": {
				Hours: "22:00-06:00",
			},
			"working-hours": {
				Hours: "09:00-17:00",
			},
			"anytime": {
				Hours: "00:00-24:00",
			},
		},
		GlobalConcurrencyCap: DefaultGlobalConcurrencyCap,
	}
}

// Validate checks the operator config for Phase 1 invariants.
func (c *GlobalConfig) Validate() error {
	if c == nil {
		return errors.New("global config is nil")
	}
	if c.Bok == nil || strings.TrimSpace(c.Bok.Path) == "" {
		return errors.New("bok.path is required")
	}
	if c.Storage == nil || strings.TrimSpace(c.Storage.StateDir) == "" {
		return errors.New("storage.state_dir is required")
	}
	if c.Prompt == nil {
		return errors.New("prompt is required")
	}
	if c.Prompt.PerFileBytes <= 0 {
		return errors.New("prompt.per_file_bytes must be greater than zero")
	}
	if c.Prompt.TotalScopeBytes <= 0 {
		return errors.New("prompt.total_scope_bytes must be greater than zero")
	}
	if c.API == nil {
		return errors.New("api is required")
	}
	if strings.TrimSpace(c.API.Listen) == "" {
		return errors.New("api.listen is required")
	}
	if c.API.TLS != nil {
		cert := strings.TrimSpace(c.API.TLS.Cert)
		key := strings.TrimSpace(c.API.TLS.Key)
		if (cert == "") != (key == "") {
			return errors.New("api.tls.cert and api.tls.key must be configured together")
		}
	}
	if c.GlobalConcurrencyCap <= 0 {
		return errors.New("global_concurrency_cap must be greater than zero")
	}
	if len(c.Windows) == 0 {
		return errors.New("windows must not be empty")
	}
	for name, window := range c.Windows {
		if err := state.ValidateIDComponent(name); err != nil {
			return fmt.Errorf("windows.%s: %w", name, err)
		}
		if window == nil || strings.TrimSpace(window.Hours) == "" {
			return fmt.Errorf("windows.%s.hours is required", name)
		}
		if err := validateWindowHours(window.Hours); err != nil {
			return fmt.Errorf("windows.%s.hours: %w", name, err)
		}
	}
	if len(c.Reviewers) == 0 {
		return errors.New("reviewers must not be empty")
	}
	for name, reviewer := range c.Reviewers {
		if err := state.ValidateIDComponent(name); err != nil {
			return fmt.Errorf("reviewers.%s: %w", name, err)
		}
		if reviewer == nil {
			return fmt.Errorf("reviewers.%s is required", name)
		}
		if reviewer.ConcurrencyCap <= 0 {
			return fmt.Errorf("reviewers.%s.concurrency_cap must be greater than zero", name)
		}
		if reviewer.Window == "" {
			return fmt.Errorf("reviewers.%s.window is required", name)
		}
		if _, ok := c.Windows[reviewer.Window]; !ok {
			return fmt.Errorf("reviewers.%s.window references unknown window %q", name, reviewer.Window)
		}
	}
	seen := map[string]struct{}{}
	for i, project := range c.Projects {
		if project == nil {
			return fmt.Errorf("projects[%d] is required", i)
		}
		if err := state.ValidateIDComponent(project.Name); err != nil {
			return fmt.Errorf("projects[%d].name: %w", i, err)
		}
		if strings.TrimSpace(project.Path) == "" {
			return fmt.Errorf("projects[%d].path is required", i)
		}
		if _, ok := seen[project.Name]; ok {
			return fmt.Errorf("duplicate project %q", project.Name)
		}
		seen[project.Name] = struct{}{}
	}
	return nil
}

// Resolve expands paths and fills convention-based project config paths.
func (c *GlobalConfig) Resolve(baseDir string) error {
	if c.Bok != nil {
		c.Bok.Path = resolvePath(baseDir, c.Bok.Path)
	}
	if c.Storage != nil {
		c.Storage.StateDir = resolvePath(baseDir, c.Storage.StateDir)
	}
	if c.API != nil && c.API.TLS != nil {
		c.API.TLS.Cert = resolvePath(baseDir, c.API.TLS.Cert)
		c.API.TLS.Key = resolvePath(baseDir, c.API.TLS.Key)
	}
	for _, reviewer := range c.Reviewers {
		if reviewer != nil {
			reviewer.Binary = resolveCommandPath(baseDir, reviewer.Binary)
			reviewer.OutputPath = resolvePath(baseDir, reviewer.OutputPath)
		}
	}
	for _, project := range c.Projects {
		project.Path = resolvePath(baseDir, project.Path)
		if project.Config == "" {
			project.Config = filepath.Join(c.Bok.Path, "projects", project.Name, "otis.yaml")
		} else {
			project.Config = resolvePath(baseDir, project.Config)
		}
	}
	return nil
}

func validateWindowHours(value string) error {
	parts := strings.Split(value, "-")
	if len(parts) != 2 {
		return fmt.Errorf("expected HH:MM-HH:MM")
	}
	if _, err := parseWindowMinute(parts[0], false); err != nil {
		return fmt.Errorf("start: %w", err)
	}
	if _, err := parseWindowMinute(parts[1], true); err != nil {
		return fmt.Errorf("end: %w", err)
	}
	return nil
}

func parseWindowMinute(value string, allowEndOfDay bool) (int, error) {
	parts := strings.Split(value, ":")
	if len(parts) != 2 {
		return 0, fmt.Errorf("expected HH:MM")
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, err
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, err
	}
	if allowEndOfDay && hour == 24 && minute == 0 {
		return 24 * 60, nil
	}
	if hour < 0 || hour > 23 {
		return 0, fmt.Errorf("hour must be 00-23")
	}
	if minute < 0 || minute > 59 {
		return 0, fmt.Errorf("minute must be 00-59")
	}
	return hour*60 + minute, nil
}
