package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

const sampleConfig = `# azlocal configuration
# Docs: https://github.com/DanielTangnes/azlocal

services:
  blob:
    containers:
      - uploads
      - thumbnails
  queue:
    queues:
      - work-items
  table:
    tables:
      - users

  cosmos:
    databases:
      - name: app
        containers:
          - name: users
            partitionKey: /tenantId
          - name: orders
            partitionKey: /customerId

  servicebus:
    queues:
      - orders
      - notifications
    topics:
      - name: events
        subscriptions:
          - audit
          - billing

# Optional: seed data loaded by "azlocal seed" (and automatically after
# "azlocal up -d"). Supported targets:
#   blob://<container>          from: a file or directory of files to upload
#   queue://<queue>             from: a JSON array of messages
#   table://<table>             from: a JSON array of entities (PartitionKey/RowKey)
#   cosmos://<db>/<container>    from: a JSON array or NDJSON of documents
#   servicebus://<queue|topic>  from: a JSON array of messages
# seed:
#   - target: blob://uploads
#     from: ./fixtures/sample-files/
#   - target: cosmos://app/users
#     from: ./fixtures/users.json
#   - target: servicebus://orders
#     from: ./fixtures/orders.json
`

func newInitCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create a starter azlocal.yaml in the current directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := os.Stat(cfgFile); err == nil && !force {
				return errors.New(cfgFile + " already exists (use --force to overwrite)")
			}
			if err := os.WriteFile(cfgFile, []byte(sampleConfig), 0o644); err != nil {
				return err
			}
			fmt.Println("created", cfgFile)
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "overwrite existing file")
	return cmd
}
