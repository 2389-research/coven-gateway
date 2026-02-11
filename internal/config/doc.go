// Package config handles configuration loading for coven-gateway.
//
// # Overview
//
// Configuration is loaded from YAML files with environment variable expansion.
// The package provides validation and sensible defaults.
//
// # Configuration File
//
// Default locations (in order):
//
//  1. Path from COVEN_CONFIG environment variable
//  2. ./config.yaml (current directory)
//  3. ~/.config/coven/gateway.yaml
//
// # Environment Variable Expansion
//
// Configuration values can reference environment variables:
//
//	auth:
//	  jwt_secret: "${COVEN_JWT_SECRET}"
//
// Syntax: ${VAR_NAME} or $VAR_NAME
//
// # Duration Parsing
//
// Duration values use Go's time.ParseDuration syntax:
//
//	agents:
//	  heartbeat_interval: "30s"
//	  heartbeat_timeout: "90s"
//	  reconnect_grace_period: "5m"
//
// Supported units: ns, us, ms, s, m, h
//
// # Configuration Sections
//
// Server settings:
//
//	server:
//	  grpc_addr: "0.0.0.0:50051"  # Agent connections
//	  http_addr: "0.0.0.0:8080"   # API and web admin
//
// Database:
//
//	database:
//	  path: "/var/lib/coven/gateway.db"
//
// Authentication:
//
//	auth:
//	  jwt_secret: "${COVEN_JWT_SECRET}"          # Required for auth
//	  agent_auto_registration: "pending"         # pending, approved, disabled
//
// Agent timing:
//
//	agents:
//	  heartbeat_interval: "30s"
//	  heartbeat_timeout: "90s"
//	  reconnect_grace_period: "5m"
//
// Tailscale:
//
//	tailscale:
//	  enabled: false
//	  hostname: "coven-gateway"
//	  auth_key: "${TS_AUTHKEY}"
//	  https: true
//	  funnel: false
//
// Logging:
//
//	logging:
//	  level: "info"   # debug, info, warn, error
//	  format: "text"  # text, json
//
// # Validation
//
// Load() validates:
//
//   - JWT secret minimum length (32 bytes)
//   - Database path writability
//   - Duration format validity
//   - Agent registration mode values
//
// # Usage
//
// Load configuration:
//
//	cfg, err := config.Load()
//	if err != nil {
//	    log.Fatal(err)
//	}
//
// Load from specific path:
//
//	cfg, err := config.LoadFromPath("/etc/coven/gateway.yaml")
package config
