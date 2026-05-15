package scheduler

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/michaelquigley/otis/internal/config"
	"github.com/michaelquigley/otis/internal/state"
)

func TestDueHonorsCadenceAndWindows(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "repo")
	if err := os.MkdirAll(repoPath, 0o700); err != nil {
		t.Fatal(err)
	}
	store, err := state.NewStore(filepath.Join(dir, "state"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	cfg := schedulerTestConfig(repoPath, store.Root())
	s, err := New(cfg, Options{Store: store})
	if err != nil {
		t.Fatalf("new scheduler: %v", err)
	}

	start := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	due, err := s.Due(start)
	if err != nil {
		t.Fatalf("due: %v", err)
	}
	if got := dueNames(due); got != "fast,slow" {
		t.Fatalf("initial due = %q, want fast,slow", got)
	}
	project := store.Project("testproj")
	for _, pass := range []string{"fast", "slow"} {
		if err := project.RecordLastRun(pass, "testproj/"+pass+"/2026-05-15/120000Z-001", start); err != nil {
			t.Fatalf("record last run %s: %v", pass, err)
		}
	}

	due, err = s.Due(start.Add(2 * time.Minute))
	if err != nil {
		t.Fatalf("due after two minutes: %v", err)
	}
	if got := dueNames(due); got != "fast" {
		t.Fatalf("two-minute due = %q, want fast", got)
	}
	due, err = s.Due(start.Add(6 * time.Minute))
	if err != nil {
		t.Fatalf("due after six minutes: %v", err)
	}
	if got := dueNames(due); got != "fast,slow" {
		t.Fatalf("six-minute due = %q, want fast,slow", got)
	}
}

func schedulerTestConfig(repoPath string, statePath string) *config.ResolvedConfig {
	return &config.ResolvedConfig{
		Global: &config.GlobalConfig{
			Storage: &config.StorageConfig{StateDir: statePath},
			Reviewers: map[string]*config.ReviewerConfig{
				"dummy": {
					ConcurrencyCap: 2,
					Window:         "anytime",
				},
			},
			Windows: map[string]*config.WindowConfig{
				"anytime": {Hours: "00:00-24:00"},
			},
			GlobalConcurrencyCap: 2,
		},
		Projects: map[string]*config.ResolvedProject{
			"testproj": {
				RepoPath: repoPath,
				Project:  &config.ProjectBlock{Name: "testproj"},
				Passes: []*config.Pass{
					{Name: "fast", Cadence: "1m", Reviewer: &config.PassReviewerConfig{Kind: "dummy"}},
					{Name: "slow", Cadence: "5m", Reviewer: &config.PassReviewerConfig{Kind: "dummy"}},
				},
			},
		},
	}
}

func dueNames(due []DuePass) string {
	out := ""
	for i, item := range due {
		if i > 0 {
			out += ","
		}
		out += item.PassName
	}
	return out
}
