package reportcmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dock-at-the-yards/shipyard-cli/cmd/cmdline/commands"
	"github.com/dock-at-the-yards/shipyard-core/pkg/baseline"
	"github.com/dock-at-the-yards/shipyard-core/pkg/compare"
	"github.com/dock-at-the-yards/shipyard-core/pkg/gate"
	"github.com/dock-at-the-yards/shipyard-core/pkg/report"
	"github.com/dock-at-the-yards/shipyard-core/pkg/results"

	"github.com/spf13/cobra"
)

var ReportCmd = &cobra.Command{
	Use:   "report [path]",
	Short: "Re-render markdown/JSON/JUnit reports from an existing run",
	Long: `Report re-renders the human reports from an existing rollout results file,
so ` + "`run`" + ` and ` + "`report`" + ` can be different CI jobs.

By default it reads the newest run under <path>/rollouts and, when a baseline
exists at baselines/<name>.json, includes the delta table.`,
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		targetPath := "."
		if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
			targetPath = strings.TrimSpace(args[0])
		}

		resultsFlag, _ := cmd.Flags().GetString("results")
		formatFlag, _ := cmd.Flags().GetString("format")
		outFlag, _ := cmd.Flags().GetString("out")
		baselineFlag, _ := cmd.Flags().GetString("baseline")
		baselineName, _ := cmd.Flags().GetString("baseline-name")

		defer func() {
			_ = cmd.Flags().Set("results", "")
			_ = cmd.Flags().Set("format", "markdown")
			_ = cmd.Flags().Set("out", "")
			_ = cmd.Flags().Set("baseline", "")
			_ = cmd.Flags().Set("baseline-name", "main")
		}()

		// Resolve the source results.
		resultsPath := strings.TrimSpace(resultsFlag)
		if resultsPath == "" {
			p, err := results.LatestResults(targetPath)
			if err != nil {
				return err
			}
			resultsPath = p
		}
		res, err := results.LoadSuiteResults(resultsPath)
		if err != nil {
			return err
		}

		// Optional baseline for the delta table.
		var comparison *compare.Report
		bPath := strings.TrimSpace(baselineFlag)
		if bPath == "" {
			candidate := baseline.Path(targetPath, baselineName)
			if _, err := os.Stat(candidate); err == nil {
				bPath = candidate
			}
		}
		if bPath != "" {
			bl, err := baseline.Load(bPath)
			if err != nil {
				return err
			}
			policy, _ := gate.LoadDefault(targetPath, res.TargetType)
			cmp, cmpErr := compare.Compare(bl, res, compare.Options{AllowSuiteMismatch: true, Policy: policy})
			if cmpErr != nil {
				return cmpErr
			}
			comparison = cmp
		}

		outDir := strings.TrimSpace(outFlag)
		if outDir == "" {
			outDir = targetPath
		}

		wrote := 0
		for _, f := range strings.Split(formatFlag, ",") {
			switch strings.TrimSpace(f) {
			case "markdown", "md":
				out := report.Markdown(res, report.Options{Comparison: comparison})
				if outDir == "-" {
					cmd.Println(out)
					wrote++
					continue
				}
				path := filepath.Join(outDir, "shipyard-report.md")
				if err := os.WriteFile(path, []byte(out), 0644); err != nil {
					return err
				}
				cmd.Printf("report written: %s\n", filepath.Clean(path))
				wrote++
			case "json":
				out, err := report.JSON(res, report.Options{Comparison: comparison})
				if err != nil {
					return err
				}
				if outDir == "-" {
					cmd.Println(string(out))
					wrote++
					continue
				}
				path := filepath.Join(outDir, "shipyard-report.json")
				if err := os.WriteFile(path, append(out, '\n'), 0644); err != nil {
					return err
				}
				cmd.Printf("report written: %s\n", filepath.Clean(path))
				wrote++
			case "junit":
				out, err := report.JUnit(res)
				if err != nil {
					return err
				}
				if outDir == "-" {
					cmd.Println(string(out))
					wrote++
					continue
				}
				path := filepath.Join(outDir, "shipyard-junit.xml")
				if err := os.WriteFile(path, out, 0644); err != nil {
					return err
				}
				cmd.Printf("report written: %s\n", filepath.Clean(path))
				wrote++
			default:
				return commands.WithExitCode(2, fmt.Errorf("unknown report format %q (use markdown, json, junit)", f))
			}
		}
		if wrote == 0 {
			return commands.WithExitCode(2, fmt.Errorf("no report formats selected"))
		}
		return nil
	},
}

func init() {
	ReportCmd.Flags().String("results", "", "Path to a results.json (default: newest run under the target)")
	ReportCmd.Flags().String("format", "markdown", "Comma-separated: markdown,json,junit (use --out - to print)")
	ReportCmd.Flags().String("out", "", "Output directory (default: the target path; '-' prints to stdout)")
	ReportCmd.Flags().String("baseline", "", "Baseline JSON for the delta table")
	ReportCmd.Flags().String("baseline-name", "main", "Baseline name to look up under <path>/baselines")

	commands.Register(ReportCmd)
}
