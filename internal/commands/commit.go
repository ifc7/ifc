package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ifc7/ifc/internal/project"
)

var commitCmd = &cobra.Command{
	Use:   "commit",
	Short: "Commit changes to locally owned interfaces to local manifest.",
	Long: `
Commit changes to locally owned interfaces to local manifest.

`,
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := project.Load()
		if err != nil {
			return fmt.Errorf("error loading project config: %w", err)
		}
		err = proj.Commit(cmd.Context(), project.CommitParams{})
		if err != nil {
			return fmt.Errorf("error committing interfaces: %w", err)
		}
		err = proj.Write()
		if err != nil {
			return fmt.Errorf("error writing project changes: %w", err)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(commitCmd)
}
