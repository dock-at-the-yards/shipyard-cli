package initcmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/dock-at-the-yards/shipyard-cli/cmd/cmdline/commands"
	"github.com/dock-at-the-yards/shipyard-core/pkg/scaffolder"

	"github.com/spf13/cobra"
)

// ScaffolderFunc defines the signature for the scaffolding executor.
// It can be swapped in tests to verify inputs without touching the filesystem.
type ScaffolderFunc func(structure scaffolder.StructureType, path ...string) ([]string, error)

var (
	// ActiveScaffolder is called by the init subcommands. Defaults to scaffolder.Scaffold.
	ActiveScaffolder ScaffolderFunc = scaffolder.Scaffold
)

var InitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize resources (evalset, eval)",
	Args:  cobra.NoArgs,
}

func resolveResourceName(cmd *cobra.Command, args []string, resourceType string) (string, error) {
	if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
		return strings.TrimSpace(args[0]), nil
	}

	nameFlag, _ := cmd.Flags().GetString("name")
	if strings.TrimSpace(nameFlag) != "" {
		return strings.TrimSpace(nameFlag), nil
	}

	// Prompt user for name
	cmd.Print(fmt.Sprintf("Enter %s name: ", resourceType))
	reader := bufio.NewReader(cmd.InOrStdin())
	input, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("reading input: %w", err)
	}

	name := strings.TrimSpace(input)
	if name == "" {
		return "", fmt.Errorf("%s name cannot be empty", resourceType)
	}

	return name, nil
}

func runInit(cmd *cobra.Command, args []string, structure scaffolder.StructureType) error {
	name, err := resolveResourceName(cmd, args, string(structure))
	if err != nil {
		return err
	}

	basePath, _ := cmd.Flags().GetString("path")
	if strings.TrimSpace(basePath) == "" {
		basePath = "."
	}
	targetDir := filepath.Join(basePath, name)

	cmd.Printf("Initialising the %s now...\n", structure)
	createdFiles, err := ActiveScaffolder(structure, targetDir)
	if err != nil {
		return fmt.Errorf("failed to scaffold %s: %w", structure, err)
	}
	for _, f := range createdFiles {
		cmd.Printf("Created: %s\n", f)
	}
	return nil
}

var initEvalsetCmd = &cobra.Command{
	Use:   "evalset [name]",
	Short: "Initialize an evalset",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runInit(cmd, args, scaffolder.StructureEvalSet)
	},
}

var initEvalCmd = &cobra.Command{
	Use:   "eval [name]",
	Short: "Initialize an eval",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runInit(cmd, args, scaffolder.StructureEval)
	},
}

func init() {
	initEvalsetCmd.Flags().StringP("name", "n", "", "name of the evalset")
	initEvalsetCmd.Flags().StringP("path", "p", "", "parent path where evalset folder will be created (defaults to current directory)")

	initEvalCmd.Flags().StringP("name", "n", "", "name of the eval")
	initEvalCmd.Flags().StringP("path", "p", "", "parent path where eval folder will be created (defaults to current directory)")

	InitCmd.AddCommand(initEvalsetCmd)
	InitCmd.AddCommand(initEvalCmd)
	commands.Register(InitCmd)
}
