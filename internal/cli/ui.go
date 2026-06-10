package cli

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/spf13/cobra"

	"github.com/DanielTangnes/azlocal/internal/config"
	"github.com/DanielTangnes/azlocal/internal/ui"
)

func newUICmd() *cobra.Command {
	var port int
	cmd := &cobra.Command{
		Use:   "ui",
		Short: "Serve the azlocal web dashboard",
		Long: `Serves a local web dashboard for the running suite: browse blob
containers, peek queue and Service Bus messages, scan tables, run Cosmos
queries, and inspect the Key Vault / Event Grid mocks.

The suite should already be running (see "azlocal up -d").`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(cfgFile)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			addr := fmt.Sprintf("127.0.0.1:%d", port)
			ln, err := net.Listen("tcp", addr)
			if err != nil {
				return fmt.Errorf("listen on %s: %w", addr, err)
			}
			srv := &http.Server{
				Handler:           ui.New(cfg).Handler(),
				ReadHeaderTimeout: 10 * time.Second,
			}

			go func() {
				<-cmd.Context().Done()
				_ = srv.Close()
			}()

			fmt.Printf("azlocal dashboard: http://localhost:%d\n", port)
			if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
				return err
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&port, "port", 8900, "port to serve the dashboard on")
	return cmd
}
