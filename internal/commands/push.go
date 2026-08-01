package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ifc7/ifc/internal/project"
	"github.com/ifc7/ifc/internal/ui"
)

var pushCmd = &cobra.Command{
	Use:   "push [ref]",
	Short: "Push committed owned interfaces to the hub",
	Long: `
Push owned interfaces from the local manifest to the ifc7.dev hub.

Arguments:
  ref  Optional hub ref of one owned interface (as in ifc.yaml); omit to push all

Requires login and a prior ifc commit. When an owned interface has no ref yet,
creates it on the hub and prompts for the owner (user or organization).
`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := project.Load()
		if err != nil {
			return fmt.Errorf("error loading project config: %w", err)
		}
		params := project.PushParams{}
		if len(args) == 1 {
			params.Name = args[0]
		}
		messages, pushErr := proj.Push(cmd.Context(), params)
		// Persist any successful updates even if a later interface failed.
		// Otherwise the next push retries work that already landed on the server.
		if writeErr := proj.Write(); writeErr != nil {
			if pushErr != nil {
				return fmt.Errorf("error pushing interfaces: %w (also failed to write project changes: %v)", pushErr, writeErr)
			}
			return fmt.Errorf("error writing project changes: %w", writeErr)
		}
		ui.PrintMessages(messages)
		if pushErr != nil {
			return fmt.Errorf("error pushing interfaces: %w", pushErr)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(pushCmd)
}
