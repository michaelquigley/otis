package config

import (
	"fmt"
	"path/filepath"

	"github.com/michaelquigley/df/dd"
)

// ProfileLoader loads a named profile from the BoK root.
type ProfileLoader func(name string) (*ProfileConfig, error)

// ResolvedProject is a post-composition project config.
type ResolvedProject struct {
	ConfigPath string
	RepoPath   string
	Project    *ProjectBlock
	Passes     []*Pass
}

// Compose resolves included profiles, disables, and project overrides.
func Compose(project *ProjectConfig, loader ProfileLoader) (*ResolvedProject, error) {
	if project == nil {
		return nil, fmt.Errorf("project config is nil")
	}
	if loader == nil {
		return nil, fmt.Errorf("profile loader is nil")
	}

	composed := map[string]*Pass{}
	sources := map[string]string{}
	order := []string{}

	for _, include := range project.IncludeConfigs {
		profile, err := loader(include)
		if err != nil {
			return nil, fmt.Errorf("%s: include_configs %q: %w", project.ConfigPath, include, err)
		}
		for _, pass := range profile.Passes {
			if previous, ok := sources[pass.Name]; ok {
				return nil, fmt.Errorf("pass %q appears in both %s and %s", pass.Name, previous, profile.ConfigPath)
			}
			cloned, err := clonePass(pass)
			if err != nil {
				return nil, fmt.Errorf("%s: clone pass %q: %w", profile.ConfigPath, pass.Name, err)
			}
			composed[pass.Name] = cloned
			sources[pass.Name] = profile.ConfigPath
			order = append(order, pass.Name)
		}
	}

	for _, disabled := range project.Disable {
		if _, ok := composed[disabled]; ok {
			delete(composed, disabled)
			delete(sources, disabled)
			order = removeName(order, disabled)
		}
	}

	for _, pass := range project.Passes {
		if existing, ok := composed[pass.Name]; ok {
			if err := mergePass(existing, pass); err != nil {
				return nil, fmt.Errorf("%s: override pass %q: %w", project.ConfigPath, pass.Name, err)
			}
			existing.Source = project.ConfigPath
			sources[pass.Name] = project.ConfigPath
			continue
		}
		cloned, err := clonePass(pass)
		if err != nil {
			return nil, fmt.Errorf("%s: clone pass %q: %w", project.ConfigPath, pass.Name, err)
		}
		composed[pass.Name] = cloned
		sources[pass.Name] = project.ConfigPath
		order = append(order, pass.Name)
	}

	passes := make([]*Pass, 0, len(order))
	for _, name := range order {
		pass := composed[name]
		if pass.TopFindings == 0 && project.Project != nil && project.Project.TopFindings > 0 {
			pass.TopFindings = project.Project.TopFindings
		}
		if err := validateResolvedPass(pass, sources[name]); err != nil {
			return nil, err
		}
		passes = append(passes, pass)
	}

	return &ResolvedProject{
		ConfigPath: project.ConfigPath,
		Project:    cloneProjectBlock(project.Project),
		Passes:     passes,
	}, nil
}

func ProfileLoaderForBok(bokPath string) ProfileLoader {
	return func(name string) (*ProfileConfig, error) {
		path := name
		if filepath.Ext(path) == "" {
			path += ".yaml"
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(bokPath, path)
		}
		return LoadProfile(path)
	}
}

func clonePass(pass *Pass) (*Pass, error) {
	data, err := dd.Unbind(pass)
	if err != nil {
		return nil, err
	}
	cloned, err := dd.New[Pass](data)
	if err != nil {
		return nil, err
	}
	cloned.Source = pass.Source
	return cloned, nil
}

func mergePass(base *Pass, override *Pass) error {
	data, err := dd.Unbind(override)
	if err != nil {
		return err
	}
	return dd.Merge(base, data)
}

func cloneProjectBlock(project *ProjectBlock) *ProjectBlock {
	if project == nil {
		return nil
	}
	out := *project
	if project.Notify != nil {
		notify := *project.Notify
		out.Notify = &notify
	}
	return &out
}

func removeName(names []string, target string) []string {
	out := names[:0]
	for _, name := range names {
		if name != target {
			out = append(out, name)
		}
	}
	return out
}
