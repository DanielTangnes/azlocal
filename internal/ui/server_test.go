package ui

import (
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/DanielTangnes/azlocal/internal/config"
	"github.com/DanielTangnes/azlocal/internal/mock"
)

func testConfig() *config.Config {
	return &config.Config{Services: config.Services{
		Blob:   &config.BlobService{Containers: []string{"uploads"}},
		Cosmos: &config.CosmosService{Databases: []config.CosmosDatabase{{Name: "app", Containers: []config.CosmosContainer{{Name: "users", PartitionKey: "/id"}}}}},
		ServiceBus: &config.ServiceBusService{
			Queues: []string{"orders"},
			Topics: []config.ServiceBusTopic{{Name: "events", Subscriptions: []string{"audit"}}},
		},
	}}
}

func TestOverview(t *testing.T) {
	srv := httptest.NewServer(New(testConfig()).Handler())
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL + "/api/overview")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var o struct {
		Services          map[string]bool `json:"services"`
		ConnectionStrings []string        `json:"connectionStrings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&o); err != nil {
		t.Fatal(err)
	}
	if !o.Services["blob"] || !o.Services["cosmos"] || o.Services["keyvault"] {
		t.Errorf("services = %v", o.Services)
	}
	if len(o.ConnectionStrings) != 4 { // storage + cosmos endpoint/key + servicebus
		t.Errorf("connection strings = %v", o.ConnectionStrings)
	}
}

func TestServiceBusMeta(t *testing.T) {
	srv := httptest.NewServer(New(testConfig()).Handler())
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL + "/api/servicebus/meta")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var meta struct {
		Queues []string                 `json:"queues"`
		Topics []config.ServiceBusTopic `json:"topics"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		t.Fatal(err)
	}
	if len(meta.Queues) != 1 || len(meta.Topics) != 1 {
		t.Errorf("meta = %+v", meta)
	}
}

func TestParamValidation(t *testing.T) {
	srv := httptest.NewServer(New(testConfig()).Handler())
	defer srv.Close()

	for _, path := range []string{
		"/api/blob/list",
		"/api/queue/peek",
		"/api/table/rows",
		"/api/servicebus/peek",
		"/api/keyvault/secret",
		"/api/eventgrid/events",
	} {
		resp, err := srv.Client().Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 400 {
			t.Errorf("%s without params: status %d, want 400", path, resp.StatusCode)
		}
	}
}

func TestKeyVaultProxy(t *testing.T) {
	// Run a real Key Vault mock over TLS and point the UI at its port.
	kvSrv := httptest.NewTLSServer(mock.NewKeyVault(map[string]string{"db-password": "hunter2"}).Handler())
	defer kvSrv.Close()
	u, _ := url.Parse(kvSrv.URL)
	port, _ := strconv.Atoi(u.Port())

	cfg := testConfig()
	cfg.Services.KeyVault = &config.KeyVaultService{Port: port}
	srv := httptest.NewServer(New(cfg).Handler())
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL + "/api/keyvault/secrets")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var list struct {
		Value []struct {
			ID string `json:"id"`
		} `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list.Value) != 1 || !strings.Contains(list.Value[0].ID, "db-password") {
		t.Fatalf("list = %+v", list)
	}

	resp2, err := srv.Client().Get(srv.URL + "/api/keyvault/secret?name=db-password")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	var secret struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(resp2.Body).Decode(&secret); err != nil {
		t.Fatal(err)
	}
	if secret.Value != "hunter2" {
		t.Fatalf("secret value = %q", secret.Value)
	}
}

func TestStaticIndexServed(t *testing.T) {
	srv := httptest.NewServer(New(testConfig()).Handler())
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	buf := make([]byte, 4096)
	n, _ := resp.Body.Read(buf)
	if resp.StatusCode != 200 || !strings.Contains(string(buf[:n]), "azlocal") {
		t.Fatalf("index not served, status %d", resp.StatusCode)
	}
}
