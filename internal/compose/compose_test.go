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

func TestServiceBusSQL_ImageAndHealthcheck(t *testing.T) {
	_, sql := serviceBusServices(&config.ServiceBusService{Queues: []string{"orders"}})

	if sql.Image != "mcr.microsoft.com/mssql/server:2022-latest" {
		t.Errorf("unexpected sql image %q", sql.Image)
	}
	// SQL Server 2022 has no native arm64 image; the platform hint must stay
	// so it runs under emulation on Apple Silicon.
	if sql.Platform != "linux/amd64" {
		t.Errorf("expected platform linux/amd64, got %q", sql.Platform)
	}
	if sql.Healthcheck == nil {
		t.Fatal("expected a healthcheck on servicebus-sql")
	}
	got := strings.Join(sql.Healthcheck.Test, " ")
	for _, want := range []string{"/opt/mssql-tools18/bin/sqlcmd", "-C"} {
		if !strings.Contains(got, want) {
			t.Errorf("healthcheck missing %q, got: %s", want, got)
		}
	}
	if strings.Contains(got, "/opt/mssql-tools/bin/sqlcmd") {
		t.Errorf("healthcheck still uses the stale mssql-tools path: %s", got)
	}
}

func TestGenerate_NoServicesIsError(t *testing.T) {
	_, err := Generate(&config.Config{Services: config.Services{}})
	if err == nil {
		t.Fatal("expected error for empty config")
	}
}

func TestServiceBusConfig_ContainsEntities(t *testing.T) {
	sb := &config.ServiceBusService{
		Queues: []string{"orders", "notifications"},
		Topics: []config.ServiceBusTopic{
			{Name: "events", Subscriptions: []string{"audit", "billing"}},
		},
	}
	out, err := GenerateServiceBusConfig(sb)
	if err != nil {
		t.Fatalf("GenerateServiceBusConfig: %v", err)
	}
	s := string(out)
	for _, want := range []string{
		`"Name": "sbemulatorns"`, // default namespace
		`"Name": "orders"`,
		`"Name": "notifications"`,
		`"Name": "events"`,
		`"Name": "audit"`,
		`"Name": "billing"`,
		`"DefaultMessageTimeToLive": "PT1H"`, // default properties emitted
		`"MaxDeliveryCount": 3`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("service bus config missing %q\ngot: %s", want, s)
		}
	}
}

func TestServiceBusConfig_CustomNamespace(t *testing.T) {
	out, err := GenerateServiceBusConfig(&config.ServiceBusService{
		Namespace: "myns",
		Queues:    []string{"q1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"Name": "myns"`) {
		t.Errorf("expected custom namespace in output: %s", out)
	}
}

func TestGenerate_MountsServiceBusConfig(t *testing.T) {
	cfg := &config.Config{Services: config.Services{
		ServiceBus: &config.ServiceBusService{Queues: []string{"orders"}},
	}}
	p, err := Generate(cfg)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	sb, ok := p.Services["servicebus"]
	if !ok {
		t.Fatal("expected servicebus service")
	}
	found := false
	for _, v := range sb.Volumes {
		if strings.Contains(v, "servicebus-config.json") &&
			strings.Contains(v, "/ServiceBus_Emulator/ConfigFiles/Config.json") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected servicebus-config.json mount, got volumes: %v", sb.Volumes)
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
