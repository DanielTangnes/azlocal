package mock

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/DanielTangnes/azlocal/internal/config"
)

// Default mock ports (mirrored in provision.ConnectionStrings).
const (
	DefaultKeyVaultPort  = 8200
	DefaultEventGridPort = 8210
)

// Suite owns the configured mock servers for one azlocal project.
type Suite struct {
	KeyVault  *KeyVault
	EventGrid *EventGrid

	kvPort, egPort int
	certDir        string
}

// NewSuite builds the mocks declared in cfg. certDir is where the shared
// self-signed TLS certificate lives (typically .azlocal/certs).
func NewSuite(cfg *config.Config, certDir string) *Suite {
	s := &Suite{certDir: certDir}
	if kv := cfg.Services.KeyVault; kv != nil {
		s.KeyVault = NewKeyVault(kv.Secrets)
		s.kvPort = kv.Port
		if s.kvPort == 0 {
			s.kvPort = DefaultKeyVaultPort
		}
	}
	if eg := cfg.Services.EventGrid; eg != nil {
		s.EventGrid = NewEventGrid(eg)
		s.egPort = eg.Port
		if s.egPort == 0 {
			s.egPort = DefaultEventGridPort
		}
	}
	return s
}

// Empty reports whether no mocks are configured.
func (s *Suite) Empty() bool { return s.KeyVault == nil && s.EventGrid == nil }

// Run starts every configured mock server and blocks until ctx is canceled
// or a server fails. Key Vault serves HTTPS (Azure SDKs refuse bearer tokens
// over plain HTTP); Event Grid serves plain HTTP for curl-friendliness.
func (s *Suite) Run(ctx context.Context, logf func(format string, args ...any)) error {
	if s.Empty() {
		return errors.New("no mock services configured (add services.keyvault or services.eventgrid to azlocal.yaml)")
	}
	if logf == nil {
		logf = func(string, ...any) {}
	}

	errc := make(chan error, 2)
	var servers []*http.Server

	if s.KeyVault != nil {
		cert, certPath, err := EnsureCert(s.certDir)
		if err != nil {
			return fmt.Errorf("key vault mock: tls certificate: %w", err)
		}
		srv := &http.Server{
			Addr:              fmt.Sprintf("127.0.0.1:%d", s.kvPort),
			Handler:           s.KeyVault.Handler(),
			TLSConfig:         &tls.Config{Certificates: []tls.Certificate{cert}},
			ReadHeaderTimeout: 10 * time.Second,
		}
		ln, err := net.Listen("tcp", srv.Addr)
		if err != nil {
			return fmt.Errorf("key vault mock: listen on %s: %w", srv.Addr, err)
		}
		servers = append(servers, srv)
		go func() {
			logf("key vault mock listening on https://localhost:%d (cert: %s)", s.kvPort, certPath)
			if err := srv.ServeTLS(ln, "", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errc <- fmt.Errorf("key vault mock: %w", err)
			}
		}()
	}

	if s.EventGrid != nil {
		srv := &http.Server{
			Addr:              fmt.Sprintf("127.0.0.1:%d", s.egPort),
			Handler:           s.EventGrid.Handler(),
			ReadHeaderTimeout: 10 * time.Second,
		}
		ln, err := net.Listen("tcp", srv.Addr)
		if err != nil {
			return fmt.Errorf("event grid mock: listen on %s: %w", srv.Addr, err)
		}
		servers = append(servers, srv)
		go func() {
			logf("event grid mock listening on http://localhost:%d", s.egPort)
			if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errc <- fmt.Errorf("event grid mock: %w", err)
			}
		}()
	}

	var err error
	select {
	case <-ctx.Done():
	case err = <-errc:
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, srv := range servers {
		_ = srv.Shutdown(shutdownCtx)
	}
	if s.EventGrid != nil {
		s.EventGrid.Flush()
	}
	return err
}

// Probe checks whether a mock endpoint is accepting requests. It treats any
// HTTP response (including 401 challenges) as alive.
func Probe(url string) bool {
	client := &http.Client{
		Timeout: 2 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // local mock only
		},
	}
	resp, err := client.Get(url)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return true
}
