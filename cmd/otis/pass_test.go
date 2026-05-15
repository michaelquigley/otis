package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/michaelquigley/otis/internal/config"
	"github.com/michaelquigley/otis/internal/dispatcher"
	"github.com/michaelquigley/otis/internal/prompt"
	"github.com/michaelquigley/otis/internal/state"
)

func TestPassRunDummyEndToEnd(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "repo")
	initPassTestRepo(t, repoPath)
	writeTestFile(t, filepath.Join(repoPath, "sample", "main.go"), "package sample\n\nfunc View() string { return \"view\" }\n")
	gitPass(t, repoPath, "add", ".")
	gitPass(t, repoPath, "-c", "user.name=Otis Test", "-c", "user.email=otis@example.com", "-c", "commit.gpgsign=false", "commit", "-m", "initial")

	bokPath := filepath.Join(dir, "bok")
	statePath := filepath.Join(dir, "state")
	writePhase5Config(t, dir)
	writeTestFile(t, filepath.Join(bokPath, "vocabulary", "lens-vs-view.md"), `
---
title: lens vs view
tags: [vocabulary]
---

# lens vs view

Prefer lens for perspectival surfaces.
`)

	store, err := state.NewStore(statePath)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	project := store.Project("testproj")
	openFinding := seedPassFinding(t, project, "open finding", state.DispositionOpen)
	acceptedFinding := seedPassFinding(t, project, "accepted finding", state.DispositionAccepted)
	deferredFinding := seedPassFinding(t, project, "deferred finding", state.DispositionDeferred)
	rejectedFinding := seedPassFinding(t, project, "rejected finding", state.DispositionRejected)
	if _, err := project.ChangeDisposition(acceptedFinding.ID, state.DispositionAccepted, "michael", "accepted note"); err != nil {
		t.Fatalf("accepted: %v", err)
	}
	if _, err := project.ChangeDisposition(deferredFinding.ID, state.DispositionDeferred, "michael", "deferred note"); err != nil {
		t.Fatalf("deferred: %v", err)
	}
	if _, err := project.ChangeDisposition(rejectedFinding.ID, state.DispositionRejected, "michael", "rejected note"); err != nil {
		t.Fatalf("rejected: %v", err)
	}

	dummyOutputPath := filepath.Join(dir, "dummy-output.json")
	err = dispatcher.WriteDummyOutput(dummyOutputPath, []prompt.ReviewerFinding{
		{
			ID:       openFinding.ID,
			Severity: "medium",
			Title:    "existing view should remain lens",
			Location: prompt.ReviewerLocation{
				File:  "sample/main.go",
				Lines: "3-3",
			},
			BokRefs:      []string{"vocabulary/lens-vs-view"},
			Description:  "The existing issue is still visible.",
			SuggestedFix: "Rename view to lens.",
		},
		{
			Severity: "high",
			Title:    "new view vocabulary drift",
			Location: prompt.ReviewerLocation{
				File:  "sample/main.go",
				Lines: "3-3",
			},
			BokRefs:      []string{"vocabulary/lens-vs-view"},
			Description:  "A new finding should be allocated.",
			SuggestedFix: "Use lens vocabulary.",
		},
	})
	if err != nil {
		t.Fatalf("write dummy output: %v", err)
	}

	cfg, err := config.Load(filepath.Join(dir, "global.yaml"))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	result, err := dispatcher.Run(context.Background(), dispatcher.RunRequest{
		Config:           cfg,
		Store:            store,
		ProjectName:      "testproj",
		PassName:         "vocabulary-sweep",
		ReviewerOverride: "dummy",
		DummyOutputPath:  dummyOutputPath,
	})
	if err != nil {
		t.Fatalf("run pass: %v", err)
	}
	if len(result.Findings) != 2 {
		t.Fatalf("findings = %d, want 2", len(result.Findings))
	}
	runID := result.RunID
	parsedRunID, err := state.ParseRunID(runID)
	if err != nil {
		t.Fatalf("parse run id: %v", err)
	}
	runDir := state.RunDir(statePath, parsedRunID.Project, parsedRunID.Pass, parsedRunID.Date, parsedRunID.TimeSeq)
	for _, name := range []string{"prompt.md", "output.json", "findings.json", "report.md", "git-head.txt"} {
		if _, err := os.Stat(filepath.Join(runDir, name)); err != nil {
			t.Fatalf("missing run artifact %s: %v", name, err)
		}
	}
	if _, err := os.Stat(state.RunScratchDir(statePath, runID)); !os.IsNotExist(err) {
		t.Fatalf("scratch worktree still exists or stat failed: %v", err)
	}
	promptRaw := readString(t, filepath.Join(runDir, "prompt.md"))
	for _, want := range []string{
		"## BoK slice",
		"vocabulary/lens-vs-view",
		"sample/main.go",
		openFinding.ID,
		acceptedFinding.ID,
		"accepted note",
		"deferred note",
		"rejected note",
	} {
		if !strings.Contains(promptRaw, want) {
			t.Fatalf("prompt missing %q:\n%s", want, promptRaw)
		}
	}
	if !strings.Contains(readString(t, filepath.Join(runDir, "output.json")), "new view vocabulary drift") {
		t.Fatalf("output.json did not contain raw reviewer output")
	}
	openFindings, err := project.ListFindings(state.FindingFilter{OpenOnly: true})
	if err != nil {
		t.Fatalf("list open findings: %v", err)
	}
	if len(openFindings) != 2 {
		t.Fatalf("open findings = %d, want 2: %+v", len(openFindings), openFindings)
	}
	updatedOpen, err := project.GetFinding(openFinding.ID)
	if err != nil {
		t.Fatalf("get open finding: %v", err)
	}
	if updatedOpen.LastRunID != runID {
		t.Fatalf("reobserved last run = %q, want %q", updatedOpen.LastRunID, runID)
	}
	backlog := readString(t, state.BacklogPath(statePath, "testproj"))
	if strings.Contains(backlog, acceptedFinding.ID) {
		t.Fatalf("accepted finding stayed in backlog:\n%s", backlog)
	}
	if !strings.Contains(backlog, "new view vocabulary drift") {
		t.Fatalf("fresh finding missing from backlog:\n%s", backlog)
	}
	events := readString(t, state.DispositionsPath(statePath, "testproj"))
	if !strings.Contains(events, state.EventFindingReobserved) || !strings.Contains(events, state.EventFindingCreated) {
		t.Fatalf("expected reobserved and created events:\n%s", events)
	}
}

func TestPassRunEmptyRecentWritesNoopArtifacts(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "repo")
	initPassTestRepo(t, repoPath)
	writeTestFile(t, filepath.Join(repoPath, "sample", "main.go"), "package sample\n")
	gitPass(t, repoPath, "add", ".")
	gitPass(t, repoPath, "-c", "user.name=Otis Test", "-c", "user.email=otis@example.com", "-c", "commit.gpgsign=false", "commit", "-m", "initial")
	writePhase5Config(t, dir)
	writeTestFile(t, filepath.Join(dir, "bok", "vocabulary", "lens-vs-view.md"), "---\ntitle: lens\n---\n\nbody\n")

	cfg, err := config.Load(filepath.Join(dir, "global.yaml"))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	result, err := dispatcher.Run(context.Background(), dispatcher.RunRequest{
		Config:           cfg,
		ProjectName:      "testproj",
		PassName:         "recent-empty",
		ReviewerOverride: "dummy",
		Now:              time.Date(2099, 1, 1, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("run recent-empty: %v", err)
	}
	if !result.Noop {
		t.Fatalf("expected noop result, got runID=%q runDir=%q findings=%d", result.RunID, result.RunDir, len(result.Findings))
	}
	parsed, err := state.ParseRunID(result.RunID)
	if err != nil {
		t.Fatalf("parse run: %v", err)
	}
	runDir := state.RunDir(filepath.Join(dir, "state"), parsed.Project, parsed.Pass, parsed.Date, parsed.TimeSeq)
	if !strings.Contains(readString(t, filepath.Join(runDir, "prompt.md")), "no commits in window") {
		t.Fatalf("no-op prompt missing marker")
	}
	if strings.TrimSpace(readString(t, filepath.Join(runDir, "findings.json"))) != "[]" {
		t.Fatalf("no-op findings should be empty")
	}
	if _, err := os.Stat(state.RunScratchDir(filepath.Join(dir, "state"), result.RunID)); !os.IsNotExist(err) {
		t.Fatalf("scratch worktree still exists or stat failed: %v", err)
	}
}

func writePhase5Config(t *testing.T, dir string) {
	t.Helper()
	writeTestFile(t, filepath.Join(dir, "bok", "standard.yaml"), `
passes:
  - name: vocabulary-sweep
    description: vocabulary sweep
    scope:
      project:
        type: paths
        paths: ["sample/"]
      bok:
        include: [vocabulary/]
    reviewer: { kind: codex }
    cadence: 24h
    top_findings: 5
  - name: recent-empty
    description: recent empty
    scope:
      project:
        type: recent
        window: 1h
      bok:
        include: [vocabulary/]
    reviewer: { kind: codex }
    cadence: 24h
    top_findings: 5
`)
	writeTestFile(t, filepath.Join(dir, "bok", "projects", "testproj", "otis.yaml"), `
include_configs: [standard]
project:
  name: testproj
  description: phase five test project
  primary_language: go
  top_findings: 5
`)
	writeTestFile(t, filepath.Join(dir, "global.yaml"), `
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
}

func seedPassFinding(t *testing.T, project *state.Project, title string, disposition string) *state.Finding {
	t.Helper()
	finding, err := project.CreateFinding(state.CreateFindingRequest{
		Pass:         "vocabulary-sweep",
		Reviewer:     "codex",
		RunID:        "testproj/vocabulary-sweep/2026-05-15/010203Z-001",
		Severity:     "medium",
		Title:        title,
		Location:     state.Location{File: "sample/main.go", Lines: "3-3"},
		BokRefs:      []string{"vocabulary/lens-vs-view"},
		Description:  title + " description",
		SuggestedFix: "fix " + disposition,
	})
	if err != nil {
		t.Fatalf("create finding: %v", err)
	}
	return finding
}

func initPassTestRepo(t *testing.T, repoPath string) {
	t.Helper()
	if err := os.MkdirAll(repoPath, 0o700); err != nil {
		t.Fatal(err)
	}
	gitPass(t, repoPath, "init")
}

func gitPass(t *testing.T, repo string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, string(out))
	}
}

func readString(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(raw)
}
