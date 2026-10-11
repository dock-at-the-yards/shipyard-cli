package commands_test

import (
	"testing"

	"github.com/dock-at-the-yards/shipyard-cli/cmd/cmdline/commands"
	_ "github.com/dock-at-the-yards/shipyard-cli/cmd/cmdline/commands/init"
	_ "github.com/dock-at-the-yards/shipyard-cli/cmd/cmdline/commands/run"
)

func TestRegistry(t *testing.T) {
	cmds := commands.GetAll()
	if len(cmds) == 0 {
		t.Fatalf("expected at least 1 command in registry, got 0")
	}

	foundInit := false
	foundRun := false
	for _, cmd := range cmds {
		if cmd.Name() == "init" {
			foundInit = true
		}
		if cmd.Name() == "run" {
			foundRun = true
		}
	}

	if !foundInit {
		t.Errorf("expected 'init' command to be registered in GetAll()")
	}
	if !foundRun {
		t.Errorf("expected 'run' command to be registered in GetAll()")
	}
}
