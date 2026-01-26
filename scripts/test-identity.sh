#!/bin/bash
# ABOUTME: Manual testing script for COVEN v1 identity foundation
# ABOUTME: Creates test data, generates JWT tokens, and starts the gateway

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
DB_PATH="${1:-/tmp/coven-test.db}"
JWT_SECRET="test-secret-key-for-jwt-signing-minimum-32-bytes!"
CONFIG_PATH="/tmp/test-gateway.yaml"

echo -e "${CYAN}=== COVEN v1 Identity Foundation Test ===${NC}"
echo ""

# Check for required tools
if ! command -v sqlite3 &> /dev/null; then
    echo -e "${RED}Error: sqlite3 is required${NC}"
    exit 1
fi

if ! command -v go &> /dev/null; then
    echo -e "${RED}Error: go is required${NC}"
    exit 1
fi

# Step 1: Create test database with schema
echo -e "${YELLOW}Step 1: Setting up test database${NC}"
echo "  Database: $DB_PATH"

# Remove old test DB
rm -f "$DB_PATH"

# Create the full schema that matches the gateway's sqlite.go createSchema()
sqlite3 "$DB_PATH" <<'SQL'
-- Threads table
CREATE TABLE IF NOT EXISTS threads (
    id TEXT PRIMARY KEY,
    frontend_name TEXT NOT NULL,
    external_id TEXT NOT NULL,
    agent_id TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_threads_frontend_external
    ON threads(frontend_name, external_id);

-- Messages table
CREATE TABLE IF NOT EXISTS messages (
    id TEXT PRIMARY KEY,
    thread_id TEXT NOT NULL,
    sender TEXT NOT NULL,
    content TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    FOREIGN KEY (thread_id) REFERENCES threads(id)
);

CREATE INDEX IF NOT EXISTS idx_messages_thread_id ON messages(thread_id);
CREATE INDEX IF NOT EXISTS idx_messages_thread_created ON messages(thread_id, created_at);

-- Agent state table
CREATE TABLE IF NOT EXISTS agent_state (
    agent_id TEXT PRIMARY KEY,
    state BLOB NOT NULL,
    updated_at DATETIME NOT NULL
);

-- Channel bindings table
CREATE TABLE IF NOT EXISTS channel_bindings (
    frontend TEXT NOT NULL,
    channel_id TEXT NOT NULL,
    agent_id TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    PRIMARY KEY (frontend, channel_id)
);

-- Principals table
CREATE TABLE IF NOT EXISTS principals (
    principal_id       TEXT PRIMARY KEY,
    type               TEXT NOT NULL,
    pubkey_fingerprint TEXT NOT NULL UNIQUE,
    display_name       TEXT NOT NULL,
    status             TEXT NOT NULL,
    created_at         TEXT NOT NULL,
    last_seen          TEXT,
    metadata_json      TEXT,

    CHECK (type IN ('client', 'agent', 'pack')),
    CHECK (status IN ('pending', 'approved', 'revoked', 'offline', 'online'))
);

CREATE INDEX IF NOT EXISTS idx_principals_status ON principals(status);
CREATE INDEX IF NOT EXISTS idx_principals_type ON principals(type);
CREATE INDEX IF NOT EXISTS idx_principals_pubkey ON principals(pubkey_fingerprint);

-- Roles table
CREATE TABLE IF NOT EXISTS roles (
    subject_type TEXT NOT NULL,
    subject_id   TEXT NOT NULL,
    role         TEXT NOT NULL,
    created_at   TEXT NOT NULL,

    PRIMARY KEY (subject_type, subject_id, role),
    CHECK (subject_type IN ('principal', 'member')),
    CHECK (role IN ('owner', 'admin', 'member'))
);

CREATE INDEX IF NOT EXISTS idx_roles_subject ON roles(subject_type, subject_id);

-- Audit log table
CREATE TABLE IF NOT EXISTS audit_log (
    audit_id           TEXT PRIMARY KEY,
    actor_principal_id TEXT NOT NULL,
    actor_member_id    TEXT,
    action             TEXT NOT NULL,
    target_type        TEXT NOT NULL,
    target_id          TEXT NOT NULL,
    ts                 TEXT NOT NULL,
    detail_json        TEXT,

    CHECK (action IN (
        'approve_principal',
        'revoke_principal',
        'grant_capability',
        'revoke_capability',
        'create_binding',
        'update_binding',
        'delete_binding'
    ))
);

CREATE INDEX IF NOT EXISTS idx_audit_ts ON audit_log(ts DESC);
CREATE INDEX IF NOT EXISTS idx_audit_actor ON audit_log(actor_principal_id);
CREATE INDEX IF NOT EXISTS idx_audit_target ON audit_log(target_type, target_id);

-- Ledger events table
CREATE TABLE IF NOT EXISTS ledger_events (
    event_id           TEXT PRIMARY KEY,
    conversation_key   TEXT NOT NULL,
    direction          TEXT NOT NULL,
    author             TEXT NOT NULL,
    timestamp          TEXT NOT NULL,
    type               TEXT NOT NULL,
    text               TEXT,
    raw_transport      TEXT,
    raw_payload_ref    TEXT,
    actor_principal_id TEXT,
    actor_member_id    TEXT,

    CHECK (direction IN ('inbound_to_agent', 'outbound_from_agent')),
    CHECK (type IN ('message', 'tool_call', 'tool_result', 'system', 'error'))
);

CREATE INDEX IF NOT EXISTS idx_ledger_conversation ON ledger_events(conversation_key, timestamp);
CREATE INDEX IF NOT EXISTS idx_ledger_actor ON ledger_events(actor_principal_id);
CREATE INDEX IF NOT EXISTS idx_ledger_timestamp ON ledger_events(timestamp);

-- Bindings table (v2 admin API)
CREATE TABLE IF NOT EXISTS bindings (
    binding_id TEXT PRIMARY KEY,
    frontend   TEXT NOT NULL,
    channel_id TEXT NOT NULL,
    agent_id   TEXT NOT NULL,
    created_at TEXT NOT NULL,
    created_by TEXT,

    UNIQUE(frontend, channel_id)
);

CREATE INDEX IF NOT EXISTS idx_bindings_frontend ON bindings(frontend);
CREATE INDEX IF NOT EXISTS idx_bindings_agent ON bindings(agent_id);
SQL

echo -e "  ${GREEN}✓ Database schema created${NC}"

# Step 2: Create test principals
echo ""
echo -e "${YELLOW}Step 2: Creating test principals${NC}"

ADMIN_ID="admin-$(uuidgen | tr '[:upper:]' '[:lower:]')"
CLIENT_ID="client-$(uuidgen | tr '[:upper:]' '[:lower:]')"
AGENT_ID="agent-$(uuidgen | tr '[:upper:]' '[:lower:]')"
NOW=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

sqlite3 "$DB_PATH" <<SQL
-- Admin user
INSERT INTO principals (principal_id, type, pubkey_fingerprint, display_name, status, created_at)
VALUES ('$ADMIN_ID', 'client', '$(echo -n "admin-key-$ADMIN_ID" | shasum -a 256 | cut -d' ' -f1)', 'Test Admin', 'approved', '$NOW');

-- Regular client
INSERT INTO principals (principal_id, type, pubkey_fingerprint, display_name, status, created_at)
VALUES ('$CLIENT_ID', 'client', '$(echo -n "client-key-$CLIENT_ID" | shasum -a 256 | cut -d' ' -f1)', 'Test Client', 'approved', '$NOW');

-- Agent
INSERT INTO principals (principal_id, type, pubkey_fingerprint, display_name, status, created_at)
VALUES ('$AGENT_ID', 'agent', '$(echo -n "agent-key-$AGENT_ID" | shasum -a 256 | cut -d' ' -f1)', 'Test Agent', 'online', '$NOW');

-- Give admin the admin role
INSERT INTO roles (subject_type, subject_id, role, created_at)
VALUES ('principal', '$ADMIN_ID', 'admin', '$NOW');

-- Give client the member role
INSERT INTO roles (subject_type, subject_id, role, created_at)
VALUES ('principal', '$CLIENT_ID', 'member', '$NOW');
SQL

echo -e "  ${GREEN}✓ Created admin:  $ADMIN_ID${NC}"
echo -e "  ${GREEN}✓ Created client: $CLIENT_ID${NC}"
echo -e "  ${GREEN}✓ Created agent:  $AGENT_ID${NC}"

# Step 3: Generate JWT tokens
echo ""
echo -e "${YELLOW}Step 3: Generating JWT tokens${NC}"

# Create a small Go program to generate tokens
TEMP_DIR=$(mktemp -d)
cat > "$TEMP_DIR/gen_token.go" <<'GOCODE'
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "Usage: gen_token <principal_id> <secret>")
		os.Exit(1)
	}

	principalID := os.Args[1]
	secret := []byte(os.Args[2])

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": principalID,
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(24 * time.Hour).Unix(),
	})

	tokenString, err := token.SignedString(secret)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	fmt.Println(tokenString)
}
GOCODE

cd "$TEMP_DIR"
go mod init temp > /dev/null 2>&1
go get github.com/golang-jwt/jwt/v5 > /dev/null 2>&1

ADMIN_TOKEN=$(go run gen_token.go "$ADMIN_ID" "$JWT_SECRET")
CLIENT_TOKEN=$(go run gen_token.go "$CLIENT_ID" "$JWT_SECRET")

cd - > /dev/null
rm -rf "$TEMP_DIR"

echo -e "  ${GREEN}✓ Tokens generated (valid for 24 hours)${NC}"

# Step 4: Create config file
echo ""
echo -e "${YELLOW}Step 4: Creating gateway config${NC}"

cat > "$CONFIG_PATH" << EOF
server:
  grpc_addr: "localhost:50099"
  http_addr: "localhost:8099"

database:
  path: "$DB_PATH"

auth:
  jwt_secret: "$JWT_SECRET"

tailscale:
  enabled: false

agents:
  heartbeat_interval: "30s"
  heartbeat_timeout: "90s"
  reconnect_grace_period: "5m"

logging:
  level: "debug"
  format: "text"

metrics:
  enabled: false
EOF

echo -e "  ${GREEN}✓ Config written to $CONFIG_PATH${NC}"

# Step 5: Print test instructions
echo ""
echo -e "${CYAN}════════════════════════════════════════════════════════════════${NC}"
echo -e "${CYAN}                    TEST ENVIRONMENT READY                       ${NC}"
echo -e "${CYAN}════════════════════════════════════════════════════════════════${NC}"
echo ""
echo -e "${YELLOW}Principals:${NC}"
echo "  Admin:  $ADMIN_ID (role: admin)"
echo "  Client: $CLIENT_ID (role: member)"
echo "  Agent:  $AGENT_ID (status: online)"
echo ""
echo -e "${CYAN}════════════════════════════════════════════════════════════════${NC}"
echo -e "${YELLOW}STEP 1: Start the gateway (in this terminal)${NC}"
echo -e "${CYAN}════════════════════════════════════════════════════════════════${NC}"
echo ""
echo "cd $PROJECT_DIR"
echo "COVEN_CONFIG=$CONFIG_PATH go run ./cmd/coven-gateway serve"
echo ""
echo -e "${CYAN}════════════════════════════════════════════════════════════════${NC}"
echo -e "${YELLOW}STEP 2: Test with grpcurl (in another terminal)${NC}"
echo -e "${CYAN}════════════════════════════════════════════════════════════════${NC}"
echo ""
echo "# Install grpcurl if needed:"
echo "brew install grpcurl"
echo ""
echo "# Export tokens:"
echo "export ADMIN_TOKEN=\"$ADMIN_TOKEN\""
echo "export CLIENT_TOKEN=\"$CLIENT_TOKEN\""
echo "export AGENT_ID=\"$AGENT_ID\""
echo ""
echo "# Test GetMe as admin (should work):"
echo 'grpcurl -plaintext -H "authorization: Bearer $ADMIN_TOKEN" localhost:50099 coven.ClientService/GetMe'
echo ""
echo "# Test GetMe as client (should work):"
echo 'grpcurl -plaintext -H "authorization: Bearer $CLIENT_TOKEN" localhost:50099 coven.ClientService/GetMe'
echo ""
echo "# Test ListBindings as admin (should work - returns empty list):"
echo 'grpcurl -plaintext -H "authorization: Bearer $ADMIN_TOKEN" localhost:50099 coven.AdminService/ListBindings'
echo ""
echo "# Test ListBindings as client (should FAIL with PERMISSION_DENIED):"
echo 'grpcurl -plaintext -H "authorization: Bearer $CLIENT_TOKEN" localhost:50099 coven.AdminService/ListBindings'
echo ""
echo "# Create a binding as admin:"
echo 'grpcurl -plaintext -H "authorization: Bearer $ADMIN_TOKEN" -d "{\"frontend\":\"matrix\",\"channel_id\":\"!test:example.org\",\"agent_id\":\"'$AGENT_ID'\"}" localhost:50099 coven.AdminService/CreateBinding'
echo ""
echo -e "${CYAN}════════════════════════════════════════════════════════════════${NC}"
