package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ifc7/ifc/internal/project"
)

var addCmd = &cobra.Command{
	Use:   "add <path> <name>",
	Short: "Track a local specification file as an owned interface",
	Long: `
Add a local specification file to ifc.yaml as an owned interface.

Arguments:
  path  Path to an OpenAPI or JSON Schema file
  name  Local name used by other ifc commands

Only updates ifc.yaml. Run ifc commit to snapshot the file into the manifest.
`,
	Args: cobra.ExactArgs(2),
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return []string{"yaml", "yml", "json"}, cobra.ShellCompDirectiveFilterFileExt
		}

		return nil, cobra.ShellCompDirectiveNoFileComp
	},

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
