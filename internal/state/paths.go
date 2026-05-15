package state

import (
	"path/filepath"
)

const (
	dispositionsFileName = "dispositions.jsonl"
	backlogFileName      = "backlog.md"
	lastRunFileName      = "last-run.json"
	eventsFileName       = "events.jsonl"
)

// StateRoot returns the cleaned state directory root.
func StateRoot(root string) string {
	return filepath.Clean(root)
}

// SupervisorDir returns the supervisor state directory.
func SupervisorDir(root string) string {
	return filepath.Join(StateRoot(root), "supervisor")
}

// ProjectsDir returns the projects state directory.
func ProjectsDir(root string) string {
	return filepath.Join(StateRoot(root), "projects")
}

// ProjectDir returns the state directory for one project.
func ProjectDir(root string, project string) string {
	return filepath.Join(ProjectsDir(root), project)
}

// FindingsDir returns the findings directory for one project.
func FindingsDir(root string, project string) string {
	return filepath.Join(ProjectDir(root, project), "findings")
}

// RunsDir returns the runs directory for one project.
func RunsDir(root string, project string) string {
	return filepath.Join(ProjectDir(root, project), "runs")
}

// RunDir returns the immutable artifact directory for one run.
func RunDir(root string, project string, pass string, date string, timeSeq string) string {
	return filepath.Join(RunsDir(root, project), date, pass, timeSeq)
}

// ScratchDir returns the root for ephemeral per-run worktrees.
func ScratchDir(root string) string {
	return filepath.Join(StateRoot(root), "scratch")
}

// RunScratchDir returns the scratch path for one run id.
func RunScratchDir(root string, runID string) string {
	return filepath.Join(ScratchDir(root), filepath.FromSlash(runID))
}

// LastRunPath returns the per-project last-run state path.
func LastRunPath(root string, project string) string {
	return filepath.Join(ProjectDir(root, project), lastRunFileName)
}

// SupervisorEventsPath returns the supervisor lifecycle event log path.
func SupervisorEventsPath(root string) string {
	return filepath.Join(SupervisorDir(root), eventsFileName)
}

// DispositionsPath returns the append-only disposition event log path.
func DispositionsPath(root string, project string) string {
	return filepath.Join(ProjectDir(root, project), dispositionsFileName)
}

// BacklogPath returns the rendered backlog path.
func BacklogPath(root string, project string) string {
	return filepath.Join(ProjectDir(root, project), backlogFileName)
}
