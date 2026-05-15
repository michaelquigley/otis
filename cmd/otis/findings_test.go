package main

import (
	"bytes"
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/michaelquigley/otis/internal/api"
	"github.com/michaelquigley/otis/internal/config"
	"github.com/michaelquigley/otis/internal/dispatcher"
	"github.com/michaelquigley/otis/internal/state"
)

func TestWorkstationClientCommandsUseSupervisorAPI(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "repo")
	initPassTestRepo(t, repoPath)
	writeTestFile(t, filepath.Join(repoPath, "README.md"), "test\n")
	gitPass(t, repoPath, "add", ".")
	gitPass(t, repoPath, "-c", "user.name=Otis Test", "-c", "user.email=otis@example.com", "-c", "commit.gpgsign=false", "commit", "-m", "initial")

	statePath := filepath.Join(dir, "state")
	store, err := state.NewStore(statePath)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	project := store.Project("testproj")
	finding, err := project.CreateFinding(state.CreateFindingRequest{
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
		t.Fatalf("create finding: %v", err)
	}
	parsedReportID, err := state.ParseRunID("testproj/vocabulary-sweep/2026-05-15/010203Z-001")
	if err != nil {
		t.Fatalf("parse report id: %v", err)
	}
	reportPath := filepath.Join(state.RunDir(statePath, parsedReportID.Project, parsedReportID.Pass, parsedReportID.Date, parsedReportID.TimeSeq), "report.md")
	writeTestFile(t, reportPath, "# Stored Report\n")

	token, err := api.IssueToken(store.Root(), "workstation")
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	cfg := workstationAPIConfig(repoPath, statePath)
	started := make(chan dispatcher.RunRequest, 1)
	dispatch, err := dispatcher.New(cfg, dispatcher.Options{
		Store: store,
		Runner: dispatcher.RunnerFunc(func(ctx context.Context, req dispatcher.RunRequest) (dispatcher.RunResult, error) {
			started <- req
			return dispatcher.RunResult{RunID: req.RunID}, nil
		}),
	})
	if err != nil {
		t.Fatalf("dispatcher: %v", err)
	}
	server := httptest.NewServer(api.NewServer(cfg, store, dispatch).Handler())
	defer server.Close()

	clientConfigPath := filepath.Join(dir, "client.yaml")
	writeTestFile(t, clientConfigPath, "url: "+server.URL+"\ntoken: "+token+"\n")
	clientArgs := []string{"--client-config", clientConfigPath}

	projectsOut := runCommand(t, append(clientArgs, "projects", "list")...)
	if !strings.Contains(projectsOut, "testproj") {
		t.Fatalf("projects output missing project:\n%s", projectsOut)
	}

	passesOut := runCommand(t, append(clientArgs, "passes", "list", "--project", "testproj")...)
	if !strings.Contains(passesOut, "vocabulary-sweep") {
		t.Fatalf("passes output missing pass:\n%s", passesOut)
	}

	listOut := runCommand(t, append(clientArgs, "findings", "list", "--project", "testproj", "--open")...)
	if !strings.Contains(listOut, finding.ID) || !strings.Contains(listOut, "view should be lens") {
		t.Fatalf("open finding missing from list:\n%s", listOut)
	}

	showOut := runCommand(t, append(clientArgs, "findings", "show", finding.ID)...)
	for _, want := range []string{finding.ID, "severity: medium", "disposition: open", "The code uses view"} {
		if !strings.Contains(showOut, want) {
			t.Fatalf("show output missing %q:\n%s", want, showOut)
		}
	}

	reportOut := runCommand(t, append(clientArgs, "report", "show", parsedReportID.String())...)
	if !strings.Contains(reportOut, "Stored Report") {
		t.Fatalf("report output missing stored report:\n%s", reportOut)
	}

	runOut := runCommand(t, append(clientArgs, "pass", "run", "testproj/vocabulary-sweep")...)
	if !strings.Contains(runOut, "state: queued") {
		t.Fatalf("run output = %s", runOut)
	}
	select {
	case req := <-started:
		if req.ProjectName != "testproj" || req.PassName != "vocabulary-sweep" || req.RunID == "" {
			t.Fatalf("run request = %+v", req)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for force-run")
	}

	acceptOut := runCommand(t, append(clientArgs, "accept", finding.ID, "--note", "handled")...)
	if !strings.Contains(acceptOut, state.DispositionAccepted) {
		t.Fatalf("accept output = %s", acceptOut)
	}
	updated, err := project.GetFinding(finding.ID)
	if err != nil {
		t.Fatalf("get updated finding: %v", err)
	}
	if updated.Disposition != state.DispositionAccepted {
		t.Fatalf("disposition = %q", updated.Disposition)
	}

	listOut = runCommand(t, append(clientArgs, "findings", "list", "--project", "testproj", "--open")...)
	if strings.Contains(listOut, finding.ID) {
		t.Fatalf("accepted finding appeared in open list:\n%s", listOut)
	}
}

func workstationAPIConfig(repoPath string, statePath string) *config.ResolvedConfig {
	return &config.ResolvedConfig{
		Global: &config.GlobalConfig{
			Storage: &config.StorageConfig{StateDir: statePath},
			API:     &config.APIConfig{Listen: "127.0.0.1:0", TLS: &config.TLSConfig{}},
			Reviewers: map[string]*config.ReviewerConfig{
				"codex": {ConcurrencyCap: 1, Window: "anytime"},
			},
			Windows: map[string]*config.WindowConfig{
				"anytime": {Hours: "00:00-24:00"},
			},
			GlobalConcurrencyCap: 1,
		},
		Projects: map[string]*config.ResolvedProject{
			"testproj": {
				RepoPath: repoPath,
				Project:  &config.ProjectBlock{Name: "testproj", Description: "test project", PrimaryLanguage: "go"},
				Passes: []*config.Pass{
					{Name: "vocabulary-sweep", Description: "vocab", Cadence: "1m", Reviewer: &config.PassReviewerConfig{Kind: "codex"}, TopFindings: 3},
				},
			},
		},
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
