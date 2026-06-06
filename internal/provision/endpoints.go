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

// Default published ports, matching compose.azuriteService / cosmosService.
const (
	defaultBlobPort   = 10000
	defaultQueuePort  = 10001
	defaultTablePort  = 10002
	defaultCosmosPort = 8081
)

func portOr(port, def int) int {
	if port != 0 {
		return port
	}
	return def
}

// storageConnString builds the Azurite connection string, honoring per-service
// port overrides from the config.
func storageConnString(cfg *config.Config) string {
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

// cosmosEndpoint builds the Cosmos emulator endpoint, honoring a port override.
func cosmosEndpoint(cfg *config.Config) string {
	port := defaultCosmosPort
	if cfg.Services.Cosmos != nil {
		port = portOr(cfg.Services.Cosmos.Port, port)
	}
	return fmt.Sprintf("https://localhost:%d", port)
}

// serviceBusConnString returns the emulator data-plane connection string.
func serviceBusConnString() string { return serviceBusConn }

// insecureHTTPClient trusts the Cosmos emulator's self-signed certificate.
func insecureHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // local emulator only
		},
	}
}
