// Package compose generates a docker-compose project from an azlocal Config.
package compose

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/DanielTangnes/azlocal/internal/config"
)

// Project is a minimal subset of the docker-compose v3 schema, enough
// for our purposes. It marshals as standard compose YAML.
type Project struct {
	Name     string             `yaml:"name,omitempty"`
	Services map[string]Service `yaml:"services"`
	Volumes  map[string]any     `yaml:"volumes,omitempty"`
	Networks map[string]any     `yaml:"networks,omitempty"`
}

type Service struct {
	Image       string            `yaml:"image,omitempty"`
	Build       any               `yaml:"build,omitempty"`
	Command     any               `yaml:"command,omitempty"`
	Ports       []string          `yaml:"ports,omitempty"`
	Environment map[string]string `yaml:"environment,omitempty"`
	Volumes     []string          `yaml:"volumes,omitempty"`
	DependsOn   any               `yaml:"depends_on,omitempty"`
	Healthcheck *Healthcheck      `yaml:"healthcheck,omitempty"`
	Restart     string            `yaml:"restart,omitempty"`
	Networks    []string          `yaml:"networks,omitempty"`
	Platform    string            `yaml:"platform,omitempty"`
}

type Healthcheck struct {
	Test        []string `yaml:"test"`
	Interval    string   `yaml:"interval,omitempty"`
	Timeout     string   `yaml:"timeout,omitempty"`
	Retries     int      `yaml:"retries,omitempty"`
	StartPeriod string   `yaml:"start_period,omitempty"`
}

// Generate builds a Project from an azlocal Config.
func Generate(cfg *config.Config) (*Project, error) {
	p := &Project{
		Name:     "azlocal",
		Services: map[string]Service{},
		Volumes:  map[string]any{},
		Networks: map[string]any{"azlocal": map[string]any{}},
	}

	s := cfg.Services
	storageEnabled := s.Blob != nil || s.Queue != nil || s.Table != nil

	if storageEnabled {
		p.Services["azurite"] = azuriteService(s)
		p.Volumes["azurite-data"] = nil
	}

	if s.Cosmos != nil {
		p.Services["cosmos"] = cosmosService(s.Cosmos)
		p.Volumes["cosmos-data"] = nil
	}

	if s.ServiceBus != nil {
		sb, sql := serviceBusServices(s.ServiceBus)
		p.Services["servicebus"] = sb
		p.Services["servicebus-sql"] = sql
		p.Volumes["servicebus-sql-data"] = nil
	}

	if len(p.Services) == 0 {
		return nil, fmt.Errorf("no enabled services in config")
	}
	return p, nil
}

func azuriteService(s config.Services) Service {
	blobPort := portOr(s.Blob, 10000)
	queuePort := 10001
	if s.Queue != nil && s.Queue.Port != 0 {
		queuePort = s.Queue.Port
	}
	tablePort := 10002
	if s.Table != nil && s.Table.Port != 0 {
		tablePort = s.Table.Port
	}
	return Service{
		Image: "mcr.microsoft.com/azure-storage/azurite:latest",
		Command: []string{
			"azurite",
			"--blobHost", "0.0.0.0",
			"--queueHost", "0.0.0.0",
			"--tableHost", "0.0.0.0",
			"--location", "/data",
		},
		Ports: []string{
			fmt.Sprintf("%d:10000", blobPort),
			fmt.Sprintf("%d:10001", queuePort),
			fmt.Sprintf("%d:10002", tablePort),
		},
		Volumes:  []string{"azurite-data:/data"},
		Networks: []string{"azlocal"},
		Restart:  "unless-stopped",
		Healthcheck: &Healthcheck{
			Test:     []string{"CMD-SHELL", "nc -z localhost 10000 || exit 1"},
			Interval: "5s",
			Timeout:  "3s",
			Retries:  10,
		},
	}
}

func cosmosService(c *config.CosmosService) Service {
	port := 8081
	if c.Port != 0 {
		port = c.Port
	}
	return Service{
		// Linux-based emulator (preview, but the only Linux option).
		Image: "mcr.microsoft.com/cosmosdb/linux/azure-cosmos-emulator:latest",
		Ports: []string{
			fmt.Sprintf("%d:8081", port),
			"10250-10255:10250-10255",
		},
		Environment: map[string]string{
			"AZURE_COSMOS_EMULATOR_PARTITION_COUNT":         "3",
			"AZURE_COSMOS_EMULATOR_ENABLE_DATA_PERSISTENCE": "true",
			"AZURE_COSMOS_EMULATOR_IP_ADDRESS_OVERRIDE":     "127.0.0.1",
		},
		Volumes:  []string{"cosmos-data:/tmp/cosmos/appdata"},
		Networks: []string{"azlocal"},
		Restart:  "unless-stopped",
		Healthcheck: &Healthcheck{
			Test:        []string{"CMD-SHELL", "curl -sk https://localhost:8081/_explorer/emulator.pem -o /dev/null || exit 1"},
			Interval:    "10s",
			Timeout:     "5s",
			Retries:     30,
			StartPeriod: "60s",
		},
	}
}

func serviceBusServices(cfg *config.ServiceBusService) (Service, Service) {
	port := 5300
	if cfg != nil && cfg.Port != 0 {
		port = cfg.Port
	}
	sb := Service{
		Image: "mcr.microsoft.com/azure-messaging/servicebus-emulator:latest",
		Ports: []string{
			"5672:5672",
			fmt.Sprintf("%d:5300", port),
		},
		Environment: map[string]string{
			"ACCEPT_EULA":       "Y",
			"SQL_SERVER":        "servicebus-sql",
			"MSSQL_SA_PASSWORD": "azlocalP@ssw0rd!",
			"SQL_WAIT_INTERVAL": "5",
		},
		// The generated Config.json (queues/topics/subscriptions) is mounted at
		// the path the emulator reads on startup. It lives next to the compose
		// file, so the bind source is relative to that directory.
		Volumes: []string{"./servicebus-config.json:/ServiceBus_Emulator/ConfigFiles/Config.json"},
		DependsOn: map[string]map[string]string{
			"servicebus-sql": {"condition": "service_healthy"},
		},
		Networks: []string{"azlocal"},
		Restart:  "unless-stopped",
	}
	sql := Service{
		// Azure SQL Edge is retired; Microsoft's Service Bus emulator docs use
		// SQL Server 2022 as the backend. That image is published for
		// linux/amd64 only (no native arm64 build), so the platform hint is
		// kept to let it run under emulation on Apple Silicon.
		Image:    "mcr.microsoft.com/mssql/server:2022-latest",
		Platform: "linux/amd64",
		Environment: map[string]string{
			"ACCEPT_EULA":       "Y",
			"MSSQL_SA_PASSWORD": "azlocalP@ssw0rd!",
		},
		Volumes:  []string{"servicebus-sql-data:/var/opt/mssql"},
		Networks: []string{"azlocal"},
		Restart:  "unless-stopped",
		Healthcheck: &Healthcheck{
			// SQL Server 2022 ships mssql-tools18 (sqlcmd defaults to an
			// encrypted connection, so -C trusts the self-signed cert).
			Test:     []string{"CMD-SHELL", "/opt/mssql-tools18/bin/sqlcmd -S localhost -U sa -P azlocalP@ssw0rd! -C -Q 'SELECT 1' || exit 1"},
			Interval: "10s",
			Timeout:  "5s",
			Retries:  20,
		},
	}
	return sb, sql
}

// portOr returns the configured port for a Blob service, falling back to def.
func portOr(b *config.BlobService, def int) int {
	if b != nil && b.Port != 0 {
		return b.Port
	}
	return def
}

// Marshal serializes a Project to YAML bytes.
func Marshal(p *Project) ([]byte, error) {
	return yaml.Marshal(p)
}

// DefaultPath returns the path where azlocal writes the generated compose file.
func DefaultPath() string {
	return filepath.Join(".azlocal", "docker-compose.yaml")
}

// ServiceBusConfigPath returns the path of the generated Service Bus emulator
// config, written next to the compose file so the bind mount resolves.
func ServiceBusConfigPath() string {
	return filepath.Join(".azlocal", "servicebus-config.json")
}

// Write renders the project and writes it to DefaultPath, returning the path.
func Write(p *Project) (string, error) {
	out, err := Marshal(p)
	if err != nil {
		return "", err
	}
	path := DefaultPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// WriteProject generates and writes the compose project — plus the Service Bus
// emulator config when Service Bus is enabled — under .azlocal, returning the
// compose file path.
func WriteProject(cfg *config.Config) (string, error) {
	project, err := Generate(cfg)
	if err != nil {
		return "", err
	}
	path, err := Write(project)
	if err != nil {
		return "", err
	}
	if cfg.Services.ServiceBus != nil {
		data, err := GenerateServiceBusConfig(cfg.Services.ServiceBus)
		if err != nil {
			return "", err
		}
		if err := os.WriteFile(ServiceBusConfigPath(), data, 0o644); err != nil {
			return "", fmt.Errorf("write service bus config: %w", err)
		}
	}
	return path, nil
}
