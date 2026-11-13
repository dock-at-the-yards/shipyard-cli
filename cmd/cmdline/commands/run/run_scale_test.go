package run_test

import (
	"testing"

	"github.com/dock-at-the-yards/shipyard-cli/cmd/cmdline/commands"
	runcmd "github.com/dock-at-the-yards/shipyard-cli/cmd/cmdline/commands/run"
	"github.com/dock-at-the-yards/shipyard-core/common/types"

	"github.com/spf13/cobra"
)

func TestRunScaleFlagsPlumbed(t *testing.T) {
	origWithOptions := runcmd.ActiveRunnerWithOptions
	defer func() { runcmd.ActiveRunnerWithOptions = origWithOptions }()

	var gotOpts types.RunOptions
	runcmd.ActiveRunnerWithOptions = func(path string, opts types.RunOptions) (*types.RunSummary, error) {
		gotOpts = opts
		return &types.RunSummary{
			TargetType: types.TargetTypeEvalSet,
			TargetPath: path,
			TotalEvals: 2,
			Passed:     2,
			Verdict:    types.VerdictPassed,
		}, nil
	}

	root := &cobra.Command{Use: "shipyard"}
	commands.AttachAll(root)
	root.SetArgs([]string{"run", "some/set", "--jobs", "4", "--retries", "2", "--scenario", "/tmp/s.toml", "--shard", "1/2"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if gotOpts.Jobs != 4 {
		t.Errorf("jobs = %d, want 4", gotOpts.Jobs)
	}
	if gotOpts.Retries != 2 {
		t.Errorf("retries = %d, want 2", gotOpts.Retries)
	}
	if gotOpts.ScenarioPath != "/tmp/s.toml" {
		t.Errorf("scenario = %q", gotOpts.ScenarioPath)
	}
	if gotOpts.ShardIndex != 1 || gotOpts.ShardTotal != 2 {
		t.Errorf("shard = %d/%d, want 1/2", gotOpts.ShardIndex, gotOpts.ShardTotal)
	}
}

func TestRunScaleFlagValidation(t *testing.T) {
	cases := [][]string{
		{"run", "x", "--jobs", "0"},
		{"run", "x", "--jobs", "-3"},
		{"run", "x", "--retries", "-1"},
		{"run", "x", "--shard", "bogus"},
		{"run", "x", "--shard", "1"},
		{"run", "x", "--shard", "0/2"},
		{"run", "x", "--shard", "3/2"},
		{"run", "x", "--shard", "1/0"},
	}
	for _, args := range cases {
		root := &cobra.Command{Use: "shipyard"}
		commands.AttachAll(root)
		root.SetArgs(args)
		if err := root.Execute(); err == nil {
			t.Errorf("expected error for %v", args)
		}
	}
}
