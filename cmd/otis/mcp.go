package main

import (
	mcpbridge "github.com/michaelquigley/otis/internal/mcp"
	"github.com/spf13/cobra"
)

func newMCPCommand(clientConfigPath *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "run the Otis MCP stdio bridge",
		RunE: func(cmd *cobra.Command, args []string) error {
			return mcpbridge.Run(cmd.Context(), *clientConfigPath, Version)
		},
	}
	return cmd
}
