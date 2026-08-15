package config

import (
	"fmt"
	"os"
	"strings"

	"go.yaml.in/yaml/v3"
)

type Config struct {
	Mode     string   `yaml:"mode"`
	HTTP     HTTP     `yaml:"http"`
	Database Database `yaml:"database"`
	Temporal Temporal `yaml:"temporal"`
	Planning Planning `yaml:"planning"`
}

type HTTP struct {
	Address string `yaml:"address"`
}
type Database struct {
	URL         string `yaml:"url"`
	AutoMigrate bool   `yaml:"auto_migrate"`
}
type Temporal struct {
	Address   string `yaml:"address"`
	Namespace string `yaml:"namespace"`
	TaskQueue string `yaml:"task_queue"`
}
type Planning struct {
	Contracts     string `yaml:"contracts"`
	Profiles      string `yaml:"profiles"`
	LegacyCatalog string `yaml:"legacy_catalog"`
}

func Load(path string) (Config, error) {
	result := Config{Mode: "all", HTTP: HTTP{Address: ":8080"}, Database: Database{AutoMigrate: true}, Temporal: Temporal{Address: "localhost:7233", Namespace: "default", TaskQueue: "agent-write-gateway-releases"}, Planning: Planning{Contracts: "examples/contracts", Profiles: "examples/profiles"}}
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return Config{}, fmt.Errorf("read gateway config: %w", err)
		}
		decoder := yaml.NewDecoder(strings.NewReader(string(data)))
		decoder.KnownFields(true)
		if err := decoder.Decode(&result); err != nil {
			return Config{}, fmt.Errorf("decode gateway config: %w", err)
		}
	}
	applyEnv(&result)
	if err := result.Validate(); err != nil {
		return Config{}, err
	}
	return result, nil
}

func (c Config) Validate() error {
	switch c.Mode {
	case "gateway", "worker", "all":
	default:
		return fmt.Errorf("mode must be gateway, worker, or all")
	}
	if c.Database.URL == "" {
		return fmt.Errorf("database.url is required")
	}
	if c.Temporal.Address == "" || c.Temporal.Namespace == "" || c.Temporal.TaskQueue == "" {
		return fmt.Errorf("temporal address, namespace, and task_queue are required")
	}
	if c.Planning.LegacyCatalog == "" && (c.Planning.Contracts == "" || c.Planning.Profiles == "") {
		return fmt.Errorf("planning contract and profile paths are required")
	}
	return nil
}

func applyEnv(c *Config) {
	setString("AWG_MODE", &c.Mode)
	setString("AWG_HTTP_ADDRESS", &c.HTTP.Address)
	setString("AWG_DATABASE_URL", &c.Database.URL)
	setString("AWG_TEMPORAL_ADDRESS", &c.Temporal.Address)
	setString("AWG_TEMPORAL_NAMESPACE", &c.Temporal.Namespace)
	setString("AWG_TEMPORAL_TASK_QUEUE", &c.Temporal.TaskQueue)
	setString("AWG_CONTRACTS", &c.Planning.Contracts)
	setString("AWG_PROFILES", &c.Planning.Profiles)
	setString("AWG_LEGACY_CATALOG", &c.Planning.LegacyCatalog)
}
func setString(name string, target *string) {
	if value, ok := os.LookupEnv(name); ok {
		*target = value
	}
}
