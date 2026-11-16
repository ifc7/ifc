package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ifc7/ifc/internal/project"
	"github.com/ifc7/ifc/internal/ui"
)

var commitCmd = &cobra.Command{
	Use:   "commit",
	Short: "Record owned interface files into the local manifest",
	Long: `
Commit each owned interface file on disk into .ifc/manifest.json as the latest
revision (checksum + base64 specification).

May prompt for a description (new interfaces) or revision notes (changes).
--notes sets the notes for every revision created and skips the notes prompt.

With --non-interactive, never prompts: notes default to empty, and owned
interfaces without a ref are skipped with a warning.

Does not push to the hub; run ifc push afterward. Does not take arguments.
`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := project.Load()
		if err != nil {
			return fmt.Errorf("error loading project config: %w", err)
		}
		params := project.CommitParams{
			NonInteractive: nonInteractive(cmd),
			Warn: func(message string) {
				ui.Fwarnln(cmd.ErrOrStderr(), "%s", message)
			},
		}
		if cmd.Flags().Changed("notes") {
			notes, err := cmd.Flags().GetString("notes")
			if err != nil {
				return err
			}
			params.Notes = &notes
		}
		err = proj.Commit(cmd.Context(), params)
		if err != nil {
			return err
		}
		err = proj.Write()
		if err != nil {
			return fmt.Errorf("error writing project changes: %w", err)
		}
		return nil
	},
}

func init() {
	commitCmd.Flags().String("notes", "", "notes for every revision created by this commit")
	rootCmd.AddCommand(commitCmd)
}
