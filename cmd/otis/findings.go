package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/michaelquigley/otis/internal/client"
	"github.com/michaelquigley/otis/internal/state"
	"github.com/spf13/cobra"
)

func newFindingsCommand(clientConfigPath *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "findings",
		Short: "inspect findings through the supervisor API",
	}
	cmd.AddCommand(newFindingsListCommand(clientConfigPath))
	cmd.AddCommand(newFindingsShowCommand(clientConfigPath))
	return cmd
}

func newFindingsListCommand(clientConfigPath *string) *cobra.Command {
	var projectName string
	var passName string
	var openOnly bool
	var disposition string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "list findings from the supervisor",
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
				findings, err := remoteFindings(cmd.Context(), supervisor, project, passName, disposition, openOnly)
				if err != nil {
					return err
				}
				for _, finding := range findings {
					total++
					fmt.Fprintf(out, "%s\t%s\t%s\t%s\n", finding.ID, finding.Severity, finding.Disposition, finding.Title)
				}
			}
			if total == 0 {
				fmt.Fprintln(out, "no findings")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&projectName, "project", "", "project name")
	cmd.Flags().StringVar(&passName, "pass", "", "pass name")
	cmd.Flags().StringVar(&disposition, "disposition", "", "finding disposition")
	cmd.Flags().BoolVar(&openOnly, "open", false, "only show open findings")
	return cmd
}

func newFindingsShowCommand(clientConfigPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "show <finding-id>",
		Short: "show one finding from the supervisor",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := state.ParseID(args[0])
			if err != nil {
				return err
			}
			supervisor, err := newSupervisorClient(*clientConfigPath)
			if err != nil {
				return err
			}
			finding := state.Finding{}
			if err := supervisor.DoJSON(cmd.Context(), http.MethodGet, findingAPIPath(id), nil, &finding); err != nil {
				return err
			}
			printFinding(cmd, &finding)
			return nil
		},
	}
}

func remoteFindings(ctx context.Context, supervisor *client.Client, projectName string, passName string, disposition string, openOnly bool) ([]*state.Finding, error) {
	response := findingsResponse{}
	if err := supervisor.DoJSON(ctx, http.MethodGet, findingsPath(projectName, passName, disposition, openOnly), nil, &response); err != nil {
		return nil, err
	}
	return response.Findings, nil
}

func printFinding(cmd *cobra.Command, finding *state.Finding) {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "id: %s\n", finding.ID)
	fmt.Fprintf(out, "project: %s\n", finding.Project)
	fmt.Fprintf(out, "pass: %s\n", finding.Pass)
	fmt.Fprintf(out, "reviewer: %s\n", finding.Reviewer)
	fmt.Fprintf(out, "severity: %s\n", finding.Severity)
	fmt.Fprintf(out, "disposition: %s\n", finding.Disposition)
	fmt.Fprintf(out, "first run: %s\n", finding.FirstRunID)
	fmt.Fprintf(out, "last run: %s\n", finding.LastRunID)
	fmt.Fprintf(out, "created: %s\n", finding.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"))
	fmt.Fprintf(out, "location: %s:%s\n", finding.Location.File, finding.Location.Lines)
	if len(finding.BokRefs) > 0 {
		fmt.Fprintf(out, "bok refs: %s\n", strings.Join(finding.BokRefs, ", "))
	}
	fmt.Fprintf(out, "\ntitle: %s\n\n", finding.Title)
	fmt.Fprintf(out, "%s\n", strings.TrimSpace(finding.Description))
	if strings.TrimSpace(finding.SuggestedFix) != "" {
		fmt.Fprintf(out, "\nsuggested fix:\n%s\n", strings.TrimSpace(finding.SuggestedFix))
	}
}

func findingsPath(projectName string, passName string, disposition string, openOnly bool) string {
	path := apiPath("projects", projectName) + "/findings"
	query := url.Values{}
	if passName != "" {
		query.Set("pass", passName)
	}
	if disposition != "" {
		query.Set("disposition", disposition)
	}
	if openOnly {
		query.Set("open", "true")
	}
	if encoded := query.Encode(); encoded != "" {
		path += "?" + encoded
	}
	return path
}
