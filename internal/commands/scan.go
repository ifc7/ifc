package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ifc7/ifc/internal/project"
)

var scanCmd = &cobra.Command{
	Use:   "scan [path]",
	Short: "Find untracked specs and add them as owned interfaces",
	Long: `
Recursively find OpenAPI or JSON Schema files (YAML or JSON) not already
listed in ifc.yaml, then interactively select which to add as owned interfaces.

Arguments:
  path  Optional directory to search (default: current directory)

Skips common vendor/cache directories (.git, node_modules, vendor, etc.).
Only updates ifc.yaml; run ifc commit to snapshot selected files.
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
