package cli

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/DanielTangnes/azlocal/internal/compose"
	"github.com/DanielTangnes/azlocal/internal/config"
	"github.com/DanielTangnes/azlocal/internal/health"
	"github.com/DanielTangnes/azlocal/internal/provision"
	"github.com/spf13/cobra"
)

func newUpCmd() *cobra.Command {
	var (
		ci          bool
		waitHealthy bool
		detach      bool
		junitPath   string
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
			ctx := cmd.Context()
			hasContainers := cfg.HasContainers()
			background := detach || ci || waitHealthy || !hasContainers

			// In-process mocks (Key Vault / Event Grid) run as a small
			// background daemon, independent of docker.
			if cfg.HasMocks() {
				if err := startMocksDaemon(cfg); err != nil {
					return err
				}
			}

			if hasContainers {
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
			}

			// --wait, -d, and --ci all return control to us with the suite
			// running, so we can provision resources and load seed data. A
			// plain foreground `up` blocks until interrupted; provision/seed
			// from another shell (or use -d) in that case.
			if background {
				if hasContainers {
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
				}
				printConnectionInfo(cfg)

				if junitPath != "" && hasContainers {
					r, err := health.Check(ctx, "azlocal", expectedServices())
					if err != nil {
						return fmt.Errorf("health check: %w", err)
					}
					if err := r.WriteJUnit(junitPath); err != nil {
						return fmt.Errorf("write junit report: %w", err)
					}
					fmt.Fprintf(os.Stderr, "wrote %s (%s)\n", junitPath, r.Summary())
					if !r.Ok {
						return fmt.Errorf("suite is unhealthy: %s", r.Summary())
					}
				}
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&ci, "ci", false, "CI mode: detach, wait for health, no persistence")
	cmd.Flags().BoolVar(&waitHealthy, "wait-healthy", false, "wait until all services are healthy")
	cmd.Flags().BoolVarP(&detach, "detach", "d", false, "run in background")
	cmd.Flags().StringVar(&junitPath, "junit", "", "write a JUnit XML health report after startup (implies a health check)")

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
