package prompt

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const (
	ScopeFull   = "full"
	ScopePaths  = "paths"
	ScopeRecent = "recent"
)

type ScopeOptions struct {
	PerFileBytes    int
	TotalScopeBytes int
}

type ScopeContent struct {
	Kind    string
	GitHead string
	Files   []ManifestFile
	Inline  []InlineContent
}

type ManifestFile struct {
	Path      string
	Size      int64
	Truncated string
	Inline    bool
}

type InlineContent struct {
	Path    string
	Content string
	Diff    bool
}

// BuildScopeContent turns a resolved file set into manifest-plus-inline prompt content.
func BuildScopeContent(ctx context.Context, scopeKind string, files []string, baseSHA string, projectPath string, opts ScopeOptions) (ScopeContent, error) {
	if opts.PerFileBytes <= 0 {
		opts.PerFileBytes = 8192
	}
	if opts.TotalScopeBytes <= 0 {
		opts.TotalScopeBytes = 262144
	}
	head, err := gitHead(ctx, projectPath)
	if err != nil {
		return ScopeContent{}, err
	}
	content := ScopeContent{
		Kind:    scopeKind,
		GitHead: head,
	}
	switch scopeKind {
	case ScopeFull, ScopePaths:
		return buildFileInlineContent(content, files, projectPath, opts)
	case ScopeRecent:
		if baseSHA == "" {
			return ScopeContent{}, fmt.Errorf("base sha is required for recent scope content")
		}
		return buildRecentDiffContent(ctx, content, files, baseSHA, projectPath)
	default:
		return ScopeContent{}, fmt.Errorf("unknown scope kind %q", scopeKind)
	}
}

func buildFileInlineContent(content ScopeContent, files []string, projectPath string, opts ScopeOptions) (ScopeContent, error) {
	total := 0
	for _, relpath := range files {
		path := filepath.Join(projectPath, filepath.FromSlash(relpath))
		raw, err := os.ReadFile(path)
		if err != nil {
			return ScopeContent{}, err
		}
		manifest := ManifestFile{
			Path: relpath,
			Size: int64(len(raw)),
		}
		limit := opts.PerFileBytes
		if limit > len(raw) {
			limit = len(raw)
		}
		if total >= opts.TotalScopeBytes {
			manifest.Truncated = fmt.Sprintf("%d->0 bytes", len(raw))
			content.Files = append(content.Files, manifest)
			continue
		}
		remaining := opts.TotalScopeBytes - total
		if limit > remaining {
			limit = remaining
		}
		if limit < len(raw) {
			manifest.Truncated = fmt.Sprintf("%d->%d bytes", len(raw), limit)
		}
		manifest.Inline = limit > 0
		content.Files = append(content.Files, manifest)
		if limit > 0 {
			content.Inline = append(content.Inline, InlineContent{
				Path:    relpath,
				Content: string(raw[:limit]),
			})
			total += limit
		}
	}
	return content, nil
}

func buildRecentDiffContent(ctx context.Context, content ScopeContent, files []string, baseSHA string, projectPath string) (ScopeContent, error) {
	for _, relpath := range files {
		diff, err := gitDiff(ctx, projectPath, baseSHA, relpath)
		if err != nil {
			return ScopeContent{}, err
		}
		content.Files = append(content.Files, ManifestFile{
			Path:   relpath,
			Size:   int64(len(diff)),
			Inline: true,
		})
		content.Inline = append(content.Inline, InlineContent{
			Path:    relpath,
			Content: diff,
			Diff:    true,
		})
	}
	return content, nil
}

func gitHead(ctx context.Context, projectPath string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", projectPath, "rev-parse", "HEAD")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	raw, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse HEAD: %w%s", err, commandStderr(stderr.Bytes()))
	}
	return string(bytes.TrimSpace(raw)), nil
}

func gitDiff(ctx context.Context, projectPath string, baseSHA string, relpath string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", projectPath, "diff", baseSHA+"..HEAD", "--", relpath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	raw, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git diff %s..HEAD -- %s: %w%s", baseSHA, relpath, err, commandStderr(stderr.Bytes()))
	}
	return string(raw), nil
}

func commandStderr(raw []byte) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return ""
	}
	return ": " + string(trimmed)
}
