package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ifc7/ifc/internal/project"
)

var addCmd = &cobra.Command{
	Use:   "add [path] [name]",
	Short: "Add a locally owned interface to your project.",
	Long: `
Add a locally owned interface to your project.

`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := project.Load()
		if err != nil {
			return fmt.Errorf("error loading project config: %w", err)
		}
		err = proj.Add(cmd.Context(), project.AddParams{
			Path: args[0],
			Name: args[1],
		})
		if err != nil {
			return fmt.Errorf("error adding locally owned interface: %w", err)
		}
		err = proj.Write()
		if err != nil {
			return fmt.Errorf("error writing project changes: %w", err)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(addCmd)
}
