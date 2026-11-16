package commands

import (
	"encoding/json"
	"fmt"
	"io"

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

Requires login (or IFC_TOKEN) and a prior ifc commit. When an owned interface
has no ref yet, creates it on the hub and prompts for the owner (user or
organization). With --non-interactive, such interfaces are skipped instead.

With --json, prints one JSON document to stdout when finished, even on failure:
  {"interfaces": [{"name", "ref", "path", "status", "revisionIds", "reason", "error"}]}
where status is pushed, up_to_date, skipped, failed or not_attempted. Messages
go to stderr. Exits 0 only if every interface not skipped was pushed or up to date.
`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		asJSON, err := cmd.Flags().GetBool("json")
		if err != nil {
			return err
		}
		// In JSON mode stdout carries only the report.
		messageOut := cmd.OutOrStdout()
		if asJSON {
			messageOut = cmd.ErrOrStderr()
		}
		report, pushErr := runPush(cmd, args)
		ui.FprintMessages(messageOut, report.Messages)
		if asJSON {
			if err := writePushReport(cmd.OutOrStdout(), report); err != nil {
				return err
			}
		}
		return pushErr
	},
}

// runPush loads the project, pushes and persists the result. The report is never nil.
func runPush(cmd *cobra.Command, args []string) (*project.PushReport, error) {
	proj, err := project.Load()
	if err != nil {
		return project.NewPushReport(), fmt.Errorf("error loading project config: %w", err)
	}
	params := project.PushParams{NonInteractive: nonInteractive(cmd)}
	if len(args) == 1 {
		params.Name = args[0]
	}
	report, pushErr := proj.Push(cmd.Context(), params)
	// Persist any successful updates even if a later interface failed.
	// Otherwise the next push retries work that already landed on the server.
	if writeErr := proj.Write(); writeErr != nil {
		if pushErr != nil {
			return report, fmt.Errorf("error pushing interfaces: %w (also failed to write project changes: %v)", pushErr, writeErr)
		}
		return report, fmt.Errorf("error writing project changes: %w", writeErr)
	}
	if pushErr != nil {
		return report, fmt.Errorf("error pushing interfaces: %w", pushErr)
	}
	return report, nil
}

func writePushReport(w io.Writer, report *project.PushReport) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(report); err != nil {
		return fmt.Errorf("error writing JSON report: %w", err)
	}
	return nil
}

func init() {
	pushCmd.Flags().Bool("json", false, "print a JSON report of every owned interface to stdout")
	rootCmd.AddCommand(pushCmd)
}
