package mergecmd_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/dock-at-the-yards/shipyard-core/common/types"
	"github.com/dock-at-the-yards/shipyard-core/pkg/results"

	mergecmd "github.com/dock-at-the-yards/shipyard-cli/cmd/cmdline/commands/merge"
	"github.com/dock-at-the-yards/shipyard-cli/cmd/cmdline/commands"
	"github.com/spf13/cobra"
)

func agg(name string, verdict types.Verdict, score float64) types.EvalAggregate {
	return types.EvalAggregate{
		SchemaVersion: "1",
		EvalName:      name,
		Verdict:       verdict,
		MeanScore:     score,
		K:             1,
		StartedAt:     "2026-11-13T00:00:00Z",
		EndedAt:       "2026-11-13T00:01:00Z",
	}
}

func suite(runID string, aggs ...types.EvalAggregate) *types.SuiteResults {
	return &types.SuiteResults{
		SchemaVersion: "1",
		TargetType:    types.TargetTypeEvalSet,
		TargetName:    "evalset",
		RunID:         runID,
		SuiteDigest:   "sha256:abc",
		StartedAt:     "2026-11-13T00:00:00Z",
		EndedAt:       "2026-11-13T00:05:00Z",
		Evals:         aggs,
	}
}

func writeSuite(t *testing.T, dir, name string, s *types.SuiteResults) string {
	t.Helper()
	path := filepath.Join(dir, name)
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestMergeShardsEqualsUnsharded(t *testing.T) {
	full := suite("full",
		agg("a", types.VerdictPassed, 1.0),
		agg("b", types.VerdictFailed, 0.0),
		agg("c", types.VerdictPassed, 0.8),
	)
	shard1 := suite("shard-1", agg("a", types.VerdictPassed, 1.0), agg("c", types.VerdictPassed, 0.8))
	shard2 := suite("shard-2", agg("b", types.VerdictFailed, 0.0))

	merged, err := mergecmd.Merge(shard1, shard2)
	if err != nil {
		t.Fatal(err)
	}
	if merged.Verdict != types.VerdictFailed {
		t.Fatalf("merged verdict = %s, want failed", merged.Verdict)
	}
	if merged.Totals.Evals != 3 || merged.Totals.Passed != 2 || merged.Totals.Failed != 1 {
		t.Fatalf("merged totals wrong: %+v", merged.Totals)
	}
	if merged.TargetName != "evalset" || merged.SuiteDigest != "sha256:abc" {
		t.Fatalf("merged provenance wrong: %+v", merged)
	}
	// Eval order deterministic regardless of shard input order.
	if merged.Evals[0].EvalName != "a" || merged.Evals[1].EvalName != "b" || merged.Evals[2].EvalName != "c" {
		t.Fatalf("evals not sorted: %v", merged.Evals)
	}
	_ = full
}

func TestMergeErrorPrecedence(t *testing.T) {
	merged, err := mergecmd.Merge(
		suite("r1", agg("a", types.VerdictFlaked, 0.5)),
		suite("r2", types.EvalAggregate{SchemaVersion: "1", EvalName: "b", Verdict: types.VerdictErrored}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if merged.Verdict != types.VerdictErrored {
		t.Fatalf("errored must win, got %s", merged.Verdict)
	}
}

func TestMergeRejects(t *testing.T) {
	if _, err := mergecmd.Merge(); err == nil {
		t.Fatal("expected error for zero inputs")
	}
	other := suite("r1", agg("a", types.VerdictPassed, 1.0))
	other.TargetName = "other-set"
	if _, err := mergecmd.Merge(suite("r1", agg("b", types.VerdictPassed, 1.0)), other); err == nil {
		t.Fatal("expected error for different suites")
	}
	dup1 := suite("r1", agg("a", types.VerdictPassed, 1.0))
	dup2 := suite("r2", agg("a", types.VerdictFailed, 0.0))
	if _, err := mergecmd.Merge(dup1, dup2); err == nil {
		t.Fatal("expected error for duplicate eval")
	}
	badDigest := suite("r2", agg("b", types.VerdictPassed, 1.0))
	badDigest.SuiteDigest = "sha256:other"
	if _, err := mergecmd.Merge(suite("r1", agg("a", types.VerdictPassed, 1.0)), badDigest); err == nil {
		t.Fatal("expected error for different digests")
	}
}

func TestMergeCommandOutFile(t *testing.T) {
	dir := t.TempDir()
	f1 := writeSuite(t, dir, "s1.json", suite("r1", agg("a", types.VerdictPassed, 1.0)))
	f2 := writeSuite(t, dir, "s2.json", suite("r2", agg("b", types.VerdictPassed, 1.0)))
	out := filepath.Join(dir, "merged.json")

	root := &cobra.Command{Use: "shipyard"}
	commands.AttachAll(root)
	root.SetArgs([]string{"merge", f1, f2, "--out", out})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	merged, err := results.LoadSuiteResults(out)
	if err != nil {
		t.Fatal(err)
	}
	if merged.Verdict != types.VerdictPassed || merged.Totals.Evals != 2 {
		t.Fatalf("merged file wrong: %+v", merged.Totals)
	}
	if merged.RunID != "merged" {
		t.Fatalf("expected RunID merged for differing inputs, got %q", merged.RunID)
	}
}
