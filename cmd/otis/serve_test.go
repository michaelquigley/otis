package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/michaelquigley/otis/internal/state"
)

func TestServeOnceRunsDuePassesAndRecordsLastRun(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "repo")
	initPassTestRepo(t, repoPath)
	writeTestFile(t, filepath.Join(repoPath, "sample", "main.go"), "package sample\n")
	gitPass(t, repoPath, "add", ".")
	gitPass(t, repoPath, "-c", "user.name=Otis Test", "-c", "user.email=otis@example.com", "-c", "commit.gpgsign=false", "commit", "-m", "initial")
	writeServeConfig(t, dir)
	writeTestFile(t, filepath.Join(dir, "bok", "vocabulary", "lens-vs-view.md"), "---\ntitle: lens\n---\n\nbody\n")

	globalPath := filepath.Join(dir, "global.yaml")
	out := runCommand(t, "--config", globalPath, "serve", "--once")
	if !strings.Contains(out, "enqueued: 2") {
		t.Fatalf("serve output = %s", out)
	}
	store, err := state.NewStore(filepath.Join(dir, "state"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	records, err := store.Project("testproj").LastRuns()
	if err != nil {
		t.Fatalf("last runs: %v", err)
	}
	if records["fast"].RunID == "" || records["slow"].RunID == "" {
		t.Fatalf("last runs missing: %+v", records)
	}
	out = runCommand(t, "--config", globalPath, "serve", "--once")
	if !strings.Contains(out, "enqueued: 0") {
		t.Fatalf("second serve output = %s", out)
	}
}

func writeServeConfig(t *testing.T, dir string) {
	t.Helper()
	writeTestFile(t, filepath.Join(dir, "bok", "standard.yaml"), `
passes:
  - name: fast
    description: fast pass
    scope:
      project:
        type: paths
        paths: ["sample/"]
      bok:
        include: [vocabulary/]
    reviewer: { kind: dummy }
    cadence: 1m
    top_findings: 5
  - name: slow
    description: slow pass
    scope:
      project:
        type: paths
        paths: ["sample/"]
      bok:
        include: [vocabulary/]
    reviewer: { kind: dummy }
    cadence: 5m
    top_findings: 5
`)
	writeTestFile(t, filepath.Join(dir, "bok", "projects", "testproj", "otis.yaml"), `
include_configs: [standard]
project:
  name: testproj
  description: serve test project
  primary_language: go
  top_findings: 5
`)
	writeTestFile(t, filepath.Join(dir, "global.yaml"), `
bok:
  path: ./bok
storage:
  state_dir: ./state
reviewers:
  dummy:
    binary: dummy
    concurrency_cap: 2
    window: anytime
windows:
  anytime:
    hours: "00:00-24:00"
global_concurrency_cap: 2
projects:
  - name: testproj
    path: ./repo
`)
}
