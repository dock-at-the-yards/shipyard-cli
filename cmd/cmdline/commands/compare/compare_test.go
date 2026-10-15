package comparecmd_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dock-at-the-yards/shipyard-cli/cmd/cmdline/commands"
	_ "github.com/dock-at-the-yards/shipyard-cli/cmd/cmdline/commands/compare"
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

func floatPtr(v float64) *float64 { return &v }

func writeSuite(t *testing.T, dir, name string, digest string, verdict types.Verdict) *types.SuiteResults {
	t.Helper()
	res := &types.SuiteResults{
		SchemaVersion: "1",
		TargetType:    types.TargetTypeEvalSet,
		TargetName:    "suite",
		RunID:         name,
		SuiteDigest:   digest,
		K:             3,
		Verdict:       verdict,
		Totals:        types.SuiteTotals{Evals: 1, Passed: 1, MeanScore: 1.0},
		Evals: []types.EvalAggregate{
			{
				SchemaVersion:    "1",
				EvalName:         "order-refund",
				K:                3,
				PassedCount:      3,
				Verdict:          verdict,
				PassAtK:          1.0,
				PassPowK:         1.0,
				MeanScore:        1.0,
				DefinitionDigest: "sha256:def-1",
				DefinitionFiles:  map[string]string{"tests/rubric.py": "sha256:one"},
				Repeats: []types.RepeatResult{
					{RepeatIndex: 1, Verdict: verdict, Score: 1.0},
					{RepeatIndex: 2, Verdict: verdict, Score: 1.0},
					{RepeatIndex: 3, Verdict: verdict, Score: 1.0},
				},
			},
		},
	}
	path := filepath.Join(dir, "rollouts", name, "results.json")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	data, _ := json.MarshalIndent(res, "", "  ")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	return res
}

func saveBaseline(t *testing.T, dir string, res *types.SuiteResults) {
	t.Helper()
	bl := baseline.FromSuite("main", res, "shipyard test")
	if err := baseline.Save(baseline.Path(dir, "main"), bl); err != nil {
		t.Fatal(err)
	}
}

func TestCompareExitCodesAndOutput(t *testing.T) {
	dir := t.TempDir()
	writeSuite(t, dir, "run-1", "sha256:aaa", types.VerdictPassed)
	res, _ := loadResults(t, filepath.Join(dir, "rollouts", "run-1", "results.json"))
	saveBaseline(t, dir, res)

	root := &cobra.Command{Use: "shipyard"}
	commands.AttachAll(root)

	// Unchanged: exit 0.
	out, err := executeCommand(root, "compare", dir)
	if err != nil {
		t.Fatalf("expected clean compare, got %v\n%s", err, out)
	}
	if !strings.Contains(out, "gate passed") || !strings.Contains(out, "noise") {
		t.Errorf("expected gate passed + noise line, got:\n%s", out)
	}

	// Regressed: exit 1.
	writeSuite(t, dir, "run-2", "sha256:aaa", types.VerdictPassed) // same digest
	regress := types.EvalAggregate{
		SchemaVersion:    "1",
		EvalName:         "order-refund",
		K:                3,
		FailedCount:      3,
		Verdict:          types.VerdictFailed,
		MeanScore:        0.0,
		DefinitionDigest: "sha256:def-1",
		DefinitionFiles:  map[string]string{"tests/rubric.py": "sha256:one"},
		Repeats: []types.RepeatResult{
			{RepeatIndex: 1, Verdict: types.VerdictFailed, Score: 0},
			{RepeatIndex: 2, Verdict: types.VerdictFailed, Score: 0},
			{RepeatIndex: 3, Verdict: types.VerdictFailed, Score: 0},
		},
	}
	writeSuiteWithEval(t, dir, "run-2", "sha256:aaa", regress)

	out, err = executeCommand(root, "compare", dir)
	var exitErr *commands.ExitCodeError
	if !errors.As(err, &exitErr) || exitErr.Code != 1 {
		t.Fatalf("expected exit code 1, got %v (out: %s)", err, out)
	}
	if !strings.Contains(out, "order-refund") || !strings.Contains(out, "regressed") {
		t.Errorf("expected the eval named and regressed, got:\n%s", out)
	}

	// Missing baseline: exit 2.
	other := t.TempDir()
	_, err = executeCommand(root, "compare", other)
	if !errors.As(err, &exitErr) || exitErr.Code != 2 {
		t.Fatalf("expected exit code 2 for missing baseline, got %v", err)
	}
}

func loadResults(t *testing.T, path string) (*types.SuiteResults, error) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var res types.SuiteResults
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

func writeSuiteWithEval(t *testing.T, dir, name, digest string, eval types.EvalAggregate) {
	t.Helper()
	res := &types.SuiteResults{
		SchemaVersion: "1",
		TargetType:    types.TargetTypeEvalSet,
		TargetName:    "suite",
		RunID:         name,
		SuiteDigest:   digest,
		K:             3,
		Verdict:       eval.Verdict,
		Totals:        types.SuiteTotals{Evals: 1, Failed: 1},
		Evals:         []types.EvalAggregate{eval},
	}
	path := filepath.Join(dir, "rollouts", name, "results.json")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	data, _ := json.MarshalIndent(res, "", "  ")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
}

func TestCompareRefusesDefinitionChange(t *testing.T) {
	dir := t.TempDir()
	writeSuite(t, dir, "run-1", "sha256:aaa", types.VerdictPassed)
	res, _ := loadResults(t, filepath.Join(dir, "rollouts", "run-1", "results.json"))
	saveBaseline(t, dir, res)

	// Candidate from a different definition digest.
	writeSuite(t, dir, "run-2", "sha256:bbb", types.VerdictPassed)

	root := &cobra.Command{Use: "shipyard"}
	commands.AttachAll(root)
	out, err := executeCommand(root, "compare", dir)
	var exitErr *commands.ExitCodeError
	if !errors.As(err, &exitErr) || exitErr.Code != 1 {
		t.Fatalf("expected refusal exit 1, got %v (out %s)", err, out)
	}
	if !strings.Contains(out, "refusing to compare different suites") {
		t.Errorf("expected refusal message, got:\n%s", out)
	}

	// JSON mode carries the refusal too.
	out, err = executeCommand(root, "compare", dir, "--json")
	if !errors.As(err, &exitErr) || exitErr.Code != 1 {
		t.Fatalf("expected refusal exit 1 in json mode, got %v", err)
	}
	if !strings.Contains(out, `"refusal"`) {
		t.Errorf("expected refusal in JSON, got:\n%s", out)
	}
}
