package scheduler

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/michaelquigley/df/dl"
	"github.com/michaelquigley/otis/internal/config"
	"github.com/michaelquigley/otis/internal/dispatcher"
	"github.com/michaelquigley/otis/internal/state"
)

const DefaultInterval = 30 * time.Second

type Enqueuer interface {
	Enqueue(context.Context, dispatcher.EnqueueRequest) (*dispatcher.EnqueueHandle, error)
}

type Options struct {
	Store    *state.Store
	Enqueuer Enqueuer
	Now      func() time.Time
	Interval time.Duration
}

type Scheduler struct {
	cfg      *config.ResolvedConfig
	store    *state.Store
	enqueuer Enqueuer
	now      func() time.Time
	interval time.Duration
}

type DuePass struct {
	ProjectName  string
	PassName     string
	ReviewerKind string
	WindowName   string
	Cadence      time.Duration
	LastRun      *state.LastRunRecord
}

func New(cfg *config.ResolvedConfig, opts Options) (*Scheduler, error) {
	if cfg == nil || cfg.Global == nil {
		return nil, fmt.Errorf("config is required")
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
	enqueuer := opts.Enqueuer
	if enqueuer == nil {
		dispatch, err := dispatcher.New(cfg, dispatcher.Options{
			Store: store,
			Now:   now,
		})
		if err != nil {
			return nil, err
		}
		enqueuer = dispatch
	}
	interval := opts.Interval
	if interval <= 0 {
		interval = DefaultInterval
	}
	return &Scheduler{
		cfg:      cfg,
		store:    store,
		enqueuer: enqueuer,
		now:      now,
		interval: interval,
	}, nil
}

func (s *Scheduler) Run(ctx context.Context) error {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	if _, err := s.RunOnce(ctx); err != nil {
		dl.Warnf("scheduler tick failed: %v", err)
	}
	for {
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.Canceled) {
				return nil
			}
			return ctx.Err()
		case <-ticker.C:
			if _, err := s.RunOnce(ctx); err != nil {
				dl.Warnf("scheduler tick failed: %v", err)
			}
		}
	}
}

func (s *Scheduler) RunOnce(ctx context.Context) ([]*dispatcher.EnqueueHandle, error) {
	now := s.now()
	due, err := s.Due(now)
	if err != nil {
		return nil, err
	}
	handles := make([]*dispatcher.EnqueueHandle, 0, len(due))
	for _, item := range due {
		reviewerKind := item.ReviewerKind
		handle, err := s.enqueuer.Enqueue(ctx, dispatcher.EnqueueRequest{
			ProjectName: item.ProjectName,
			PassName:    item.PassName,
			Source:      dispatcher.SourceScheduled,
			WindowOpen: func(at time.Time) bool {
				ok, err := ReviewerWindowOpen(s.cfg.Global, reviewerKind, at)
				return err == nil && ok
			},
		})
		if err != nil {
			return handles, err
		}
		if handle != nil {
			handles = append(handles, handle)
		}
	}
	return handles, nil
}

func (s *Scheduler) WaitAll(ctx context.Context, handles []*dispatcher.EnqueueHandle) []error {
	errs := []error{}
	for _, handle := range handles {
		result, err := handle.Wait(ctx)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if result.Dropped {
			continue
		}
		if result.RunResult.RunID != "" {
			dl.Infof("pass run completed: %s", result.RunResult.RunID)
		}
	}
	return errs
}

func (s *Scheduler) Due(now time.Time) ([]DuePass, error) {
	names := make([]string, 0, len(s.cfg.Projects))
	for name := range s.cfg.Projects {
		names = append(names, name)
	}
	sort.Strings(names)

	var due []DuePass
	for _, projectName := range names {
		project := s.cfg.Projects[projectName]
		if project == nil {
			continue
		}
		if err := repoAvailable(project.RepoPath); err != nil {
			dl.Warnf("project %s unavailable: %v", projectName, err)
			continue
		}
		for _, pass := range project.Passes {
			if pass == nil || pass.Reviewer == nil {
				continue
			}
			reviewer := s.cfg.Global.Reviewers[pass.Reviewer.Kind]
			if reviewer == nil {
				return nil, fmt.Errorf("project %q pass %q references unknown reviewer %q", projectName, pass.Name, pass.Reviewer.Kind)
			}
			open, err := ReviewerWindowOpen(s.cfg.Global, pass.Reviewer.Kind, now)
			if err != nil {
				return nil, err
			}
			if !open {
				continue
			}
			cadence, err := config.ParseDuration(pass.Cadence)
			if err != nil {
				return nil, fmt.Errorf("project %q pass %q cadence: %w", projectName, pass.Name, err)
			}
			record, ok, err := s.store.Project(projectName).LastRun(pass.Name)
			if err != nil {
				return nil, err
			}
			if ok && now.UTC().Sub(record.At) < cadence {
				continue
			}
			item := DuePass{
				ProjectName:  projectName,
				PassName:     pass.Name,
				ReviewerKind: pass.Reviewer.Kind,
				WindowName:   reviewer.Window,
				Cadence:      cadence,
			}
			if ok {
				item.LastRun = &record
			}
			due = append(due, item)
		}
	}
	return due, nil
}

func repoAvailable(path string) error {
	if path == "" {
		return fmt.Errorf("path is required")
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("not a directory")
	}
	return nil
}
