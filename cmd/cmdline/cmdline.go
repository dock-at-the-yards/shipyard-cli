package cmdline

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/dock-at-the-yards/shipyard-cli/cmd/cmdline/commands"
	_ "github.com/dock-at-the-yards/shipyard-cli/cmd/cmdline/commands/init"
	_ "github.com/dock-at-the-yards/shipyard-cli/cmd/cmdline/commands/run"
	shiperrs "github.com/dock-at-the-yards/shipyard-core/pkg/errors"

	"github.com/spf13/cobra"
)

// Version is the current release version of Shipyard CLI.
// Can be overridden at build time via -ldflags or at runtime via SHIPYARD_VERSION env var.
var Version = "alpha 0.0.1"

// Commit is the git commit the binary was built from.
// Injected at link time by the release pipeline; empty for local builds.
var Commit = ""

// EngineCommit is the shipyard-core commit the binary was built against.
// The engine is a private repository resolved through a filesystem replace,
// so this is the only record tying a released binary to an engine state.
var EngineCommit = ""

// Channel is the release channel the binary was published on
// (stable, test, or dev). Injected at link time; empty for local builds.
var Channel = ""

// GetVersion returns the active Shipyard version, prioritizing the SHIPYARD_VERSION env var.
func GetVersion() string {
	if envVer := os.Getenv("SHIPYARD_VERSION"); envVer != "" {
		return envVer
	}
	return Version
}

// GetCommit returns the commit the binary was built from, if known.
func GetCommit() string {
	if envCommit := os.Getenv("SHIPYARD_COMMIT"); envCommit != "" {
		return envCommit
	}
	return Commit
}

// GetEngineCommit returns the shipyard-core commit the binary was built
// against, if recorded.
func GetEngineCommit() string {
	if envEngine := os.Getenv("SHIPYARD_ENGINE_COMMIT"); envEngine != "" {
		return envEngine
	}
	return EngineCommit
}

// GetChannel returns the release channel this binary was published on.
func GetChannel() string {
	if envChannel := os.Getenv("SHIPYARD_CHANNEL"); envChannel != "" {
		return envChannel
	}
	if Channel != "" {
		return Channel
	}
	// Infer the channel from the version suffix so untagged local builds and
	// anything not stamped by the pipeline still report something sensible.
	v := GetVersion()
	switch {
	case strings.Contains(v, "-dev"):
		return "dev"
	case strings.Contains(v, "-alpha"), strings.Contains(v, "-beta"), strings.Contains(v, "-rc"):
		return "test"
	default:
		return "dev"
	}
}

var rootCmd = &cobra.Command{
	Use:           "shipyard",
	Short:         "shipyard CLI",
	Version:       GetVersion(),
	SilenceErrors: true,
}

func init() {
	rootCmd.Version = GetVersion()
	rootCmd.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print the shipyard version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("shipyard version %s\n", GetVersion())
			if c := GetCommit(); c != "" {
				fmt.Printf("commit:      %s\n", c)
			}
			if e := GetEngineCommit(); e != "" {
				fmt.Printf("engine:      %s\n", e)
			}
			fmt.Printf("channel:     %s\n", GetChannel())
		},
	})
}

func CmdLine() {
	rootCmd.Version = GetVersion()
	commands.AttachAll(rootCmd)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		if sErr, ok := err.(*shiperrs.ShipyardError); ok {
			fmt.Fprintln(os.Stderr, sErr.FormatColored())
		} else {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}
}
