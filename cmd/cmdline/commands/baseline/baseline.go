package baselinecmd

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/dock-at-the-yards/shipyard-cli/cmd/cmdline/commands"
	"github.com/dock-at-the-yards/shipyard-core/common/types"
	"github.com/dock-at-the-yards/shipyard-core/pkg/baseline"
	"github.com/dock-at-the-yards/shipyard-core/pkg/engine"
	"github.com/dock-at-the-yards/shipyard-core/pkg/results"

	"github.com/spf13/cobra"
)

// ActiveLoader can be swapped in tests to avoid touching the filesystem.
var ActiveLoader = results.LoadSuiteResults

var BaselineCmd = &cobra.Command{
	Use:   "baseline",
	Short: "Manage committed baselines (baselines/<name>.json)",
	Args:  cobra.NoArgs,
}

var saveCmd = &cobra.Command{
	Use:   "save [path]",
	Short: "Save a committed baseline from a completed run",
	Long: `Save baselines/<name>.json from the newest run under <path>/rollouts (or a
specific run with --run-id, or an explicit results file with --results).

The baseline is schema-stable, carries the suite digest it was taken from, the
repeat count k, and creation metadata. Commit it: compare refuses to compare
different suite digests unless --allow-suite-mismatch is passed.

The rule "only runs on main may regenerate a baseline" is enforced by the CI
workflow, not by this command.`,
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		targetPath := "."
		if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
			targetPath = strings.TrimSpace(args[0])
		}

		name, _ := cmd.Flags().GetString("name")
		runID, _ := cmd.Flags().GetString("run-id")
		resultsFlag, _ := cmd.Flags().GetString("results")
		jsonFlag, _ := cmd.Flags().GetBool("json")
		createdBy, _ := cmd.Flags().GetString("created-by")

		defer func() {
			_ = cmd.Flags().Set("name", "main")
			_ = cmd.Flags().Set("run-id", "")
			_ = cmd.Flags().Set("results", "")
			_ = cmd.Flags().Set("json", "false")
			_ = cmd.Flags().Set("created-by", "")
		}()

		// Resolve the source results file: explicit file > run id > latest run.
		var resultsPath string
		switch {
		case strings.TrimSpace(resultsFlag) != "":
			resultsPath = strings.TrimSpace(resultsFlag)
		case strings.TrimSpace(runID) != "":
			p, err := results.FindResults(targetPath, strings.TrimSpace(runID))
			if err != nil {
				return err
			}
			resultsPath = p
		default:
			p, err := results.LatestResults(targetPath)
			if err != nil {
				return err
			}
			resultsPath = p
		}

		res, err := ActiveLoader(resultsPath)
		if err != nil {
			return err
		}
		if res.SuiteDigest == "" {
			return fmt.Errorf("results %s has no suite digest; rerun with the v0.0.2 CLI", resultsPath)
		}

		bl := baseline.FromSuite(name, res, createdBy)
		if bl.CreatedBy == "" {
			bl.CreatedBy = "shipyard"
		}
		outPath := baseline.Path(targetPath, name)
		if err := baseline.Save(outPath, bl); err != nil {
			return err
		}

		if jsonFlag {
			data, err := json.MarshalIndent(bl, "", "  ")
			if err != nil {
				return err
			}
			commands.PrintStdout(cmd, string(data))
			return nil
		}

		cmd.Printf("baseline saved: %s\n", filepath.Clean(outPath))
		cmd.Printf("  name:   %s\n", bl.Name)
		cmd.Printf("  source: run %s (%s)\n", bl.SourceRunID, filepath.Clean(resultsPath))
		cmd.Printf("  k:      %d\n", bl.K)
		cmd.Printf("  digest: %s\n", bl.SuiteDigest)
		cmd.Printf("  verdict:%s\n", string(bl.Verdict))
		if bl.Verdict != types.VerdictPassed {
			cmd.Printf("warning: baseline recorded from a %s run — comparisons will treat it as the reference anyway\n", bl.Verdict)
		}
		return nil
	},
}

// targetTypeHint documents that eval/evalset detection happens at run time.
var _ = engine.DetectTarget

func init() {
	saveCmd.Flags().String("name", "main", "Baseline name (writes baselines/<name>.json)")
	saveCmd.Flags().String("run-id", "", "Use the run with this id (default: newest run)")
	saveCmd.Flags().String("results", "", "Use an explicit results.json path")
	saveCmd.Flags().Bool("json", false, "Print the baseline JSON to stdout")
	saveCmd.Flags().String("created-by", "", "Agent/person recording the baseline (defaults to 'shipyard')")

	BaselineCmd.AddCommand(saveCmd)
	commands.Register(BaselineCmd)
}
