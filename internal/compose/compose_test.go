package compose

import (
	"strings"
	"testing"

	"github.com/DanielTangnes/azlocal/internal/config"
)

func TestGenerate_StorageOnly(t *testing.T) {
	cfg := &config.Config{
		Services: config.Services{
			Blob: &config.BlobService{Containers: []string{"uploads"}},
		},
	}
	p, err := Generate(cfg)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if _, ok := p.Services["azurite"]; !ok {
		t.Fatal("expected azurite service")
	}
	if _, ok := p.Services["cosmos"]; ok {
		t.Fatal("did not expect cosmos service")
	}
}

func TestGenerate_AllServices(t *testing.T) {
	cfg := &config.Config{
		Services: config.Services{
			Blob:   &config.BlobService{},
			Cosmos: &config.CosmosService{Databases: []config.CosmosDatabase{{Name: "app"}}},
			ServiceBus: &config.ServiceBusService{
				Queues: []string{"orders"},
			},
		},
	}
	p, err := Generate(cfg)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	for _, name := range []string{"azurite", "cosmos", "servicebus", "servicebus-sql"} {
		if _, ok := p.Services[name]; !ok {
			t.Errorf("missing service %q", name)
		}
	}
}

func TestGenerate_NoServicesIsError(t *testing.T) {
	_, err := Generate(&config.Config{Services: config.Services{}})
	if err == nil {
		t.Fatal("expected error for empty config")
	}
}

func TestMarshal_ContainsServices(t *testing.T) {
	cfg := &config.Config{
		Services: config.Services{Blob: &config.BlobService{}},
	}
	p, _ := Generate(cfg)
	out, err := Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "azurite") {
		t.Fatalf("expected azurite in output, got: %s", out)
	}
}
