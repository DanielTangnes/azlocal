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
