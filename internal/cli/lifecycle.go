package cli

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/DanielTangnes/azlocal/internal/compose"
	"github.com/DanielTangnes/azlocal/internal/config"
	"github.com/DanielTangnes/azlocal/internal/health"
	"github.com/spf13/cobra"
)

func newDownCmd() *cobra.Command {
	var volumes bool

	cmd := &cobra.Command{
		Use:   "down",
		Short: "Stop the local Azure emulator suite",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := compose.DefaultPath()
			if _, err := os.Stat(path); os.IsNotExist(err) {
				cfg, err := config.Load(cfgFile)
				if err != nil {
					return fmt.Errorf("load config: %w", err)
				}
				path, err = compose.WriteProject(cfg)
				if err != nil {
					return err
				}
			}

			args2 := []string{"compose", "-f", path, "-p", "azlocal", "down"}
			if volumes {
				args2 = append(args2, "-v")
			}
			c := exec.Command("docker", args2...)
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			return c.Run()
		},
	}

	cmd.Flags().BoolVar(&volumes, "volumes", false, "also remove named volumes (data is lost)")
	return cmd
}

func newStatusCmd() *cobra.Command {
	var (
		jsonOut   bool
		junitPath string
	)
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show status of running emulator services",
		Long: `Shows the status of the emulator suite.

With --json or --junit the status is checked programmatically and the command
exits non-zero if any expected service is missing or unhealthy, making it
suitable as a CI gate:

  azlocal status --junit health-report.xml`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !jsonOut && junitPath == "" {
				c := exec.Command("docker", "compose", "-p", "azlocal", "ps")
				c.Stdout = os.Stdout
				c.Stderr = os.Stderr
				return c.Run()
			}
			r, err := health.Check(cmd.Context(), "azlocal", expectedServices())
			if err != nil {
				return err
			}
			if jsonOut {
				out, err := r.JSON()
				if err != nil {
					return err
				}
				fmt.Println(string(out))
			}
			if junitPath != "" {
				if err := r.WriteJUnit(junitPath); err != nil {
					return fmt.Errorf("write junit report: %w", err)
				}
				fmt.Fprintf(os.Stderr, "wrote %s (%s)\n", junitPath, r.Summary())
			}
			if !r.Ok {
				return fmt.Errorf("suite is unhealthy: %s", r.Summary())
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "print a machine-readable health report")
	cmd.Flags().StringVar(&junitPath, "junit", "", "write a JUnit XML health report to this path")
	return cmd
}

// expectedServices derives the compose service names the config implies, so
// health reports flag services that never started. Best-effort: an unloadable
// config just means no absence detection.
func expectedServices() []string {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return nil
	}
	project, err := compose.Generate(cfg)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(project.Services))
	for name := range project.Services {
		names = append(names, name)
	}
	return names
}

func newLogsCmd() *cobra.Command {
	var follow bool
	cmd := &cobra.Command{
		Use:   "logs [service]",
		Short: "Tail logs from one or all services",
		RunE: func(cmd *cobra.Command, args []string) error {
			a := []string{"compose", "-p", "azlocal", "logs"}
			if follow {
				a = append(a, "-f")
			}
			a = append(a, args...)
			c := exec.Command("docker", a...)
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			return c.Run()
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "follow log output")
	return cmd
}

func newRenderCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "render",
		Short: "Print the generated docker-compose.yaml without starting it",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(cfgFile)
			if err != nil {
				return err
			}
			project, err := compose.Generate(cfg)
			if err != nil {
				return err
			}
			out, err := compose.Marshal(project)
			if err != nil {
				return err
			}
			fmt.Print(string(out))

			if cfg.Services.ServiceBus != nil {
				sbCfg, err := compose.GenerateServiceBusConfig(cfg.Services.ServiceBus)
				if err != nil {
					return err
				}
				fmt.Printf("\n---\n# servicebus-config.json\n%s\n", sbCfg)
			}
			return nil
		},
	}
}
