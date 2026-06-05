// Package config defines the schema for azlocal.yaml and helpers to load it.
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config is the root azlocal configuration.
type Config struct {
	Version  string   `yaml:"version,omitempty"`
	Services Services `yaml:"services"`
	Seed     []Seed   `yaml:"seed,omitempty"`
}

// Services groups all emulator service configurations.
// Pointer fields are used so we can detect whether a service was declared.
type Services struct {
	Blob       *BlobService       `yaml:"blob,omitempty"`
	Queue      *QueueService      `yaml:"queue,omitempty"`
	Table      *TableService      `yaml:"table,omitempty"`
	Cosmos     *CosmosService     `yaml:"cosmos,omitempty"`
	ServiceBus *ServiceBusService `yaml:"servicebus,omitempty"`
}

type BlobService struct {
	Containers []string `yaml:"containers,omitempty"`
	Port       int      `yaml:"port,omitempty"`
}

type QueueService struct {
	Queues []string `yaml:"queues,omitempty"`
	Port   int      `yaml:"port,omitempty"`
}

type TableService struct {
	Tables []string `yaml:"tables,omitempty"`
	Port   int      `yaml:"port,omitempty"`
}

type CosmosService struct {
	Databases []CosmosDatabase `yaml:"databases,omitempty"`
	Port      int              `yaml:"port,omitempty"`
}

type CosmosDatabase struct {
	Name       string            `yaml:"name"`
	Containers []CosmosContainer `yaml:"containers,omitempty"`
}

type CosmosContainer struct {
	Name         string `yaml:"name"`
	PartitionKey string `yaml:"partitionKey"`
}

type ServiceBusService struct {
	Namespace string            `yaml:"namespace,omitempty"`
	Queues    []string          `yaml:"queues,omitempty"`
	Topics    []ServiceBusTopic `yaml:"topics,omitempty"`
	Port      int               `yaml:"port,omitempty"`
}

type ServiceBusTopic struct {
	Name          string   `yaml:"name"`
	Subscriptions []string `yaml:"subscriptions,omitempty"`
}

// Seed describes a single seed-data target.
type Seed struct {
	Target string `yaml:"target"`
	From   string `yaml:"from"`
}

// Load reads and parses an azlocal config file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var c Config
	dec := yaml.NewDecoder(bytesReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

// Validate performs lightweight semantic checks on a Config.
func (c *Config) Validate() error {
	if c.Services == (Services{}) {
		return fmt.Errorf("no services configured")
	}
	if c.Services.Cosmos != nil {
		for _, db := range c.Services.Cosmos.Databases {
			if db.Name == "" {
				return fmt.Errorf("cosmos: database name is required")
			}
			for _, con := range db.Containers {
				if con.Name == "" {
					return fmt.Errorf("cosmos: container name is required in db %q", db.Name)
				}
				if con.PartitionKey == "" {
					return fmt.Errorf("cosmos: partitionKey is required for container %q", con.Name)
				}
			}
		}
	}
	return nil
}
