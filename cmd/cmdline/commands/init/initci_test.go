package initcmd_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dock-at-the-yards/shipyard-cli/cmd/cmdline/commands"
	_ "github.com/dock-at-the-yards/shipyard-cli/cmd/cmdline/commands/init"

	"github.com/spf13/cobra"
)

func executeCommandCI(root *cobra.Command, args ...string) (string, error) {
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(args)
	_, err := root.ExecuteC()
	return buf.String(), err
}

func TestInitCIGitHub(t *testing.T) {
	dir := t.TempDir()
	evalDir := filepath.Join(dir, "evalsets", "demo")
	if err := os.MkdirAll(evalDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(evalDir, "evalset.toml"), []byte("name = \"demo\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	root := &cobra.Command{Use: "shipyard"}
	commands.AttachAll(root)

	out, err := executeCommandCI(root, "init", "ci", "--provider", "github", "--target", "evalsets/demo", "--path", dir)
	if err != nil {
		t.Fatalf("init ci failed: %v\n%s", err, out)
	}
	for _, f := range []string{
		filepath.Join(dir, ".github", "workflows", "shipyard.yml"),
		filepath.Join(dir, "baselines", "README.md"),
	} {
		if _, err := os.Stat(f); err != nil {
			t.Errorf("expected %s to exist: %v", f, err)
		}
	}
	workflow, _ := os.ReadFile(filepath.Join(dir, ".github", "workflows", "shipyard.yml"))
	if !strings.Contains(string(workflow), "dock-at-the-yards/shipyard-ci") {
		t.Errorf("workflow should wire the released shipyard-ci action")
	}

	// Gate policy appended exactly once.
	toml, _ := os.ReadFile(filepath.Join(evalDir, "evalset.toml"))
	if !strings.Contains(string(toml), "[gate]") {
		t.Fatalf("expected [gate] policy appended, got:\n%s", toml)
	}
	out, err = executeCommandCI(root, "init", "ci", "--provider", "github", "--target", "evalsets/demo", "--path", dir)
	if err != nil {
		t.Fatalf("second init ci should not fail on the existing workflow: %v\n%s", err, out)
	}
	toml2, _ := os.ReadFile(filepath.Join(evalDir, "evalset.toml"))
	if strings.Count(string(toml2), "[gate]") != 1 {
		t.Errorf("gate policy should not be appended twice:\n%s", toml2)
	}
}

func TestInitCIUnknownProvider(t *testing.T) {
	root := &cobra.Command{Use: "shipyard"}
	commands.AttachAll(root)
	_, err := executeCommandCI(root, "init", "ci", "--provider", "circleci")
	if err == nil {
		t.Fatalf("expected an error for an unknown provider")
	}
}
