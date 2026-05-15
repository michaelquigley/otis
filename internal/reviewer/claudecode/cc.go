package claudecode

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/michaelquigley/otis/internal/prompt"
	"github.com/michaelquigley/otis/internal/reviewer"
)

const defaultBinaryPath = "claude"

type Options struct {
	BinaryPath string
	Model      string
	DryRun     bool
	ExtraArgs  []string
}

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
		return reviewer.Result{}, errors.New("claude-code reviewer working directory is required")
	}
	if len(req.Schema) == 0 {
		return reviewer.Result{}, errors.New("claude-code reviewer schema is required")
	}
	args := r.args(req.Prompt, req.Schema, req.Model)
	if r.options.DryRun {
		return reviewer.DryRunResult(req.Schema, "dry_run='true', command='"+reviewer.CommandLine(r.options.BinaryPath, displayArgs(args))+"'")
	}
	stdout, stderr, err := r.run(ctx, req.WorkingDir, args)
	if err != nil {
		return reviewer.Result{}, err
	}
	raw, output, err := reviewer.ParseCLIOutput(stdout, req.Schema)
	if err != nil {
		return reviewer.Result{}, fmt.Errorf("parse claude-code output: %w%s", err, reviewer.CommandOutputSuffix(stdout, stderr))
	}
	return reviewer.Result{
		Raw:        raw,
		Output:     output,
		Findings:   append([]prompt.ReviewerFinding(nil), output.Findings...),
		UsageNotes: r.usageNotes(stdout, stderr, req.Model),
	}, nil
}

func (r *Reviewer) run(ctx context.Context, workingDir string, args []string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, r.options.BinaryPath, args...)
	cmd.Dir = workingDir
	cmd.Env = os.Environ()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stdout.Bytes(), stderr.Bytes(), fmt.Errorf("claude-code reviewer failed: %w%s", err, reviewer.CommandOutputSuffix(stdout.Bytes(), stderr.Bytes()))
	}
	return stdout.Bytes(), stderr.Bytes(), nil
}

func (r *Reviewer) args(promptText string, schema json.RawMessage, model string) []string {
	args := []string{
		"-p", promptText,
		"--output-format", "json",
		"--json-schema", string(schema),
		"--bare",
		"--tools", "Read,Glob,Grep,LS",
		"--permission-mode", "plan",
	}
	if model == "" {
		model = r.options.Model
	}
	if model != "" {
		args = append(args, "--model", model)
	}
	args = append(args, r.options.ExtraArgs...)
	return args
}

func (r *Reviewer) usageNotes(stdout []byte, stderr []byte, model string) string {
	parts := []string{fmt.Sprintf("binary='%s'", r.options.BinaryPath)}
	if model == "" {
		model = r.options.Model
	}
	if model != "" {
		parts = append(parts, fmt.Sprintf("model='%s'", model))
	}
	parts = append(parts, fmt.Sprintf("stdout_bytes='%d'", len(stdout)))
	parts = append(parts, fmt.Sprintf("stderr_bytes='%d'", len(stderr)))
	return strings.Join(parts, ", ")
}

func displayArgs(args []string) []string {
	out := append([]string(nil), args...)
	for i := 0; i < len(out); i++ {
		switch out[i] {
		case "-p":
			if i+1 < len(out) {
				out[i+1] = "<prompt>"
			}
		case "--json-schema":
			if i+1 < len(out) {
				out[i+1] = "<schema>"
			}
		}
	}
	return out
}
