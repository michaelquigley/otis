package main

import (
	"fmt"
	"net/http"

	"github.com/spf13/cobra"
)

func newPassCommand(clientConfigPath *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pass",
		Short: "run and inspect passes",
	}
	cmd.AddCommand(newPassRunCommand(clientConfigPath))
	return cmd
}

func newPassRunCommand(clientConfigPath *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run <project>/<pass>",
		Short: "force-run a pass through the supervisor",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectName, passName, err := splitProjectPass(args[0])
			if err != nil {
				return err
			}
			supervisor, err := newSupervisorClient(*clientConfigPath)
			if err != nil {
				return err
			}
			response := runResponse{}
			if err := supervisor.DoJSON(cmd.Context(), http.MethodPost, apiPath("projects", projectName, "passes", passName, "run"), nil, &response); err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "state: %s\n", response.State)
			if response.Project != "" {
				fmt.Fprintf(out, "project: %s\n", response.Project)
			}
			if response.Pass != "" {
				fmt.Fprintf(out, "pass: %s\n", response.Pass)
			}
			if response.RunID != "" {
				fmt.Fprintf(out, "run: %s\n", response.RunID)
			}
			return nil
		},
	}
	return cmd
}
