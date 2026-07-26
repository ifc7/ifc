package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ifc7/ifc/internal/project"
)

var compareCmd = &cobra.Command{
	Use:   "compare <before> <after>",
	Short: "Compare two interface specifications for breaking changes",
	Long: `
Run the default change-detector plugin between two specifications.

Arguments may be owned interface names from ifc.yaml or paths to specification
files. Both documents must share the same interface type.

Exit status is non-zero only when a plugin fails to run or input is invalid.
breaking=true does not fail the command.
`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		before := args[0]
		after := args[1]
		if proj, err := project.Load(); err == nil {
			before, err = proj.ResolveComparePath(before)
			if err != nil {
				return fmt.Errorf("before: %w", err)
			}
			after, err = proj.ResolveComparePath(after)
			if err != nil {
				return fmt.Errorf("after: %w", err)
			}
		}
		result, err := project.CompareFiles(before, after)
		if err != nil {
			return err
		}
		fmt.Printf("before: %s\n", result.Before)
		fmt.Printf("after:  %s\n", result.After)
		fmt.Printf("plugin: %s\n", result.PluginID)
		fmt.Printf("breaking: %v\n", result.Output.Breaking)
		if result.Output.Raw != "" {
			fmt.Printf("\n%s", result.Output.Raw)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(compareCmd)
}
