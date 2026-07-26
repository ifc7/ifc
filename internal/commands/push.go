package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ifc7/ifc/internal/project"
)

var pushCmd = &cobra.Command{
	Use:   "push",
	Short: "Push the latest copies of interfaces managed by this project to a remote hub.",
	Long: `
Push the latest copies of interfaces managed by this project to a remote hub.

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
		for _, message := range messages {
			fmt.Println(message)
		}
		if pushErr != nil {
			return fmt.Errorf("error pushing interfaces: %w", pushErr)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(pushCmd)
}
