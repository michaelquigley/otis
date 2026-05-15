package dispatcher

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/michaelquigley/otis/internal/config"
	"github.com/michaelquigley/otis/internal/prompt"
)

const emptyTreeSHA = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"

type ResolvedScope struct {
	Kind    string
	Files   []string
	BaseSHA string
}

func ResolveScope(ctx context.Context, worktreePath string, scope *config.ProjectScopeConfig, now time.Time) (ResolvedScope, error) {
	if scope == nil {
		return ResolvedScope{}, fmt.Errorf("scope.project is required")
	}
	switch scope.Type {
	case "full":
		files, err := gitListFiles(ctx, worktreePath)
		return ResolvedScope{Kind: prompt.ScopeFull, Files: files}, err
	case "paths":
		files, err := resolvePathScope(ctx, worktreePath, scope.Paths)
		return ResolvedScope{Kind: prompt.ScopePaths, Files: files}, err
	case "recent":
		files, base, err := resolveRecentScope(ctx, worktreePath, scope.Window, now)
		return ResolvedScope{Kind: prompt.ScopeRecent, Files: files, BaseSHA: base}, err
	default:
		return ResolvedScope{}, fmt.Errorf("unknown project scope type %q", scope.Type)
	}
}

func resolvePathScope(ctx context.Context, worktreePath string, entries []string) ([]string, error) {
	seen := map[string]struct{}{}
	for _, entry := range entries {
		entry = filepath.ToSlash(strings.TrimSpace(entry))
		if entry == "" {
			continue
		}
		abs := filepath.Join(worktreePath, filepath.FromSlash(entry))
		if info, err := os.Stat(abs); err == nil && info.IsDir() {
			files, err := gitListFiles(ctx, worktreePath, entry)
			if err != nil {
				return nil, err
			}
			addFiles(seen, files)
			continue
		}
		if containsGlobMeta(entry) {
			matches, err := filepath.Glob(abs)
			if err != nil {
				return nil, err
			}
			for _, match := range matches {
				info, err := os.Stat(match)
				if err != nil {
					return nil, err
				}
				rel, err := filepath.Rel(worktreePath, match)
				if err != nil {
					return nil, err
				}
				rel = filepath.ToSlash(rel)
				if info.IsDir() {
					files, err := gitListFiles(ctx, worktreePath, rel)
					if err != nil {
						return nil, err
					}
					addFiles(seen, files)
				} else {
					seen[rel] = struct{}{}
				}
			}
			continue
		}
		if info, err := os.Stat(abs); err != nil {
			return nil, fmt.Errorf("%s: %w", entry, err)
		} else if info.IsDir() {
			files, err := gitListFiles(ctx, worktreePath, entry)
			if err != nil {
				return nil, err
			}
			addFiles(seen, files)
		} else {
			seen[entry] = struct{}{}
		}
	}
	files := make([]string, 0, len(seen))
	for file := range seen {
		files = append(files, file)
	}
	sort.Strings(files)
	return files, nil
}

func resolveRecentScope(ctx context.Context, worktreePath string, window string, now time.Time) ([]string, string, error) {
	duration, err := config.ParseDuration(window)
	if err != nil {
		return nil, "", err
	}
	if now.IsZero() {
		now = time.Now()
	}
	since := now.UTC().Add(-duration).Format(time.RFC3339)
	commits, err := gitLines(ctx, worktreePath, "log", "--first-parent", "--since="+since, "--pretty=%H", "HEAD")
	if err != nil {
		return nil, "", err
	}
	if len(commits) == 0 {
		return nil, "", nil
	}
	oldest := commits[len(commits)-1]
	base, err := gitLine(ctx, worktreePath, "rev-parse", oldest+"^1")
	if err != nil {
		base = emptyTreeSHA
	}
	files, err := gitLines(ctx, worktreePath, "diff", "--name-only", base+"..HEAD")
	if err != nil {
		return nil, "", err
	}
	return files, base, nil
}

func gitListFiles(ctx context.Context, worktreePath string, paths ...string) ([]string, error) {
	args := []string{"ls-files"}
	if len(paths) > 0 {
		args = append(args, "--")
		args = append(args, paths...)
	}
	files, err := gitLines(ctx, worktreePath, args...)
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func gitLine(ctx context.Context, worktreePath string, args ...string) (string, error) {
	lines, err := gitLines(ctx, worktreePath, args...)
	if err != nil {
		return "", err
	}
	if len(lines) == 0 {
		return "", fmt.Errorf("git %s produced no output", strings.Join(args, " "))
	}
	return lines[0], nil
}

func gitLines(ctx context.Context, worktreePath string, args ...string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", worktreePath}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	raw, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w%s", strings.Join(args, " "), err, stderrSuffix(stderr.Bytes()))
	}
	var lines []string
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(filepath.ToSlash(line))
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines, nil
}

func containsGlobMeta(value string) bool {
	return strings.ContainsAny(value, "*?[")
}

func addFiles(seen map[string]struct{}, files []string) {
	for _, file := range files {
		seen[file] = struct{}{}
	}
}
