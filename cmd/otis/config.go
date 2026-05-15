package main

import (
	"fmt"

	"github.com/michaelquigley/otis/internal/config"
	"github.com/spf13/cobra"
)

func newConfigCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "inspect otis configuration",
	}
	cmd.AddCommand(newConfigCheckCommand())
	return cmd
}

func newConfigCheckCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "check <path>",
		Short: "validate a global otis config",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(args[0])
			if err != nil {
				return err
			}
			passCount := 0
			for _, project := range cfg.Projects {
				passCount += len(project.Passes)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "config '%s' ok\n", cfg.Global.ConfigPath)
			fmt.Fprintf(cmd.OutOrStdout(), "projects: %d\n", len(cfg.Projects))
			fmt.Fprintf(cmd.OutOrStdout(), "passes: %d\n", passCount)
			return nil
		},
	}
}
