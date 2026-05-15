package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/michaelquigley/df/dl"
	"github.com/michaelquigley/otis/internal/config"
	"github.com/spf13/cobra"
)

const Version = "v0.0.0-dev"

func main() {
	configureLogging(false)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	root := newRootCommand()
	if err := root.ExecuteContext(ctx); err != nil {
		dl.Fatalf("otis failed: %v", err)
	}
}

func newRootCommand() *cobra.Command {
	var configPath string
	var verbose bool

	root := &cobra.Command{
		Use:           "otis",
		Short:         "continuous code-quality agent",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			configureLogging(verbose)
		},
	}
	root.PersistentFlags().StringVar(&configPath, "config", config.DefaultConfigPath, "path to otis global config")
	root.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose stderr logging")
	root.AddCommand(newVersionCommand())
	root.AddCommand(newServeCommand(&configPath))
	root.AddCommand(newConfigCommand())
	root.AddCommand(newFindingsCommand(&configPath))
	root.AddCommand(newBokCommand())
	root.AddCommand(newPassCommand(&configPath))
	return root
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "print version information",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "otis %s %s/%s\n", Version, runtime.GOOS, runtime.GOARCH)
			return nil
		},
	}
}

func configureLogging(verbose bool) {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}
	dl.Init(dl.DefaultOptions().
		SetOutput(os.Stderr).
		SetTrimPrefix("github.com/michaelquigley/otis/").
		SetLevel(level))
}
