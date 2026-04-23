package body

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.Equal(t, "hydra", cfg.Hydra.Name)
	assert.Equal(t, "0.1.0", cfg.Hydra.Version)
	assert.Equal(t, "info", cfg.Hydra.LogLevel)

	assert.Equal(t, "localhost", cfg.Gateway.Host)
	assert.Equal(t, 8080, cfg.Gateway.Port)
	assert.Equal(t, 30, cfg.Gateway.ReadTimeout)
	assert.Equal(t, 30, cfg.Gateway.WriteTimeout)

	assert.Equal(t, 10000, cfg.Policy.DefaultBudget)
	assert.Equal(t, 100, cfg.Policy.RateLimit)

	assert.Equal(t, "info", cfg.Logging.Level)
	assert.Equal(t, "json", cfg.Logging.Format)
}

func TestLoadConfig_ValidFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "hydra.yaml")

	content := []byte(`hydra:
  name: test-hydra
  version: "1.0.0"
  log_level: debug

gateway:
  host: 0.0.0.0
  port: 9090
  read_timeout: 10
  write_timeout: 20

policy:
  default_budget: 5000
  rate_limit: 200

logging:
  level: warn
  format: text
`)
	require.NoError(t, os.WriteFile(cfgPath, content, 0644))

	cfg, err := LoadConfig(cfgPath)
	require.NoError(t, err)

	assert.Equal(t, "test-hydra", cfg.Hydra.Name)
	assert.Equal(t, "1.0.0", cfg.Hydra.Version)
	assert.Equal(t, "debug", cfg.Hydra.LogLevel)

	assert.Equal(t, "0.0.0.0", cfg.Gateway.Host)
	assert.Equal(t, 9090, cfg.Gateway.Port)
	assert.Equal(t, 10, cfg.Gateway.ReadTimeout)
	assert.Equal(t, 20, cfg.Gateway.WriteTimeout)

	assert.Equal(t, 5000, cfg.Policy.DefaultBudget)
	assert.Equal(t, 200, cfg.Policy.RateLimit)

	assert.Equal(t, "warn", cfg.Logging.Level)
	assert.Equal(t, "text", cfg.Logging.Format)
}

func TestLoadConfig_MissingFile(t *testing.T) {
	_, err := LoadConfig("/nonexistent/path/hydra.yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "loading config file")
}

func TestLoadConfig_MergesDefaults(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "partial.yaml")

	partialContent := []byte(`hydra:
  name: partial-hydra

gateway:
  port: 3000
`)
	require.NoError(t, os.WriteFile(cfgPath, partialContent, 0644))

	cfg, err := LoadConfig(cfgPath)
	require.NoError(t, err)

	assert.Equal(t, "partial-hydra", cfg.Hydra.Name)
	assert.Equal(t, "0.1.0", cfg.Hydra.Version, "missing field should get default")
	assert.Equal(t, "info", cfg.Hydra.LogLevel, "missing field should get default")

	assert.Equal(t, 3000, cfg.Gateway.Port, "explicit value should be used")
	assert.Equal(t, "localhost", cfg.Gateway.Host, "missing field should get default")
	assert.Equal(t, 30, cfg.Gateway.ReadTimeout, "missing field should get default")
	assert.Equal(t, 30, cfg.Gateway.WriteTimeout, "missing field should get default")

	assert.Equal(t, 10000, cfg.Policy.DefaultBudget, "missing section should get defaults")
	assert.Equal(t, 100, cfg.Policy.RateLimit)

	assert.Equal(t, "info", cfg.Logging.Level)
	assert.Equal(t, "json", cfg.Logging.Format)
}

func TestLoadConfig_EnvOverride(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "hydra.yaml")

	content := []byte(`gateway:
  port: 8080
`)
	require.NoError(t, os.WriteFile(cfgPath, content, 0644))

	t.Setenv("HYDRA_GATEWAY__PORT", "9090")
	t.Setenv("HYDRA_HYDRA__LOG_LEVEL", "debug")
	t.Setenv("HYDRA_LOGGING__FORMAT", "text")

	cfg, err := LoadConfig(cfgPath)
	require.NoError(t, err)

	assert.Equal(t, 9090, cfg.Gateway.Port, "env should override file value")
	assert.Equal(t, "debug", cfg.Hydra.LogLevel, "env should override default")
	assert.Equal(t, "text", cfg.Logging.Format, "env should override default")
}

func TestConfig_Validate(t *testing.T) {
	t.Run("valid config passes", func(t *testing.T) {
		cfg := DefaultConfig()
		assert.NoError(t, cfg.Validate())
	})

	t.Run("valid with debug level", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Hydra.LogLevel = "debug"
		cfg.Logging.Level = "debug"
		assert.NoError(t, cfg.Validate())
	})

	t.Run("valid with warn level", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Hydra.LogLevel = "warn"
		cfg.Logging.Level = "warn"
		assert.NoError(t, cfg.Validate())
	})

	t.Run("valid with error level", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Hydra.LogLevel = "error"
		cfg.Logging.Level = "error"
		assert.NoError(t, cfg.Validate())
	})

	t.Run("port too low", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Gateway.Port = 0
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid gateway port")
	})

	t.Run("port too high", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Gateway.Port = 70000
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid gateway port")
	})

	t.Run("port boundary min", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Gateway.Port = 1
		assert.NoError(t, cfg.Validate())
	})

	t.Run("port boundary max", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Gateway.Port = 65535
		assert.NoError(t, cfg.Validate())
	})

	t.Run("invalid hydra log_level", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Hydra.LogLevel = "verbose"
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid hydra log_level")
	})

	t.Run("invalid logging level", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Logging.Level = "trace"
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid logging level")
	})

	t.Run("invalid logging format", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Logging.Format = "xml"
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid logging format")
	})

	t.Run("negative read_timeout", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Gateway.ReadTimeout = -1
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read_timeout")
	})

	t.Run("negative write_timeout", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Gateway.WriteTimeout = -1
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "write_timeout")
	})

	t.Run("negative default_budget", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Policy.DefaultBudget = -1
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "default_budget")
	})

	t.Run("negative rate_limit", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Policy.RateLimit = -1
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "rate_limit")
	})

	t.Run("valid text format", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Logging.Format = "text"
		assert.NoError(t, cfg.Validate())
	})

	t.Run("valid console format", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Logging.Format = "console"
		assert.NoError(t, cfg.Validate())
	})
}
