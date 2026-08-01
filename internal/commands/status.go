package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ifc7/ifc/internal/project"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show owned interface status vs the local manifest",
	Long: `
Compare each owned interface file on disk to the latest revision in
.ifc/manifest.json.

Reports status, name, slug, and path for each owned interface.
Status is one of: clean, modified, new, missing, or error.
Local only; does not contact the hub. Does not take arguments.
`,
	Args: cobra.NoArgs,
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
