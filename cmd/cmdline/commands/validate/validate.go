package validatecmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dock-at-the-yards/shipyard-cli/cmd/cmdline/commands"
	"github.com/dock-at-the-yards/shipyard-core/pkg/validate"

	"github.com/spf13/cobra"
)

var ValidateCmd = &cobra.Command{
	Use:   "validate [path]",
	Short: "Statically lint an eval or evalset before any container starts",
	Long: `Validate parses every manifest, resolves every referenced file, checks the
gate policy and rubric sanity, and probes the rubric script. It catches a broken
eval before any container starts (and before any tokens are spent).

Exit code 0 when no errors are found; warnings do not fail.`,
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		targetPath := "."
		if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
			targetPath = strings.TrimSpace(args[0])
		}
		jsonFlag, _ := cmd.Flags().GetBool("json")
		skipProbe, _ := cmd.Flags().GetBool("skip-rubric-probe")

		defer func() {
			_ = cmd.Flags().Set("json", "false")
			_ = cmd.Flags().Set("skip-rubric-probe", "false")
		}()

		result, err := validate.Validate(targetPath, validate.Options{SkipRubricProbe: skipProbe})
		if err != nil {
			return err
		}

		if jsonFlag {
			data, jerr := json.MarshalIndent(result, "", "  ")
			if jerr != nil {
				return jerr
			}
			cmd.Println(string(data))
		} else {
			cmd.Printf("Validating %s (%s, %d eval(s))\n", result.Target, result.Type, result.Evals)
			if len(result.Problems) == 0 {
				cmd.Println("OK — no problems found")
			}
			for _, p := range result.Problems {
				cmd.Printf("%s: %s: %s\n", strings.ToUpper(string(p.Severity)), p.Path, p.Message)
			}
		}

		if !result.OK() {
			return commands.WithExitCode(1, fmt.Errorf("validation failed"))
		}
		return nil
	},
}

func init() {
	ValidateCmd.Flags().Bool("json", false, "Print findings as JSON")
	ValidateCmd.Flags().Bool("skip-rubric-probe", false, "Skip executing the rubric with --help")
	commands.Register(ValidateCmd)
}
