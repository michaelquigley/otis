package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/michaelquigley/otis/internal/bok"
	"github.com/spf13/cobra"
)

func newBokCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bok",
		Short: "inspect BoK entries",
	}
	cmd.AddCommand(newBokListCommand())
	cmd.AddCommand(newBokResolveCommand())
	return cmd
}

func newBokListCommand() *cobra.Command {
	var bokPath string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "list BoK entries",
		RunE: func(cmd *cobra.Command, args []string) error {
			if bokPath == "" {
				return fmt.Errorf("--bok-path is required")
			}
			entries, err := bok.ListEntries(bokPath)
			if err != nil {
				return err
			}
			grouped := map[string][]*bok.Entry{}
			var groups []string
			for _, entry := range entries {
				group := topLevel(entry.Relpath)
				if _, ok := grouped[group]; !ok {
					groups = append(groups, group)
				}
				grouped[group] = append(grouped[group], entry)
			}
			sort.Strings(groups)
			out := cmd.OutOrStdout()
			if len(groups) == 0 {
				fmt.Fprintln(out, "no BoK entries")
				return nil
			}
			for _, group := range groups {
				fmt.Fprintf(out, "%s/\n", group)
				for _, entry := range grouped[group] {
					if entry.Title == "" {
						fmt.Fprintf(out, "  %s\n", entry.Relpath)
						continue
					}
					fmt.Fprintf(out, "  %s\t%s\n", entry.Relpath, entry.Title)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&bokPath, "bok-path", "", "path to the BoK root")
	return cmd
}

func newBokResolveCommand() *cobra.Command {
	var bokPath string
	var includeCSV string
	var projectName string
	cmd := &cobra.Command{
		Use:   "resolve",
		Short: "resolve a pass include list",
		RunE: func(cmd *cobra.Command, args []string) error {
			if bokPath == "" {
				return fmt.Errorf("--bok-path is required")
			}
			if includeCSV == "" {
				return fmt.Errorf("--include is required")
			}
			if projectName == "" {
				return fmt.Errorf("--project is required")
			}
			entries, err := bok.Resolve(bokPath, splitIncludes(includeCSV), projectName)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if len(entries) == 0 {
				fmt.Fprintln(out, "no BoK entries")
				return nil
			}
			for _, entry := range entries {
				if entry.Title == "" {
					fmt.Fprintln(out, entry.Relpath)
					continue
				}
				fmt.Fprintf(out, "%s\t%s\n", entry.Relpath, entry.Title)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&bokPath, "bok-path", "", "path to the BoK root")
	cmd.Flags().StringVar(&includeCSV, "include", "", "comma-separated include entries")
	cmd.Flags().StringVar(&projectName, "project", "", "project name for location-based filtering")
	return cmd
}

func splitIncludes(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func topLevel(relpath string) string {
	before, _, found := strings.Cut(relpath, "/")
	if !found {
		return relpath
	}
	return before
}
