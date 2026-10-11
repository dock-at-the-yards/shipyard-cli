package commands

import (
	"github.com/spf13/cobra"
)

/**
DO NOT EDIT THIS.
*/

var registry []*cobra.Command

func Register(cmd *cobra.Command) {
	registry = append(registry, cmd)
}

func GetAll() []*cobra.Command {
	return registry
}

func AttachAll(rootCmd *cobra.Command) {
	for _, cmd := range registry {
		rootCmd.AddCommand(cmd)
	}
}
