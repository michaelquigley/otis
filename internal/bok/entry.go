package bok

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Entry is one markdown BoK entry read from disk.
type Entry struct {
	Relpath string
	Title   string
	Tags    []string
	Body    string
}

// ReadEntry reads one extensionless BoK entry from disk.
func ReadEntry(bokPath string, relpath string) (*Entry, error) {
	cleanRelpath, err := cleanEntryRelpath(relpath)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(filepath.Join(bokPath, filepath.FromSlash(cleanRelpath)+".md"))
	if err != nil {
		return nil, err
	}
	fm, body, err := parseFrontmatter(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", cleanRelpath, err)
	}
	return &Entry{
		Relpath: cleanRelpath,
		Title:   fm.Title,
		Tags:    append([]string(nil), fm.Tags...),
		Body:    strings.TrimLeft(string(body), "\n"),
	}, nil
}

func cleanEntryRelpath(relpath string) (string, error) {
	relpath = strings.TrimSpace(filepath.ToSlash(relpath))
	if relpath == "" {
		return "", fmt.Errorf("entry relpath is required")
	}
	if strings.HasPrefix(relpath, "/") {
		return "", fmt.Errorf("entry relpath must be relative")
	}
	if strings.HasSuffix(relpath, ".md") {
		return "", fmt.Errorf("entry relpath must omit .md extension")
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(relpath)))
	if clean == "." || clean == "" {
		return "", fmt.Errorf("entry relpath is required")
	}
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("entry relpath must not escape the BoK root")
	}
	return clean, nil
}
