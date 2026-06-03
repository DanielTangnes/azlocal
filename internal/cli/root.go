package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	versionStr = "dev"
	commitStr  = "none"
	dateStr    = "unknown"
)

// SetVersion injects build metadata from main.
func SetVersion(v, c, d string) {
	versionStr, commitStr, dateStr = v, c, d
}

var (
	cfgFile string
	verbose bool
)

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "azlocal",
		Short: "A unified local Azure emulator suite",
		Long: `azlocal orchestrates Azurite, Cosmos DB emulator, Service Bus emulator,
and lightweight mocks for other Azure services behind a single CLI.

Run "azlocal up" in a directory containing an azlocal.yaml to start the
emulator suite. Run "azlocal status" to see what's running.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       fmt.Sprintf("%s (commit %s, built %s)", versionStr, commitStr, dateStr),
	}

	root.PersistentFlags().StringVarP(&cfgFile, "config", "c", "azlocal.yaml", "path to azlocal config file")
	root.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")

	root.AddCommand(
		newUpCmd(),
		newDownCmd(),
		newStatusCmd(),
		newLogsCmd(),
		newInitCmd(),
		newRenderCmd(),
	)

	return root
}

// Execute runs the root command.
func Execute() error {
	return newRootCmd().Execute()
}
