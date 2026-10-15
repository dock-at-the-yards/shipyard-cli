package validatecmd_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dock-at-the-yards/shipyard-cli/cmd/cmdline/commands"
	_ "github.com/dock-at-the-yards/shipyard-cli/cmd/cmdline/commands/validate"

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

func writeEval(t *testing.T, dir string, instruction string) string {
	t.Helper()
	evalDir := filepath.Join(dir, "some-eval")
	if err := os.MkdirAll(filepath.Join(evalDir, "tests"), 0755); err != nil {
		t.Fatal(err)
	}
	toml := `name = "some-eval"
instruction = "instruction.md"
timeout_s = 60

[[agent]]
name = "a"
plugin = "cli"
command = "true"

[environment]
type = "local"
workdir = "."

[tests]
rubric = "tests/rubric.py"
pass_threshold = 1.0
`
	if err := os.WriteFile(filepath.Join(evalDir, "eval.toml"), []byte(toml), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(evalDir, "instruction.md"), []byte(instruction), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(evalDir, "tests", "rubric.py"), []byte("print('{}')"), 0644); err != nil {
		t.Fatal(err)
	}
	return evalDir
}

func TestValidateCommand(t *testing.T) {
	root := &cobra.Command{Use: "shipyard"}
	commands.AttachAll(root)

	dir := t.TempDir()
	evalDir := writeEval(t, dir, "Do the thing.")

	out, err := executeCommand(root, "validate", evalDir, "--skip-rubric-probe")
	if err != nil {
		t.Fatalf("healthy eval should pass: %v\n%s", err, out)
	}
	if !strings.Contains(out, "OK") {
		t.Errorf("expected OK, got %s", out)
	}

	// Break it: empty instruction -> exit 1.
	_ = os.WriteFile(filepath.Join(evalDir, "instruction.md"), []byte(" \n"), 0644)
	out, err = executeCommand(root, "validate", evalDir, "--skip-rubric-probe")
	var exitErr *commands.ExitCodeError
	if !errors.As(err, &exitErr) || exitErr.Code != 1 {
		t.Fatalf("expected exit 1, got %v\n%s", err, out)
	}
	if !strings.Contains(out, "instruction file is empty") {
		t.Errorf("expected the empty-instruction finding, got:\n%s", out)
	}

	// JSON mode.
	_ = os.WriteFile(filepath.Join(evalDir, "instruction.md"), []byte("Do it."), 0644)
	out, err = executeCommand(root, "validate", evalDir, "--json", "--skip-rubric-probe")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, `"problems"`) {
		t.Errorf("expected JSON output, got %s", out)
	}
}
