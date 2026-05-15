package prompt_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/michaelquigley/otis/internal/bok"
	"github.com/michaelquigley/otis/internal/prompt"
	"github.com/michaelquigley/otis/internal/reviewer"
	"github.com/michaelquigley/otis/internal/reviewer/dummy"
	"github.com/michaelquigley/otis/internal/state"
)

func TestPromptAssemblyAndDummyReviewer(t *testing.T) {
	ctx := context.Background()
	repo := initGitRepo(t)
	writePromptTestFile(t, repo, "sample/small.go", "package sample\n\nfunc Lens() string { return \"lens\" }\n")
	writePromptTestFile(t, repo, "sample/large.txt", strings.Repeat("x", 64))
	git(t, repo, "add", ".")
	git(t, repo, "-c", "user.name=Otis Test", "-c", "user.email=otis@example.com", "-c", "commit.gpgsign=false", "commit", "-m", "initial")

	bokRoot := t.TempDir()
	writePromptTestFile(t, bokRoot, "vocabulary/lens-vs-view.md", `
---
title: lens vs view
tags: [vocabulary, naming]
created: 2026-05-13
---

# lens vs view

Prefer lens when naming perspectival surfaces.
`)
	writePromptTestFile(t, bokRoot, "projects/testproj/conventions.md", `
---
title: testproj conventions
tags: [project]
---

# testproj conventions

Keep the example project small.
`)
	entries, err := bok.Resolve(bokRoot, []string{"vocabulary/", "projects/testproj/"}, "testproj")
	if err != nil {
		t.Fatalf("resolve bok: %v", err)
	}

	store, err := state.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	project := store.Project("testproj")
	open := createPromptFinding(t, project, "open finding", "open")
	accepted := createPromptFinding(t, project, "accepted finding", "accepted")
	deferred := createPromptFinding(t, project, "deferred finding", "deferred")
	rejected := createPromptFinding(t, project, "rejected finding", "rejected")
	if _, err := project.ChangeDisposition(accepted.ID, state.DispositionAccepted, "michael", "accepted note"); err != nil {
		t.Fatalf("accepted: %v", err)
	}
	if _, err := project.ChangeDisposition(deferred.ID, state.DispositionDeferred, "michael", "deferred note"); err != nil {
		t.Fatalf("deferred: %v", err)
	}
	if _, err := project.ChangeDisposition(rejected.ID, state.DispositionRejected, "michael", "rejected note"); err != nil {
		t.Fatalf("rejected: %v", err)
	}
	contexts, err := project.FindingContexts("vocabulary-sweep")
	if err != nil {
		t.Fatalf("finding contexts: %v", err)
	}

	scope, err := prompt.BuildScopeContent(ctx, prompt.ScopePaths, []string{"sample/small.go", "sample/large.txt"}, "", repo, prompt.ScopeOptions{
		PerFileBytes:    16,
		TotalScopeBytes: 48,
	})
	if err != nil {
		t.Fatalf("scope content: %v", err)
	}
	req := prompt.Request{
		Project: prompt.ProjectContext{
			Name:            "testproj",
			Description:     "small test project",
			PrimaryLanguage: "go",
		},
		Pass: prompt.PassContext{
			Name:        "vocabulary-sweep",
			Description: "naming consistency",
		},
		TopFindings:   3,
		BokEntries:    entries,
		Scope:         scope,
		PriorFindings: prompt.PriorFindingsFromState(contexts),
	}
	built := prompt.Build(req)
	for _, want := range []string{
		"## Role and goal",
		"## BoK slice",
		"vocabulary/lens-vs-view",
		"## Project context",
		"Primary language: go",
		"## Scope content",
		"sample/small.go",
		"truncated:",
		"## Prior findings context",
		open.ID,
		"accepted note",
		"deferred note",
		"rejected note",
		"## Output schema and budget",
		"Return at most 3 entries",
	} {
		if !strings.Contains(built.Prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, built.Prompt)
		}
	}

	raw := json.RawMessage(`{
		"findings": [
			{
				"id": "` + open.ID + `",
				"severity": "medium",
				"title": "view should be lens",
				"location": {"file": "sample/small.go", "lines": "3-3"},
				"bok_refs": ["vocabulary/lens-vs-view"],
				"description": "The sample uses view wording where lens is clearer.",
				"suggested_fix": "Rename the surface to lens."
			}
		]
	}`)
	outputPath := filepath.Join(t.TempDir(), "reviewer-output.json")
	if err := os.WriteFile(outputPath, raw, 0o600); err != nil {
		t.Fatalf("write canned output: %v", err)
	}
	dummyReviewer := dummy.New(dummy.Options{OutputPath: outputPath, ArtifactsDir: filepath.Join(repo, ".artifacts")})
	result, err := dummyReviewer.Review(ctx, reviewerRequest(built, repo))
	if err != nil {
		t.Fatalf("dummy review: %v", err)
	}
	if len(result.Findings) != 1 || result.Findings[0].ID != open.ID {
		t.Fatalf("unexpected reviewer findings: %+v", result.Findings)
	}
}

func TestReviewerOutputValidationRejectsBadOutput(t *testing.T) {
	schema := prompt.ReviewerOutputSchema(2)
	tests := []struct {
		name string
		raw  json.RawMessage
		want string
	}{
		{
			name: "missing required field",
			raw: json.RawMessage(`{
				"findings": [
					{
						"severity": "medium",
						"title": "missing suggested fix",
						"location": {"file": "sample.go", "lines": "1-2"},
						"bok_refs": [],
						"description": "bad"
					}
				]
			}`),
			want: "reviewer output schema violation",
		},
		{
			name: "invalid severity",
			raw: json.RawMessage(`{
				"findings": [
					{
						"severity": "critical",
						"title": "bad severity",
						"location": {"file": "sample.go", "lines": "1-2"},
						"bok_refs": [],
						"description": "bad",
						"suggested_fix": "fix it"
					}
				]
			}`),
			want: "reviewer output schema violation",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := prompt.ParseReviewerOutput(test.raw, schema)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected %q in error, got %v", test.want, err)
			}
		})
	}
}

func TestBuildScopeContentRecentUsesSuppliedBase(t *testing.T) {
	ctx := context.Background()
	repo := initGitRepo(t)
	writePromptTestFile(t, repo, "sample/recent.go", "package sample\n\nfunc Name() string { return \"old\" }\n")
	git(t, repo, "add", ".")
	git(t, repo, "-c", "user.name=Otis Test", "-c", "user.email=otis@example.com", "-c", "commit.gpgsign=false", "commit", "-m", "initial")
	base := gitOutput(t, repo, "rev-parse", "HEAD")
	writePromptTestFile(t, repo, "sample/recent.go", "package sample\n\nfunc Name() string { return \"new\" }\n")
	git(t, repo, "add", ".")
	git(t, repo, "-c", "user.name=Otis Test", "-c", "user.email=otis@example.com", "-c", "commit.gpgsign=false", "commit", "-m", "update")

	scope, err := prompt.BuildScopeContent(ctx, prompt.ScopeRecent, []string{"sample/recent.go"}, base, repo, prompt.ScopeOptions{})
	if err != nil {
		t.Fatalf("recent scope: %v", err)
	}
	if len(scope.Inline) != 1 || !scope.Inline[0].Diff {
		t.Fatalf("unexpected inline content: %+v", scope.Inline)
	}
	if !strings.Contains(scope.Inline[0].Content, `"new"`) {
		t.Fatalf("diff did not include new content:\n%s", scope.Inline[0].Content)
	}
}

func reviewerRequest(built prompt.BuildResult, repo string) reviewer.Request {
	return reviewer.Request{
		Prompt:     built.Prompt,
		Schema:     built.Schema,
		WorkingDir: repo,
	}
}

func createPromptFinding(t *testing.T, project *state.Project, title string, disposition string) *state.Finding {
	t.Helper()
	finding, err := project.CreateFinding(state.CreateFindingRequest{
		Pass:         "vocabulary-sweep",
		Reviewer:     "codex",
		RunID:        "testproj/vocabulary-sweep/2026-05-15/010203Z-001",
		Severity:     "medium",
		Title:        title,
		Location:     state.Location{File: "sample/small.go", Lines: "1-3"},
		BokRefs:      []string{"vocabulary/lens-vs-view"},
		Description:  title + " description",
		SuggestedFix: "fix " + disposition,
	})
	if err != nil {
		t.Fatalf("create finding: %v", err)
	}
	return finding
}

func initGitRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	git(t, repo, "init")
	return repo
}

func writePromptTestFile(t *testing.T, root string, relpath string, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relpath))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.TrimLeft(body, "\n")), 0o600); err != nil {
		t.Fatal(err)
	}
}

func git(t *testing.T, repo string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, string(out))
	}
}

func gitOutput(t *testing.T, repo string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, string(out))
	}
	return strings.TrimSpace(string(out))
}
