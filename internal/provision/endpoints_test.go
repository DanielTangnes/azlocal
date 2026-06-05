package provision

import (
	"strings"
	"testing"

	"github.com/DanielTangnes/azlocal/internal/config"
)

func TestStorageConnString_Defaults(t *testing.T) {
	cfg := &config.Config{Services: config.Services{Blob: &config.BlobService{}}}
	got := storageConnString(cfg)
	for _, want := range []string{
		"AccountName=devstoreaccount1",
		"AccountKey=" + devAccountKey,
		"BlobEndpoint=http://127.0.0.1:10000/devstoreaccount1",
		"QueueEndpoint=http://127.0.0.1:10001/devstoreaccount1",
		"TableEndpoint=http://127.0.0.1:10002/devstoreaccount1",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("connection string missing %q\ngot: %s", want, got)
		}
	}
}

func TestStorageConnString_PortOverrides(t *testing.T) {
	cfg := &config.Config{Services: config.Services{
		Blob:  &config.BlobService{Port: 20000},
		Queue: &config.QueueService{Port: 20001},
		Table: &config.TableService{Port: 20002},
	}}
	got := storageConnString(cfg)
	for _, want := range []string{
		"BlobEndpoint=http://127.0.0.1:20000/devstoreaccount1",
		"QueueEndpoint=http://127.0.0.1:20001/devstoreaccount1",
		"TableEndpoint=http://127.0.0.1:20002/devstoreaccount1",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("connection string missing override %q\ngot: %s", want, got)
		}
	}
}

func TestCosmosEndpoint(t *testing.T) {
	def := &config.Config{Services: config.Services{Cosmos: &config.CosmosService{}}}
	if got := cosmosEndpoint(def); got != "https://localhost:8081" {
		t.Errorf("default cosmos endpoint = %q", got)
	}
	custom := &config.Config{Services: config.Services{Cosmos: &config.CosmosService{Port: 9090}}}
	if got := cosmosEndpoint(custom); got != "https://localhost:9090" {
		t.Errorf("custom cosmos endpoint = %q", got)
	}
}
