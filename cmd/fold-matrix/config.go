// ABOUTME: Configuration loading for fold-matrix bridge
// ABOUTME: Loads TOML config from XDG path with environment variable expansion

package main

import (
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Matrix  MatrixConfig  `toml:"matrix"`
	Gateway GatewayConfig `toml:"gateway"`
	Bridge  BridgeConfig  `toml:"bridge"`
	Logging LoggingConfig `toml:"logging"`
}

type MatrixConfig struct {
	Homeserver  string `toml:"homeserver"`
	Username    string `toml:"username"`
	Password    string `toml:"password"`
	RecoveryKey string `toml:"recovery_key"`
}

type GatewayConfig struct {
	URL string `toml:"url"`
}

type BridgeConfig struct {
	AllowedRooms    []string `toml:"allowed_rooms"`
	CommandPrefix   string   `toml:"command_prefix"`
	TypingIndicator bool     `toml:"typing_indicator"`
}

type LoggingConfig struct {
	Level string `toml:"level"`
}

// Load reads config from the given path, expanding environment variables.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	// Expand environment variables (${VAR} syntax)
	expanded := expandEnvVars(string(data))

	var cfg Config
	if _, err := toml.Decode(expanded, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	return &cfg, nil
}

// expandEnvVars replaces ${VAR} with environment variable values.
func expandEnvVars(s string) string {
	re := regexp.MustCompile(`\$\{([^}]+)\}`)
	return re.ReplaceAllStringFunc(s, func(match string) string {
		varName := strings.TrimSuffix(strings.TrimPrefix(match, "${"), "}")
		return os.Getenv(varName)
	})
}

// Validate checks that required config fields are present and valid.
func (c *Config) Validate() error {
	if c.Matrix.Homeserver == "" {
		return fmt.Errorf("matrix.homeserver is required")
	}
	// Validate homeserver URL
	if _, err := url.Parse(c.Matrix.Homeserver); err != nil {
		return fmt.Errorf("matrix.homeserver is not a valid URL: %w", err)
	}
	if c.Matrix.Username == "" {
		return fmt.Errorf("matrix.username is required")
	}
	if c.Matrix.Password == "" {
		return fmt.Errorf("matrix.password is required")
	}
	if c.Gateway.URL == "" {
		return fmt.Errorf("gateway.url is required")
	}
	// Validate gateway URL
	u, err := url.Parse(c.Gateway.URL)
	if err != nil {
		return fmt.Errorf("gateway.url is not a valid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("gateway.url must use http or https scheme")
	}
	return nil
}
