package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ifc7/ifc/internal/pkg/auth"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate with the IFC7 server",
	Long: `
Authenticate with the IFC7 server, obtaining credentials for API access.

You will be prompted to open a URL in your browser and log in.

`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !projectIsInitialized() {
			return fmt.Errorf("project is not initialized")
		}
		client, err := auth.NewCredentialsService()
		if err != nil {
			return fmt.Errorf("failed to initialize credentials service: %w", err)
		}
		err = client.Login(cmd.Context())
		if err != nil {
			return fmt.Errorf("failed to login: %w", err)
		}
		err = client.WriteCredentials()
		if err != nil {
			return fmt.Errorf("failed to write credentials: %w", err)
		}
		fmt.Println("\nLogged in successfully.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(loginCmd)
}
