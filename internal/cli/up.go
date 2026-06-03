package cli

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
	"github.com/DanielTangnes/azlocal/internal/compose"
	"github.com/DanielTangnes/azlocal/internal/config"
)

func newUpCmd() *cobra.Command {
	var (
		ci          bool
		waitHealthy bool
		detach      bool
	)

	cmd := &cobra.Command{
		Use:   "up",
		Short: "Start the local Azure emulator suite",
		Long: `Reads azlocal.yaml, generates a docker-compose project, and starts
the configured emulators.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(cfgFile)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			project, err := compose.Generate(cfg)
			if err != nil {
				return fmt.Errorf("generate compose project: %w", err)
			}

			path, err := compose.Write(project)
			if err != nil {
				return fmt.Errorf("write compose file: %w", err)
			}
			if verbose {
				fmt.Fprintf(os.Stderr, "wrote compose file: %s\n", path)
			}

			composeArgs := []string{"compose", "-f", path, "-p", "azlocal", "up"}
			if detach || ci {
				composeArgs = append(composeArgs, "-d")
			}
			if waitHealthy || ci {
				composeArgs = append(composeArgs, "--wait")
			}

			c := exec.Command("docker", composeArgs...)
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			c.Stdin = os.Stdin
			if err := c.Run(); err != nil {
				return fmt.Errorf("docker compose up: %w", err)
			}

			if detach || ci {
				printConnectionInfo(cfg)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&ci, "ci", false, "CI mode: detach, wait for health, no persistence")
	cmd.Flags().BoolVar(&waitHealthy, "wait-healthy", false, "wait until all services are healthy")
	cmd.Flags().BoolVarP(&detach, "detach", "d", false, "run in background")

	return cmd
}

func printConnectionInfo(cfg *config.Config) {
	fmt.Println()
	fmt.Println("azlocal is running. Connection strings:")
	fmt.Println()
	if cfg.Services.Blob != nil || cfg.Services.Queue != nil || cfg.Services.Table != nil {
		fmt.Println("  AZURE_STORAGE_CONNECTION_STRING=" +
			"DefaultEndpointsProtocol=http;" +
			"AccountName=devstoreaccount1;" +
			"AccountKey=Eby8vdM02xNOcqFlqUwJPLlmEEGc...;" +
			"BlobEndpoint=http://127.0.0.1:10000/devstoreaccount1;" +
			"QueueEndpoint=http://127.0.0.1:10001/devstoreaccount1;" +
			"TableEndpoint=http://127.0.0.1:10002/devstoreaccount1;")
	}
	if cfg.Services.Cosmos != nil {
		fmt.Println("  COSMOS_ENDPOINT=https://localhost:8081")
		fmt.Println("  COSMOS_KEY=C2y6yDjf5/R+ob0N8A7Cgv30VRDJIWEHLM+4QDU5DE2nQ9nDuVTqobD4b8mGGyPMbIZnqyMsEcaGQy67XIw/Jw==")
	}
	if cfg.Services.ServiceBus != nil {
		fmt.Println("  SERVICEBUS_CONNECTION_STRING=Endpoint=sb://localhost;SharedAccessKeyName=RootManageSharedAccessKey;SharedAccessKey=SAS_KEY_VALUE;UseDevelopmentEmulator=true;")
	}
	fmt.Println()
}
