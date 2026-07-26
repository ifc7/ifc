package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ifc7/ifc/internal/project"
)

var scanCmd = &cobra.Command{
	Use:   "scan [path]",
	Short: "Find untracked OpenAPI / JSON Schema files and add them to the project.",
	Long: `
Recursively search for valid OpenAPI or JSON Schema documents (YAML or JSON)
that are not already listed in ifc.yaml. Present any matches so they can be
added as owned interfaces.

If path is provided, the search starts in that subdirectory; otherwise the
current directory is used.
`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := project.Load()
		if err != nil {
			return fmt.Errorf("error loading project config: %w", err)
		}
		root := "."
		if len(args) == 1 {
			root = args[0]
		}
		messages, err := proj.Scan(cmd.Context(), root)
		if err != nil {
			return fmt.Errorf("error scanning for interfaces: %w", err)
		}
		if err := proj.Write(); err != nil {
			return fmt.Errorf("error writing project changes: %w", err)
		}
		for _, message := range messages {
			fmt.Println(message)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(scanCmd)
}
