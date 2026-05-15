package config

import (
	"errors"
	"fmt"
	"strings"

	"github.com/michaelquigley/otis/internal/state"
)

const DefaultTopFindings = 3

// ProjectConfig is the BoK-owned per-project configuration.
type ProjectConfig struct {
	IncludeConfigs []string
	Project        *ProjectBlock
	Disable        []string
	Passes         []*Pass
	ConfigPath     string `dd:"-"`
}

type ProjectBlock struct {
	Name            string
	Description     string
	PrimaryLanguage string
	Notify          *ProjectNotifyConfig
	TopFindings     int
}

type ProjectNotifyConfig struct {
	Mattermost string
}

type Pass struct {
	Name        string
	Description string              `dd:",+omitempty"`
	Scope       *ScopeConfig        `dd:",+omitempty"`
	Reviewer    *PassReviewerConfig `dd:",+omitempty"`
	Cadence     string              `dd:",+omitempty"`
	TopFindings int                 `dd:",+omitempty"`
	Source      string              `dd:"-"`
}

type ScopeConfig struct {
	Project *ProjectScopeConfig `dd:",+omitempty"`
	Bok     *BokScopeConfig     `dd:",+omitempty"`
}

type ProjectScopeConfig struct {
	Type   string
	Paths  []string `dd:",+omitempty"`
	Window string   `dd:",+omitempty"`
}

type BokScopeConfig struct {
	Include []string
}

type PassReviewerConfig struct {
	Kind  string
	Model string `dd:",+omitempty"`
}

// DefaultProjectConfig returns per-project defaults.
func DefaultProjectConfig() *ProjectConfig {
	return &ProjectConfig{
		Project: &ProjectBlock{
			Notify:      &ProjectNotifyConfig{},
			TopFindings: DefaultTopFindings,
		},
	}
}

// Validate checks raw project config fields that do not require composition.
func (c *ProjectConfig) Validate() error {
	if c == nil {
		return errors.New("project config is nil")
	}
	if c.Project == nil {
		return errors.New("project block is required")
	}
	if err := state.ValidateIDComponent(c.Project.Name); err != nil {
		return fmt.Errorf("project.name: %w", err)
	}
	if c.Project.TopFindings < 0 {
		return errors.New("project.top_findings must not be negative")
	}
	if err := validateNameList("disable", c.Disable); err != nil {
		return err
	}
	seen := map[string]struct{}{}
	for i, pass := range c.Passes {
		if pass == nil {
			return fmt.Errorf("passes[%d] is required", i)
		}
		if err := validatePassName(pass.Name, fmt.Sprintf("passes[%d].name", i)); err != nil {
			return err
		}
		if _, ok := seen[pass.Name]; ok {
			return fmt.Errorf("duplicate pass %q in %s", pass.Name, c.ConfigPath)
		}
		seen[pass.Name] = struct{}{}
		if err := validatePartialPass(pass, fmt.Sprintf("passes[%d]", i)); err != nil {
			return err
		}
		pass.Source = c.ConfigPath
	}
	return nil
}

// Resolve expands paths in project-owned config. Phase 1 has none.
func (c *ProjectConfig) Resolve(baseDir string) error {
	return nil
}

func validatePartialPass(pass *Pass, path string) error {
	if pass.Cadence != "" {
		if _, err := ParseDuration(pass.Cadence); err != nil {
			return fmt.Errorf("%s.cadence: %w", path, err)
		}
	}
	if pass.TopFindings < 0 {
		return fmt.Errorf("%s.top_findings must not be negative", path)
	}
	if pass.Scope != nil {
		if pass.Scope.Project != nil {
			if err := validateProjectScope(pass.Scope.Project, path+".scope.project", false); err != nil {
				return err
			}
		}
		if pass.Scope.Bok != nil {
			if err := validateBokInclude(pass.Scope.Bok.Include, path+".scope.bok.include", true); err != nil {
				return err
			}
		}
	}
	if pass.Reviewer != nil && pass.Reviewer.Kind != "" {
		if err := state.ValidateIDComponent(pass.Reviewer.Kind); err != nil {
			return fmt.Errorf("%s.reviewer.kind: %w", path, err)
		}
	}
	return nil
}

func validateResolvedPass(pass *Pass, source string) error {
	return validateCompletePass(pass, source, true)
}

func validateProfilePass(pass *Pass, source string) error {
	return validateCompletePass(pass, source, false)
}

func validateCompletePass(pass *Pass, source string, requireTopFindings bool) error {
	if err := validatePassName(pass.Name, "pass.name"); err != nil {
		return fmt.Errorf("%s: %w", source, err)
	}
	if pass.Cadence == "" {
		return fmt.Errorf("%s: pass %q cadence is required", source, pass.Name)
	}
	if _, err := ParseDuration(pass.Cadence); err != nil {
		return fmt.Errorf("%s: pass %q cadence: %w", source, pass.Name, err)
	}
	if requireTopFindings && pass.TopFindings <= 0 {
		return fmt.Errorf("%s: pass %q top_findings must be greater than zero", source, pass.Name)
	}
	if pass.TopFindings < 0 {
		return fmt.Errorf("%s: pass %q top_findings must not be negative", source, pass.Name)
	}
	if pass.Reviewer == nil || pass.Reviewer.Kind == "" {
		return fmt.Errorf("%s: pass %q reviewer.kind is required", source, pass.Name)
	}
	if err := state.ValidateIDComponent(pass.Reviewer.Kind); err != nil {
		return fmt.Errorf("%s: pass %q reviewer.kind: %w", source, pass.Name, err)
	}
	if pass.Scope == nil {
		return fmt.Errorf("%s: pass %q scope is required", source, pass.Name)
	}
	if pass.Scope.Project == nil {
		return fmt.Errorf("%s: pass %q scope.project is required", source, pass.Name)
	}
	if err := validateProjectScope(pass.Scope.Project, "scope.project", true); err != nil {
		return fmt.Errorf("%s: pass %q: %w", source, pass.Name, err)
	}
	if pass.Scope.Bok == nil {
		return fmt.Errorf("%s: pass %q scope.bok is required", source, pass.Name)
	}
	if err := validateBokInclude(pass.Scope.Bok.Include, "scope.bok.include", true); err != nil {
		return fmt.Errorf("%s: pass %q: %w", source, pass.Name, err)
	}
	return nil
}

func validateProjectScope(scope *ProjectScopeConfig, path string, complete bool) error {
	if scope.Type == "" {
		if complete {
			return fmt.Errorf("%s.type is required", path)
		}
		return nil
	}
	switch scope.Type {
	case "full":
	case "paths":
		if complete && len(scope.Paths) == 0 {
			return fmt.Errorf("%s.paths is required when type is paths", path)
		}
	case "recent":
		if scope.Window == "" {
			return fmt.Errorf("%s.window is required when type is recent", path)
		}
		if _, err := ParseDuration(scope.Window); err != nil {
			return fmt.Errorf("%s.window: %w", path, err)
		}
	default:
		return fmt.Errorf("%s.type must be one of full, paths, recent", path)
	}
	return nil
}

func validateBokInclude(include []string, path string, complete bool) error {
	if len(include) == 0 {
		if complete {
			return fmt.Errorf("%s must not be empty", path)
		}
		return nil
	}
	for i, entry := range include {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			return fmt.Errorf("%s[%d] must not be empty", path, i)
		}
		if !strings.Contains(entry, "/") {
			return fmt.Errorf("%s[%d]: bare-term entries are reserved for a future capability; use a directory ('vocabulary/') or file path ('vocabulary/lens-vs-view') instead", path, i)
		}
	}
	return nil
}

func validatePassName(name string, path string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("%s is required", path)
	}
	if err := state.ValidateIDComponent(name); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

func validateNameList(path string, names []string) error {
	for i, name := range names {
		if err := validatePassName(name, fmt.Sprintf("%s[%d]", path, i)); err != nil {
			return err
		}
	}
	return nil
}
