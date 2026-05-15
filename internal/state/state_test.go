package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindingIDParsingAndFilename(t *testing.T) {
	id, err := ParseID("baab/vocabulary-sweep/0042")
	if err != nil {
		t.Fatalf("parse id: %v", err)
	}
	if id.Filename() != "baab--vocabulary-sweep--0042.json" {
		t.Fatalf("filename = %q", id.Filename())
	}
	parsed, err := ParseFilename(id.Filename())
	if err != nil {
		t.Fatalf("parse filename: %v", err)
	}
	if parsed.String() != id.String() {
		t.Fatalf("round trip = %q", parsed.String())
	}
}

func TestFindingLifecycleRoundTrip(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	project := store.Project("testproj")

	first, err := project.CreateFinding(CreateFindingRequest{
		Pass:     "vocabulary-sweep",
		Reviewer: "codex",
		RunID:    "testproj/vocabulary-sweep/2026-05-15/010203Z-001",
		Severity: "medium",
		Title:    "view should be lens",
		Location: Location{
			File:  "internal/hive/library.go",
			Lines: "42-78",
		},
		BokRefs:      []string{"vocabulary/lens-vs-view"},
		Description:  "The code uses view where lens matches the project vocabulary.",
		SuggestedFix: "Rename the perspectival surface to lens.",
	})
	if err != nil {
		t.Fatalf("create first finding: %v", err)
	}
	second, err := project.CreateFinding(CreateFindingRequest{
		Pass:     "vocabulary-sweep",
		Reviewer: "codex",
		RunID:    "testproj/vocabulary-sweep/2026-05-15/010203Z-001",
		Severity: "low",
		Title:    "library term overloaded",
		Location: Location{
			File:  "internal/hive/storage.go",
			Lines: "12-20",
		},
		BokRefs:      []string{"vocabulary/library-overloads"},
		Description:  "Library is carrying two meanings.",
		SuggestedFix: "Split storage vocabulary from hive vocabulary.",
	})
	if err != nil {
		t.Fatalf("create second finding: %v", err)
	}
	if first.ID != "testproj/vocabulary-sweep/0001" || second.ID != "testproj/vocabulary-sweep/0002" {
		t.Fatalf("unexpected ids: %s %s", first.ID, second.ID)
	}

	changed, err := project.ChangeDisposition(first.ID, DispositionAccepted, "michael", "will fix with hive refactor")
	if err != nil {
		t.Fatalf("change disposition: %v", err)
	}
	if changed.Disposition != DispositionAccepted {
		t.Fatalf("disposition = %q", changed.Disposition)
	}
	reobserved, err := project.ReobserveFinding(second.ID, "testproj/vocabulary-sweep/2026-05-15/020304Z-001")
	if err != nil {
		t.Fatalf("reobserve: %v", err)
	}
	if !strings.Contains(reobserved.LastRunID, "020304Z-001") {
		t.Fatalf("last run was not updated: %q", reobserved.LastRunID)
	}

	open, err := project.ListFindings(FindingFilter{OpenOnly: true})
	if err != nil {
		t.Fatalf("list open: %v", err)
	}
	if len(open) != 1 || open[0].ID != second.ID {
		t.Fatalf("open findings = %+v", open)
	}
	loaded, err := project.GetFinding(first.ID)
	if err != nil {
		t.Fatalf("get finding: %v", err)
	}
	if loaded.Disposition != DispositionAccepted {
		t.Fatalf("loaded disposition = %q", loaded.Disposition)
	}

	reduced, err := project.ReduceDispositionEvents()
	if err != nil {
		t.Fatalf("reduce events: %v", err)
	}
	firstState := reduced[first.ID]
	if firstState.Disposition != DispositionAccepted || len(firstState.Notes) != 1 {
		t.Fatalf("unexpected first reduced state: %+v", firstState)
	}
	if firstState.Notes[0].Note != "will fix with hive refactor" {
		t.Fatalf("note history = %+v", firstState.Notes)
	}
	secondState := reduced[second.ID]
	if !strings.Contains(secondState.LastRunID, "020304Z-001") {
		t.Fatalf("second reduced state = %+v", secondState)
	}

	backlogPath := BacklogPath(store.Root(), "testproj")
	raw, err := os.ReadFile(backlogPath)
	if err != nil {
		t.Fatalf("read backlog: %v", err)
	}
	backlog := string(raw)
	if strings.Contains(backlog, first.ID) {
		t.Fatalf("accepted finding remained in backlog:\n%s", backlog)
	}
	if !strings.Contains(backlog, second.ID) {
		t.Fatalf("open finding missing from backlog:\n%s", backlog)
	}
	if _, err := os.Stat(filepath.Join(FindingsDir(store.Root(), "testproj"), "testproj--vocabulary-sweep--0001.json")); err != nil {
		t.Fatalf("finding file missing: %v", err)
	}
}
