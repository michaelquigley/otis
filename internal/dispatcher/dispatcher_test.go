package dispatcher

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/michaelquigley/otis/internal/config"
	"github.com/michaelquigley/otis/internal/state"
)

func TestDispatcherRejectsOverlappingForceRunAndRecordsLastRun(t *testing.T) {
	dir := t.TempDir()
	repoPath := initDispatcherRepo(t, filepath.Join(dir, "repo"))
	store, err := state.NewStore(filepath.Join(dir, "state"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	runner := newBlockingRunner()
	dispatch, err := New(dispatcherTestConfig(repoPath, store.Root(), 2), Options{
		Store:  store,
		Runner: runner,
		Now: func() time.Time {
			return time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
		},
	})
	if err != nil {
		t.Fatalf("new dispatcher: %v", err)
	}

	handle, err := dispatch.Enqueue(context.Background(), EnqueueRequest{
		ProjectName: "testproj",
		PassName:    "one",
		Source:      SourceForce,
	})
	if err != nil {
		t.Fatalf("enqueue first: %v", err)
	}
	started := runner.waitStarted(t)
	if started.RunID == "" {
		t.Fatal("runner started without run id")
	}

	_, err = dispatch.Enqueue(context.Background(), EnqueueRequest{
		ProjectName: "testproj",
		PassName:    "one",
		Source:      SourceForce,
	})
	var inFlight *ErrInFlight
	if !errors.As(err, &inFlight) {
		t.Fatalf("overlapping force error = %v, want ErrInFlight", err)
	}
	if inFlight.State != InFlightRunning || inFlight.RunID != started.RunID {
		t.Fatalf("in-flight = %+v, want running %s", inFlight, started.RunID)
	}
	skipped, err := dispatch.Enqueue(context.Background(), EnqueueRequest{
		ProjectName: "testproj",
		PassName:    "one",
		Source:      SourceScheduled,
	})
	if err != nil {
		t.Fatalf("scheduled overlap: %v", err)
	}
	if skipped != nil {
		t.Fatal("scheduled overlap should skip silently")
	}

	runner.releaseOne()
	if _, err := handle.Wait(context.Background()); err != nil {
		t.Fatalf("wait first: %v", err)
	}
	record, ok, err := store.Project("testproj").LastRun("one")
	if err != nil {
		t.Fatalf("last run: %v", err)
	}
	if !ok || record.RunID != started.RunID {
		t.Fatalf("last run = %+v ok=%v, want %s", record, ok, started.RunID)
	}
}

func TestDispatcherGlobalCapGatesParallelRuns(t *testing.T) {
	dir := t.TempDir()
	repoPath := initDispatcherRepo(t, filepath.Join(dir, "repo"))
	store, err := state.NewStore(filepath.Join(dir, "state"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	runner := newBlockingRunner()
	dispatch, err := New(dispatcherTestConfig(repoPath, store.Root(), 1), Options{
		Store:  store,
		Runner: runner,
		Now: func() time.Time {
			return time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
		},
	})
	if err != nil {
		t.Fatalf("new dispatcher: %v", err)
	}

	first, err := dispatch.Enqueue(context.Background(), EnqueueRequest{ProjectName: "testproj", PassName: "one", Source: SourceForce})
	if err != nil {
		t.Fatalf("enqueue one: %v", err)
	}
	firstReq := runner.waitStarted(t)
	second, err := dispatch.Enqueue(context.Background(), EnqueueRequest{ProjectName: "testproj", PassName: "two", Source: SourceForce})
	if err != nil {
		t.Fatalf("enqueue two: %v", err)
	}
	runner.assertNoStart(t, 50*time.Millisecond)

	runner.releaseOne()
	if _, err := first.Wait(context.Background()); err != nil {
		t.Fatalf("wait first: %v", err)
	}
	secondReq := runner.waitStarted(t)
	if firstReq.PassName == secondReq.PassName {
		t.Fatalf("expected distinct passes, got %s twice", firstReq.PassName)
	}
	runner.releaseOne()
	if _, err := second.Wait(context.Background()); err != nil {
		t.Fatalf("wait second: %v", err)
	}
}

type blockingRunner struct {
	started chan RunRequest
	release chan struct{}
}

func newBlockingRunner() *blockingRunner {
	return &blockingRunner{
		started: make(chan RunRequest, 8),
		release: make(chan struct{}, 8),
	}
}

func (r *blockingRunner) Run(ctx context.Context, req RunRequest) (RunResult, error) {
	select {
	case r.started <- req:
	case <-ctx.Done():
		return RunResult{}, ctx.Err()
	}
	select {
	case <-r.release:
		return RunResult{RunID: req.RunID}, nil
	case <-ctx.Done():
		return RunResult{}, ctx.Err()
	}
}

func (r *blockingRunner) waitStarted(t *testing.T) RunRequest {
	t.Helper()
	select {
	case req := <-r.started:
		return req
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for runner start")
		return RunRequest{}
	}
}

func (r *blockingRunner) assertNoStart(t *testing.T, duration time.Duration) {
	t.Helper()
	select {
	case req := <-r.started:
		t.Fatalf("unexpected runner start for %s/%s", req.ProjectName, req.PassName)
	case <-time.After(duration):
	}
}

func (r *blockingRunner) releaseOne() {
	r.release <- struct{}{}
}

func dispatcherTestConfig(repoPath string, statePath string, globalCap int) *config.ResolvedConfig {
	return &config.ResolvedConfig{
		Global: &config.GlobalConfig{
			Storage: &config.StorageConfig{StateDir: statePath},
			Reviewers: map[string]*config.ReviewerConfig{
				"dummy": {ConcurrencyCap: 2, Window: "anytime"},
			},
			Windows: map[string]*config.WindowConfig{
				"anytime": {Hours: "00:00-24:00"},
			},
			GlobalConcurrencyCap: globalCap,
		},
		Projects: map[string]*config.ResolvedProject{
			"testproj": {
				RepoPath: repoPath,
				Project:  &config.ProjectBlock{Name: "testproj"},
				Passes: []*config.Pass{
					{Name: "one", Cadence: "1m", Reviewer: &config.PassReviewerConfig{Kind: "dummy"}},
					{Name: "two", Cadence: "1m", Reviewer: &config.PassReviewerConfig{Kind: "dummy"}},
				},
			},
		},
	}
}

func initDispatcherRepo(t *testing.T, repoPath string) string {
	t.Helper()
	if err := os.MkdirAll(repoPath, 0o700); err != nil {
		t.Fatal(err)
	}
	gitDispatcher(t, repoPath, "init")
	if err := os.WriteFile(filepath.Join(repoPath, "README.md"), []byte("test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitDispatcher(t, repoPath, "add", ".")
	gitDispatcher(t, repoPath, "-c", "user.name=Otis Test", "-c", "user.email=otis@example.com", "-c", "commit.gpgsign=false", "commit", "-m", "initial")
	return repoPath
}

func gitDispatcher(t *testing.T, repo string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, string(out))
	}
}
