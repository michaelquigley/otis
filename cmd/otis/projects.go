package main

import (
	"fmt"
	"net/http"

	"github.com/spf13/cobra"
)

func newProjectsCommand(clientConfigPath *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "projects",
		Short: "inspect supervisor projects",
	}
	cmd.AddCommand(newProjectsListCommand(clientConfigPath))
	return cmd
}

func newProjectsListCommand(clientConfigPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list projects from the supervisor",
		RunE: func(cmd *cobra.Command, args []string) error {
			supervisor, err := newSupervisorClient(*clientConfigPath)
			if err != nil {
				return err
			}
			response := projectsResponse{}
			if err := supervisor.DoJSON(cmd.Context(), http.MethodGet, apiPath("projects"), nil, &response); err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if len(response.Projects) == 0 {
				fmt.Fprintln(out, "no projects")
				return nil
			}
			for _, project := range response.Projects {
				fmt.Fprintf(out, "%s\t%s\t%s\n", project.Name, project.PrimaryLanguage, project.Description)
			}
			return nil
		},
	}
}
