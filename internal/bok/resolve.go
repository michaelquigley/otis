package bok

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const BareTermError = "bare-term entries are reserved for a future capability; use a directory ('vocabulary/') or file path ('vocabulary/lens-vs-view') instead"

// Resolve resolves include entries to a deduplicated, project-filtered entry slice.
func Resolve(bokPath string, include []string, projectName string) ([]*Entry, error) {
	relpaths, err := ResolveRelpaths(bokPath, include, projectName)
	if err != nil {
		return nil, err
	}
	entries := make([]*Entry, 0, len(relpaths))
	for _, relpath := range relpaths {
		entry, err := ReadEntry(bokPath, relpath)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// ResolveRelpaths resolves include entries to deterministic extensionless paths.
func ResolveRelpaths(bokPath string, include []string, projectName string) ([]string, error) {
	if bokPath == "" {
		return nil, fmt.Errorf("bok path is required")
	}
	if len(include) == 0 {
		return nil, fmt.Errorf("include list must not be empty")
	}

	seen := map[string]struct{}{}
	for _, raw := range include {
		entry := strings.TrimSpace(filepath.ToSlash(raw))
		if entry == "" {
			return nil, fmt.Errorf("include entry must not be empty")
		}
		if !strings.Contains(entry, "/") {
			return nil, fmt.Errorf("%s", BareTermError)
		}
		switch {
		case strings.HasSuffix(entry, "/"):
			relpaths, err := walkIncludeDir(bokPath, entry)
			if err != nil {
				return nil, err
			}
			for _, relpath := range relpaths {
				if inProjectScope(relpath, projectName) {
					seen[relpath] = struct{}{}
				}
			}
		default:
			relpath, err := cleanEntryRelpath(entry)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", entry, err)
			}
			if err := requireEntryFile(bokPath, relpath); err != nil {
				return nil, err
			}
			if inProjectScope(relpath, projectName) {
				seen[relpath] = struct{}{}
			}
		}
	}

	relpaths := make([]string, 0, len(seen))
	for relpath := range seen {
		relpaths = append(relpaths, relpath)
	}
	sort.Strings(relpaths)
	return relpaths, nil
}

// ListRelpaths lists every BoK markdown entry, including project-scoped entries.
func ListRelpaths(bokPath string) ([]string, error) {
	return walkIncludeDir(bokPath, "")
}

// ListEntries reads every BoK markdown entry.
func ListEntries(bokPath string) ([]*Entry, error) {
	relpaths, err := ListRelpaths(bokPath)
	if err != nil {
		return nil, err
	}
	entries := make([]*Entry, 0, len(relpaths))
	for _, relpath := range relpaths {
		entry, err := ReadEntry(bokPath, relpath)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func walkIncludeDir(bokPath string, include string) ([]string, error) {
	cleanInclude, err := cleanIncludeDir(include)
	if err != nil {
		return nil, err
	}
	root := filepath.Join(bokPath, filepath.FromSlash(cleanInclude))
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", include, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s: include directory is not a directory", include)
	}

	var relpaths []string
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".md" {
			return nil
		}
		rel, err := filepath.Rel(bokPath, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if strings.Count(rel, "/") == 0 {
			return nil
		}
		relpaths = append(relpaths, strings.TrimSuffix(rel, ".md"))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(relpaths)
	return relpaths, nil
}

func cleanIncludeDir(include string) (string, error) {
	include = strings.TrimSpace(filepath.ToSlash(include))
	include = strings.TrimSuffix(include, "/")
	if include == "" {
		return "", nil
	}
	if strings.HasPrefix(include, "/") {
		return "", fmt.Errorf("include directory must be relative")
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(include)))
	if clean == "." {
		return "", nil
	}
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("include directory must not escape the BoK root")
	}
	return clean, nil
}

func requireEntryFile(bokPath string, relpath string) error {
	path := filepath.Join(bokPath, filepath.FromSlash(relpath)+".md")
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%s: BoK entry not found", relpath)
		}
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("%s: BoK entry is a directory", relpath)
	}
	return nil
}

func inProjectScope(relpath string, projectName string) bool {
	parts := strings.Split(relpath, "/")
	if len(parts) < 2 || parts[0] != "projects" {
		return true
	}
	return parts[1] == projectName
}
