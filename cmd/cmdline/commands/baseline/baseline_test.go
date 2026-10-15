package baselinecmd_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dock-at-the-yards/shipyard-cli/cmd/cmdline/commands"
	_ "github.com/dock-at-the-yards/shipyard-cli/cmd/cmdline/commands/baseline"
	"github.com/dock-at-the-yards/shipyard-core/common/types"
	"github.com/dock-at-the-yards/shipyard-core/pkg/baseline"

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

func writeResults(t *testing.T, dir, runID string) {
	t.Helper()
	res := types.SuiteResults{
		SchemaVersion: "1",
		TargetType:    types.TargetTypeEvalSet,
		TargetName:    "suite",
		RunID:         runID,
		SuiteDigest:   "sha256:aaa",
		K:             3,
		Verdict:       types.VerdictPassed,
		Totals:        types.SuiteTotals{Evals: 1, Passed: 1, MeanScore: 1.0},
		Evals: []types.EvalAggregate{
			{SchemaVersion: "1", EvalName: "e1", K: 3, PassedCount: 3, Verdict: types.VerdictPassed, MeanScore: 1.0},
		},
	}
	path := filepath.Join(dir, "rollouts", runID, "results.json")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	data, _ := json.MarshalIndent(res, "", "  ")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
}

func TestBaselineSaveFromLatestRun(t *testing.T) {
	dir := t.TempDir()
	writeResults(t, dir, "run-1")

	root := &cobra.Command{Use: "shipyard"}
	commands.AttachAll(root)

	out, err := executeCommand(root, "baseline", "save", dir, "--name", "main", "--created-by", "shipyard test")
	if err != nil {
		t.Fatalf("baseline save failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "baseline saved") {
		t.Errorf("expected confirmation, got: %s", out)
	}

	bl, err := baseline.Load(baseline.Path(dir, "main"))
	if err != nil {
		t.Fatal(err)
	}
	if bl.Name != "main" || bl.K != 3 || bl.SuiteDigest != "sha256:aaa" || bl.SourceRunID != "run-1" {
		t.Errorf("baseline wrong: %+v", bl)
	}
	if bl.CreatedBy != "shipyard test" || bl.CreatedAt == "" {
		t.Errorf("creation metadata missing: %+v", bl)
	}
}

func TestBaselineSaveFromRunIDAndJSON(t *testing.T) {
	dir := t.TempDir()
	writeResults(t, dir, "run-a")
	writeResults(t, dir, "run-b")

	root := &cobra.Command{Use: "shipyard"}
	commands.AttachAll(root)

	out, err := executeCommand(root, "baseline", "save", dir, "--name", "old", "--run-id", "run-a", "--json")
	if err != nil {
		t.Fatalf("baseline save failed: %v\n%s", err, out)
	}
	var bl types.Baseline
	if err := json.Unmarshal([]byte(out), &bl); err != nil {
		t.Fatalf("expected baseline JSON, got %q: %v", out, err)
	}
	if bl.SourceRunID != "run-a" {
		t.Errorf("expected run-a, got %s", bl.SourceRunID)
	}
}
