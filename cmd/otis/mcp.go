package main

import (
	"github.com/michaelquigley/otis/internal/client"
	mcpbridge "github.com/michaelquigley/otis/internal/mcp"
	"github.com/spf13/cobra"
)

func newMCPCommand() *cobra.Command {
	var clientConfigPath string
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "run the Otis MCP stdio bridge",
		RunE: func(cmd *cobra.Command, args []string) error {
			return mcpbridge.Run(cmd.Context(), clientConfigPath, Version)
		},
	}
	cmd.Flags().StringVar(&clientConfigPath, "client-config", client.DefaultConfigPath, "path to otis client config")
	return cmd
}
