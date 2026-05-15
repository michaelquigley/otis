package pi

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/michaelquigley/otis/internal/prompt"
	"github.com/michaelquigley/otis/internal/reviewer"
)

func TestReviewerRunsPi(t *testing.T) {
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "args.txt")
	cwdPath := filepath.Join(dir, "cwd.txt")
	binary := filepath.Join(dir, "pi")
	script := "#!/bin/sh\npwd > " + cwdPath + "\nprintf '%s\\n' \"$@\" > " + argsPath + "\nprintf '%s\\n' '{\"type\":\"final\",\"result\":\"{\\\"findings\\\":[]}\"}'\n"
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	r := New(Options{BinaryPath: binary, Model: "openai/test"})
	result, err := r.Review(context.Background(), reviewer.Request{
		Prompt:     "review this",
		Schema:     prompt.ReviewerOutputSchema(3),
		WorkingDir: dir,
	})
	if err != nil {
		t.Fatalf("review: %v", err)
	}
	if len(result.Findings) != 0 {
		t.Fatalf("findings = %d", len(result.Findings))
	}
	args := readTestFile(t, argsPath)
	for _, want := range []string{"--tools", "read,grep,find,ls", "--no-session", "--mode", "json", "-p", "review this", "--model", "openai/test"} {
		if !strings.Contains(args, want) {
			t.Fatalf("args missing %q:\n%s", want, args)
		}
	}
	if cwd := strings.TrimSpace(readTestFile(t, cwdPath)); cwd != dir {
		t.Fatalf("cwd = %q, want %q", cwd, dir)
	}
}

func TestDryRun(t *testing.T) {
	r := New(Options{BinaryPath: "missing-pi", DryRun: true})
	result, err := r.Review(context.Background(), reviewer.Request{
		Prompt:     "review this",
		Schema:     prompt.ReviewerOutputSchema(3),
		WorkingDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("review: %v", err)
	}
	if !strings.Contains(result.UsageNotes, "dry_run='true'") {
		t.Fatalf("usage notes = %q", result.UsageNotes)
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
