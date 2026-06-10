package cli

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
	"github.com/DanielTangnes/azlocal/internal/compose"
	"github.com/DanielTangnes/azlocal/internal/config"
	"github.com/DanielTangnes/azlocal/internal/provision"
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

			path, err := compose.WriteProject(cfg)
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

			background := detach || ci || waitHealthy
			if !background {
				fmt.Fprintln(os.Stderr, "note: foreground mode; resources will not be provisioned or seeded automatically (use -d, or run \"azlocal provision\"/\"azlocal seed\" from another shell)")
			}

			c := exec.Command("docker", composeArgs...)
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			c.Stdin = os.Stdin
			if err := c.Run(); err != nil {
				return fmt.Errorf("docker compose up: %w", err)
			}

			// --wait, -d, and --ci all return control to us with the suite
			// running, so we can provision resources and load seed data. A
			// plain foreground `up` blocks until interrupted; provision/seed
			// from another shell (or use -d) in that case.
			if background {
				ctx := cmd.Context()
				fmt.Println("\nProvisioning resources...")
				if err := provision.CreateResources(ctx, cfg); err != nil {
					return fmt.Errorf("provision resources: %w", err)
				}
				if len(cfg.Seed) > 0 {
					fmt.Println("Seeding data...")
					if err := provision.Seed(ctx, cfg); err != nil {
						return fmt.Errorf("seed data: %w", err)
					}
				}
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

// printConnectionInfo prints the connection lines built by the provision
// package, so the output always honors port overrides and never drifts from
// the endpoints the SDK clients actually use.
func printConnectionInfo(cfg *config.Config) {
	fmt.Println()
	fmt.Println("azlocal is running. Connection strings:")
	fmt.Println()
	for _, line := range provision.ConnectionStrings(cfg) {
		fmt.Println("  " + line)
	}
	fmt.Println()
}
