package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ifc7/ifc/internal/project"
)

var commitCmd = &cobra.Command{
	Use:   "commit",
	Short: "Record owned interface files into the local manifest",
	Long: `
Commit each owned interface file on disk into .ifc/manifest.json as the latest
revision (checksum + base64 specification).

May prompt for a description (new interfaces) or revision notes (changes).
Does not push to the hub; run ifc push afterward. Does not take arguments.
`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := project.Load()
		if err != nil {
			return fmt.Errorf("error loading project config: %w", err)
		}
		err = proj.Commit(cmd.Context(), project.CommitParams{})
		if err != nil {
			return fmt.Errorf("error committing interfaces: %w", err)
		}
		err = proj.Write()
		if err != nil {
			return fmt.Errorf("error writing project changes: %w", err)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(commitCmd)
}
