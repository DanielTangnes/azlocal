package cli

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
	"github.com/DanielTangnes/azlocal/internal/compose"
	"github.com/DanielTangnes/azlocal/internal/config"
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
	return &cobra.Command{
		Use:   "status",
		Short: "Show status of running emulator services",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := exec.Command("docker", "compose", "-p", "azlocal", "ps")
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			return c.Run()
		},
	}
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
