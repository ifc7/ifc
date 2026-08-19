package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ifc7/ifc/internal/pkg/auth"
	"github.com/ifc7/ifc/internal/ui"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate with the ifc7.dev hub",
	Long: `
Obtain API credentials for ifc7.dev via browser device login.

Can be run from any directory; does not require an initialized project.
Stores credentials in the user config directory for fetch and push. Prints a
URL to open and complete authentication.
`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := auth.NewCredentialsService()
		if err != nil {
			return fmt.Errorf("failed to initialize credentials service: %w", err)
		}
		err = client.Login(cmd.Context())
		if err != nil {
			return fmt.Errorf("failed to login: %w", err)
		}
		ui.Successln("Logged in successfully.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(loginCmd)
}
