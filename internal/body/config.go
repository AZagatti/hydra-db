package body

import (
	"errors"
	"fmt"
	"strings"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

// Config is the top-level configuration for a Hydra instance.
type Config struct {
	Hydra   HydraConfig   `koanf:"hydra"`
	Gateway GatewayConfig `koanf:"gateway"`
	Policy  PolicyConfig  `koanf:"policy"`
	Logging LoggingConfig `koanf:"logging"`
}

// HydraConfig controls core Hydra identity and behavior.
type HydraConfig struct {
	Name     string `koanf:"name"`
	Version  string `koanf:"version"`
	LogLevel string `koanf:"log_level"`
}

// GatewayConfig controls the HTTP listener that fronts Hydra.
type GatewayConfig struct {
	Host         string `koanf:"host"`
	Port         int    `koanf:"port"`
	ReadTimeout  int    `koanf:"read_timeout"`
	WriteTimeout int    `koanf:"write_timeout"`
}

// PolicyConfig controls tenant-level resource budgets and rate limits.
type PolicyConfig struct {
	DefaultBudget int `koanf:"default_budget"`
	RateLimit     int `koanf:"rate_limit"`
}

// LoggingConfig controls log output format and verbosity.
type LoggingConfig struct {
	Level  string `koanf:"level"`
	Format string `koanf:"format"`
}

// DefaultConfig returns a Config populated with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Hydra: HydraConfig{
			Name:     "hydra",
			Version:  "0.1.0",
			LogLevel: "info",
		},
		Gateway: GatewayConfig{
			Host:         "localhost",
			Port:         8080,
			ReadTimeout:  30,
			WriteTimeout: 30,
		},
		Policy: PolicyConfig{
			DefaultBudget: 10000,
			RateLimit:     100,
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
		},
	}
}

// LoadConfig reads configuration from a YAML file, applies defaults first,
// then overlays the file values, and finally overrides with HYDRA_* environment
// variables.
func LoadConfig(path string) (*Config, error) {
	k := koanf.New(".")

	defaultsYAML := []byte(`hydra:
  name: hydra
  version: "0.1.0"
  log_level: info
gateway:
  host: localhost
  port: 8080
  read_timeout: 30
  write_timeout: 30
policy:
  default_budget: 10000
  rate_limit: 100
logging:
  level: info
  format: json
`)
	if err := k.Load(&rawBytesProvider{data: defaultsYAML}, yaml.Parser()); err != nil {
		return nil, fmt.Errorf("loading defaults: %w", err)
	}

	if err := k.Load(file.Provider(path), yaml.Parser()); err != nil {
		return nil, fmt.Errorf("loading config file %s: %w", path, err)
	}

	if err := k.Load(env.Provider("HYDRA_", ".", func(s string) string {
		return strings.ReplaceAll(
			strings.ToLower(strings.TrimPrefix(s, "HYDRA_")),
			"__", ".",
		)
	}), nil); err != nil {
		return nil, fmt.Errorf("loading env overrides: %w", err)
	}

	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		return nil, fmt.Errorf("unmarshaling config: %w", err)
	}

	return &cfg, nil
}

// Validate checks that all config fields hold legal values.
func (c *Config) Validate() error {
	if c.Gateway.Port < 1 || c.Gateway.Port > 65535 {
		return fmt.Errorf("invalid gateway port: %d (must be 1-65535)", c.Gateway.Port)
	}

	validLogLevels := map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
	}

	if !validLogLevels[c.Hydra.LogLevel] {
		return fmt.Errorf("invalid hydra log_level: %q (must be debug, info, warn, or error)", c.Hydra.LogLevel)
	}

	if !validLogLevels[c.Logging.Level] {
		return fmt.Errorf("invalid logging level: %q (must be debug, info, warn, or error)", c.Logging.Level)
	}

	validFormats := map[string]bool{
		"json":    true,
		"text":    true,
		"console": true,
	}

	if !validFormats[c.Logging.Format] {
		return fmt.Errorf("invalid logging format: %q (must be json, text, or console)", c.Logging.Format)
	}

	if c.Gateway.ReadTimeout < 0 {
		return errors.New("gateway read_timeout must be non-negative")
	}

	if c.Gateway.WriteTimeout < 0 {
		return errors.New("gateway write_timeout must be non-negative")
	}

	if c.Policy.DefaultBudget < 0 {
		return errors.New("policy default_budget must be non-negative")
	}

	if c.Policy.RateLimit < 0 {
		return errors.New("policy rate_limit must be non-negative")
	}

	return nil
}

// rawBytesProvider is a koanf provider that serves static YAML bytes,
// used to load default config values without a file on disk.
type rawBytesProvider struct {
	data []byte
}

func (p *rawBytesProvider) ReadBytes() ([]byte, error) {
	return p.data, nil
}

func (p *rawBytesProvider) Read() (map[string]interface{}, error) {
	return nil, errors.New("rawBytesProvider does not support Read")
}
