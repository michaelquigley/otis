package dispatcher

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/michaelquigley/otis/internal/config"
	"github.com/michaelquigley/otis/internal/state"
)

type Source string

const (
	SourceScheduled Source = "scheduled"
	SourceForce     Source = "force"

	InFlightQueued  = "queued"
	InFlightRunning = "running"
)

type ErrInFlight struct {
	Project string
	Pass    string
	State   string
	RunID   string
}

func (e *ErrInFlight) Error() string {
	if e.RunID == "" {
		return fmt.Sprintf("%s/%s is already %s", e.Project, e.Pass, e.State)
	}
	return fmt.Sprintf("%s/%s is already %s as %s", e.Project, e.Pass, e.State, e.RunID)
}

type EnqueueRequest struct {
	ProjectName      string
	PassName         string
	Source           Source
	ReviewerOverride string
	DummyOutputPath  string
	Now              time.Time
	WindowOpen       func(time.Time) bool
}

type DispatchResult struct {
	RunResult RunResult
	Dropped   bool
	Err       error
}

type EnqueueHandle struct {
	ProjectName string
	PassName    string
	done        <-chan DispatchResult
}

func (h *EnqueueHandle) Wait(ctx context.Context) (DispatchResult, error) {
	if h == nil {
		return DispatchResult{Dropped: true}, nil
	}
	select {
	case result := <-h.done:
		return result, result.Err
	case <-ctx.Done():
		return DispatchResult{}, ctx.Err()
	}
}

type Runner interface {
	Run(context.Context, RunRequest) (RunResult, error)
}

type RunnerFunc func(context.Context, RunRequest) (RunResult, error)

func (f RunnerFunc) Run(ctx context.Context, req RunRequest) (RunResult, error) {
	return f(ctx, req)
}

type Options struct {
	Store  *state.Store
	Now    func() time.Time
	Runner Runner
}

type Dispatcher struct {
	cfg          *config.ResolvedConfig
	store        *state.Store
	now          func() time.Time
	runner       Runner
	globalSem    chan struct{}
	reviewerSems map[string]chan struct{}
	mu           sync.Mutex
	inFlight     map[passKey]*inFlightEntry
}

type passKey struct {
	project string
	pass    string
}

type inFlightEntry struct {
	state string
	runID string
}

func New(cfg *config.ResolvedConfig, opts Options) (*Dispatcher, error) {
	if cfg == nil || cfg.Global == nil {
		return nil, fmt.Errorf("config is required")
	}
	if cfg.Global.GlobalConcurrencyCap <= 0 {
		return nil, fmt.Errorf("global_concurrency_cap must be greater than zero")
	}
	store := opts.Store
	if store == nil {
		var err error
		store, err = state.NewStore(cfg.Global.Storage.StateDir)
		if err != nil {
			return nil, err
		}
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	runner := opts.Runner
	if runner == nil {
		runner = RunnerFunc(Run)
	}
	d := &Dispatcher{
		cfg:          cfg,
		store:        store,
		now:          now,
		runner:       runner,
		globalSem:    make(chan struct{}, cfg.Global.GlobalConcurrencyCap),
		reviewerSems: map[string]chan struct{}{},
		inFlight:     map[passKey]*inFlightEntry{},
	}
	for name, reviewer := range cfg.Global.Reviewers {
		if reviewer == nil || reviewer.ConcurrencyCap <= 0 {
			return nil, fmt.Errorf("reviewers.%s.concurrency_cap must be greater than zero", name)
		}
		d.reviewerSems[name] = make(chan struct{}, reviewer.ConcurrencyCap)
	}
	return d, nil
}

func (d *Dispatcher) Enqueue(ctx context.Context, req EnqueueRequest) (*EnqueueHandle, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if req.Source == "" {
		req.Source = SourceForce
	}
	project, pass, reviewerKind, err := d.resolve(req)
	if err != nil {
		return nil, err
	}
	key := passKey{project: req.ProjectName, pass: req.PassName}

	d.mu.Lock()
	if existing := d.inFlight[key]; existing != nil {
		state := existing.state
		runID := existing.runID
		d.mu.Unlock()
		if req.Source == SourceScheduled {
			return nil, nil
		}
		return nil, &ErrInFlight{
			Project: req.ProjectName,
			Pass:    req.PassName,
			State:   state,
			RunID:   runID,
		}
	}
	d.inFlight[key] = &inFlightEntry{state: InFlightQueued}
	reviewerSem := d.reviewerSemaphoreLocked(reviewerKind)
	d.mu.Unlock()

	done := make(chan DispatchResult, 1)
	handle := &EnqueueHandle{
		ProjectName: req.ProjectName,
		PassName:    req.PassName,
		done:        done,
	}
	go d.execute(ctx, req, project, pass, reviewerKind, key, reviewerSem, done)
	return handle, nil
}

func (d *Dispatcher) resolve(req EnqueueRequest) (*config.ResolvedProject, *config.Pass, string, error) {
	project, ok := d.cfg.Projects[req.ProjectName]
	if !ok {
		return nil, nil, "", fmt.Errorf("project %q is not configured", req.ProjectName)
	}
	pass := findPass(project, req.PassName)
	if pass == nil {
		return nil, nil, "", fmt.Errorf("project %q pass %q is not configured", req.ProjectName, req.PassName)
	}
	reviewerKind := req.ReviewerOverride
	if reviewerKind == "" && pass.Reviewer != nil {
		reviewerKind = pass.Reviewer.Kind
	}
	if reviewerKind == "" {
		return nil, nil, "", fmt.Errorf("reviewer is required")
	}
	return project, pass, reviewerKind, nil
}

func (d *Dispatcher) reviewerSemaphoreLocked(kind string) chan struct{} {
	sem := d.reviewerSems[kind]
	if sem == nil {
		sem = make(chan struct{}, 1)
		d.reviewerSems[kind] = sem
	}
	return sem
}

func (d *Dispatcher) execute(ctx context.Context, req EnqueueRequest, project *config.ResolvedProject, pass *config.Pass, reviewerKind string, key passKey, reviewerSem chan struct{}, done chan<- DispatchResult) {
	var result DispatchResult
	reviewerAcquired := false
	globalAcquired := false
	defer func() {
		if recovered := recover(); recovered != nil {
			result.Err = fmt.Errorf("dispatcher run panic: %v", recovered)
		}
		if globalAcquired {
			release(d.globalSem)
		}
		if reviewerAcquired {
			release(reviewerSem)
		}
		d.mu.Lock()
		delete(d.inFlight, key)
		d.mu.Unlock()
		done <- result
		close(done)
	}()

	if err := acquire(ctx, reviewerSem); err != nil {
		result.Err = err
		return
	}
	reviewerAcquired = true
	if err := acquire(ctx, d.globalSem); err != nil {
		result.Err = err
		return
	}
	globalAcquired = true

	now := req.Now
	if now.IsZero() {
		now = d.now()
	}
	if req.Source == SourceScheduled && req.WindowOpen != nil && !req.WindowOpen(now) {
		result.Dropped = true
		return
	}
	capturedSHA, err := CaptureHEAD(ctx, project.RepoPath)
	if err != nil {
		result.Err = err
		return
	}
	runID, err := d.store.Project(req.ProjectName).AllocateRunStart(pass.Name, now)
	if err != nil {
		result.Err = err
		return
	}
	d.markRunning(key, runID)
	if err := d.store.AppendSupervisorEvent(state.SupervisorEvent{
		Type:    state.EventPassDispatched,
		Project: req.ProjectName,
		Pass:    pass.Name,
		RunID:   runID,
		At:      now,
	}); err != nil {
		result.Err = err
		return
	}

	runResult, err := d.runner.Run(ctx, RunRequest{
		Config:           d.cfg,
		Store:            d.store,
		ProjectName:      req.ProjectName,
		PassName:         pass.Name,
		ReviewerOverride: req.ReviewerOverride,
		DummyOutputPath:  req.DummyOutputPath,
		RunID:            runID,
		GitHead:          capturedSHA,
		Now:              now,
	})
	if runResult.RunID == "" {
		runResult.RunID = runID
	}
	result.RunResult = runResult
	result.Err = err
}

func (d *Dispatcher) markRunning(key passKey, runID string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if entry := d.inFlight[key]; entry != nil {
		entry.state = InFlightRunning
		entry.runID = runID
	}
}

func acquire(ctx context.Context, sem chan struct{}) error {
	select {
	case sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func release(sem chan struct{}) {
	<-sem
}
