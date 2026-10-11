package run_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/dock-at-the-yards/shipyard-cli/cmd/cmdline/commands"
	runcmd "github.com/dock-at-the-yards/shipyard-cli/cmd/cmdline/commands/run"
	"github.com/dock-at-the-yards/shipyard-core/common/types"

	"github.com/spf13/cobra"
)

func executeCommand(root *cobra.Command, args ...string) (string, error) {
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(args)

	_, err := root.ExecuteC()
	return buf.String(), err
}

func TestRunCommand(t *testing.T) {
	var calledPath string
	var calledRunID string
	origRunner := runcmd.ActiveRunner
	origRunnerWithOptions := runcmd.ActiveRunnerWithOptions

	defer func() {
		runcmd.ActiveRunner = origRunner
		runcmd.ActiveRunnerWithOptions = origRunnerWithOptions
	}()

	runcmd.ActiveRunner = func(path string, runID ...string) (*types.RunSummary, error) {
		calledPath = path
		if len(runID) > 0 {
			calledRunID = runID[0]
		} else {
			calledRunID = ""
		}
		return &types.RunSummary{
			TargetType: types.TargetTypeEval,
			TargetPath: path,
			TotalEvals: 1,
			Passed:     1,
			Failed:     0,
			Results: []types.EvalRunResult{
				{
					EvalName:   "my-test-eval",
					Path:       path,
					RolloutDir: path + "/rollouts",
					Passed:     true,
					Score:      1.0,
					Duration:   10 * time.Millisecond,
				},
			},
		}, nil
	}

	root := &cobra.Command{Use: "shipyard"}
	commands.AttachAll(root)

	// Test run with default path
	calledPath = ""
	calledRunID = ""
	out, err := executeCommand(root, "run")
	if err != nil {
		t.Fatalf("expected no error running shipyard run: %v", err)
	}
	if calledPath != "." {
		t.Errorf("expected calledPath '.', got %q", calledPath)
	}
	if !strings.Contains(out, "Total Evals: 1 (Passed: 1, Failed: 0)") {
		t.Errorf("expected summary in output, got: %s", out)
	}

	// Test run with explicit path
	calledPath = ""
	calledRunID = ""
	out, err = executeCommand(root, "run", "custom/eval/path")
	if err != nil {
		t.Fatalf("expected no error running shipyard run with path: %v", err)
	}
	if calledPath != "custom/eval/path" {
		t.Errorf("expected calledPath 'custom/eval/path', got %q", calledPath)
	}
	if !strings.Contains(out, "my-test-eval: PASSED") {
		t.Errorf("expected eval pass output, got: %s", out)
	}

	// Test run with --run-name flag
	calledPath = ""
	calledRunID = ""
	_, err = executeCommand(root, "run", "custom/eval/path", "--run-name", "experiment-1")
	if err != nil {
		t.Fatalf("expected no error running shipyard run with --run-name: %v", err)
	}
	if calledRunID != "experiment-1" {
		t.Errorf("expected calledRunID 'experiment-1', got %q", calledRunID)
	}

	// Test run with --run-id flag
	calledPath = ""
	calledRunID = ""
	_, err = executeCommand(root, "run", "custom/eval/path", "--run-id", "run-42")
	if err != nil {
		t.Fatalf("expected no error running shipyard run with --run-id: %v", err)
	}
	if calledRunID != "run-42" {
		t.Errorf("expected calledRunID 'run-42', got %q", calledRunID)
	}

	// Test run when runner returns error
	runcmd.ActiveRunner = func(path string, runID ...string) (*types.RunSummary, error) {
		return nil, errors.New("file not found")
	}
	_, err = executeCommand(root, "run", "nonexistent")
	if err == nil {
		t.Fatalf("expected error when runner fails, got nil")
	}

	// Test run when eval fails
	runcmd.ActiveRunner = func(path string, runID ...string) (*types.RunSummary, error) {
		return &types.RunSummary{
			TargetType: types.TargetTypeEval,
			TargetPath: path,
			TotalEvals: 1,
			Passed:     0,
			Failed:     1,
			Results: []types.EvalRunResult{
				{
					EvalName:   "failed-eval",
					Path:       path,
					RolloutDir: path + "/rollouts",
					Passed:     false,
					Score:      0.0,
				},
			},
		}, nil
	}
	_, err = executeCommand(root, "run", "failed-dir")
	if err == nil {
		t.Fatalf("expected command error when eval failed, got nil")
	}

	// Test run with agent override flags (--agent, --provider, --model, --command)
	var capturedOpts types.RunOptions
	var capturedPath string
	runcmd.ActiveRunnerWithOptions = func(path string, opts types.RunOptions) (*types.RunSummary, error) {
		capturedPath = path
		capturedOpts = opts
		return &types.RunSummary{
			TargetType: types.TargetTypeEval,
			TargetPath: path,
			TotalEvals: 1,
			Passed:     1,
			Results: []types.EvalRunResult{
				{
					EvalName:   "override-eval",
					Path:       path,
					RolloutDir: path + "/rollouts",
					Passed:     true,
					Score:      1.0,
				},
			},
		}, nil
	}

	_, err = executeCommand(root, "run", "test/eval", "--agent", "claude-code")
	if err != nil {
		t.Fatalf("unexpected error with --agent: %v", err)
	}
	if capturedPath != "test/eval" {
		t.Errorf("expected capturedPath 'test/eval', got %q", capturedPath)
	}
	if capturedOpts.AgentOverride.Agent != "claude-code" {
		t.Errorf("expected AgentOverride.Agent 'claude-code', got %q", capturedOpts.AgentOverride.Agent)
	}

	// Test --provider and --model
	_, err = executeCommand(root, "run", "test/eval", "--provider", "openai", "--model", "gpt-4o-mini")
	if err != nil {
		t.Fatalf("unexpected error with --provider and --model: %v", err)
	}
	if capturedOpts.AgentOverride.Provider != "openai" || capturedOpts.AgentOverride.Model != "gpt-4o-mini" {
		t.Errorf("expected provider 'openai' model 'gpt-4o-mini', got provider=%q model=%q",
			capturedOpts.AgentOverride.Provider, capturedOpts.AgentOverride.Model)
	}

	// Test --command
	_, err = executeCommand(root, "run", "test/eval", "--command", "python my_agent.py")
	if err != nil {
		t.Fatalf("unexpected error with --command: %v", err)
	}
	// Test context propagation
	if capturedOpts.Context == nil {
		t.Errorf("expected capturedOpts.Context to be non-nil and propagated from command context")
	}
}
