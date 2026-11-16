package commands

import (
	"os"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

const nonInteractiveFlag = "non-interactive"

// stdinIsTerminal reports whether stdin is a TTY. Tests replace it.
var stdinIsTerminal = func() bool {
	fd := os.Stdin.Fd()
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}

// nonInteractive reports whether cmd must never prompt: --non-interactive was given,
// CI=true, or stdin is not a terminal.
func nonInteractive(cmd *cobra.Command) bool {
	if on, err := cmd.Flags().GetBool(nonInteractiveFlag); err == nil && on {
		return true
	}
	if ci := strings.TrimSpace(os.Getenv("CI")); strings.EqualFold(ci, "true") || ci == "1" {
		return true
	}
	return !stdinIsTerminal()
}
