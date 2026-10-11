package run

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/dock-at-the-yards/shipyard-cli/cmd/cmdline/commands"
	"github.com/dock-at-the-yards/shipyard-core/common/types"
	"github.com/dock-at-the-yards/shipyard-core/pkg/engine"
	shiperrs "github.com/dock-at-the-yards/shipyard-core/pkg/errors"

	"github.com/spf13/cobra"
)

// EngineRunFunc defines the legacy signature for the engine runner executor.
// Maintained for backward compatibility with existing tests.
type EngineRunFunc func(path string, runID ...string) (*types.RunSummary, error)

// EngineRunWithOptionsFunc defines the execution signature accepting full RunOptions.
type EngineRunWithOptionsFunc func(path string, opts types.RunOptions) (*types.RunSummary, error)

// defaultRunner indicates whether ActiveRunner was customized by tests or callers.
var defaultRunner EngineRunFunc = engine.Run

var (
	// ActiveRunner is called by the run command. Defaults to engine.Run.
	ActiveRunner EngineRunFunc = defaultRunner

	// ActiveRunnerWithOptions can be set by tests or callers to inspect full RunOptions.
	ActiveRunnerWithOptions EngineRunWithOptionsFunc
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

		cmd.Printf("Running evaluation at %s...\n", targetPath)

		runName, _ := cmd.Flags().GetString("run-name")
		runID, _ := cmd.Flags().GetString("run-id")
		agentFlag, _ := cmd.Flags().GetString("agent")
		providerFlag, _ := cmd.Flags().GetString("provider")
		modelFlag, _ := cmd.Flags().GetString("model")
		commandFlag, _ := cmd.Flags().GetString("command")
		envFlag, _ := cmd.Flags().GetString("env")
		keepEnvFlag, _ := cmd.Flags().GetBool("keep-env")

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
		}

		// Reset parsed flag values on cmd after extracting options to prevent sticky state across test runs
		defer func() {
			_ = cmd.Flags().Set("run-name", "")
			_ = cmd.Flags().Set("run-id", "")
			_ = cmd.Flags().Set("agent", "")
			_ = cmd.Flags().Set("provider", "")
			_ = cmd.Flags().Set("model", "")
			_ = cmd.Flags().Set("command", "")
			_ = cmd.Flags().Set("env", "")
			_ = cmd.Flags().Set("keep-env", "false")
		}()

		var summary *types.RunSummary
		var err error
		if ActiveRunnerWithOptions != nil {
			summary, err = ActiveRunnerWithOptions(targetPath, opts)
		} else if ActiveRunner != nil {
			// If options like agent/provider/env overrides are present, delegate to engine.RunWithOptions
			if opts.AgentOverride.HasOverrides() || opts.EnvironmentOverride != "" || opts.KeepEnv {
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

		cmd.Printf("Target Type: %s\n", summary.TargetType)
		cmd.Printf("Total Evals: %d (Passed: %d, Failed: %d)\n", summary.TotalEvals, summary.Passed, summary.Failed)

		for _, res := range summary.Results {
			if res.Error != nil {
				cmd.Printf("  - %s: FAILED (%v)\n", res.EvalName, res.Error)
			} else {
				status := "PASSED"
				if !res.Passed {
					status = "FAILED"
				}
				cmd.Printf("  - %s: %s (score: %.2f, duration: %v, rollouts: %s)\n",
					res.EvalName, status, res.Score, res.Duration, filepath.Clean(res.RolloutDir))
			}
		}

		if summary.Failed > 0 {
			return shiperrs.New(shiperrs.ErrorCodeAgentExecutionFailed, fmt.Sprintf("%d evaluation(s) failed", summary.Failed))
		}

		return nil
	},
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

	commands.Register(RunCmd)
}
