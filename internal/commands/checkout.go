package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ifc7/ifc/internal/project"
)

var checkoutForce bool

var checkoutCmd = &cobra.Command{
	Use:   "checkout [name|slug]...",
	Short: "Update owned interface files from the local manifest",
	Long: `
Write owned interface working-tree files from the latest revision in
.ifc/manifest.json.

Arguments:
  name|slug  Optional owned interface names or slugs; omit to update all

Local only — does not contact the hub. Run ifc fetch first to refresh the
manifest from the server, then ifc checkout to update files on disk.

Missing files are created. Files that already match the manifest are left
unchanged. Files with local modifications are skipped unless --force is set.
`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := project.Load()
		if err != nil {
			return fmt.Errorf("error loading project config: %w", err)
		}
		messages, err := proj.Checkout(cmd.Context(), project.CheckoutParams{
			Targets: args,
			Force:   checkoutForce,
		})
		for _, message := range messages {
			fmt.Println(message)
		}
		if err != nil {
			return err
		}
		return nil
	},
}

func init() {
	checkoutCmd.Flags().BoolVar(&checkoutForce, "force", false, "overwrite local files that differ from the manifest")
	rootCmd.AddCommand(checkoutCmd)
}
