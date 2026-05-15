package main

import (
	"github.com/michaelquigley/df/dl"
	"github.com/michaelquigley/otis/internal/config"
	"github.com/spf13/cobra"
)

func newServeCommand(configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "run the otis supervisor",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(*configPath)
			if err != nil {
				return err
			}
			dl.Infof("supervisor would start here")
			dl.Infof("loaded %d project(s) from '%s'", len(cfg.Projects), cfg.Global.ConfigPath)
			return nil
		},
	}
}
