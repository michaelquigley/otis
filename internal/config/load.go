package config

import (
	"fmt"
	"path/filepath"

	"github.com/michaelquigley/df/dd"
)

// ResolvedConfig is the complete supervisor configuration.
type ResolvedConfig struct {
	Global   *GlobalConfig
	Projects map[string]*ResolvedProject
}

// LoadGlobal reads, resolves, and validates the global config file.
func LoadGlobal(path string) (*GlobalConfig, error) {
	if path == "" {
		path = DefaultConfigPath
	}
	absPath, err := filepath.Abs(expandPath(path))
	if err != nil {
		return nil, err
	}
	cfg := DefaultGlobalConfig()
	if err := dd.MergeYAMLFile(cfg, absPath); err != nil {
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

// LoadProject reads and composes a per-project config file.
func LoadProject(path string, loader ProfileLoader) (*ResolvedProject, error) {
	absPath, err := filepath.Abs(expandPath(path))
	if err != nil {
		return nil, err
	}
	cfg := DefaultProjectConfig()
	if err := dd.MergeYAMLFile(cfg, absPath); err != nil {
		return nil, err
	}
	cfg.ConfigPath = absPath
	if err := cfg.Resolve(filepath.Dir(absPath)); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return Compose(cfg, loader)
}

// Load reads the global config and every declared project config.
func Load(path string) (*ResolvedConfig, error) {
	global, err := LoadGlobal(path)
	if err != nil {
		return nil, err
	}
	loader := ProfileLoaderForBok(global.Bok.Path)
	resolved := &ResolvedConfig{
		Global:   global,
		Projects: map[string]*ResolvedProject{},
	}
	for _, project := range global.Projects {
		if _, ok := resolved.Projects[project.Name]; ok {
			return nil, fmt.Errorf("duplicate project %q", project.Name)
		}
		loaded, err := LoadProject(project.Config, loader)
		if err != nil {
			return nil, fmt.Errorf("project %q: %w", project.Name, err)
		}
		if loaded.Project == nil || loaded.Project.Name != project.Name {
			return nil, fmt.Errorf("project %q config declares project.name %q", project.Name, loadedProjectName(loaded))
		}
		loaded.RepoPath = project.Path
		for _, pass := range loaded.Passes {
			if pass.Reviewer == nil {
				continue
			}
			if _, ok := global.Reviewers[pass.Reviewer.Kind]; !ok {
				return nil, fmt.Errorf("project %q pass %q references unknown reviewer %q", project.Name, pass.Name, pass.Reviewer.Kind)
			}
		}
		resolved.Projects[project.Name] = loaded
	}
	return resolved, nil
}

func loadedProjectName(project *ResolvedProject) string {
	if project == nil || project.Project == nil {
		return ""
	}
	return project.Project.Name
}
