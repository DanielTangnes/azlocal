// Package config defines the schema for azlocal.yaml and helpers to load it.
package config

import (
	"bytes"
	"fmt"
	"os"
	"strings"

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
	KeyVault   *KeyVaultService   `yaml:"keyvault,omitempty"`
	EventGrid  *EventGridService  `yaml:"eventgrid,omitempty"`
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
	Name       string            `yaml:"name" json:"name"`
	Containers []CosmosContainer `yaml:"containers,omitempty" json:"containers"`
}

type CosmosContainer struct {
	Name         string `yaml:"name" json:"name"`
	PartitionKey string `yaml:"partitionKey" json:"partitionKey"`
}

type ServiceBusService struct {
	Namespace string            `yaml:"namespace,omitempty"`
	Queues    []string          `yaml:"queues,omitempty"`
	Topics    []ServiceBusTopic `yaml:"topics,omitempty"`
	Port      int               `yaml:"port,omitempty"`
}

type ServiceBusTopic struct {
	Name          string   `yaml:"name" json:"name"`
	Subscriptions []string `yaml:"subscriptions,omitempty" json:"subscriptions"`
}

// KeyVaultService configures the built-in Key Vault mock (secrets API).
// The mock runs in-process (see "azlocal mocks"), serving HTTPS with a
// self-signed certificate, like the Cosmos emulator does.
type KeyVaultService struct {
	Port    int               `yaml:"port,omitempty"`    // default 8200
	Secrets map[string]string `yaml:"secrets,omitempty"` // pre-created secrets
}

// EventGridService configures the built-in Event Grid mock. Events published
// to a topic endpoint are fanned out to that topic's webhook subscriptions.
type EventGridService struct {
	Port   int              `yaml:"port,omitempty"` // default 8210
	Topics []EventGridTopic `yaml:"topics,omitempty"`
}

type EventGridTopic struct {
	Name          string                  `yaml:"name" json:"name"`
	Subscriptions []EventGridSubscription `yaml:"subscriptions,omitempty" json:"subscriptions"`
}

type EventGridSubscription struct {
	Name     string `yaml:"name" json:"name"`
	Endpoint string `yaml:"endpoint" json:"endpoint"` // webhook URL events are delivered to
}

// Seed describes a single seed-data target.
type Seed struct {
	Target string `yaml:"target"`
	From   string `yaml:"from"`
}

// SeedTarget is a parsed seed target URI, e.g. "cosmos://app/users".
type SeedTarget struct {
	Scheme string
	Parts  []string
}

// seedSchemes maps each supported seed scheme to its expected path arity and
// usage string.
var seedSchemes = map[string]struct {
	arity int
	usage string
}{
	"blob":       {1, "blob://<container>"},
	"queue":      {1, "queue://<queue>"},
	"table":      {1, "table://<table>"},
	"cosmos":     {2, "cosmos://<database>/<container>"},
	"servicebus": {1, "servicebus://<queue-or-topic>"},
}

// ParseSeedTarget splits a seed target of the form "<scheme>://<path>" into
// its scheme and slash-separated path parts, validating the scheme and arity.
// The "sb" scheme is an alias for "servicebus".
func ParseSeedTarget(s string) (SeedTarget, error) {
	i := strings.Index(s, "://")
	if i < 0 {
		return SeedTarget{}, fmt.Errorf("invalid target %q: expected scheme://path", s)
	}
	scheme := strings.ToLower(s[:i])
	if scheme == "sb" {
		scheme = "servicebus"
	}
	spec, ok := seedSchemes[scheme]
	if !ok {
		return SeedTarget{}, fmt.Errorf("unknown seed target scheme %q in %q", scheme, s)
	}
	rest := strings.Trim(s[i+len("://"):], "/")
	if rest == "" {
		return SeedTarget{}, fmt.Errorf("invalid target %q: missing path", s)
	}
	parts := strings.Split(rest, "/")
	if len(parts) != spec.arity {
		return SeedTarget{}, fmt.Errorf("invalid target %q: expected %s", s, spec.usage)
	}
	return SeedTarget{Scheme: scheme, Parts: parts}, nil
}

// Load reads and parses an azlocal config file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var c Config
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

// HasMocks reports whether any in-process mock service is configured.
func (c *Config) HasMocks() bool {
	return c.Services.KeyVault != nil || c.Services.EventGrid != nil
}

// HasContainers reports whether any docker-based emulator is configured.
func (c *Config) HasContainers() bool {
	s := c.Services
	return s.Blob != nil || s.Queue != nil || s.Table != nil || s.Cosmos != nil || s.ServiceBus != nil
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
	if c.Services.EventGrid != nil {
		for _, t := range c.Services.EventGrid.Topics {
			if t.Name == "" {
				return fmt.Errorf("eventgrid: topic name is required")
			}
			for _, sub := range t.Subscriptions {
				if sub.Endpoint == "" {
					return fmt.Errorf("eventgrid: endpoint is required for subscription %q of topic %q", sub.Name, t.Name)
				}
			}
		}
	}
	for i, s := range c.Seed {
		if _, err := ParseSeedTarget(s.Target); err != nil {
			return fmt.Errorf("seed[%d]: %w", i, err)
		}
		if s.From == "" {
			return fmt.Errorf("seed[%d] (%s): \"from\" is required", i, s.Target)
		}
	}
	return nil
}
