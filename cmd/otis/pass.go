package main

import (
	"fmt"

	"github.com/michaelquigley/otis/internal/config"
	"github.com/michaelquigley/otis/internal/dispatcher"
	"github.com/spf13/cobra"
)

func newPassCommand(configPath *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pass",
		Short: "run and inspect passes",
	}
	cmd.AddCommand(newPassRunCommand(configPath))
	return cmd
}

func newPassRunCommand(configPath *string) *cobra.Command {
	var reviewerOverride string
	var dummyOutputPath string
	cmd := &cobra.Command{
		Use:   "run <project>/<pass>",
		Short: "force-run a pass",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectName, passName, err := dispatcher.SplitProjectPass(args[0])
			if err != nil {
				return err
			}
			cfg, err := config.Load(*configPath)
			if err != nil {
				return err
			}
			dispatch, err := dispatcher.New(cfg, dispatcher.Options{})
			if err != nil {
				return err
			}
			handle, err := dispatch.Enqueue(cmd.Context(), dispatcher.EnqueueRequest{
				ProjectName:      projectName,
				PassName:         passName,
				Source:           dispatcher.SourceForce,
				ReviewerOverride: reviewerOverride,
				DummyOutputPath:  dummyOutputPath,
			})
			if err != nil {
				return err
			}
			dispatched, err := handle.Wait(cmd.Context())
			if err != nil {
				return err
			}
			result := dispatched.RunResult
			fmt.Fprintf(cmd.OutOrStdout(), "run: %s\n", result.RunID)
			fmt.Fprintf(cmd.OutOrStdout(), "report: %s\n", result.RunDir)
			if result.Noop {
				fmt.Fprintln(cmd.OutOrStdout(), "result: no commits in window")
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "findings: %d\n", len(result.Findings))
			return nil
		},
	}
	cmd.Flags().StringVar(&reviewerOverride, "reviewer", "", "override reviewer kind for this run")
	cmd.Flags().StringVar(&dummyOutputPath, "dummy-output", "", "path to canned dummy reviewer JSON output")
	return cmd
}
