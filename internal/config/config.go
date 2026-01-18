// ABOUTME: Configuration loading and parsing for fold-gateway
// ABOUTME: Supports YAML files with environment variable expansion and duration parsing

package config

import (
	"fmt"
	"os"
	"regexp"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the complete fold-gateway configuration
type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Tailscale TailscaleConfig `yaml:"tailscale"`
	Database  DatabaseConfig  `yaml:"database"`
	Auth      AuthConfig      `yaml:"auth"`
	Agents    AgentsConfig    `yaml:"agents"`
	Frontends FrontendsConfig `yaml:"frontends"`
	Logging   LoggingConfig   `yaml:"logging"`
	Metrics   MetricsConfig   `yaml:"metrics"`
	WebAdmin  WebAdminConfig  `yaml:"webadmin"`
}

// AuthConfig holds authentication configuration
type AuthConfig struct {
	JWTSecret string `yaml:"jwt_secret"`
}

// TailscaleConfig holds Tailscale tsnet configuration
type TailscaleConfig struct {
	Enabled   bool   `yaml:"enabled"`
	Hostname  string `yaml:"hostname"`
	AuthKey   string `yaml:"auth_key"`
	StateDir  string `yaml:"state_dir"`
	Ephemeral bool   `yaml:"ephemeral"`
	CertFile  string `yaml:"cert_file"` // TLS cert file (generate via: tailscale cert <hostname>)
	KeyFile   string `yaml:"key_file"`  // TLS key file
	Funnel    bool   `yaml:"funnel"`    // Enable public Funnel (implies HTTPS)
}

// ServerConfig holds server address configuration
type ServerConfig struct {
	GRPCAddr string `yaml:"grpc_addr"`
	HTTPAddr string `yaml:"http_addr"`
}

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	Path string `yaml:"path"`
}

// AgentsConfig holds agent-related timing configuration
type AgentsConfig struct {
	HeartbeatInterval    time.Duration `yaml:"-"`
	HeartbeatTimeout     time.Duration `yaml:"-"`
	ReconnectGracePeriod time.Duration `yaml:"-"`

	// Raw string values for YAML unmarshaling
	HeartbeatIntervalRaw    string `yaml:"heartbeat_interval"`
	HeartbeatTimeoutRaw     string `yaml:"heartbeat_timeout"`
	ReconnectGracePeriodRaw string `yaml:"reconnect_grace_period"`
}

// FrontendsConfig holds configuration for all frontend integrations
type FrontendsConfig struct {
	Slack  SlackConfig  `yaml:"slack"`
	Matrix MatrixConfig `yaml:"matrix"`
}

// SlackConfig holds Slack integration configuration
type SlackConfig struct {
	Enabled         bool     `yaml:"enabled"`
	AppToken        string   `yaml:"app_token"`
	BotToken        string   `yaml:"bot_token"`
	AllowedChannels []string `yaml:"allowed_channels"`
}

// MatrixConfig holds Matrix integration configuration
type MatrixConfig struct {
	Enabled      bool     `yaml:"enabled"`
	Homeserver   string   `yaml:"homeserver"`
	UserID       string   `yaml:"user_id"`
	AccessToken  string   `yaml:"access_token"`
	AllowedUsers []string `yaml:"allowed_users"`
	AllowedRooms []string `yaml:"allowed_rooms"`
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// MetricsConfig holds metrics endpoint configuration
type MetricsConfig struct {
	Enabled bool   `yaml:"enabled"`
	Path    string `yaml:"path"`
}

// WebAdminConfig holds web admin UI configuration
type WebAdminConfig struct {
	// BaseURL is the external URL for the admin UI (used for invite links)
	// If not set, it's auto-detected from server.http_addr or tailscale hostname
	BaseURL string `yaml:"base_url"`
}

// Load reads a configuration file from the given path and returns a parsed Config.
// Environment variables in the format ${VAR_NAME} are expanded.
// Duration strings are parsed into time.Duration values.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	// Expand environment variables in the raw YAML content
	expandedData := expandEnvVars(string(data))

	var cfg Config
	if err := yaml.Unmarshal([]byte(expandedData), &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	// Parse duration fields
	if err := parseDurations(&cfg); err != nil {
		return nil, fmt.Errorf("parsing durations: %w", err)
	}

	// Validate required fields
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	return &cfg, nil
}

// expandEnvVars replaces ${VAR_NAME} patterns with the corresponding environment variable values.
// If the environment variable is not set, it is replaced with an empty string.
func expandEnvVars(s string) string {
	// Match ${VAR_NAME} pattern
	re := regexp.MustCompile(`\$\{([^}]+)\}`)

	return re.ReplaceAllStringFunc(s, func(match string) string {
		// Extract variable name from ${VAR_NAME}
		varName := re.FindStringSubmatch(match)[1]
		return os.Getenv(varName)
	})
}

// Validate checks that all required configuration fields are present and valid.
// Returns an error describing the first validation failure encountered.
func (c *Config) Validate() error {
	// Server addresses are required unless Tailscale is enabled
	if !c.Tailscale.Enabled {
		if c.Server.GRPCAddr == "" {
			return fmt.Errorf("server.grpc_addr is required (or enable tailscale)")
		}
		if c.Server.HTTPAddr == "" {
			return fmt.Errorf("server.http_addr is required (or enable tailscale)")
		}
	}

	// Tailscale requires a hostname
	if c.Tailscale.Enabled && c.Tailscale.Hostname == "" {
		return fmt.Errorf("tailscale.hostname is required when tailscale is enabled")
	}

	if c.Database.Path == "" {
		return fmt.Errorf("database.path is required")
	}

	return nil
}

// parseDurations converts the raw duration strings into time.Duration values
func parseDurations(cfg *Config) error {
	var err error

	if cfg.Agents.HeartbeatIntervalRaw != "" {
		cfg.Agents.HeartbeatInterval, err = time.ParseDuration(cfg.Agents.HeartbeatIntervalRaw)
		if err != nil {
			return fmt.Errorf("parsing heartbeat_interval %q: %w", cfg.Agents.HeartbeatIntervalRaw, err)
		}
	}

	if cfg.Agents.HeartbeatTimeoutRaw != "" {
		cfg.Agents.HeartbeatTimeout, err = time.ParseDuration(cfg.Agents.HeartbeatTimeoutRaw)
		if err != nil {
			return fmt.Errorf("parsing heartbeat_timeout %q: %w", cfg.Agents.HeartbeatTimeoutRaw, err)
		}
	}

	if cfg.Agents.ReconnectGracePeriodRaw != "" {
		cfg.Agents.ReconnectGracePeriod, err = time.ParseDuration(cfg.Agents.ReconnectGracePeriodRaw)
		if err != nil {
			return fmt.Errorf("parsing reconnect_grace_period %q: %w", cfg.Agents.ReconnectGracePeriodRaw, err)
		}
	}

	return nil
}
