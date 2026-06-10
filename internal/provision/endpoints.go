// Package provision creates the resources declared in an azlocal Config and
// loads seed data into the running emulators, using the Azure SDK for Go.
package provision

import (
	"crypto/tls"
	"fmt"
	"net/http"

	"github.com/DanielTangnes/azlocal/internal/config"
)

// Well-known emulator credentials. These are public, fixed test values shipped
// by the emulators themselves — they carry no security significance.
const (
	devAccountName = "devstoreaccount1"
	devAccountKey  = "Eby8vdM02xNOcqFlqUwJPLlmEtlCDXJ1OUzFT50uSRZ6IFsuFq2UVErCz4I6tq/K1SZFPTOtr/KBHBeksoGMGw=="
	cosmosKey      = "C2y6yDjf5/R+ob0N8A7Cgv30VRDJIWEHLM+4QDU5DE2nQ9nDuVTqobD4b8mGGyPMbIZnqyMsEcaGQy67XIw/Jw=="
	// serviceBusConn is the static emulator connection string. With
	// UseDevelopmentEmulator=true the SDK connects over AMQP on localhost:5672.
	serviceBusConn = "Endpoint=sb://localhost;SharedAccessKeyName=RootManageSharedAccessKey;SharedAccessKey=SAS_KEY_VALUE;UseDevelopmentEmulator=true;"
)

// Default published ports, matching compose.azuriteService / cosmosService and
// the built-in mocks.
const (
	defaultBlobPort      = 10000
	defaultQueuePort     = 10001
	defaultTablePort     = 10002
	defaultCosmosPort    = 8081
	defaultKeyVaultPort  = 8200
	defaultEventGridPort = 8210
)

func portOr(port, def int) int {
	if port != 0 {
		return port
	}
	return def
}

// StorageConnString builds the Azurite connection string, honoring per-service
// port overrides from the config.
func StorageConnString(cfg *config.Config) string {
	blob, queue, table := defaultBlobPort, defaultQueuePort, defaultTablePort
	if cfg.Services.Blob != nil {
		blob = portOr(cfg.Services.Blob.Port, blob)
	}
	if cfg.Services.Queue != nil {
		queue = portOr(cfg.Services.Queue.Port, queue)
	}
	if cfg.Services.Table != nil {
		table = portOr(cfg.Services.Table.Port, table)
	}
	return fmt.Sprintf(
		"DefaultEndpointsProtocol=http;AccountName=%s;AccountKey=%s;"+
			"BlobEndpoint=http://127.0.0.1:%d/%s;"+
			"QueueEndpoint=http://127.0.0.1:%d/%s;"+
			"TableEndpoint=http://127.0.0.1:%d/%s;",
		devAccountName, devAccountKey,
		blob, devAccountName,
		queue, devAccountName,
		table, devAccountName,
	)
}

// CosmosEndpoint builds the Cosmos emulator endpoint, honoring a port override.
func CosmosEndpoint(cfg *config.Config) string {
	port := defaultCosmosPort
	if cfg.Services.Cosmos != nil {
		port = portOr(cfg.Services.Cosmos.Port, port)
	}
	return fmt.Sprintf("https://localhost:%d", port)
}

// CosmosKey returns the well-known Cosmos emulator key.
func CosmosKey() string { return cosmosKey }

// ServiceBusConnString returns the emulator data-plane connection string.
func ServiceBusConnString() string { return serviceBusConn }

// KeyVaultEndpoint builds the Key Vault mock endpoint, honoring a port override.
func KeyVaultEndpoint(cfg *config.Config) string {
	port := defaultKeyVaultPort
	if cfg.Services.KeyVault != nil {
		port = portOr(cfg.Services.KeyVault.Port, port)
	}
	return fmt.Sprintf("https://localhost:%d", port)
}

// EventGridEndpoint builds the Event Grid mock base endpoint, honoring a port
// override. Topic publish endpoints are <base>/<topic>/api/events.
func EventGridEndpoint(cfg *config.Config) string {
	port := defaultEventGridPort
	if cfg.Services.EventGrid != nil {
		port = portOr(cfg.Services.EventGrid.Port, port)
	}
	return fmt.Sprintf("http://localhost:%d", port)
}

// ConnectionStrings returns the environment-style connection lines for every
// enabled service, honoring port overrides. Used by "up" output and the UI.
func ConnectionStrings(cfg *config.Config) []string {
	var out []string
	s := cfg.Services
	if s.Blob != nil || s.Queue != nil || s.Table != nil {
		out = append(out, "AZURE_STORAGE_CONNECTION_STRING="+StorageConnString(cfg))
	}
	if s.Cosmos != nil {
		out = append(out,
			"COSMOS_ENDPOINT="+CosmosEndpoint(cfg),
			"COSMOS_KEY="+cosmosKey,
		)
	}
	if s.ServiceBus != nil {
		out = append(out, "SERVICEBUS_CONNECTION_STRING="+serviceBusConn)
	}
	if s.KeyVault != nil {
		out = append(out, "KEYVAULT_URL="+KeyVaultEndpoint(cfg))
	}
	if s.EventGrid != nil {
		out = append(out, "EVENTGRID_ENDPOINT="+EventGridEndpoint(cfg))
	}
	return out
}

// InsecureHTTPClient trusts the self-signed certificates served by the Cosmos
// emulator and the Key Vault mock.
func InsecureHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // local emulator only
		},
	}
}
