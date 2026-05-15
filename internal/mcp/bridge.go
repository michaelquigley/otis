package mcpbridge

import (
	"context"
	"errors"

	"github.com/michaelquigley/otis/internal/client"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func New(supervisor Supervisor, version string) *mcpsdk.Server {
	server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "otis", Version: version}, nil)
	RegisterTools(server, supervisor)
	return server
}

func Run(ctx context.Context, configPath string, version string) error {
	cfg, err := client.LoadConfig(configPath)
	if err != nil {
		return err
	}
	supervisor, err := client.New(cfg)
	if err != nil {
		return err
	}
	server := New(supervisor, version)
	err = server.Run(ctx, &mcpsdk.StdioTransport{})
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}
