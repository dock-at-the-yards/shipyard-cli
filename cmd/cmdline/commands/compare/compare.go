package comparecmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dock-at-the-yards/shipyard-cli/cmd/cmdline/commands"
	"github.com/dock-at-the-yards/shipyard-core/common/types"
	"github.com/dock-at-the-yards/shipyard-core/pkg/baseline"
	"github.com/dock-at-the-yards/shipyard-core/pkg/compare"
	"github.com/dock-at-the-yards/shipyard-core/pkg/engine"
	"github.com/dock-at-the-yards/shipyard-core/pkg/gate"
	"github.com/dock-at-the-yards/shipyard-core/pkg/results"

	"github.com/spf13/cobra"
)

var CompareCmd = &cobra.Command{
	Use:   "compare [path]",
	Short: "Compare a candidate run against a committed baseline (the delta gate)",
	Long: `Compare the newest run under <path>/rollouts (or --candidate/--run-id)
against a committed baseline and report a per-eval delta table:

  improved | same | regressed | flaked | errored

Exit codes:
  0  gate passed (no regressions, flakes, or infra errors)
  1  gate blocked
  2  usage error (suite mismatch without --allow-suite-mismatch, missing inputs)

Gate rules: infra errors fail closed and are labeled infra; a pass->fail flip
always blocks; flakes block by default; score drops must exceed both the score
budget and the 95% t-interval noise margin (k>=3) to block.`,
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		targetPath := "."
		if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
			targetPath = strings.TrimSpace(args[0])
		}

		baselineFlag, _ := cmd.Flags().GetString("baseline")
		candidateFlag, _ := cmd.Flags().GetString("candidate")
		runID, _ := cmd.Flags().GetString("run-id")
		jsonFlag, _ := cmd.Flags().GetBool("json")
		allowMismatch, _ := cmd.Flags().GetBool("allow-suite-mismatch")
		policyFlag, _ := cmd.Flags().GetString("policy")

		defer func() {
			_ = cmd.Flags().Set("baseline", "")
			_ = cmd.Flags().Set("candidate", "")
			_ = cmd.Flags().Set("run-id", "")
			_ = cmd.Flags().Set("json", "false")
			_ = cmd.Flags().Set("allow-suite-mismatch", "false")
			_ = cmd.Flags().Set("policy", "")
		}()

		// Resolve the baseline.
		baselinePath := strings.TrimSpace(baselineFlag)
		if baselinePath == "" {
			baselinePath = baseline.Path(targetPath, "main")
		}
		if _, err := os.Stat(baselinePath); err != nil {
			return commands.WithExitCode(2, fmt.Errorf("baseline not found at %s (save one with `shipyard baseline save`)", baselinePath))
		}
		bl, err := baseline.Load(baselinePath)
		if err != nil {
			return commands.WithExitCode(2, err)
		}

		// Resolve the candidate.
		var candidatePath string
		switch {
		case strings.TrimSpace(candidateFlag) != "":
			candidatePath = strings.TrimSpace(candidateFlag)
		case strings.TrimSpace(runID) != "":
			p, err := results.FindResults(targetPath, strings.TrimSpace(runID))
			if err != nil {
				return commands.WithExitCode(2, err)
			}
			candidatePath = p
		default:
			p, err := results.LatestResults(targetPath)
			if err != nil {
				return commands.WithExitCode(2, err)
			}
			candidatePath = p
		}
		candidate, err := results.LoadSuiteResults(candidatePath)
		if err != nil {
			return commands.WithExitCode(2, err)
		}

		// Resolve the policy: explicit file > target manifest > defaults.
		var policy types.GateConfig
		if strings.TrimSpace(policyFlag) != "" {
			policy, err = gate.LoadFile(strings.TrimSpace(policyFlag))
			if err != nil {
				return commands.WithExitCode(2, err)
			}
		} else {
			policy, err = gate.LoadDefault(targetPath, candidate.TargetType)
			if err != nil {
				return commands.WithExitCode(2, err)
			}
			if candidate.TargetType == "" {
				// Fall back to detecting from the filesystem.
				if t, derr := engine.DetectTarget(targetPath); derr == nil {
					policy, _ = gate.LoadDefault(targetPath, t)
				}
			}
		}

		report, err := compare.Compare(bl, candidate, compare.Options{
			AllowSuiteMismatch: allowMismatch,
			Policy:             policy,
		})
		if err != nil {
			return commands.WithExitCode(2, err)
		}

		if jsonFlag {
			data, jerr := json.MarshalIndent(report, "", "  ")
			if jerr != nil {
				return jerr
			}
			cmd.Println(string(data))
		} else {
			printReport(cmd, report, baselinePath, candidatePath)
		}

		if report.ExitCode() != 0 {
			return commands.WithExitCode(report.ExitCode(), fmt.Errorf("gate blocked: %s", strings.Join(report.BlockedEvals, ", ")))
		}
		return nil
	},
}

// printReport renders the human delta table.
func printReport(cmd *cobra.Command, r *compare.Report, baselinePath, candidatePath string) {
	cmd.Printf("baseline:  %s (%s, k=%d, digest %s)\n", filepath.Clean(baselinePath), r.BaselineName, baselineK(r), shortDigest(r.BaselineDigest))
	cmd.Printf("candidate: %s (run %s, k=%d, digest %s)\n", filepath.Clean(candidatePath), r.CandidateRunID, candidateK(r), shortDigest(r.CandidateDigest))
	cmd.Printf("suite:     %s\n", map[bool]string{true: "match", false: "MISMATCH"}[r.SuiteMatch])
	if r.SuiteMismatchNote != "" {
		cmd.Printf("           %s\n", r.SuiteMismatchNote)
	}
	if r.Refusal != "" {
		cmd.Println()
		cmd.Println(r.Refusal)
		cmd.Println()
	}
	cmd.Println()

	fmtStr := "%-34s %8s %8s %8s  %-10s %10s  %s\n"
	cmd.Printf(fmtStr, "eval", "baseline", "candidate", "delta", "status", "noise", "reason")
	for _, e := range r.Evals {
		margin := "—"
		if e.NoiseMarginPct > 0 {
			margin = fmt.Sprintf("±%.1f%%", e.NoiseMarginPct)
		} else if e.NoiseMarginAbs > 0 {
			margin = fmt.Sprintf("±%.3f", e.NoiseMarginAbs)
		}
		reason := e.Reason
		if e.Divergence != nil && e.Blocks {
			reason = strings.TrimSpace(reason + " " + divergenceText(e.Divergence))
		}
		cmd.Printf(fmtStr, truncate(e.Name, 34), fmt.Sprintf("%.2f", e.BaselineScore), fmt.Sprintf("%.2f", e.CandidateScore),
			fmt.Sprintf("%+.2f", e.Delta), string(e.Status), margin, reason)
	}
	cmd.Println()
	if r.NoiseNote != "" {
		cmd.Printf("noise: %s\n", r.NoiseNote)
	}
	cmd.Println(r.SummaryLine())
}

func divergenceText(d *compare.Divergence) string {
	var parts []string
	if len(d.MissingToolCalls) > 0 {
		parts = append(parts, fmt.Sprintf("missing tool call %q", strings.Join(d.MissingToolCalls, `", "`)))
	}
	if len(d.NewToolCalls) > 0 {
		parts = append(parts, fmt.Sprintf("new tool call %q", strings.Join(d.NewToolCalls, `", "`)))
	}
	if d.FirstDivergentStep > 0 {
		parts = append(parts, fmt.Sprintf("first divergence at step %d", d.FirstDivergentStep))
	}
	if d.StepCountDelta != 0 {
		parts = append(parts, fmt.Sprintf("steps %+d", d.StepCountDelta))
	}
	if len(parts) == 0 {
		return ""
	}
	return "(" + strings.Join(parts, "; ") + ")"
}

func baselineK(r *compare.Report) int {
	if r.BaselineK > 0 {
		return r.BaselineK
	}
	if len(r.Evals) > 0 {
		return r.Evals[0].BaselineK
	}
	return 0
}

func candidateK(r *compare.Report) int {
	if r.CandidateK > 0 {
		return r.CandidateK
	}
	if len(r.Evals) > 0 {
		return r.Evals[0].CandidateK
	}
	return 0
}

func shortDigest(d string) string {
	d = strings.TrimPrefix(d, "sha256:")
	if len(d) > 12 {
		return d[:12]
	}
	if d == "" {
		return "—"
	}
	return d
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func init() {
	CompareCmd.Flags().String("baseline", "", "Path to baselines/<name>.json (default: baselines/main.json under the target)")
	CompareCmd.Flags().String("candidate", "", "Path to a candidate results.json (default: newest run under the target)")
	CompareCmd.Flags().String("run-id", "", "Use the candidate run with this id")
	CompareCmd.Flags().Bool("json", false, "Print the full comparison as JSON")
	CompareCmd.Flags().Bool("allow-suite-mismatch", false, "Compare only the shared evals when suite digests differ")
	CompareCmd.Flags().String("policy", "", "Gate policy TOML file (default: [gate] in evalset.toml / eval.toml)")

	commands.Register(CompareCmd)
}
