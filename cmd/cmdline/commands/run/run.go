package run

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
	shiperrs "github.com/dock-at-the-yards/shipyard-core/pkg/errors"
	"github.com/dock-at-the-yards/shipyard-core/pkg/gate"
	"github.com/dock-at-the-yards/shipyard-core/pkg/report"
	"github.com/dock-at-the-yards/shipyard-core/pkg/results"

	"github.com/spf13/cobra"
)

// EngineRunFunc defines the legacy signature for the engine runner executor.
// Maintained for backward compatibility with existing tests.
type EngineRunFunc func(path string, runID ...string) (*types.RunSummary, error)

// EngineRunWithOptionsFunc defines the execution signature accepting full RunOptions.
type EngineRunWithOptionsFunc func(path string, opts types.RunOptions) (*types.RunSummary, error)

// EnginePlanFunc resolves a run plan without executing (--dry-run).
type EnginePlanFunc func(path string, opts types.RunOptions) (*engine.RunPlan, error)

// defaultRunner indicates whether ActiveRunner was customized by tests or callers.
var defaultRunner EngineRunFunc = engine.Run

var (
	// ActiveRunner is called by the run command. Defaults to engine.Run.
	ActiveRunner EngineRunFunc = defaultRunner

	// ActiveRunnerWithOptions can be set by tests or callers to inspect full RunOptions.
	ActiveRunnerWithOptions EngineRunWithOptionsFunc

	// ActivePlanner resolves the dry-run plan. Defaults to engine.ResolvePlan.
	ActivePlanner EnginePlanFunc = engine.ResolvePlan
)

var RunCmd = &cobra.Command{
	Use:          "run [path]",
	Short:        "Run an eval or evalset",
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		targetPath := "."
		if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
			targetPath = strings.TrimSpace(args[0])
		}

		runName, _ := cmd.Flags().GetString("run-name")
		runID, _ := cmd.Flags().GetString("run-id")
		agentFlag, _ := cmd.Flags().GetString("agent")
		providerFlag, _ := cmd.Flags().GetString("provider")
		modelFlag, _ := cmd.Flags().GetString("model")
		commandFlag, _ := cmd.Flags().GetString("command")
		envFlag, _ := cmd.Flags().GetString("env")
		keepEnvFlag, _ := cmd.Flags().GetBool("keep-env")
		keepWorkdirFlag, _ := cmd.Flags().GetBool("keep-workdir")
		repeatFlag, _ := cmd.Flags().GetInt("repeat")
		jsonFlag, _ := cmd.Flags().GetBool("json")
		dryRunFlag, _ := cmd.Flags().GetBool("dry-run")
		noRedactFlag, _ := cmd.Flags().GetBool("no-redact")
		reportFlag, _ := cmd.Flags().GetString("report")

		var selectedRunID string
		if strings.TrimSpace(runName) != "" {
			selectedRunID = strings.TrimSpace(runName)
		} else if strings.TrimSpace(runID) != "" {
			selectedRunID = strings.TrimSpace(runID)
		}

		override := types.AgentOverride{
			Agent:    strings.TrimSpace(agentFlag),
			Provider: strings.TrimSpace(providerFlag),
			Model:    strings.TrimSpace(modelFlag),
			Command:  strings.TrimSpace(commandFlag),
		}

		opts := types.RunOptions{
			Context:             cmd.Context(),
			RunID:               selectedRunID,
			AgentOverride:       override,
			EnvironmentOverride: strings.TrimSpace(envFlag),
			KeepEnv:             keepEnvFlag,
			KeepWorkdir:         keepWorkdirFlag || keepEnvFlag,
			Repeat:              repeatFlag,
			NoRedact:            noRedactFlag,
			Provenance: types.ProvenanceInfo{
				CLIVersion:   cliVersion(),
				CLICommit:    cliCommit(),
				EngineCommit: engineCommit(),
			},
		}

		// Reset parsed flag values on cmd after extracting options to prevent sticky state across test runs.
		defer func() {
			_ = cmd.Flags().Set("run-name", "")
			_ = cmd.Flags().Set("run-id", "")
			_ = cmd.Flags().Set("agent", "")
			_ = cmd.Flags().Set("provider", "")
			_ = cmd.Flags().Set("model", "")
			_ = cmd.Flags().Set("command", "")
			_ = cmd.Flags().Set("env", "")
			_ = cmd.Flags().Set("keep-env", "false")
			_ = cmd.Flags().Set("keep-workdir", "false")
			_ = cmd.Flags().Set("repeat", "0")
			_ = cmd.Flags().Set("json", "false")
			_ = cmd.Flags().Set("dry-run", "false")
			_ = cmd.Flags().Set("no-redact", "false")
			_ = cmd.Flags().Set("report", "")
		}()

		// --dry-run: resolve and print the plan, execute nothing.
		if dryRunFlag {
			plan, err := ActivePlanner(targetPath, opts)
			if err != nil {
				return err
			}
			if jsonFlag {
				data, err := json.MarshalIndent(plan, "", "  ")
				if err != nil {
					return err
				}
				commands.PrintStdout(cmd, string(data))
				return nil
			}
			printPlan(cmd, plan)
			return nil
		}

		if !jsonFlag {
			cmd.Printf("Running evaluation at %s...\n", targetPath)
		}

		var summary *types.RunSummary
		var err error
		if ActiveRunnerWithOptions != nil {
			summary, err = ActiveRunnerWithOptions(targetPath, opts)
		} else if ActiveRunner != nil {
			// If options like agent/provider/env overrides are present, delegate to engine.RunWithOptions
			if opts.AgentOverride.HasOverrides() || opts.EnvironmentOverride != "" || opts.KeepEnv || opts.Repeat > 0 || opts.NoRedact || opts.KeepWorkdir {
				summary, err = engine.RunWithOptions(targetPath, opts)
			} else {
				if opts.RunID != "" {
					summary, err = ActiveRunner(targetPath, opts.RunID)
				} else {
					summary, err = ActiveRunner(targetPath)
				}
			}
		}

		if err != nil {
			return err
		}

		failed := summary.Failed > 0 || summary.Errored > 0 || summary.Flaked > 0 ||
			(summary.Verdict != "" && summary.Verdict != types.VerdictPassed)

		// Machine-readable output mode: JSON only on stdout.
		if jsonFlag {
			if err := emitJSON(cmd, summary, targetPath); err != nil {
				return err
			}
			if failed {
				return commands.WithExitCode(1, shiperrs.New(shiperrs.ErrorCodeAgentExecutionFailed,
					fmt.Sprintf("run verdict: %s", verdictLabel(summary))))
			}
			return nil
		}

		cmd.Printf("Target Type: %s\n", summary.TargetType)
		cmd.Printf("Total Evals: %d (Passed: %d, Failed: %d)\n", summary.TotalEvals, summary.Passed, summary.Failed)
		if summary.Flaked > 0 || summary.Errored > 0 || summary.K > 0 {
			cmd.Printf("Verdict: %s (k=%d, flaked: %d, errored: %d)\n", verdictLabel(summary), summary.K, summary.Flaked, summary.Errored)
		}
		if summary.SuiteDigest != "" {
			cmd.Printf("Suite digest: %s\n", shortDigest(summary.SuiteDigest))
		}
		if summary.Usage != nil {
			cmd.Printf("Usage: %d in / %d out tokens", summary.Usage.InputTokens, summary.Usage.OutputTokens)
			if summary.Usage.CostUSD != nil {
				cmd.Printf(" · ~$%.4f (estimate)", *summary.Usage.CostUSD)
			}
			cmd.Println()
		}

		for _, res := range summary.Results {
			if res.Error != nil && res.Verdict == types.VerdictErrored {
				cmd.Printf("  - %s: ERRORED (infra/rubric, not an agent failure: %v)\n", res.EvalName, res.Error)
			} else if res.Error != nil {
				cmd.Printf("  - %s: FAILED (%v)\n", res.EvalName, res.Error)
			} else {
				status := strings.ToUpper(string(res.EffectiveVerdict()))
				detail := ""
				if res.K > 1 {
					detail = fmt.Sprintf(", k=%d, pass@k=%.0f%%, pass^k=%.0f%%", res.K, res.PassAtK*100, res.PassPowK*100)
				}
				cmd.Printf("  - %s: %s (score: %.2f%s, duration: %v, rollouts: %s)\n",
					res.EvalName, status, res.Score, detail, res.Duration, filepath.Clean(res.RolloutDir))
			}
		}

		// Reports (markdown / json / junit) written at the target root.
		if strings.TrimSpace(reportFlag) != "" {
			if err := writeReports(cmd, targetPath, summary, reportFlag); err != nil {
				return err
			}
		}

		if failed {
			if summary.Verdict == types.VerdictErrored {
				return commands.WithExitCode(1, shiperrs.New(shiperrs.ErrorCodeEnvironmentSetupFailed,
					"run blocked by infra errors (fail closed, labeled infra)"))
			}
			notPassed := summary.Failed + summary.Flaked
			return shiperrs.New(shiperrs.ErrorCodeAgentExecutionFailed, fmt.Sprintf("%d evaluation(s) failed or flaked", notPassed))
		}

		return nil
	},
}

// verdictLabel renders the suite verdict, falling back for legacy summaries.
func verdictLabel(summary *types.RunSummary) string {
	if summary.Verdict != "" {
		return string(summary.Verdict)
	}
	if summary.Failed > 0 {
		return string(types.VerdictFailed)
	}
	return string(types.VerdictPassed)
}

// emitJSON prints the suite results JSON (or a minimal summary when the run
// did not produce a results file, e.g. under test stubs).
func emitJSON(cmd *cobra.Command, summary *types.RunSummary, targetPath string) error {
	if summary.ResultsPath != "" {
		if res, err := results.LoadSuiteResults(summary.ResultsPath); err == nil {
			data, err := report.JSON(res, report.Options{})
			if err != nil {
				return err
			}
			commands.PrintStdout(cmd, string(data))
			return nil
		}
	}
	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return err
	}
	commands.PrintStdout(cmd, string(data))
	return nil
}

// printPlan renders a dry-run plan for humans.
func printPlan(cmd *cobra.Command, plan *engine.RunPlan) {
	cmd.Printf("Plan for %s (%s)\n", plan.TargetPath, plan.TargetType)
	cmd.Printf("  run id:        %s\n", plan.RunID)
	cmd.Printf("  suite digest:  %s\n", shortDigest(plan.SuiteDigest))
	cmd.Printf("  pricing:       %s\n", plan.Pricing)
	cmd.Printf("  gate:          max_score_drop_pct=%.1f max_cost_increase_pct=%.1f block_on_flake=%v min_k_noise_floor=%d\n",
		plan.Gate.MaxScoreDropPct, plan.Gate.MaxCostIncreasePct, plan.Gate.BlockOnFlakeValue(), plan.Gate.MinKNoiseFloor)
	for _, e := range plan.Evals {
		cmd.Printf("  eval %q (%s)\n", e.EvalName, e.Path)
		if e.Plugin != "" {
			cmd.Printf("    agent:       plugin=%s model=%s repeat=%d\n", e.Plugin, e.Model, e.Repeat)
		}
		if e.Command != "" {
			cmd.Printf("    command:     %s\n", e.Command)
		}
		if e.Entrypoint != "" {
			cmd.Printf("    entrypoint:  %s\n", e.Entrypoint)
		}
		cmd.Printf("    environment: %s", e.EnvironmentType)
		if e.Image != "" {
			cmd.Printf(" image=%s", e.Image)
		}
		if e.Dockerfile != "" {
			cmd.Printf(" dockerfile=%s", e.Dockerfile)
		}
		cmd.Println()
		if e.HostDir != "" {
			mount := e.SandboxDir
			if mount == "" {
				mount = "(host paths)"
			}
			cmd.Printf("    mount:       %s -> %s\n", e.HostDir, mount)
		}
		cmd.Printf("    config hash: %s\n", shortDigest(e.ConfigHash))
	}
}

// writeReports renders the requested report formats at the target root.
func writeReports(cmd *cobra.Command, targetPath string, summary *types.RunSummary, formats string) error {
	if summary.ResultsPath == "" {
		cmd.Println("report: no results file available for this run; skipping report output")
		return nil
	}
	res, err := results.LoadSuiteResults(summary.ResultsPath)
	if err != nil {
		return err
	}

	// Delta table when a baseline is committed; a mismatch is noted, not fatal.
	var comparison *compare.Report
	baselinePath := baseline.Path(targetPath, "main")
	if _, statErr := os.Stat(baselinePath); statErr == nil {
		bl, loadErr := baseline.Load(baselinePath)
		if loadErr != nil {
			cmd.Printf("report: could not load baseline %s: %v\n", baselinePath, loadErr)
		} else {
			policy, _ := gate.LoadDefault(targetPath, res.TargetType)
			cmp, cmpErr := compare.Compare(bl, res, compare.Options{AllowSuiteMismatch: true, Policy: policy})
			if cmpErr != nil {
				cmd.Printf("report: comparison skipped: %v\n", cmpErr)
			} else {
				comparison = cmp
			}
		}
	}

	for _, f := range strings.Split(formats, ",") {
		switch strings.TrimSpace(f) {
		case "markdown", "md":
			out := report.Markdown(res, report.Options{Comparison: comparison})
			path := filepath.Join(targetPath, "shipyard-report.md")
			if err := os.WriteFile(path, []byte(out), 0644); err != nil {
				return err
			}
			cmd.Printf("report written: %s\n", path)
		case "json":
			out, err := report.JSON(res, report.Options{Comparison: comparison})
			if err != nil {
				return err
			}
			path := filepath.Join(targetPath, "shipyard-report.json")
			if err := os.WriteFile(path, append(out, '\n'), 0644); err != nil {
				return err
			}
			cmd.Printf("report written: %s\n", path)
		case "junit":
			out, err := report.JUnit(res)
			if err != nil {
				return err
			}
			path := filepath.Join(targetPath, "shipyard-junit.xml")
			if err := os.WriteFile(path, out, 0644); err != nil {
				return err
			}
			cmd.Printf("report written: %s\n", path)
		default:
			cmd.Printf("report: unknown format %q (use markdown, json, junit)\n", f)
		}
	}
	return nil
}

func shortDigest(d string) string {
	d = strings.TrimPrefix(d, "sha256:")
	if len(d) > 12 {
		return d[:12]
	}
	return d
}

// Build-time identity hooks; the root command fills these in.
var (
	cliVersionFn   = func() string { return "" }
	cliCommitFn    = func() string { return "" }
	engineCommitFn = func() string { return "" }
)

func cliVersion() string   { return cliVersionFn() }
func cliCommit() string    { return cliCommitFn() }
func engineCommit() string { return engineCommitFn() }

// SetBuildInfo wires the root command's build identity into run provenance.
func SetBuildInfo(version, commit, engineCommit func() string) {
	cliVersionFn = version
	cliCommitFn = commit
	engineCommitFn = engineCommit
}

func init() {
	RunCmd.Flags().String("run-name", "", "Name of the run (creates rollouts/{run-name}/)")
	RunCmd.Flags().String("run-id", "", "ID of the run (alias for --run-name)")

	RunCmd.Flags().String("agent", "", "Agent or plugin runtime override (e.g. claude-code, langgraph, codex)")
	RunCmd.Flags().String("provider", "", "LLM provider runtime override (e.g. openai, anthropic, gemini, ollama)")
	RunCmd.Flags().String("model", "", "Model name runtime override (e.g. gpt-4o-mini, claude-3-5-sonnet-20241022)")
	RunCmd.Flags().String("command", "", "Agent execution command runtime override (e.g. 'python my_agent.py')")
	RunCmd.Flags().String("env", "", "Environment runtime override (e.g. 'docker', 'local', 'custom')")
	RunCmd.Flags().Bool("keep-env", false, "Keep sandbox environment running after evaluation (for debugging)")
	RunCmd.Flags().Bool("keep-workdir", false, "Leave agent changes in the eval workspace after the run (default: restored so the next run starts clean; implied by --keep-env)")

	RunCmd.Flags().Int("repeat", 0, "Run each eval K times and aggregate (pass@k, pass^k, mean/sigma, flake detection); overrides repeat in eval.toml")
	RunCmd.Flags().String("report", "", "Write reports at the target root: comma-separated markdown,json,junit")
	RunCmd.Flags().Bool("json", false, "Print machine-readable suite results JSON to stdout")
	RunCmd.Flags().Bool("dry-run", false, "Print the resolved plan (agent command, image, mounts, config hash) without executing")
	RunCmd.Flags().Bool("no-redact", false, "Disable best-effort credential scrubbing of captured artifacts")

	commands.Register(RunCmd)
}
