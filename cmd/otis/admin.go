package main

import (
	"fmt"

	"github.com/michaelquigley/otis/internal/api"
	"github.com/michaelquigley/otis/internal/config"
	"github.com/spf13/cobra"
)

func newAdminCommand(configPath *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "admin",
		Short: "administrative supervisor operations",
	}
	cmd.AddCommand(newAdminTokenCommand(configPath))
	return cmd
}

func newAdminTokenCommand(configPath *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "token",
		Short: "manage API bearer tokens",
	}
	cmd.AddCommand(newAdminTokenIssueCommand(configPath))
	return cmd
}

func newAdminTokenIssueCommand(configPath *string) *cobra.Command {
	var label string
	cmd := &cobra.Command{
		Use:   "issue",
		Short: "issue a new API bearer token",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadGlobal(*configPath)
			if err != nil {
				return err
			}
			token, err := api.IssueToken(cfg.Storage.StateDir, label)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), token)
			return nil
		},
	}
	cmd.Flags().StringVar(&label, "label", "", "optional token label")
	return cmd
}
