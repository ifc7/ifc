package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ifc7/ifc/internal/project"
)

var useCmd = &cobra.Command{
	Use:   "use",
	Short: "Use an externally owned interface in your project.",
	Long: `
Use an externally owned interface in your project.

`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := project.Load()
		if err != nil {
			return fmt.Errorf("error loading project config: %w", err)
		}
		err = proj.Use(cmd.Context(), project.UseParams{
			Ref: args[0],
		})
		if err != nil {
			return fmt.Errorf("error using remote interfaces: %w", err)
		}
		err = proj.Write()
		if err != nil {
			return fmt.Errorf("error writing project changes: %w", err)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(useCmd)
}
