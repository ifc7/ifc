package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/ifc7/ifc/internal"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "ifc",
	Short: "Manage software interfaces with the ifc7.dev hub",
	Long: fmt.Sprintf(`
ifc manages OpenAPI and JSON Schema interfaces locally and on ifc7.dev.

Typical flow: init → add/scan → commit → login → push.
Track remote interfaces with use + fetch. Inspect local changes with status/diff.

https://ifc7.dev

Build Version: %s
Git Commit: %s
Build Time: %s`,
		internal.BuildVersion, internal.GitCommit, internal.BuildTime),
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.CompletionOptions.HiddenDefaultCmd = true
	//cobra.OnInitialize(initConfig)
	//rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./ifc/config.yaml)")
}
