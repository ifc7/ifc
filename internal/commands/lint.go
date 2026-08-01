package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ifc7/ifc/internal/project"
)

var lintCmd = &cobra.Command{
	Use:   "lint [name|path]...",
	Short: "Lint interface specifications",
	Long: `
Run the default linter plugin on one or more specifications.

Arguments:
  name|path  Owned interface name from ifc.yaml, or a file path
             With no arguments, lints all owned interfaces
             File paths work even without an ifc.yaml project

Prints plugin id and quality score. Exit status is non-zero only when a plugin
fails or input is invalid; a low score does not fail the command.
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := project.Load()
		if err != nil {
			// Allow linting bare file paths without a project.
			if len(args) == 0 {
				return fmt.Errorf("error loading project config: %w", err)
			}
			for _, path := range args {
				if err := printLint(path); err != nil {
					return err
				}
			}
			return nil
		}
		paths, err := proj.ResolveLintTargets(args)
		if err != nil {
			return err
		}
		for _, path := range paths {
			if err := printLint(path); err != nil {
				return err
			}
		}
		return nil
	},
}

func printLint(path string) error {
	result, err := project.LintFile(path)
	if err != nil {
		return err
	}
	fmt.Printf("%s\n", result.Target)
	fmt.Printf("  plugin: %s\n", result.PluginID)
	fmt.Printf("  score:  %d\n", result.Output.Score)
	if result.Output.Raw != "" {
		fmt.Printf("\n%s", result.Output.Raw)
	}
	return nil
}

func init() {
	rootCmd.AddCommand(lintCmd)
}
