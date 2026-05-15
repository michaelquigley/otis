package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/michaelquigley/otis/internal/config"
	"github.com/michaelquigley/otis/internal/dispatcher"
	"github.com/michaelquigley/otis/internal/prompt"
	"github.com/michaelquigley/otis/internal/state"
)

func TestDemoSmokeDummyReviewerAcceptThenQuietPass(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "testproj")
	initPassTestRepo(t, repoPath)
	writeTestFile(t, filepath.Join(repoPath, "sample", "main.go"), "package sample\n\nfunc View(value string) string { return \"view:\" + value }\n")
	gitPass(t, repoPath, "add", ".")
	gitPass(t, repoPath, "-c", "user.name=Otis Test", "-c", "user.email=otis@example.com", "-c", "commit.gpgsign=false", "commit", "-m", "seed demo")

	writeDemoSmokeConfig(t, dir)
	dummyOutputPath := filepath.Join(dir, "dummy-output.json")
	if err := dispatcher.WriteDummyOutput(dummyOutputPath, []prompt.ReviewerFinding{
		{
			Severity: "high",
			Title:    "view vocabulary drifts from lens convention",
			Location: prompt.ReviewerLocation{
				File:  "sample/main.go",
				Lines: "3-5",
			},
			BokRefs:      []string{"vocabulary/lens-vs-view"},
			Description:  "The demo code exposes View even though the BoK prefers lens.",
			SuggestedFix: "Rename View to Lens.",
		},
	}); err != nil {
		t.Fatalf("write first dummy output: %v", err)
	}

	cfg, err := config.Load(filepath.Join(dir, "global.yaml"))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	store, err := state.NewStore(filepath.Join(dir, "state"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	first, err := dispatcher.Run(context.Background(), dispatcher.RunRequest{
		Config:      cfg,
		Store:       store,
		ProjectName: "testproj",
		PassName:    "vocabulary-sweep",
	})
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if len(first.Findings) != 1 {
		t.Fatalf("first findings = %d, want 1", len(first.Findings))
	}

	project := store.Project("testproj")
	finding := first.Findings[0]
	if _, err := project.ChangeDisposition(finding.ID, state.DispositionAccepted, "human", "accepted demo finding"); err != nil {
		t.Fatalf("accept finding: %v", err)
	}
	if err := dispatcher.WriteDummyOutput(dummyOutputPath, nil); err != nil {
		t.Fatalf("write quiet dummy output: %v", err)
	}

	second, err := dispatcher.Run(context.Background(), dispatcher.RunRequest{
		Config:      cfg,
		Store:       store,
		ProjectName: "testproj",
		PassName:    "vocabulary-sweep",
	})
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if len(second.Findings) != 0 {
		t.Fatalf("second findings = %d, want 0", len(second.Findings))
	}
	openFindings, err := project.ListFindings(state.FindingFilter{OpenOnly: true})
	if err != nil {
		t.Fatalf("list open findings: %v", err)
	}
	if len(openFindings) != 0 {
		t.Fatalf("open findings = %d, want 0", len(openFindings))
	}

	runID, err := state.ParseRunID(second.RunID)
	if err != nil {
		t.Fatalf("parse second run id: %v", err)
	}
	promptPath := filepath.Join(state.RunDir(store.Root(), runID.Project, runID.Pass, runID.Date, runID.TimeSeq), "prompt.md")
	promptRaw := readString(t, promptPath)
	for _, want := range []string{finding.ID, finding.Title, "accepted demo finding"} {
		if !strings.Contains(promptRaw, want) {
			t.Fatalf("second prompt missing %q:\n%s", want, promptRaw)
		}
	}
	backlog := readString(t, state.BacklogPath(store.Root(), "testproj"))
	if strings.Contains(backlog, finding.ID) {
		t.Fatalf("accepted finding stayed in backlog:\n%s", backlog)
	}
}

func writeDemoSmokeConfig(t *testing.T, dir string) {
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
    reviewer:
      kind: dummy
    cadence: 24h
    top_findings: 3
`)
	writeTestFile(t, filepath.Join(dir, "bok", "projects", "testproj", "otis.yaml"), `
include_configs: [standard]
project:
  name: testproj
  description: demo smoke project
  primary_language: go
  top_findings: 3
`)
	writeTestFile(t, filepath.Join(dir, "bok", "vocabulary", "lens-vs-view.md"), `
---
title: lens vs view
tags: [vocabulary]
---

# lens vs view

Prefer lens for perspectival surfaces.
`)
	writeTestFile(t, filepath.Join(dir, "global.yaml"), `
bok:
  path: ./bok
storage:
  state_dir: ./state
reviewers:
  dummy:
    concurrency_cap: 1
    window: anytime
    output_path: ./dummy-output.json
windows:
  anytime:
    hours: "00:00-24:00"
global_concurrency_cap: 1
projects:
  - name: testproj
    path: ./testproj
`)
}
