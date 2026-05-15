package dispatcher

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func CaptureHEAD(ctx context.Context, projectPath string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", projectPath, "rev-parse", "HEAD")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	raw, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("capture HEAD: %w%s", err, stderrSuffix(stderr.Bytes()))
	}
	return strings.TrimSpace(string(raw)), nil
}

func CreateWorktree(ctx context.Context, projectPath string, capturedSHA string, scratchPath string) error {
	if err := os.MkdirAll(filepath.Dir(scratchPath), 0o700); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "git", "-C", projectPath, "worktree", "add", "--detach", scratchPath, capturedSHA)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("create worktree: %w%s", err, stderrSuffix(stderr.Bytes()))
	}
	return nil
}

func RemoveWorktree(ctx context.Context, projectPath string, scratchPath string) error {
	cmd := exec.CommandContext(ctx, "git", "-C", projectPath, "worktree", "remove", "--force", scratchPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("remove worktree: %w%s", err, stderrSuffix(stderr.Bytes()))
	}
	return nil
}

func PruneWorktrees(ctx context.Context, projectPath string) error {
	cmd := exec.CommandContext(ctx, "git", "-C", projectPath, "worktree", "prune")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("prune worktrees: %w%s", err, stderrSuffix(stderr.Bytes()))
	}
	return nil
}

func stderrSuffix(raw []byte) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return ""
	}
	return ": " + string(raw)
}
