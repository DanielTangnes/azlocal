package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_Minimal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "azlocal.yaml")
	content := `services:
  blob:
    containers: [uploads]
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if c.Services.Blob == nil || len(c.Services.Blob.Containers) != 1 {
		t.Fatalf("unexpected config: %+v", c)
	}
}

func TestValidate_RejectsEmpty(t *testing.T) {
	c := &Config{}
	if err := c.Validate(); err == nil {
		t.Fatal("expected error for empty config")
	}
}

func TestValidate_CosmosRequiresPartitionKey(t *testing.T) {
	c := &Config{
		Services: Services{
			Cosmos: &CosmosService{
				Databases: []CosmosDatabase{
					{Name: "app", Containers: []CosmosContainer{{Name: "users"}}},
				},
			},
		},
	}
	if err := c.Validate(); err == nil {
		t.Fatal("expected error for missing partitionKey")
	}
}

func TestParseSeedTarget(t *testing.T) {
	tests := []struct {
		in         string
		wantScheme string
		wantParts  []string
		wantErr    bool
	}{
		{in: "blob://uploads", wantScheme: "blob", wantParts: []string{"uploads"}},
		{in: "queue://work-items", wantScheme: "queue", wantParts: []string{"work-items"}},
		{in: "table://users", wantScheme: "table", wantParts: []string{"users"}},
		{in: "cosmos://app/users", wantScheme: "cosmos", wantParts: []string{"app", "users"}},
		{in: "servicebus://orders", wantScheme: "servicebus", wantParts: []string{"orders"}},
		{in: "sb://orders", wantScheme: "servicebus", wantParts: []string{"orders"}}, // alias
		{in: "BLOB://Uploads", wantScheme: "blob", wantParts: []string{"Uploads"}},   // scheme lowercased, path preserved
		{in: "blob://uploads/", wantScheme: "blob", wantParts: []string{"uploads"}},  // trailing slash trimmed
		{in: "no-scheme", wantErr: true},
		{in: "blob://", wantErr: true},
		{in: "blob:///", wantErr: true},
		{in: "ftp://nope", wantErr: true},          // unknown scheme
		{in: "cosmos://only-db", wantErr: true},    // wrong arity
		{in: "blob://a/b", wantErr: true},          // wrong arity
	}
	for _, tc := range tests {
		got, err := ParseSeedTarget(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseSeedTarget(%q): expected error, got %+v", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseSeedTarget(%q): unexpected error: %v", tc.in, err)
			continue
		}
		if got.Scheme != tc.wantScheme {
			t.Errorf("ParseSeedTarget(%q) scheme = %q, want %q", tc.in, got.Scheme, tc.wantScheme)
		}
		if len(got.Parts) != len(tc.wantParts) {
			t.Errorf("ParseSeedTarget(%q) parts = %v, want %v", tc.in, got.Parts, tc.wantParts)
			continue
		}
		for i := range got.Parts {
			if got.Parts[i] != tc.wantParts[i] {
				t.Errorf("ParseSeedTarget(%q) parts[%d] = %q, want %q", tc.in, i, got.Parts[i], tc.wantParts[i])
			}
		}
	}
}

func TestValidate_SeedTargets(t *testing.T) {
	base := Services{Blob: &BlobService{Containers: []string{"uploads"}}}
	good := &Config{Services: base, Seed: []Seed{{Target: "blob://uploads", From: "./fixtures"}}}
	if err := good.Validate(); err != nil {
		t.Fatalf("valid seed rejected: %v", err)
	}
	badTarget := &Config{Services: base, Seed: []Seed{{Target: "nope://x", From: "./f"}}}
	if err := badTarget.Validate(); err == nil {
		t.Fatal("expected error for unknown seed scheme")
	}
	missingFrom := &Config{Services: base, Seed: []Seed{{Target: "blob://uploads"}}}
	if err := missingFrom.Validate(); err == nil {
		t.Fatal("expected error for missing from")
	}
}

func TestValidate_EventGrid(t *testing.T) {
	bad := &Config{Services: Services{EventGrid: &EventGridService{
		Topics: []EventGridTopic{{Name: "events", Subscriptions: []EventGridSubscription{{Name: "audit"}}}},
	}}}
	if err := bad.Validate(); err == nil {
		t.Fatal("expected error for subscription without endpoint")
	}
}
