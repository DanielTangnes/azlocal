package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/DanielTangnes/azlocal/internal/config"
	"github.com/DanielTangnes/azlocal/internal/provision"
)

func newProvisionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "provision",
		Short: "Create the resources declared in azlocal.yaml in the running emulators",
		Long: `Creates blob containers, queues, tables, and Cosmos databases/containers
declared in azlocal.yaml. Service Bus entities are created from the mounted
Config.json when the suite starts. The emulators must already be running
(see "azlocal up -d").`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(cfgFile)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			fmt.Println("Provisioning resources...")
			return provision.CreateResources(cmd.Context(), cfg)
		},
	}
}

func newSeedCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "seed",
		Short: "Load seed data declared in azlocal.yaml into the running emulators",
		Long: `Loads each entry under "seed:" into the emulators: blob files, Cosmos
documents, queue messages, table rows, and Service Bus messages. The emulators
must already be running (see "azlocal up -d").`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(cfgFile)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			if len(cfg.Seed) == 0 {
				fmt.Println("no seed entries in config; nothing to do")
				return nil
			}
			fmt.Println("Seeding data...")
			return provision.Seed(cmd.Context(), cfg)
		},
	}
}
