package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ifc7/ifc/internal/project"
)

var diffCmd = &cobra.Command{
	Use:   "diff <name|slug>",
	Short: "Show a unified diff of an owned interface vs the manifest",
	Long: `
Print a unified text diff of an owned interface file on disk against the
latest revision in .ifc/manifest.json.

Arguments:
  name|slug  Owned interface name from ifc.yaml, or its manifest slug

Prints nothing when the file matches the manifest. Local only.
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := project.Load()
		if err != nil {
			return fmt.Errorf("error loading project config: %w", err)
		}
		diff, err := proj.DiffOwned(args[0])
		if err != nil {
			return err
		}
		if diff != "" {
			fmt.Print(diff)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(diffCmd)
}
