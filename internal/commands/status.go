package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ifc7/ifc/internal/project"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show whether owned interfaces differ from the local manifest.",
	Long: `
Compare each owned interface file on disk against the latest revision stored in
the local .ifc/manifest.json and report clean, modified, new, or missing status.
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := project.Load()
		if err != nil {
			return fmt.Errorf("error loading project config: %w", err)
		}
		statuses, err := proj.Status(cmd.Context())
		if err != nil {
			return fmt.Errorf("error checking interface status: %w", err)
		}
		fmt.Print(project.FormatStatusReport(statuses))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
