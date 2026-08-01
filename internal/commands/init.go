package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ifc7/ifc/internal"
	"github.com/ifc7/ifc/internal/pkg/fileio"
	"github.com/ifc7/ifc/internal/project"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Create ifc.yaml and .ifc/ in the current directory",
	Long: `
Create a new IFC project by writing ifc.yaml and .ifc/manifest.json.

Run once per repository. Fails if ifc.yaml already exists.
`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !projectIsInitialized() {
			proj, err := project.New()
			if err != nil {
				return fmt.Errorf("error creating project: %w", err)
			}
			err = proj.Initialize()
			if err != nil {
				return fmt.Errorf("error initializing project: %w", err)
			}
			return nil
		}
		return fmt.Errorf("project is already initialized")
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}

func projectIsInitialized() bool {
	if !fileio.FileExists(internal.IfcConfigFile) {
		return false
	}
	return true
}
