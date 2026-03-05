package config

import (
	"fmt"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

// Config holds all application configuration.
type Config struct {
	QRadar      QRadarConfig      `yaml:"qradar"`
	Destination DestinationConfig `yaml:"destination"`
	Collector   CollectorConfig   `yaml:"collector"`
	Logging     LoggingConfig     `yaml:"logging"`
}

// QRadarConfig holds QRadar API connection settings.
type QRadarConfig struct {
	BaseURL     string `yaml:"base_url"`
	APIToken    string `yaml:"api_token"`
	Version     string `yaml:"version"`
	TLSInsecure bool   `yaml:"tls_insecure"`
}

// DestinationConfig holds external ingestion API settings.
type DestinationConfig struct {
	URL    string `yaml:"url"`
	APIKey string `yaml:"api_key"`
}

// CollectorConfig holds collector behavior settings.
type CollectorConfig struct {
	PollIntervalSeconds int    `yaml:"poll_interval_seconds"`
	StateFile           string `yaml:"state_file"`
	HTTPTimeoutSeconds  int    `yaml:"http_timeout_seconds"`
	WorkerCount         int    `yaml:"worker_count"`
}

// LoggingConfig holds logging settings.
type LoggingConfig struct {
	Level string `yaml:"level"`
}

// Load reads configuration from a YAML file and overrides with environment variables.
func Load(path string) (*Config, error) {
	cfg := &Config{
		Collector: CollectorConfig{
			PollIntervalSeconds: 60,
			StateFile:           "./data/state.db",
			HTTPTimeoutSeconds:  30,
			WorkerCount:         5,
		},
		Logging: LoggingConfig{
			Level: "info",
		},
		QRadar: QRadarConfig{
			Version: "20.0",
		},
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("reading config file: %w", err)
		}
		// File not found is OK — we'll rely on env vars.
	} else {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parsing config file: %w", err)
		}
	}

	// Environment variable overrides.
	overrideString(&cfg.QRadar.BaseURL, "QRADAR_BASE_URL")
	overrideString(&cfg.QRadar.APIToken, "QRADAR_API_TOKEN")
	overrideString(&cfg.QRadar.Version, "QRADAR_VERSION")
	overrideBool(&cfg.QRadar.TLSInsecure, "QRADAR_TLS_INSECURE")
	overrideString(&cfg.Destination.URL, "DESTINATION_URL")
	overrideString(&cfg.Destination.APIKey, "DESTINATION_API_KEY")
	overrideInt(&cfg.Collector.PollIntervalSeconds, "POLL_INTERVAL_SECONDS")
	overrideString(&cfg.Collector.StateFile, "STATE_FILE")
	overrideInt(&cfg.Collector.HTTPTimeoutSeconds, "HTTP_TIMEOUT_SECONDS")
	overrideInt(&cfg.Collector.WorkerCount, "WORKER_COUNT")
	overrideString(&cfg.Logging.Level, "LOG_LEVEL")

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) validate() error {
	if c.QRadar.BaseURL == "" {
		return fmt.Errorf("config: qradar.base_url is required")
	}
	if c.QRadar.APIToken == "" {
		return fmt.Errorf("config: qradar.api_token is required")
	}
	if c.Destination.URL == "" {
		return fmt.Errorf("config: destination.url is required")
	}
	if c.Destination.APIKey == "" {
		return fmt.Errorf("config: destination.api_key is required")
	}
	if c.Collector.PollIntervalSeconds <= 0 {
		return fmt.Errorf("config: collector.poll_interval_seconds must be > 0")
	}
	if c.Collector.HTTPTimeoutSeconds <= 0 {
		return fmt.Errorf("config: collector.http_timeout_seconds must be > 0")
	}
	if c.Collector.WorkerCount <= 0 {
		c.Collector.WorkerCount = 5
	}
	return nil
}

func overrideString(field *string, envKey string) {
	if val := os.Getenv(envKey); val != "" {
		*field = val
	}
}

func overrideInt(field *int, envKey string) {
	if val := os.Getenv(envKey); val != "" {
		if n, err := strconv.Atoi(val); err == nil {
			*field = n
		}
	}
}

func overrideBool(field *bool, envKey string) {
	if val := os.Getenv(envKey); val != "" {
		if b, err := strconv.ParseBool(val); err == nil {
			*field = b
		}
	}
}
