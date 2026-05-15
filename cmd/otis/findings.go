package main

import (
	"fmt"
	"strings"

	"github.com/michaelquigley/otis/internal/config"
	"github.com/michaelquigley/otis/internal/state"
	"github.com/spf13/cobra"
)

func newFindingsCommand(configPath *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "findings",
		Short: "inspect local finding state",
	}
	cmd.AddCommand(newFindingsListCommand(configPath))
	cmd.AddCommand(newFindingsShowCommand(configPath))
	return cmd
}

func newFindingsListCommand(configPath *string) *cobra.Command {
	var projectName string
	var passName string
	var openOnly bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "list findings from the local state directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			if projectName == "" {
				return fmt.Errorf("--project is required")
			}
			project, err := localStateProject(*configPath, projectName)
			if err != nil {
				return err
			}
			findings, err := project.ListFindings(state.FindingFilter{
				Pass:     passName,
				OpenOnly: openOnly,
			})
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if len(findings) == 0 {
				fmt.Fprintln(out, "no findings")
				return nil
			}
			for _, finding := range findings {
				fmt.Fprintf(out, "%s\t%s\t%s\t%s\n", finding.ID, finding.Severity, finding.Disposition, finding.Title)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&projectName, "project", "", "project name")
	cmd.Flags().StringVar(&passName, "pass", "", "pass name")
	cmd.Flags().BoolVar(&openOnly, "open", false, "only show open findings")
	return cmd
}

func newFindingsShowCommand(configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "show <finding-id>",
		Short: "show one finding from the local state directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := state.ParseID(args[0])
			if err != nil {
				return err
			}
			project, err := localStateProject(*configPath, id.Project)
			if err != nil {
				return err
			}
			finding, err := project.GetFinding(id.String())
			if err != nil {
				return err
			}
			printFinding(cmd, finding)
			return nil
		},
	}
}

func localStateProject(configPath string, projectName string) (*state.Project, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}
	if _, ok := cfg.Projects[projectName]; !ok {
		return nil, fmt.Errorf("project %q is not configured", projectName)
	}
	store, err := state.NewStore(cfg.Global.Storage.StateDir)
	if err != nil {
		return nil, err
	}
	return store.Project(projectName), nil
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
