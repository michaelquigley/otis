package api

import (
	"path/filepath"

	"github.com/michaelquigley/otis/internal/state"
)

func runReportPath(root string, id state.RunID) string {
	return filepath.Join(state.RunDir(root, id.Project, id.Pass, id.Date, id.TimeSeq), "report.md")
}
