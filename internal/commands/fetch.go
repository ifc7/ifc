package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ifc7/ifc/internal/project"
)

var fetchCmd = &cobra.Command{
	Use:   "fetch",
	Short: "Fetch copies of external interfaces you are tracking from a remote hub.",
	Long: `
Fetch copies of external interfaces you are tracking from a remote hub.

`,
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := project.Load()
		if err != nil {
			return fmt.Errorf("error loading project config: %w", err)
		}
		err = proj.Fetch(cmd.Context(), project.FetchParams{})
		if err != nil {
			return fmt.Errorf("error fetching interfaces: %w", err)
		}
		err = proj.Write()
		if err != nil {
			return fmt.Errorf("error writing project changes: %w", err)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(fetchCmd)
}
