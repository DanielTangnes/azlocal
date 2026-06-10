package provision

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/DanielTangnes/azlocal/internal/config"
)

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}

func TestCosmosPartitionKey(t *testing.T) {
	cfg := &config.Config{Services: config.Services{Cosmos: &config.CosmosService{
		Databases: []config.CosmosDatabase{{
			Name: "app",
			Containers: []config.CosmosContainer{
				{Name: "users", PartitionKey: "/tenantId"},
				{Name: "orders", PartitionKey: "/customerId"},
			},
		}},
	}}}

	if got := cosmosPartitionKey(cfg, "app", "users"); got != "/tenantId" {
		t.Errorf("users partition key = %q, want /tenantId", got)
	}
	if got := cosmosPartitionKey(cfg, "app", "orders"); got != "/customerId" {
		t.Errorf("orders partition key = %q, want /customerId", got)
	}
	if got := cosmosPartitionKey(cfg, "app", "missing"); got != "" {
		t.Errorf("missing container partition key = %q, want empty", got)
	}
	if got := cosmosPartitionKey(&config.Config{}, "app", "users"); got != "" {
		t.Errorf("no cosmos config partition key = %q, want empty", got)
	}
}

func TestExtractPartitionKey(t *testing.T) {
	cases := []struct {
		name    string
		doc     string
		field   string
		wantErr bool
	}{
		{name: "string", doc: `{"tenantId":"acme"}`, field: "tenantId"},
		{name: "number", doc: `{"shard":42}`, field: "shard"},
		{name: "bool", doc: `{"active":true}`, field: "active"},
		{name: "missing", doc: `{"other":"x"}`, field: "tenantId", wantErr: true},
		{name: "null unsupported", doc: `{"tenantId":null}`, field: "tenantId", wantErr: true},
	}
	for _, tc := range cases {
		_, err := extractPartitionKey(json.RawMessage(tc.doc), tc.field)
		if tc.wantErr && err == nil {
			t.Errorf("%s: expected error", tc.name)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("%s: unexpected error: %v", tc.name, err)
		}
	}
}

func TestReadMessages(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/msgs.json"
	if err := writeFile(path, `["hello", {"id":1,"kind":"order"}]`); err != nil {
		t.Fatal(err)
	}
	msgs, err := readMessages(path)
	if err != nil {
		t.Fatalf("readMessages: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("got %d messages, want 2", len(msgs))
	}
	if msgs[0] != "hello" {
		t.Errorf("msg[0] = %q, want hello (string used verbatim)", msgs[0])
	}
	// Object re-encoded as JSON text.
	var obj map[string]any
	if err := json.Unmarshal([]byte(msgs[1]), &obj); err != nil {
		t.Errorf("msg[1] is not valid JSON: %v", err)
	}
}

func TestReadDocs_ArrayAndNDJSON(t *testing.T) {
	dir := t.TempDir()

	arrPath := dir + "/arr.json"
	if err := writeFile(arrPath, "[{\"id\":1},\n {\"id\":2}]"); err != nil {
		t.Fatal(err)
	}
	arr, err := readDocs(arrPath)
	if err != nil || len(arr) != 2 {
		t.Fatalf("array: got %d docs, err=%v", len(arr), err)
	}

	ndPath := dir + "/nd.json"
	if err := writeFile(ndPath, "{\"id\":1}\n\n{\"id\":2}\n{\"id\":3}\n"); err != nil {
		t.Fatal(err)
	}
	nd, err := readDocs(ndPath)
	if err != nil || len(nd) != 3 {
		t.Fatalf("ndjson: got %d docs, err=%v", len(nd), err)
	}
}
