package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ifc7/ifc/internal/project"
	"github.com/ifc7/ifc/internal/ui"
)

var compareCmd = &cobra.Command{
	Use:   "compare <before> <after>",
	Short: "Compare two specifications for breaking changes",
	Long: `
Run the default change-detector plugin between two specifications.

Arguments:
  before  Owned interface name or file path (baseline)
  after   Owned interface name or file path (candidate)

Both documents must share the same interface type (OpenAPI or JSON Schema).
File paths work even without an ifc.yaml project.

Prints plugin id and breaking=true/false. Use --verbose to print the plugin's
raw change report. Exit status is non-zero only when a plugin fails or input is
invalid; breaking changes do not fail the command.
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
		fmt.Printf("%s %s\n", ui.KeyHints("before:  "), result.Before)
		fmt.Printf("%s %s\n", ui.KeyHints("after:   "), result.After)
		fmt.Printf("%s %s\n", ui.KeyHints("plugin:  "), result.PluginID)
		fmt.Printf("%s %s\n", ui.KeyHints("breaking:"), ui.FormatBreaking(result.Output.Breaking))
		if compareVerbose && result.Output.Raw != "" {
			fmt.Printf("\n%s", result.Output.Raw)
		}
		return nil
	},
}

var compareVerbose bool

func init() {
	compareCmd.Flags().BoolVarP(&compareVerbose, "verbose", "v", false, "print the plugin raw change report")
	rootCmd.AddCommand(compareCmd)
}
