package codex

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/michaelquigley/otis/internal/prompt"
	"github.com/michaelquigley/otis/internal/reviewer"
)

const defaultBinaryPath = "codex"

type Options struct {
	BinaryPath string
	Model      string
	ExtraArgs  []string
}

// Reviewer invokes codex exec for one structured Otis review.
type Reviewer struct {
	options Options
}

func New(options Options) *Reviewer {
	if options.BinaryPath == "" {
		options.BinaryPath = defaultBinaryPath
	}
	options.ExtraArgs = append([]string(nil), options.ExtraArgs...)
	return &Reviewer{options: options}
}

func (r *Reviewer) Review(ctx context.Context, req reviewer.Request) (reviewer.Result, error) {
	if err := ctx.Err(); err != nil {
		return reviewer.Result{}, err
	}
	if req.WorkingDir == "" {
		return reviewer.Result{}, errors.New("codex reviewer working directory is required")
	}
	if len(req.Schema) == 0 {
		return reviewer.Result{}, errors.New("codex reviewer schema is required")
	}

	tempDir, err := os.MkdirTemp("", "otis-codex-*")
	if err != nil {
		return reviewer.Result{}, fmt.Errorf("create codex temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	schemaPath := filepath.Join(tempDir, "schema.json")
	lastMessagePath := filepath.Join(tempDir, "last-message.json")
	if err := os.WriteFile(schemaPath, req.Schema, 0o600); err != nil {
		return reviewer.Result{}, fmt.Errorf("write codex schema file: %w", err)
	}

	stdout, stderr, runErr := r.run(ctx, req.WorkingDir, req.Prompt, schemaPath, lastMessagePath, req.Model)
	output, err := os.ReadFile(lastMessagePath)
	if err != nil {
		if runErr != nil {
			return reviewer.Result{}, runErr
		}
		return reviewer.Result{}, fmt.Errorf("read codex last message file: %w", err)
	}
	raw, err := extractReviewOutput(output)
	if err != nil {
		if runErr != nil {
			return reviewer.Result{}, fmt.Errorf("%w; extract codex last message: %v", runErr, err)
		}
		return reviewer.Result{}, err
	}
	parsed, err := prompt.ParseReviewerOutput(raw, req.Schema)
	if err != nil {
		return reviewer.Result{}, err
	}
	return reviewer.Result{
		Raw:        raw,
		Output:     parsed,
		Findings:   append([]prompt.ReviewerFinding(nil), parsed.Findings...),
		UsageNotes: r.usageNotes(stdout, stderr, req.Model, runErr != nil),
	}, nil
}

func (r *Reviewer) run(ctx context.Context, workingDir string, promptText string, schemaPath string, lastMessagePath string, model string) ([]byte, []byte, error) {
	args := r.args(workingDir, schemaPath, lastMessagePath, model)
	cmd := exec.CommandContext(ctx, r.options.BinaryPath, args...)
	cmd.Stdin = strings.NewReader(promptText)

	codexHome, cleanup, err := r.prepareCodexHome(workingDir)
	if err != nil {
		return nil, nil, err
	}
	defer cleanup()
	cmd.Env = append(os.Environ(), "CODEX_HOME="+codexHome)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return stdout.Bytes(), stderr.Bytes(), fmt.Errorf("codex reviewer failed: %w%s", err, commandOutputSuffix(stdout.Bytes(), stderr.Bytes()))
	}
	return stdout.Bytes(), stderr.Bytes(), nil
}

func (r *Reviewer) args(workingDir string, schemaPath string, lastMessagePath string, model string) []string {
	args := []string{
		"exec",
		"-C", workingDir,
		"--ephemeral",
		"--skip-git-repo-check",
		"--sandbox", "read-only",
		"--output-schema", schemaPath,
		"--output-last-message", lastMessagePath,
	}
	if model == "" {
		model = r.options.Model
	}
	if model != "" {
		args = append(args, "-m", model)
	}
	args = append(args, r.options.ExtraArgs...)
	return args
}

func (r *Reviewer) prepareCodexHome(workingDir string) (string, func(), error) {
	originalHome, err := codexHome()
	if err != nil {
		return "", func() {}, err
	}
	dir, err := os.MkdirTemp(workingDir, ".codex-home-*")
	if err != nil {
		return "", func() {}, fmt.Errorf("create codex home: %w", err)
	}
	cleanup := func() {
		_ = os.RemoveAll(dir)
	}
	for _, entry := range []string{"auth.json", "config.toml"} {
		if err := linkCodexHomeEntry(originalHome, dir, entry); err != nil {
			cleanup()
			return "", func() {}, err
		}
	}
	for _, subdir := range []string{"sessions", "log", ".tmp", "tmp"} {
		if err := os.MkdirAll(filepath.Join(dir, subdir), 0o700); err != nil {
			cleanup()
			return "", func() {}, fmt.Errorf("create codex home subdir %q: %w", subdir, err)
		}
	}
	return dir, cleanup, nil
}

func (r *Reviewer) usageNotes(stdout []byte, stderr []byte, model string, recoveredAfterError bool) string {
	parts := []string{fmt.Sprintf("binary='%s'", r.options.BinaryPath)}
	if model == "" {
		model = r.options.Model
	}
	if model != "" {
		parts = append(parts, fmt.Sprintf("model='%s'", model))
	}
	parts = append(parts, fmt.Sprintf("stdout_bytes='%d'", len(stdout)))
	parts = append(parts, fmt.Sprintf("stderr_bytes='%d'", len(stderr)))
	if recoveredAfterError {
		parts = append(parts, "recovered_last_message_after_error='true'")
	}
	return strings.Join(parts, ", ")
}

func codexHome() (string, error) {
	if path := os.Getenv("CODEX_HOME"); path != "" {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve codex home: %w", err)
	}
	return filepath.Join(home, ".codex"), nil
}

func linkCodexHomeEntry(sourceHome string, targetHome string, name string) error {
	source := filepath.Join(sourceHome, name)
	target := filepath.Join(targetHome, name)
	if _, err := os.Stat(source); err != nil {
		return fmt.Errorf("codex home entry %q is not available: %w", source, err)
	}
	if err := os.Symlink(source, target); err != nil {
		return fmt.Errorf("link codex home entry %q: %w", name, err)
	}
	return nil
}

func commandOutputSuffix(stdout []byte, stderr []byte) string {
	var parts []string
	if text := strings.TrimSpace(string(stderr)); text != "" {
		parts = append(parts, fmt.Sprintf("stderr: %s", text))
	}
	if text := strings.TrimSpace(string(stdout)); text != "" {
		parts = append(parts, fmt.Sprintf("stdout: %s", text))
	}
	if len(parts) == 0 {
		return ""
	}
	return "; " + strings.Join(parts, "; ")
}

func copyRaw(raw []byte) json.RawMessage {
	return append(json.RawMessage(nil), raw...)
}
