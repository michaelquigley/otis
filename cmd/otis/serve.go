package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/michaelquigley/df/dl"
	"github.com/michaelquigley/otis/internal/config"
	"github.com/michaelquigley/otis/internal/dispatcher"
	"github.com/michaelquigley/otis/internal/scheduler"
	"github.com/michaelquigley/otis/internal/state"
	"github.com/spf13/cobra"
)

func newServeCommand(configPath *string) *cobra.Command {
	var once bool
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "run the otis supervisor",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(*configPath)
			if err != nil {
				return err
			}
			store, err := state.NewStore(cfg.Global.Storage.StateDir)
			if err != nil {
				return err
			}
			if err := store.AppendSupervisorEvent(state.SupervisorEvent{Type: state.EventSupervisorStarted}); err != nil {
				return err
			}
			defer func() {
				if err := store.AppendSupervisorEvent(state.SupervisorEvent{Type: state.EventSupervisorStopped}); err != nil {
					dl.Warnf("write supervisor stop event: %v", err)
				}
			}()
			if err := cleanupScratch(cmd.Context(), cfg); err != nil {
				return err
			}
			s, err := scheduler.New(cfg, scheduler.Options{Store: store})
			if err != nil {
				return err
			}
			dl.Infof("supervisor loaded %d project(s) from '%s'", len(cfg.Projects), cfg.Global.ConfigPath)
			if once {
				handles, err := s.RunOnce(cmd.Context())
				if err != nil {
					return err
				}
				if errs := s.WaitAll(cmd.Context(), handles); len(errs) > 0 {
					return errors.Join(errs...)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "enqueued: %d\n", len(handles))
				return nil
			}
			return s.Run(cmd.Context())
		},
	}
	cmd.Flags().BoolVar(&once, "once", false, "run one scheduler tick and exit")
	return cmd
}

func cleanupScratch(ctx context.Context, cfg *config.ResolvedConfig) error {
	for name, project := range cfg.Projects {
		if project == nil {
			continue
		}
		if err := dispatcher.PruneWorktrees(ctx, project.RepoPath); err != nil {
			dl.Warnf("project %s worktree prune failed: %v", name, err)
		}
	}
	if err := os.RemoveAll(state.ScratchDir(cfg.Global.Storage.StateDir)); err != nil {
		return err
	}
	return nil
}
