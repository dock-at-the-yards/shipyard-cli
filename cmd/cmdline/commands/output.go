package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

// PrintStdout writes machine-readable output to the command's stdout.
//
// Cobra's Print*/Println helpers default to stderr, which silently corrupts
// shell redirection like `shipyard compare --json > compare.json` and every CI
// consumer reading stdout. Human-oriented text may keep using cmd.Print*, but
// anything a machine parses must go through here.
func PrintStdout(cmd *cobra.Command, s string) {
	fmt.Fprintln(cmd.OutOrStdout(), s)
}
