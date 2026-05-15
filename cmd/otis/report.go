package main

import (
	"fmt"
	"net/http"

	"github.com/michaelquigley/otis/internal/state"
	"github.com/spf13/cobra"
)

func newReportCommand(clientConfigPath *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "report",
		Short: "inspect run reports",
	}
	cmd.AddCommand(newReportShowCommand(clientConfigPath))
	return cmd
}

func newReportShowCommand(clientConfigPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "show <run-id>",
		Short: "show a stored run report from the supervisor",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			runID, err := state.ParseRunID(args[0])
			if err != nil {
				return err
			}
			supervisor, err := newSupervisorClient(*clientConfigPath)
			if err != nil {
				return err
			}
			report, err := supervisor.DoText(cmd.Context(), http.MethodGet, reportAPIPath(runID), nil)
			if err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), report)
			return nil
		},
	}
}
