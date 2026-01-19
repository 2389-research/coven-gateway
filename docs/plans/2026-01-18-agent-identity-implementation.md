# Agent Identity Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement magical agent auto-registration where agents connect with SSH keys and are automatically registered, with instance IDs for binding and workspace filtering for clients.

**Architecture:** Gateway auto-creates principals from SSH fingerprints, uses authenticated principal_id for registration, generates short instance IDs. Agents send workspaces in metadata. Clients filter agents by workspace.

**Tech Stack:** Go (gateway), Rust (agent), Protocol Buffers, SQLite

---

## Task 1: Add Workspaces to Proto

**Files:**
- Modify: `../fold-agent/proto/fold.proto:33-38`
- Regenerate: Gateway proto files

**Step 1: Update AgentMetadata in proto**

Edit `../fold-agent/proto/fold.proto`, add workspaces field to AgentMetadata:

```protobuf
// Environment metadata sent during registration
message AgentMetadata {
  string working_directory = 1;
  GitInfo git = 2;
  string hostname = 3;
  string os = 4;
  repeated string workspaces = 5;  // NEW: workspace tags for filtering
}
```

**Step 2: Add instance_id to Welcome message**

Edit the Welcome message in the same file:

```protobuf
// Server acknowledges registration
message Welcome {
  string server_id = 1;
  string agent_id = 2;     // Confirmed agent ID (instance name)
  string instance_id = 3;  // NEW: Short code for binding commands
  string principal_id = 4; // NEW: Principal UUID for reference
}
```

**Step 3: Regenerate Go proto files**

Run:
```bash
cd /Users/harper/Public/src/2389/fold-project/fold-gateway && make proto
```

Expected: Proto files regenerated successfully.

**Step 4: Commit**

```bash
git add proto/ ../fold-agent/proto/fold.proto
git commit -m "proto: add workspaces to AgentMetadata and instance_id to Welcome"
```

---

## Task 2: Add Auto-Registration Config Option

**Files:**
- Modify: `internal/config/config.go`
- Modify: `config.yaml`

**Step 1: Add config field**

Edit `internal/config/config.go`, add to the Auth struct:

```go
type Auth struct {
	JWTSecret             string `yaml:"jwt_secret"`
	AgentAutoRegistration string `yaml:"agent_auto_registration"` // "approved", "pending", or "disabled"
}
```

**Step 2: Update config.yaml example**

Add to the `auth:` section in `config.yaml`:

```yaml
auth:
  # JWT secret for token authentication (min 32 bytes)
  jwt_secret: "${FOLD_JWT_SECRET}"
  # Agent auto-registration mode: approved (default), pending, or disabled
  agent_auto_registration: "approved"
```

**Step 3: Commit**

```bash
git add internal/config/config.go config.yaml
git commit -m "config: add agent_auto_registration option"
```

---

## Task 3: Implement Principal Auto-Creation in Auth Interceptor

**Files:**
- Modify: `internal/auth/interceptor.go`
- Test: `internal/auth/interceptor_test.go`

**Step 1: Write failing test for auto-creation**

Add to `internal/auth/interceptor_test.go`:

```go
func TestExtractAuth_AutoCreatesPrincipal(t *testing.T) {
	store := newMockPrincipalStore()
	roles := newMockRoleStore()
	sshVerifier := NewSSHVerifier()

	// Generate a test SSH key
	privateKey, err := generateTestSSHKey()
	require.NoError(t, err)

	fingerprint := computeFingerprint(privateKey.PublicKey())

	// Create valid SSH auth metadata
	md := createSSHAuthMetadata(t, privateKey)
	ctx := metadata.NewIncomingContext(context.Background(), md)

	// Configure auto-registration as approved
	config := &AuthConfig{AgentAutoRegistration: "approved"}

	// Extract auth - should auto-create principal
	authCtx, err := extractAuthWithConfig(ctx, store, roles, nil, sshVerifier, config)
	require.NoError(t, err)
	assert.NotEmpty(t, authCtx.PrincipalID)

	// Verify principal was created
	principal, err := store.GetPrincipalByPubkey(ctx, fingerprint)
	require.NoError(t, err)
	assert.Equal(t, "approved", string(principal.Status))
}

func TestExtractAuth_RejectsUnknownWhenDisabled(t *testing.T) {
	store := newMockPrincipalStore()
	roles := newMockRoleStore()
	sshVerifier := NewSSHVerifier()

	privateKey, err := generateTestSSHKey()
	require.NoError(t, err)

	md := createSSHAuthMetadata(t, privateKey)
	ctx := metadata.NewIncomingContext(context.Background(), md)

	// Configure auto-registration as disabled
	config := &AuthConfig{AgentAutoRegistration: "disabled"}

	// Extract auth - should reject
	_, err = extractAuthWithConfig(ctx, store, roles, nil, sshVerifier, config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown public key")
}
```

**Step 2: Run test to verify it fails**

Run:
```bash
go test ./internal/auth/... -run TestExtractAuth_AutoCreates -v
```

Expected: FAIL (function doesn't exist yet)

**Step 3: Add PrincipalCreator interface**

Edit `internal/auth/interceptor.go`, add interface:

```go
// PrincipalCreator can create new principals (for auto-registration)
type PrincipalCreator interface {
	CreatePrincipal(ctx context.Context, p *store.Principal) error
}

// AuthConfig holds auth configuration options
type AuthConfig struct {
	AgentAutoRegistration string // "approved", "pending", or "disabled"
}
```

**Step 4: Implement auto-creation logic**

Modify the SSH auth section of `extractAuth` (around line 130):

```go
// In extractAuth, when fingerprint not found:
p, err := principals.GetPrincipalByPubkey(ctx, fingerprint)
if err != nil {
	if errors.Is(err, store.ErrPrincipalNotFound) {
		// Check if auto-registration is enabled
		if config == nil || config.AgentAutoRegistration == "disabled" {
			return nil, status.Error(codes.Unauthenticated, "unknown public key")
		}

		// Auto-create principal
		creator, ok := principals.(PrincipalCreator)
		if !ok {
			return nil, status.Error(codes.Internal, "principal store does not support creation")
		}

		principalStatus := store.PrincipalStatusApproved
		if config.AgentAutoRegistration == "pending" {
			principalStatus = store.PrincipalStatusPending
		}

		newPrincipal := &store.Principal{
			ID:          uuid.New().String(),
			Type:        store.PrincipalTypeAgent,
			PubkeyFP:    fingerprint,
			DisplayName: "auto-registered",
			Status:      principalStatus,
			CreatedAt:   time.Now(),
		}

		if err := creator.CreatePrincipal(ctx, newPrincipal); err != nil {
			return nil, status.Errorf(codes.Internal, "auto-creating principal: %v", err)
		}

		p = newPrincipal
	} else {
		return nil, status.Errorf(codes.Internal, "failed to lookup principal: %v", err)
	}
}
principal = p
principalID = p.ID
```

**Step 5: Run test to verify it passes**

Run:
```bash
go test ./internal/auth/... -run TestExtractAuth_AutoCreates -v
```

Expected: PASS

**Step 6: Commit**

```bash
git add internal/auth/interceptor.go internal/auth/interceptor_test.go
git commit -m "feat(auth): auto-create principal for unknown SSH fingerprints"
```

---

## Task 4: Update grpc.go to Use Authenticated Principal

**Files:**
- Modify: `internal/gateway/grpc.go`
- Modify: `internal/agent/connection.go`
- Modify: `internal/agent/manager.go`

**Step 1: Add PrincipalID to Connection**

Edit `internal/agent/connection.go`, add field:

```go
type Connection struct {
	ID           string   // Instance name (for routing)
	PrincipalID  string   // Authenticated principal UUID (for audit)
	Name         string
	Capabilities []string
	// ... rest of fields
}
```

Update `NewConnection`:

```go
func NewConnection(
	id string,
	principalID string,
	name string,
	capabilities []string,
	stream pb.FoldControl_AgentStreamServer,
	logger *slog.Logger,
) *Connection {
	return &Connection{
		ID:           id,
		PrincipalID:  principalID,
		Name:         name,
		Capabilities: capabilities,
		// ... rest
	}
}
```

**Step 2: Update grpc.go AgentStream**

Edit `internal/gateway/grpc.go`, update the AgentStream function:

```go
func (s *foldControlServer) AgentStream(stream pb.FoldControl_AgentStreamServer) error {
	s.logger.Debug("AgentStream handler invoked, waiting for registration")

	// Extract authenticated principal from context
	authCtx := auth.FromContext(stream.Context())
	var principalID string
	if authCtx != nil {
		principalID = authCtx.PrincipalID
	}

	// Send headers immediately
	if err := stream.SendHeader(nil); err != nil {
		s.logger.Error("failed to send initial headers", "error", err)
	}

	// Wait for registration message
	msg, err := stream.Recv()
	if err != nil {
		if err == io.EOF {
			return nil
		}
		return status.Errorf(codes.Internal, "receiving first message: %v", err)
	}

	reg := msg.GetRegister()
	if reg == nil {
		return status.Error(codes.InvalidArgument, "first message must be RegisterAgent")
	}

	if reg.GetAgentId() == "" {
		return status.Error(codes.InvalidArgument, "agent_id is required")
	}

	// Generate instance ID (first 8 chars of a new UUID)
	instanceID := uuid.New().String()[:8]

	// Create connection with both identifiers
	conn := agent.NewConnection(
		reg.GetAgentId(),  // Instance name for routing
		principalID,       // Principal ID for auth/audit
		reg.GetName(),
		reg.GetCapabilities(),
		stream,
		s.logger.With("agent_id", reg.GetAgentId(), "principal_id", principalID),
	)

	// Register the agent
	if err := s.gateway.agentManager.Register(conn); err != nil {
		if errors.Is(err, agent.ErrAgentAlreadyRegistered) {
			// Send RegistrationError instead of returning gRPC error
			regErr := &pb.ServerMessage{
				Payload: &pb.ServerMessage_RegistrationError{
					RegistrationError: &pb.RegistrationError{
						Reason: fmt.Sprintf("agent %s already registered", reg.GetAgentId()),
					},
				},
			}
			stream.Send(regErr)
			return nil
		}
		return status.Errorf(codes.Internal, "registering agent: %v", err)
	}

	defer s.gateway.agentManager.Unregister(conn.ID)

	// Send welcome with instance_id
	welcome := &pb.ServerMessage{
		Payload: &pb.ServerMessage_Welcome{
			Welcome: &pb.Welcome{
				ServerId:    s.gateway.serverID,
				AgentId:     reg.GetAgentId(),
				InstanceId:  instanceID,
				PrincipalId: principalID,
			},
		},
	}

	if err := stream.Send(welcome); err != nil {
		return status.Errorf(codes.Internal, "sending welcome: %v", err)
	}

	// ... rest of message loop unchanged
}
```

**Step 3: Update AgentInfo to include metadata**

Edit `internal/agent/manager.go`:

```go
type AgentInfo struct {
	ID           string
	PrincipalID  string
	Name         string
	Capabilities []string
	Workspaces   []string
	WorkingDir   string
	InstanceID   string
}
```

**Step 4: Run tests**

Run:
```bash
go test ./internal/gateway/... ./internal/agent/... -v
```

Expected: PASS (may need to update some test helpers)

**Step 5: Commit**

```bash
git add internal/gateway/grpc.go internal/agent/connection.go internal/agent/manager.go
git commit -m "feat(grpc): use authenticated principal, add instance_id to Welcome"
```

---

## Task 5: Update Agent List API to Include Workspaces

**Files:**
- Modify: `internal/gateway/api.go`

**Step 1: Update handleListAgents response**

The `/api/agents` endpoint should return workspaces. Edit `internal/gateway/api.go`:

```go
func (g *Gateway) handleListAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get optional workspace filter from query param
	workspaceFilter := r.URL.Query().Get("workspace")

	agents := g.agentManager.ListAgents()

	// Filter by workspace if specified
	if workspaceFilter != "" {
		filtered := make([]*agent.AgentInfo, 0)
		for _, a := range agents {
			for _, ws := range a.Workspaces {
				if ws == workspaceFilter {
					filtered = append(filtered, a)
					break
				}
			}
		}
		agents = filtered
	}

	type agentResponse struct {
		ID           string   `json:"id"`
		InstanceID   string   `json:"instance_id"`
		Name         string   `json:"name"`
		Capabilities []string `json:"capabilities"`
		Workspaces   []string `json:"workspaces"`
		WorkingDir   string   `json:"working_dir"`
	}

	response := make([]agentResponse, len(agents))
	for i, a := range agents {
		response[i] = agentResponse{
			ID:           a.ID,
			InstanceID:   a.InstanceID,
			Name:         a.Name,
			Capabilities: a.Capabilities,
			Workspaces:   a.Workspaces,
			WorkingDir:   a.WorkingDir,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
```

**Step 2: Run tests**

Run:
```bash
go test ./internal/gateway/... -run TestHandleListAgents -v
```

**Step 3: Commit**

```bash
git add internal/gateway/api.go
git commit -m "feat(api): add workspaces and workspace filter to agent list"
```

---

## Task 6: Store Metadata on Connection

**Files:**
- Modify: `internal/agent/connection.go`
- Modify: `internal/gateway/grpc.go`

**Step 1: Add metadata fields to Connection**

Edit `internal/agent/connection.go`:

```go
type Connection struct {
	ID           string
	PrincipalID  string
	Name         string
	Capabilities []string
	Workspaces   []string  // From metadata
	WorkingDir   string    // From metadata
	InstanceID   string    // Short code for binding
	// ... rest of fields
}
```

**Step 2: Extract metadata in grpc.go**

Update the registration section in `AgentStream`:

```go
// Extract metadata
var workspaces []string
var workingDir string
if meta := reg.GetMetadata(); meta != nil {
	workingDir = meta.GetWorkingDirectory()
	workspaces = meta.GetWorkspaces()
}

// Generate instance ID
instanceID := uuid.New().String()[:8]

// Create connection with metadata
conn := agent.NewConnection(
	reg.GetAgentId(),
	principalID,
	reg.GetName(),
	reg.GetCapabilities(),
	workspaces,
	workingDir,
	instanceID,
	stream,
	s.logger.With("agent_id", reg.GetAgentId()),
)
```

**Step 3: Update NewConnection signature**

```go
func NewConnection(
	id string,
	principalID string,
	name string,
	capabilities []string,
	workspaces []string,
	workingDir string,
	instanceID string,
	stream pb.FoldControl_AgentStreamServer,
	logger *slog.Logger,
) *Connection {
	return &Connection{
		ID:           id,
		PrincipalID:  principalID,
		Name:         name,
		Capabilities: capabilities,
		Workspaces:   workspaces,
		WorkingDir:   workingDir,
		InstanceID:   instanceID,
		// ... rest
	}
}
```

**Step 4: Update ListAgents to return full info**

```go
func (m *Manager) ListAgents() []*AgentInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	agents := make([]*AgentInfo, 0, len(m.agents))
	for _, agent := range m.agents {
		agents = append(agents, &AgentInfo{
			ID:           agent.ID,
			PrincipalID:  agent.PrincipalID,
			Name:         agent.Name,
			Capabilities: agent.Capabilities,
			Workspaces:   agent.Workspaces,
			WorkingDir:   agent.WorkingDir,
			InstanceID:   agent.InstanceID,
		})
	}
	return agents
}
```

**Step 5: Run all gateway tests**

Run:
```bash
go test ./internal/gateway/... ./internal/agent/... -v
```

**Step 6: Commit**

```bash
git add internal/agent/connection.go internal/agent/manager.go internal/gateway/grpc.go
git commit -m "feat: store agent metadata (workspaces, working_dir) on connection"
```

---

## Task 7: Update fold-agent Config to Support Workspaces

**Files:**
- Modify: `../fold-agent/crates/fold-agent/src/wizard.rs`
- Modify: `../fold-agent/crates/fold-agent/src/main.rs`

**Step 1: Add workspaces to saved config**

Edit `wizard.rs`, update `WizardApp` struct:

```rust
struct WizardApp {
    step: WizardStep,
    name: String,
    backend: Backend,
    server_host: String,
    server_port: String,
    server_field_focus: usize,
    workspaces: String,  // NEW: comma-separated workspaces
    error_message: Option<String>,
    should_quit: bool,
    saved_path: Option<String>,
    set_as_default: bool,
}
```

Update `save_config`:

```rust
fn save_config(&mut self) -> Result<()> {
    // ... existing code ...

    let workspaces_toml = if self.workspaces.trim().is_empty() {
        "workspaces = []".to_string()
    } else {
        let ws: Vec<&str> = self.workspaces.split(',').map(|s| s.trim()).collect();
        format!("workspaces = {:?}", ws)
    };

    let config_content = format!(
        r#"# Agent: {}
# Created: {}

name = "{}"
server = "{}"
backend = "{}"
{}

# Working directory: uses current directory when agent is launched
"#,
        self.name.trim(),
        date,
        self.name.trim(),
        self.server_url(),
        self.backend.as_str(),
        workspaces_toml
    );

    // ... rest of save logic
}
```

**Step 2: Add workspaces step to wizard (optional - can also just edit config manually)**

This is optional for v1 - users can edit the config file directly.

**Step 3: Load workspaces in main.rs**

The config loading already uses a generic TOML table, so workspaces will be parsed if present. Update the metadata gathering to include workspaces:

Edit `main.rs` around line 315:

```rust
// Load workspaces from config
let workspaces: Vec<String> = config
    .get("workspaces")
    .and_then(|v| v.as_array())
    .map(|arr| {
        arr.iter()
            .filter_map(|v| v.as_str().map(String::from))
            .collect()
    })
    .unwrap_or_default();

// Gather metadata with workspaces
let mut metadata = metadata::AgentMetadata::gather(&working_dir);
metadata.workspaces = workspaces;
```

**Step 4: Commit**

```bash
cd ../fold-agent
git add crates/fold-agent/src/wizard.rs crates/fold-agent/src/main.rs
git commit -m "feat(agent): add workspaces config support"
```

---

## Task 8: Update fold-agent to Send Workspaces in Metadata

**Files:**
- Modify: `../fold-agent/crates/fold-agent/src/metadata.rs`
- Modify: `../fold-agent/crates/fold-agent/src/client.rs`

**Step 1: Add workspaces to AgentMetadata struct**

Edit `metadata.rs`:

```rust
#[derive(Debug, Clone)]
pub struct AgentMetadata {
    pub working_directory: String,
    pub hostname: String,
    pub os: String,
    pub git: Option<GitInfo>,
    pub workspaces: Vec<String>,  // NEW
}

impl AgentMetadata {
    pub fn gather(working_dir: &std::path::Path) -> Self {
        Self {
            working_directory: working_dir.to_string_lossy().to_string(),
            hostname: hostname::get()
                .map(|h| h.to_string_lossy().to_string())
                .unwrap_or_default(),
            os: std::env::consts::OS.to_string(),
            git: GitInfo::gather(working_dir),
            workspaces: Vec::new(),  // Set by caller from config
        }
    }
}
```

**Step 2: Update proto conversion in client.rs**

The `impl From<AgentMetadata> for proto::AgentMetadata` needs to include workspaces:

```rust
impl From<crate::metadata::AgentMetadata> for proto::AgentMetadata {
    fn from(m: crate::metadata::AgentMetadata) -> Self {
        proto::AgentMetadata {
            working_directory: m.working_directory,
            hostname: m.hostname,
            os: m.os,
            git: m.git.map(|g| g.into()),
            workspaces: m.workspaces,  // NEW
        }
    }
}
```

**Step 3: Regenerate Rust proto files**

Run:
```bash
cd ../fold-agent
cargo build
```

Expected: Proto files regenerated, build succeeds.

**Step 4: Commit**

```bash
git add crates/fold-agent/src/metadata.rs crates/fold-agent/src/client.rs
git commit -m "feat(agent): send workspaces in registration metadata"
```

---

## Task 9: Display Instance ID in Agent TUI

**Files:**
- Modify: `../fold-agent/crates/fold-agent/src/tui.rs`
- Modify: `../fold-agent/crates/fold-agent/src/client.rs`

**Step 1: Capture instance_id from Welcome message**

In `client.rs`, the Welcome handling already captures `agent_id`. Add `instance_id`:

```rust
Some(server_message::Payload::Welcome(welcome)) => {
    eprintln!(
        "✓ Connected to gateway '{}' as agent '{}'",
        welcome.server_id, welcome.agent_id
    );
    eprintln!("  Instance ID: {}", welcome.instance_id);
    eprintln!("\nTo bind a channel to this agent:");
    eprintln!("  Slack:  /fold bind {}", welcome.instance_id);
    eprintln!("  Matrix: !fold bind {}", welcome.instance_id);
    eprintln!("\nReady and waiting for messages...");
    break (tx, inbound, welcome.agent_id, welcome.instance_id);
}
```

**Step 2: Update TUI to show instance ID**

Edit `tui.rs` to display the instance ID prominently with bind commands.

**Step 3: Commit**

```bash
git add crates/fold-agent/src/tui.rs crates/fold-agent/src/client.rs
git commit -m "feat(agent): display instance ID and bind commands in TUI"
```

---

## Task 10: Integration Test

**Files:**
- Test manually with running gateway and agent

**Step 1: Start gateway**

```bash
cd /Users/harper/Public/src/2389/fold-project/fold-gateway
./bin/fold-gateway serve
```

**Step 2: Create agent config with workspaces**

```bash
mkdir -p ~/.config/fold/agents
cat > ~/.config/fold/agents/test.toml << 'EOF'
name = "test-agent"
server = "http://127.0.0.1:50051"
backend = "cli"
workspaces = ["Code", "Test"]
EOF
```

**Step 3: Start agent**

```bash
cd /tmp/test-project
../fold-agent/target/release/fold-agent --config ~/.config/fold/agents/test.toml
```

**Step 4: Verify**

- Agent should show instance ID and bind commands
- Gateway logs should show principal auto-created
- `curl http://localhost:8080/api/agents` should return agent with workspaces
- `curl http://localhost:8080/api/agents?workspace=Code` should filter correctly

**Step 5: Commit any fixes**

If any issues found, fix and commit.

---

## Summary

| Task | Description | Commits |
|------|-------------|---------|
| 1 | Add workspaces to proto | 1 |
| 2 | Add auto-registration config | 1 |
| 3 | Implement principal auto-creation | 1 |
| 4 | Update grpc.go for authenticated principal | 1 |
| 5 | Update agent list API | 1 |
| 6 | Store metadata on connection | 1 |
| 7 | Agent config for workspaces | 1 |
| 8 | Agent sends workspaces in metadata | 1 |
| 9 | Agent TUI shows instance ID | 1 |
| 10 | Integration test | 0-1 |

Total: ~9-10 commits
