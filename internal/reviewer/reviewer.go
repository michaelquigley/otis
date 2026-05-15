package reviewer

import (
	"context"
	"encoding/json"

	"github.com/michaelquigley/otis/internal/prompt"
)

type Reviewer interface {
	Review(ctx context.Context, req Request) (Result, error)
}

type Request struct {
	Prompt     string
	Schema     json.RawMessage
	WorkingDir string
	Model      string
}

type Result struct {
	Raw          json.RawMessage
	Output       prompt.ReviewerOutput
	Findings     []prompt.ReviewerFinding
	ArtifactsDir string
	UsageNotes   string
}
