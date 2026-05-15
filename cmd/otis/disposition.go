package main

import (
	"fmt"
	"net/http"

	"github.com/michaelquigley/otis/internal/state"
	"github.com/spf13/cobra"
)

func newDispositionCommand(name string, short string, disposition string, clientConfigPath *string) *cobra.Command {
	var note string
	cmd := &cobra.Command{
		Use:   name + " <finding-id>",
		Short: short,
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
			if err := supervisor.DoJSON(cmd.Context(), http.MethodPost, findingAPIPath(id)+"/disposition", dispositionBody{
				Disposition: disposition,
				Note:        note,
			}, &finding); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\n", finding.ID, finding.Disposition, finding.Title)
			return nil
		},
	}
	cmd.Flags().StringVar(&note, "note", "", "optional disposition note")
	return cmd
}
