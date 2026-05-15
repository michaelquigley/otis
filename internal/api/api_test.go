package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/michaelquigley/df/dd"
	"github.com/michaelquigley/otis/internal/config"
	"github.com/michaelquigley/otis/internal/dispatcher"
	"github.com/michaelquigley/otis/internal/state"
)

func TestAPIAuthAndFindingDisposition(t *testing.T) {
	dir := t.TempDir()
	repoPath := initAPIRepo(t, filepath.Join(dir, "repo"))
	store, err := state.NewStore(filepath.Join(dir, "state"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	project := store.Project("testproj")
	finding, err := project.CreateFinding(state.CreateFindingRequest{
		Pass:         "vocabulary-sweep",
		Reviewer:     "dummy",
		RunID:        "testproj/vocabulary-sweep/2026-05-15/010203Z-001",
		Severity:     "medium",
		Title:        "view should be lens",
		Location:     state.Location{File: "sample/main.go", Lines: "1-1"},
		BokRefs:      []string{"vocabulary/lens-vs-view"},
		Description:  "description",
		SuggestedFix: "fix",
	})
	if err != nil {
		t.Fatalf("create finding: %v", err)
	}
	token, err := IssueToken(store.Root(), "michael")
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	dispatch, err := dispatcher.New(apiTestConfig(repoPath, store.Root()), dispatcher.Options{Store: store, Runner: dispatcher.RunnerFunc(func(ctx context.Context, req dispatcher.RunRequest) (dispatcher.RunResult, error) {
		return dispatcher.RunResult{RunID: req.RunID}, nil
	})})
	if err != nil {
		t.Fatalf("dispatcher: %v", err)
	}
	handler := NewServer(apiTestConfig(repoPath, store.Root()), store, dispatch).Handler()

	unauthorized := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	handler.ServeHTTP(unauthorized, req)
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.Code)
	}

	projects := serveAPI(t, handler, token, http.MethodGet, "/api/v1/projects", nil)
	if projects.Code != http.StatusOK || !strings.Contains(projects.Body.String(), "testproj") {
		t.Fatalf("projects response = %d %s", projects.Code, projects.Body.String())
	}
	list := serveAPI(t, handler, token, http.MethodGet, "/api/v1/projects/testproj/findings?disposition=open", nil)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), finding.ID) {
		t.Fatalf("findings response = %d %s", list.Code, list.Body.String())
	}

	body, err := dd.UnbindJSON(dispositionRequest{Disposition: state.DispositionAccepted, Note: "accepted via api"})
	if err != nil {
		t.Fatalf("body: %v", err)
	}
	posted := serveAPI(t, handler, token, http.MethodPost, "/api/v1/projects/testproj/findings/vocabulary-sweep/0001/disposition", bytes.NewReader(body))
	if posted.Code != http.StatusOK {
		t.Fatalf("disposition response = %d %s", posted.Code, posted.Body.String())
	}
	updated, err := project.GetFinding(finding.ID)
	if err != nil {
		t.Fatalf("get updated finding: %v", err)
	}
	if updated.Disposition != state.DispositionAccepted {
		t.Fatalf("disposition = %q", updated.Disposition)
	}
}

func TestAPIRunEndpointEnqueuesForceRunAndReportEndpointServesStoredReport(t *testing.T) {
	dir := t.TempDir()
	repoPath := initAPIRepo(t, filepath.Join(dir, "repo"))
	store, err := state.NewStore(filepath.Join(dir, "state"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	token, err := IssueToken(store.Root(), "runner")
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	started := make(chan dispatcher.RunRequest, 1)
	cfg := apiTestConfig(repoPath, store.Root())
	dispatch, err := dispatcher.New(cfg, dispatcher.Options{Store: store, Runner: dispatcher.RunnerFunc(func(ctx context.Context, req dispatcher.RunRequest) (dispatcher.RunResult, error) {
		started <- req
		return dispatcher.RunResult{RunID: req.RunID, RunDir: state.RunDir(store.Root(), req.ProjectName, req.PassName, "2026-05-15", "010203Z-001")}, nil
	})})
	if err != nil {
		t.Fatalf("dispatcher: %v", err)
	}
	handler := NewServer(cfg, store, dispatch).Handler()

	resp := serveAPI(t, handler, token, http.MethodPost, "/api/v1/projects/testproj/passes/vocabulary-sweep/run", nil)
	if resp.Code != http.StatusAccepted {
		t.Fatalf("run response = %d %s", resp.Code, resp.Body.String())
	}
	select {
	case req := <-started:
		if req.RunID == "" {
			t.Fatal("run request missing run id")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for run")
	}

	runID, err := store.Project("testproj").AllocateRunID("vocabulary-sweep", time.Date(2026, 5, 15, 1, 2, 3, 0, time.UTC))
	if err != nil {
		t.Fatalf("allocate run: %v", err)
	}
	parsed, err := state.ParseRunID(runID)
	if err != nil {
		t.Fatalf("parse run id: %v", err)
	}
	reportPath := runReportPath(store.Root(), parsed)
	if err := os.MkdirAll(filepath.Dir(reportPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(reportPath, []byte("# Stored Report\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	report := serveAPI(t, handler, token, http.MethodGet, "/api/v1/projects/testproj/runs/vocabulary-sweep/2026-05-15/010203Z-001/report", nil)
	if report.Code != http.StatusOK || !strings.Contains(report.Body.String(), "Stored Report") {
		t.Fatalf("report response = %d %s", report.Code, report.Body.String())
	}
}

func serveAPI(t *testing.T, handler http.Handler, token string, method string, path string, body *bytes.Reader) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		reader = body
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func apiTestConfig(repoPath string, statePath string) *config.ResolvedConfig {
	return &config.ResolvedConfig{
		Global: &config.GlobalConfig{
			Storage: &config.StorageConfig{StateDir: statePath},
			API:     &config.APIConfig{Listen: "127.0.0.1:0", TLS: &config.TLSConfig{}},
			Reviewers: map[string]*config.ReviewerConfig{
				"dummy": {ConcurrencyCap: 1, Window: "anytime"},
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
					{Name: "vocabulary-sweep", Description: "vocab", Cadence: "1m", Reviewer: &config.PassReviewerConfig{Kind: "dummy"}, TopFindings: 3},
				},
			},
		},
	}
}

func initAPIRepo(t *testing.T, repoPath string) string {
	t.Helper()
	if err := os.MkdirAll(repoPath, 0o700); err != nil {
		t.Fatal(err)
	}
	gitAPI(t, repoPath, "init")
	if err := os.WriteFile(filepath.Join(repoPath, "README.md"), []byte("test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitAPI(t, repoPath, "add", ".")
	gitAPI(t, repoPath, "-c", "user.name=Otis Test", "-c", "user.email=otis@example.com", "-c", "commit.gpgsign=false", "commit", "-m", "initial")
	return repoPath
}

func gitAPI(t *testing.T, repo string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, string(out))
	}
}
