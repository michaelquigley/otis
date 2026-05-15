package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/michaelquigley/otis/internal/state"
)

func TestFindingsCommandsUseLocalState(t *testing.T) {
	dir := t.TempDir()
	bokPath := dir + "/bok"
	statePath := dir + "/state"
	writeTestFile(t, bokPath+"/standard.yaml", `
passes:
  - name: vocabulary-sweep
    scope:
      project: { type: full }
      bok:
        include: [vocabulary/]
    reviewer: { kind: codex }
    cadence: 24h
`)
	writeTestFile(t, bokPath+"/projects/testproj/otis.yaml", `
include_configs: [standard]
project:
  name: testproj
  top_findings: 3
`)
	globalPath := dir + "/global.yaml"
	writeTestFile(t, globalPath, `
bok:
  path: ./bok
storage:
  state_dir: ./state
reviewers:
  codex:
    binary: codex
    concurrency_cap: 1
    window: anytime
windows:
  anytime:
    hours: "00:00-24:00"
global_concurrency_cap: 1
projects:
  - name: testproj
    path: ./repo
`)

	store, err := state.NewStore(statePath)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	project := store.Project("testproj")
	first, err := project.CreateFinding(state.CreateFindingRequest{
		Pass:         "vocabulary-sweep",
		Reviewer:     "codex",
		RunID:        "testproj/vocabulary-sweep/2026-05-15/010203Z-001",
		Severity:     "medium",
		Title:        "view should be lens",
		Location:     state.Location{File: "internal/hive/library.go", Lines: "42-78"},
		BokRefs:      []string{"vocabulary/lens-vs-view"},
		Description:  "The code uses view where lens matches the project vocabulary.",
		SuggestedFix: "Rename the perspectival surface to lens.",
	})
	if err != nil {
		t.Fatalf("create first finding: %v", err)
	}
	second, err := project.CreateFinding(state.CreateFindingRequest{
		Pass:         "vocabulary-sweep",
		Reviewer:     "codex",
		RunID:        "testproj/vocabulary-sweep/2026-05-15/010203Z-001",
		Severity:     "low",
		Title:        "library term overloaded",
		Location:     state.Location{File: "internal/hive/storage.go", Lines: "12-20"},
		BokRefs:      []string{"vocabulary/library-overloads"},
		Description:  "Library is carrying two meanings.",
		SuggestedFix: "Split storage vocabulary from hive vocabulary.",
	})
	if err != nil {
		t.Fatalf("create second finding: %v", err)
	}
	if _, err := project.ChangeDisposition(first.ID, state.DispositionAccepted, "michael", "will fix with hive refactor"); err != nil {
		t.Fatalf("accept finding: %v", err)
	}

	listOut := runCommand(t, "--config", globalPath, "findings", "list", "--project", "testproj", "--open")
	if strings.Contains(listOut, first.ID) {
		t.Fatalf("accepted finding appeared in open list:\n%s", listOut)
	}
	if !strings.Contains(listOut, second.ID) || !strings.Contains(listOut, "library term overloaded") {
		t.Fatalf("open finding missing from list:\n%s", listOut)
	}

	showOut := runCommand(t, "--config", globalPath, "findings", "show", second.ID)
	for _, want := range []string{second.ID, "severity: low", "disposition: open", "Library is carrying two meanings."} {
		if !strings.Contains(showOut, want) {
			t.Fatalf("show output missing %q:\n%s", want, showOut)
		}
	}
}

func runCommand(t *testing.T, args ...string) string {
	t.Helper()
	var out bytes.Buffer
	cmd := newRootCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute %v: %v\n%s", args, err, out.String())
	}
	return out.String()
}

func writeTestFile(t *testing.T, path string, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.TrimLeft(body, "\n")), 0o600); err != nil {
		t.Fatal(err)
	}
}
