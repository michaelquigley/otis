package main

import (
	"fmt"
	"net/http"

	"github.com/spf13/cobra"
)

func newPassesCommand(clientConfigPath *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "passes",
		Short: "inspect supervisor passes",
	}
	cmd.AddCommand(newPassesListCommand(clientConfigPath))
	return cmd
}

func newPassesListCommand(clientConfigPath *string) *cobra.Command {
	var projectName string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "list passes from the supervisor",
		RunE: func(cmd *cobra.Command, args []string) error {
			supervisor, err := newSupervisorClient(*clientConfigPath)
			if err != nil {
				return err
			}
			projects := []string{projectName}
			if projectName == "" {
				response := projectsResponse{}
				if err := supervisor.DoJSON(cmd.Context(), http.MethodGet, apiPath("projects"), nil, &response); err != nil {
					return err
				}
				projects = projects[:0]
				for _, project := range response.Projects {
					projects = append(projects, project.Name)
				}
			}
			out := cmd.OutOrStdout()
			total := 0
			for _, project := range projects {
				response := passesResponse{}
				if err := supervisor.DoJSON(cmd.Context(), http.MethodGet, apiPath("projects", project, "passes"), nil, &response); err != nil {
					return err
				}
				for _, pass := range response.Passes {
					total++
					if projectName == "" {
						fmt.Fprintf(out, "%s\t%s\t%s\t%s\t%s\n", project, pass.Name, pass.Reviewer, pass.Cadence, pass.Description)
					} else {
						fmt.Fprintf(out, "%s\t%s\t%s\t%s\n", pass.Name, pass.Reviewer, pass.Cadence, pass.Description)
					}
				}
			}
			if total == 0 {
				fmt.Fprintln(out, "no passes")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&projectName, "project", "", "project name")
	return cmd
}
