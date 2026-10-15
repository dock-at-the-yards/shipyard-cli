package comparecmd_test

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/dock-at-the-yards/shipyard-cli/cmd/cmdline/commands"
	_ "github.com/dock-at-the-yards/shipyard-cli/cmd/cmdline/commands/compare"
	"github.com/dock-at-the-yards/shipyard-core/common/types"

	"github.com/spf13/cobra"
)

// captureStdout runs fn with os.Stdout redirected to a pipe and returns what
// was written. This guards the cobra Print* → stderr trap: machine output must
// reach real stdout for `shipyard ... --json > file` to work.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	done := make(chan string)
	go func() {
		data, _ := io.ReadAll(r)
		done <- string(data)
	}()
	fn()
	_ = w.Close()
	os.Stdout = old
	return <-done
}

func TestCompareJSONGoesToStdout(t *testing.T) {
	dir := t.TempDir()
	writeSuite(t, dir, "run-1", "sha256:aaa", types.VerdictPassed)
	res, _ := loadResults(t, dir+"/rollouts/run-1/results.json")
	saveBaseline(t, dir, res)
	// Candidate with a regression on the same digest.
	regress := types.EvalAggregate{
		SchemaVersion:    "1",
		EvalName:         "order-refund",
		K:                3,
		FailedCount:      3,
		Verdict:          types.VerdictFailed,
		DefinitionDigest: "sha256:def-1",
		DefinitionFiles:  map[string]string{"tests/rubric.py": "sha256:one"},
		Repeats: []types.RepeatResult{
			{RepeatIndex: 1, Verdict: types.VerdictFailed},
			{RepeatIndex: 2, Verdict: types.VerdictFailed},
			{RepeatIndex: 3, Verdict: types.VerdictFailed},
		},
	}
	writeSuiteWithEval(t, dir, "run-2", "sha256:aaa", regress)

	root := &cobra.Command{Use: "shipyard"}
	commands.AttachAll(root)
	// Simulate the real CLI: no SetOut, so the default streams are in play.
	root.SetErr(os.Stderr)
	root.SetArgs([]string{"compare", dir, "--json"})

	out := captureStdout(t, func() {
		_, _ = root.ExecuteC()
	})
	if !strings.Contains(out, `"blocked": true`) || !strings.Contains(out, `"schema_version"`) {
		t.Fatalf("expected the compare JSON on stdout, got %q", out)
	}
}
