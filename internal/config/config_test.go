// ABOUTME: Tests for configuration loading and parsing
// ABOUTME: Covers YAML loading, env var expansion, and duration parsing

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoad_ValidConfig(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
server:
  grpc_addr: "0.0.0.0:50051"
  http_addr: "0.0.0.0:8080"

database:
  path: "./test.db"

agents:
  heartbeat_interval: "30s"
  heartbeat_timeout: "90s"
  reconnect_grace_period: "5m"

frontends:
  slack:
    enabled: true
    app_token: "xapp-test"
    bot_token: "xoxb-test"
    allowed_channels:
      - "general"
      - "random"

  matrix:
    enabled: false
    homeserver: "https://matrix.org"
    user_id: "@bot:matrix.org"
    access_token: "matrix-token"
    allowed_users:
      - "@user1:matrix.org"
    allowed_rooms:
      - "!room1:matrix.org"

logging:
  level: "debug"
  format: "json"

metrics:
  enabled: true
  path: "/metrics"
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Verify server config
	if cfg.Server.GRPCAddr != "0.0.0.0:50051" {
		t.Errorf("Server.GRPCAddr = %q, want %q", cfg.Server.GRPCAddr, "0.0.0.0:50051")
	}
	if cfg.Server.HTTPAddr != "0.0.0.0:8080" {
		t.Errorf("Server.HTTPAddr = %q, want %q", cfg.Server.HTTPAddr, "0.0.0.0:8080")
	}

	// Verify database config
	if cfg.Database.Path != "./test.db" {
		t.Errorf("Database.Path = %q, want %q", cfg.Database.Path, "./test.db")
	}

	// Verify agents config with duration parsing
	if cfg.Agents.HeartbeatInterval != 30*time.Second {
		t.Errorf("Agents.HeartbeatInterval = %v, want %v", cfg.Agents.HeartbeatInterval, 30*time.Second)
	}
	if cfg.Agents.HeartbeatTimeout != 90*time.Second {
		t.Errorf("Agents.HeartbeatTimeout = %v, want %v", cfg.Agents.HeartbeatTimeout, 90*time.Second)
	}
	if cfg.Agents.ReconnectGracePeriod != 5*time.Minute {
		t.Errorf("Agents.ReconnectGracePeriod = %v, want %v", cfg.Agents.ReconnectGracePeriod, 5*time.Minute)
	}

	// Verify slack frontend config
	if !cfg.Frontends.Slack.Enabled {
		t.Error("Frontends.Slack.Enabled = false, want true")
	}
	if cfg.Frontends.Slack.AppToken != "xapp-test" {
		t.Errorf("Frontends.Slack.AppToken = %q, want %q", cfg.Frontends.Slack.AppToken, "xapp-test")
	}
	if cfg.Frontends.Slack.BotToken != "xoxb-test" {
		t.Errorf("Frontends.Slack.BotToken = %q, want %q", cfg.Frontends.Slack.BotToken, "xoxb-test")
	}
	if len(cfg.Frontends.Slack.AllowedChannels) != 2 {
		t.Errorf("Frontends.Slack.AllowedChannels len = %d, want 2", len(cfg.Frontends.Slack.AllowedChannels))
	}

	// Verify matrix frontend config
	if cfg.Frontends.Matrix.Enabled {
		t.Error("Frontends.Matrix.Enabled = true, want false")
	}
	if cfg.Frontends.Matrix.Homeserver != "https://matrix.org" {
		t.Errorf("Frontends.Matrix.Homeserver = %q, want %q", cfg.Frontends.Matrix.Homeserver, "https://matrix.org")
	}
	if cfg.Frontends.Matrix.UserID != "@bot:matrix.org" {
		t.Errorf("Frontends.Matrix.UserID = %q, want %q", cfg.Frontends.Matrix.UserID, "@bot:matrix.org")
	}
	if len(cfg.Frontends.Matrix.AllowedUsers) != 1 {
		t.Errorf("Frontends.Matrix.AllowedUsers len = %d, want 1", len(cfg.Frontends.Matrix.AllowedUsers))
	}
	if len(cfg.Frontends.Matrix.AllowedRooms) != 1 {
		t.Errorf("Frontends.Matrix.AllowedRooms len = %d, want 1", len(cfg.Frontends.Matrix.AllowedRooms))
	}

	// Verify logging config
	if cfg.Logging.Level != "debug" {
		t.Errorf("Logging.Level = %q, want %q", cfg.Logging.Level, "debug")
	}
	if cfg.Logging.Format != "json" {
		t.Errorf("Logging.Format = %q, want %q", cfg.Logging.Format, "json")
	}

	// Verify metrics config
	if !cfg.Metrics.Enabled {
		t.Error("Metrics.Enabled = false, want true")
	}
	if cfg.Metrics.Path != "/metrics" {
		t.Errorf("Metrics.Path = %q, want %q", cfg.Metrics.Path, "/metrics")
	}
}

func TestLoad_EnvVarExpansion(t *testing.T) {
	// Set environment variables for testing
	t.Setenv("TEST_SLACK_APP_TOKEN", "xapp-from-env")
	t.Setenv("TEST_SLACK_BOT_TOKEN", "xoxb-from-env")
	t.Setenv("TEST_MATRIX_TOKEN", "matrix-from-env")

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
server:
  grpc_addr: "0.0.0.0:50051"
  http_addr: "0.0.0.0:8080"

database:
  path: "./test.db"

agents:
  heartbeat_interval: "30s"
  heartbeat_timeout: "90s"
  reconnect_grace_period: "5m"

frontends:
  slack:
    enabled: true
    app_token: "${TEST_SLACK_APP_TOKEN}"
    bot_token: "${TEST_SLACK_BOT_TOKEN}"
    allowed_channels: []

  matrix:
    enabled: false
    homeserver: "https://matrix.org"
    user_id: "@bot:matrix.org"
    access_token: "${TEST_MATRIX_TOKEN}"
    allowed_users: []
    allowed_rooms: []

logging:
  level: "info"
  format: "text"

metrics:
  enabled: true
  path: "/metrics"
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Verify env var expansion
	if cfg.Frontends.Slack.AppToken != "xapp-from-env" {
		t.Errorf("Frontends.Slack.AppToken = %q, want %q", cfg.Frontends.Slack.AppToken, "xapp-from-env")
	}
	if cfg.Frontends.Slack.BotToken != "xoxb-from-env" {
		t.Errorf("Frontends.Slack.BotToken = %q, want %q", cfg.Frontends.Slack.BotToken, "xoxb-from-env")
	}
	if cfg.Frontends.Matrix.AccessToken != "matrix-from-env" {
		t.Errorf("Frontends.Matrix.AccessToken = %q, want %q", cfg.Frontends.Matrix.AccessToken, "matrix-from-env")
	}
}

func TestLoad_EnvVarExpansion_UnsetVar(t *testing.T) {
	// Ensure the env var is NOT set
	os.Unsetenv("UNSET_VAR_FOR_TEST")

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
server:
  grpc_addr: "0.0.0.0:50051"
  http_addr: "0.0.0.0:8080"

database:
  path: "./test.db"

agents:
  heartbeat_interval: "30s"
  heartbeat_timeout: "90s"
  reconnect_grace_period: "5m"

frontends:
  slack:
    enabled: false
    app_token: "${UNSET_VAR_FOR_TEST}"
    bot_token: "literal-token"
    allowed_channels: []

  matrix:
    enabled: false
    homeserver: "https://matrix.org"
    user_id: "@bot:matrix.org"
    access_token: ""
    allowed_users: []
    allowed_rooms: []

logging:
  level: "info"
  format: "text"

metrics:
  enabled: true
  path: "/metrics"
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Unset env vars should expand to empty string
	if cfg.Frontends.Slack.AppToken != "" {
		t.Errorf("Frontends.Slack.AppToken = %q, want empty string for unset env var", cfg.Frontends.Slack.AppToken)
	}
}

func TestLoad_DurationParsing(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
server:
  grpc_addr: "0.0.0.0:50051"
  http_addr: "0.0.0.0:8080"

database:
  path: "./test.db"

agents:
  heartbeat_interval: "1m30s"
  heartbeat_timeout: "2h"
  reconnect_grace_period: "10m"

frontends:
  slack:
    enabled: false
    app_token: ""
    bot_token: ""
    allowed_channels: []

  matrix:
    enabled: false
    homeserver: "https://matrix.org"
    user_id: "@bot:matrix.org"
    access_token: ""
    allowed_users: []
    allowed_rooms: []

logging:
  level: "info"
  format: "text"

metrics:
  enabled: true
  path: "/metrics"
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Verify complex duration parsing
	expectedInterval := 1*time.Minute + 30*time.Second
	if cfg.Agents.HeartbeatInterval != expectedInterval {
		t.Errorf("Agents.HeartbeatInterval = %v, want %v", cfg.Agents.HeartbeatInterval, expectedInterval)
	}

	if cfg.Agents.HeartbeatTimeout != 2*time.Hour {
		t.Errorf("Agents.HeartbeatTimeout = %v, want %v", cfg.Agents.HeartbeatTimeout, 2*time.Hour)
	}

	if cfg.Agents.ReconnectGracePeriod != 10*time.Minute {
		t.Errorf("Agents.ReconnectGracePeriod = %v, want %v", cfg.Agents.ReconnectGracePeriod, 10*time.Minute)
	}
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := Load("/nonexistent/path/config.yaml")
	if err == nil {
		t.Error("Load() expected error for missing file, got nil")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Invalid YAML content
	configContent := `
server:
  grpc_addr: "0.0.0.0:50051"
  http_addr "missing colon"
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_, err = Load(configPath)
	if err == nil {
		t.Error("Load() expected error for invalid YAML, got nil")
	}
}

func TestLoad_InvalidDuration(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
server:
  grpc_addr: "0.0.0.0:50051"
  http_addr: "0.0.0.0:8080"

database:
  path: "./test.db"

agents:
  heartbeat_interval: "invalid-duration"
  heartbeat_timeout: "90s"
  reconnect_grace_period: "5m"

frontends:
  slack:
    enabled: false
    app_token: ""
    bot_token: ""
    allowed_channels: []

  matrix:
    enabled: false
    homeserver: "https://matrix.org"
    user_id: "@bot:matrix.org"
    access_token: ""
    allowed_users: []
    allowed_rooms: []

logging:
  level: "info"
  format: "text"

metrics:
  enabled: true
  path: "/metrics"
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_, err = Load(configPath)
	if err == nil {
		t.Error("Load() expected error for invalid duration, got nil")
	}
}

func TestLoad_MissingRequiredFields(t *testing.T) {
	tests := []struct {
		name          string
		configContent string
		wantErrSubstr string
	}{
		{
			name: "missing grpc_addr",
			configContent: `
server:
  grpc_addr: ""
  http_addr: "0.0.0.0:8080"
database:
  path: "./test.db"
`,
			wantErrSubstr: "server.grpc_addr is required",
		},
		{
			name: "missing http_addr",
			configContent: `
server:
  grpc_addr: "0.0.0.0:50051"
  http_addr: ""
database:
  path: "./test.db"
`,
			wantErrSubstr: "server.http_addr is required",
		},
		{
			name: "missing database path",
			configContent: `
server:
  grpc_addr: "0.0.0.0:50051"
  http_addr: "0.0.0.0:8080"
database:
  path: ""
`,
			wantErrSubstr: "database.path is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.yaml")

			err := os.WriteFile(configPath, []byte(tt.configContent), 0644)
			if err != nil {
				t.Fatalf("failed to write test config: %v", err)
			}

			_, err = Load(configPath)
			if err == nil {
				t.Errorf("Load() expected error containing %q, got nil", tt.wantErrSubstr)
				return
			}

			if !strings.Contains(err.Error(), tt.wantErrSubstr) {
				t.Errorf("Load() error = %q, want error containing %q", err.Error(), tt.wantErrSubstr)
			}
		})
	}
}

func TestExpandEnvVars(t *testing.T) {
	t.Setenv("FOO", "bar")
	t.Setenv("BAZ", "qux")

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "single env var",
			input:    "${FOO}",
			expected: "bar",
		},
		{
			name:     "env var with surrounding text",
			input:    "prefix-${FOO}-suffix",
			expected: "prefix-bar-suffix",
		},
		{
			name:     "multiple env vars",
			input:    "${FOO}/${BAZ}",
			expected: "bar/qux",
		},
		{
			name:     "no env vars",
			input:    "no-vars-here",
			expected: "no-vars-here",
		},
		{
			name:     "unset env var",
			input:    "${UNSET_VAR}",
			expected: "",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := expandEnvVars(tt.input)
			if result != tt.expected {
				t.Errorf("expandEnvVars(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestValidate_TailscaleConfig(t *testing.T) {
	tests := []struct {
		name          string
		cfg           Config
		wantErr       bool
		wantErrSubstr string
	}{
		{
			name: "tailscale enabled allows empty server addresses",
			cfg: Config{
				Server:    ServerConfig{GRPCAddr: "", HTTPAddr: ""},
				Tailscale: TailscaleConfig{Enabled: true, Hostname: "fold-gateway"},
				Database:  DatabaseConfig{Path: "./test.db"},
			},
			wantErr: false,
		},
		{
			name: "tailscale enabled requires hostname",
			cfg: Config{
				Server:    ServerConfig{GRPCAddr: "", HTTPAddr: ""},
				Tailscale: TailscaleConfig{Enabled: true, Hostname: ""},
				Database:  DatabaseConfig{Path: "./test.db"},
			},
			wantErr:       true,
			wantErrSubstr: "tailscale.hostname is required",
		},
		{
			name: "tailscale disabled requires server addresses",
			cfg: Config{
				Server:    ServerConfig{GRPCAddr: "", HTTPAddr: ""},
				Tailscale: TailscaleConfig{Enabled: false, Hostname: "fold-gateway"},
				Database:  DatabaseConfig{Path: "./test.db"},
			},
			wantErr:       true,
			wantErrSubstr: "server.grpc_addr is required",
		},
		{
			name: "tailscale with all options set",
			cfg: Config{
				Server: ServerConfig{GRPCAddr: "", HTTPAddr: ""},
				Tailscale: TailscaleConfig{
					Enabled:   true,
					Hostname:  "fold-gateway",
					AuthKey:   "tskey-auth-xxx",
					StateDir:  "/tmp/ts-state",
					Ephemeral: true,
					Funnel:    true,
				},
				Database: DatabaseConfig{Path: "./test.db"},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("Validate() expected error containing %q, got nil", tt.wantErrSubstr)
					return
				}
				if !strings.Contains(err.Error(), tt.wantErrSubstr) {
					t.Errorf("Validate() error = %q, want error containing %q", err.Error(), tt.wantErrSubstr)
				}
			} else {
				if err != nil {
					t.Errorf("Validate() unexpected error: %v", err)
				}
			}
		})
	}
}
