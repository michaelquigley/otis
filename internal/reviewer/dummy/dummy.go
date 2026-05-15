package dummy

import (
	"context"
	"encoding/json"
	"os"
	"sync"

	"github.com/michaelquigley/otis/internal/prompt"
	"github.com/michaelquigley/otis/internal/reviewer"
)

type Options struct {
	Raw          json.RawMessage
	OutputPath   string
	Err          error
	ArtifactsDir string
	UsageNotes   string
}

// Reviewer returns deterministic reviewer output for tests.
type Reviewer struct {
	mu       sync.Mutex
	options  Options
	requests []reviewer.Request
}

func New(options Options) *Reviewer {
	if len(options.Raw) == 0 && options.OutputPath == "" {
		options.Raw = defaultRaw()
	}
	if options.UsageNotes == "" {
		options.UsageNotes = "dummy reviewer"
	}
	return &Reviewer{options: options}
}

func (r *Reviewer) Review(ctx context.Context, req reviewer.Request) (reviewer.Result, error) {
	if err := ctx.Err(); err != nil {
		return reviewer.Result{}, err
	}
	r.mu.Lock()
	r.requests = append(r.requests, cloneRequest(req))
	options := r.options
	r.mu.Unlock()

	if options.Err != nil {
		return reviewer.Result{}, options.Err
	}
	raw := append(json.RawMessage(nil), options.Raw...)
	if options.OutputPath != "" {
		loaded, err := os.ReadFile(options.OutputPath)
		if err != nil {
			return reviewer.Result{}, err
		}
		raw = append(json.RawMessage(nil), loaded...)
	}
	output, err := prompt.ParseReviewerOutput(raw, req.Schema)
	if err != nil {
		return reviewer.Result{}, err
	}
	return reviewer.Result{
		Raw:          raw,
		Output:       output,
		Findings:     append([]prompt.ReviewerFinding(nil), output.Findings...),
		ArtifactsDir: options.ArtifactsDir,
		UsageNotes:   options.UsageNotes,
	}, nil
}

// Requests returns captured review requests.
func (r *Reviewer) Requests() []reviewer.Request {
	r.mu.Lock()
	defer r.mu.Unlock()

	requests := make([]reviewer.Request, 0, len(r.requests))
	for _, req := range r.requests {
		requests = append(requests, cloneRequest(req))
	}
	return requests
}

func defaultRaw() json.RawMessage {
	return json.RawMessage(`{"findings":[]}`)
}

func cloneRequest(req reviewer.Request) reviewer.Request {
	req.Schema = append(json.RawMessage(nil), req.Schema...)
	return req
}
