package config

import (
	"os"
	"path/filepath"
	"strings"
)

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

func resolveCommandPath(baseDir string, path string) string {
	if path == "" {
		return path
	}
	expanded := expandPath(path)
	if filepath.IsAbs(expanded) {
		return filepath.Clean(expanded)
	}
	if strings.ContainsRune(expanded, os.PathSeparator) || strings.HasPrefix(expanded, ".") {
		return filepath.Clean(filepath.Join(baseDir, expanded))
	}
	return expanded
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
