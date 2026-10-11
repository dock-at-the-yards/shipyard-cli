package initcmd_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/dock-at-the-yards/shipyard-cli/cmd/cmdline/commands"
	initcmd "github.com/dock-at-the-yards/shipyard-cli/cmd/cmdline/commands/init"
	"github.com/dock-at-the-yards/shipyard-core/pkg/scaffolder"

	"github.com/spf13/cobra"
)

func executeCommandWithInput(root *cobra.Command, input string, args ...string) (string, error) {
	buf := new(bytes.Buffer)
	inBuf := bytes.NewBufferString(input)
	root.SetIn(inBuf)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(args)

	_, err := root.ExecuteC()
	return buf.String(), err
}

func executeCommand(root *cobra.Command, args ...string) (string, error) {
	return executeCommandWithInput(root, "", args...)
}

func TestInitSubcommands(t *testing.T) {
	var lastStructure scaffolder.StructureType
	var lastPath string
	callCount := 0

	origScaffolder := initcmd.ActiveScaffolder
	initcmd.ActiveScaffolder = func(structure scaffolder.StructureType, path ...string) ([]string, error) {
		callCount++
		lastStructure = structure
		if len(path) > 0 {
			lastPath = path[0]
		} else {
			lastPath = ""
		}
		return []string{"mocked/file"}, nil
	}
	defer func() {
		initcmd.ActiveScaffolder = origScaffolder
	}()

	tests := []struct {
		name              string
		args              []string
		input             string
		expectedOutput    string
		expectErr         bool
		wantCall          bool
		expectedStructure scaffolder.StructureType
		expectedPath      string
	}{
		{
			name:              "init evalset with positional name and default path",
			args:              []string{"init", "evalset", "my-benchmark"},
			expectedOutput:    "Initialising the evalset now...",
			expectErr:         false,
			wantCall:          true,
			expectedStructure: scaffolder.StructureEvalSet,
			expectedPath:      "my-benchmark",
		},
		{
			name:              "init evalset with positional name and custom path",
			args:              []string{"init", "evalset", "my-benchmark", "--path", "evalsets"},
			expectedOutput:    "Initialising the evalset now...",
			expectErr:         false,
			wantCall:          true,
			expectedStructure: scaffolder.StructureEvalSet,
			expectedPath:      "evalsets/my-benchmark",
		},
		{
			name:              "init evalset with --name flag",
			args:              []string{"init", "evalset", "--name", "flag-benchmark", "-p", "custom/dir"},
			expectedOutput:    "Initialising the evalset now...",
			expectErr:         false,
			wantCall:          true,
			expectedStructure: scaffolder.StructureEvalSet,
			expectedPath:      "custom/dir/flag-benchmark",
		},
		{
			name:              "init evalset prompts for name via stdin when omitted",
			args:              []string{"init", "evalset"},
			input:             "prompted-benchmark\n",
			expectedOutput:    "Enter evalset name: Initialising the evalset now...",
			expectErr:         false,
			wantCall:          true,
			expectedStructure: scaffolder.StructureEvalSet,
			expectedPath:      "prompted-benchmark",
		},
		{
			name:           "init evalset fails when prompted name is empty",
			args:           []string{"init", "evalset"},
			input:          "\n",
			expectedOutput: "Enter evalset name: ",
			expectErr:      true,
			wantCall:       false,
		},
		{
			name:              "init eval with positional name and default path",
			args:              []string{"init", "eval", "refund-task"},
			expectedOutput:    "Initialising the eval now...",
			expectErr:         false,
			wantCall:          true,
			expectedStructure: scaffolder.StructureEval,
			expectedPath:      "refund-task",
		},
		{
			name:              "init eval with positional name and custom path",
			args:              []string{"init", "eval", "refund-task", "--path", "evals"},
			expectedOutput:    "Initialising the eval now...",
			expectErr:         false,
			wantCall:          true,
			expectedStructure: scaffolder.StructureEval,
			expectedPath:      "evals/refund-task",
		},
		{
			name:              "init eval with -n flag",
			args:              []string{"init", "eval", "-n", "flag-eval"},
			expectedOutput:    "Initialising the eval now...",
			expectErr:         false,
			wantCall:          true,
			expectedStructure: scaffolder.StructureEval,
			expectedPath:      "flag-eval",
		},
		{
			name:              "init eval prompts for name via stdin when omitted",
			args:              []string{"init", "eval"},
			input:             "prompted-eval\n",
			expectedOutput:    "Enter eval name: Initialising the eval now...",
			expectErr:         false,
			wantCall:          true,
			expectedStructure: scaffolder.StructureEval,
			expectedPath:      "prompted-eval",
		},
		{
			name:           "init evalset rejects extra positional args (>1)",
			args:           []string{"init", "evalset", "name1", "name2"},
			expectedOutput: "accepts at most 1 arg(s)",
			expectErr:      true,
			wantCall:       false,
		},
		{
			name:           "init eval rejects extra positional args (>1)",
			args:           []string{"init", "eval", "name1", "name2"},
			expectedOutput: "accepts at most 1 arg(s)",
			expectErr:      true,
			wantCall:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lastStructure = ""
			lastPath = ""
			beforeCalls := callCount

			root := &cobra.Command{Use: "shipyard"}
			commands.AttachAll(root)

			// Reset flags so they don't leak between subtests on the singleton command
			initcmd.InitCmd.ResetFlags()
			for _, sub := range initcmd.InitCmd.Commands() {
				_ = sub.Flags().Set("path", "")
				_ = sub.Flags().Set("name", "")
			}

			output, err := executeCommandWithInput(root, tt.input, tt.args...)
			if tt.expectErr && err == nil {
				t.Fatalf("expected error, got nil, output: %s", output)
			}
			if !tt.expectErr && err != nil {
				t.Fatalf("unexpected error: %v, output: %s", err, output)
			}

			if !tt.expectErr && !strings.Contains(output, tt.expectedOutput) {
				t.Errorf("expected output to contain %q, got %q", tt.expectedOutput, output)
			}

			if tt.wantCall {
				if callCount == beforeCalls {
					t.Errorf("expected ActiveScaffolder to be called, but wasn't")
				}
				if lastStructure != tt.expectedStructure {
					t.Errorf("expected structure %q, got %q", tt.expectedStructure, lastStructure)
				}
				if lastPath != tt.expectedPath {
					t.Errorf("expected path %q, got %q", tt.expectedPath, lastPath)
				}
			} else {
				if callCount != beforeCalls {
					t.Errorf("ActiveScaffolder should not be called on invalid args or failed prompt")
				}
			}
		})
	}
}
