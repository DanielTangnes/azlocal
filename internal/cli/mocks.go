package cli

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/DanielTangnes/azlocal/internal/config"
	"github.com/DanielTangnes/azlocal/internal/mock"
	"github.com/DanielTangnes/azlocal/internal/provision"
)

// Paths under .azlocal used by the mocks daemon. They live next to the
// generated compose file so "down" and "status" can find them.
var (
	mocksPidPath = filepath.Join(".azlocal", "mocks.pid")
	mocksLogPath = filepath.Join(".azlocal", "mocks.log")
	mockCertDir  = filepath.Join(".azlocal", "certs")
)

func newMocksCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mocks",
		Short: "Run the configured Key Vault / Event Grid mocks in the foreground",
		Long: `Runs the in-process mocks declared under services.keyvault and
services.eventgrid. "azlocal up" starts these in the background automatically;
this command runs them in the foreground (useful for debugging, or when you
only need the mocks and not the docker-based emulators).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(cfgFile)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			suite := mock.NewSuite(cfg, mockCertDir)
			if suite.Empty() {
				return fmt.Errorf("no mock services configured (add services.keyvault or services.eventgrid to %s)", cfgFile)
			}

			if err := writePidFile(); err != nil {
				return err
			}
			defer os.Remove(mocksPidPath)

			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			logf := func(format string, args ...any) {
				fmt.Printf(format+"\n", args...)
			}
			return suite.Run(ctx, logf)
		},
	}
}

func writePidFile() error {
	if err := os.MkdirAll(filepath.Dir(mocksPidPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(mocksPidPath, []byte(strconv.Itoa(os.Getpid())), 0o644)
}

// mocksRunning probes the configured mock endpoints. Any responding endpoint
// counts as running.
func mocksRunning(cfg *config.Config) bool {
	if cfg.Services.KeyVault != nil && mock.Probe(provision.KeyVaultEndpoint(cfg)+"/secrets?api-version=7.4") {
		return true
	}
	if cfg.Services.EventGrid != nil && mock.Probe(provision.EventGridEndpoint(cfg)+"/topics") {
		return true
	}
	return false
}

// startMocksDaemon launches "azlocal mocks" as a detached background process,
// logging to .azlocal/mocks.log. It is a no-op when no mocks are configured
// or they are already running.
func startMocksDaemon(cfg *config.Config) error {
	if !cfg.HasMocks() {
		return nil
	}
	if mocksRunning(cfg) {
		fmt.Println("mocks already running")
		return nil
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate azlocal binary: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(mocksLogPath), 0o755); err != nil {
		return err
	}
	logFile, err := os.OpenFile(mocksLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer logFile.Close()

	c := exec.Command(exe, "mocks", "-c", cfgFile)
	c.Stdout = logFile
	c.Stderr = logFile
	c.SysProcAttr = detachSysProcAttr()
	if err := c.Start(); err != nil {
		return fmt.Errorf("start mocks daemon: %w", err)
	}
	pid := c.Process.Pid
	if err := c.Process.Release(); err != nil {
		return err
	}

	// Wait briefly for the endpoints to accept requests.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if mocksRunning(cfg) {
			fmt.Printf("mocks running (pid %d, logs: %s)\n", pid, mocksLogPath)
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("mocks daemon (pid %d) did not become ready; see %s", pid, mocksLogPath)
}

// stopMocksDaemon terminates the mocks daemon recorded in the pidfile, if any.
func stopMocksDaemon() error {
	data, err := os.ReadFile(mocksPidPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		_ = os.Remove(mocksPidPath)
		return nil
	}
	proc, err := os.FindProcess(pid)
	if err == nil {
		if killErr := terminate(proc); killErr == nil {
			fmt.Printf("stopped mocks (pid %d)\n", pid)
		}
	}
	// The daemon removes its own pidfile on graceful shutdown, so a missing
	// file here is fine.
	if err := os.Remove(mocksPidPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// terminate asks the process to exit gracefully, falling back to Kill on
// platforms without SIGTERM delivery.
func terminate(p *os.Process) error {
	if err := p.Signal(syscall.SIGTERM); err == nil {
		// Give it a moment to shut down before resorting to Kill.
		done := make(chan struct{})
		go func() {
			for i := 0; i < 20; i++ {
				if p.Signal(syscall.Signal(0)) != nil {
					break
				}
				time.Sleep(100 * time.Millisecond)
			}
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
		}
		return nil
	}
	return p.Kill()
}

// mockHealthLines renders human status lines for the configured mocks.
func mockHealthLines(cfg *config.Config) []string {
	var lines []string
	if cfg.Services.KeyVault != nil {
		lines = append(lines, mockStatusLine("keyvault-mock", provision.KeyVaultEndpoint(cfg),
			mock.Probe(provision.KeyVaultEndpoint(cfg)+"/secrets?api-version=7.4")))
	}
	if cfg.Services.EventGrid != nil {
		lines = append(lines, mockStatusLine("eventgrid-mock", provision.EventGridEndpoint(cfg),
			mock.Probe(provision.EventGridEndpoint(cfg)+"/topics")))
	}
	return lines
}

func mockStatusLine(name, endpoint string, up bool) string {
	state := "stopped"
	if up {
		state = "running"
	}
	return fmt.Sprintf("%-16s %-9s %s", name, state, endpoint)
}
