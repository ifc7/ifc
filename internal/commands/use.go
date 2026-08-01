package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ifc7/ifc/internal/project"
)

var useCmd = &cobra.Command{
	Use:   "use <ref>",
	Short: "Track a remote hub interface in this project",
	Long: `
Add an externally owned interface from the hub to the use list in ifc.yaml.

Arguments:
  ref  Hub reference: ifc7.dev/i/<owner>/<name> or interface_<id>

Resolves the ref against the hub (requires login). Run ifc fetch to pull
metadata into .ifc/manifest.json.
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
