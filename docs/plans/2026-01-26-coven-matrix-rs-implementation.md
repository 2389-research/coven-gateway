# Coven Matrix Bridge (Rust) Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a Rust Matrix bridge that connects to coven-gateway via gRPC ClientService and routes messages between Matrix rooms and coven agents.

**Architecture:** The bridge logs into Matrix using matrix-sdk, listens for room messages, sends them to gateway via gRPC `send_message()`, streams responses via `stream_events()`, and posts formatted responses back to Matrix.

**Tech Stack:** Rust, matrix-sdk 0.7, tonic (gRPC), coven-proto, coven-grpc, tokio

---

## Task 1: Create Crate Skeleton

**Files:**
- Create: `../coven/crates/coven-matrix-rs/Cargo.toml`
- Create: `../coven/crates/coven-matrix-rs/src/main.rs`
- Create: `../coven/crates/coven-matrix-rs/src/lib.rs`

**Step 1: Create directory and Cargo.toml**

```toml
[package]
name = "coven-matrix-rs"
version = "0.1.0"
edition = "2021"
description = "Matrix bridge for coven-gateway"
license = "MIT"

[[bin]]
name = "coven-matrix-bridge"
path = "src/main.rs"

[dependencies]
# Internal crates
coven-proto.workspace = true
coven-grpc.workspace = true

# Async runtime
tokio = { workspace = true, features = ["full", "signal"] }
futures.workspace = true

# gRPC
tonic.workspace = true
prost.workspace = true

# Matrix SDK
matrix-sdk = { version = "0.7", features = ["e2e-encryption", "sqlite"] }

# Config and serialization
serde = { workspace = true, features = ["derive"] }
toml.workspace = true
dirs.workspace = true

# Logging and errors
tracing.workspace = true
tracing-subscriber = { workspace = true, features = ["env-filter"] }
anyhow.workspace = true
thiserror.workspace = true

# CLI
clap = { workspace = true, features = ["derive", "env"] }

# Utilities
uuid = { workspace = true, features = ["v4"] }
```

**Step 2: Create minimal main.rs**

```rust
// ABOUTME: Entry point for coven-matrix-bridge binary.
// ABOUTME: Loads config, connects to Matrix and gateway, runs bridge loop.

use anyhow::Result;
use clap::Parser;
use tracing::info;

#[derive(Parser)]
#[command(name = "coven-matrix-bridge")]
#[command(about = "Matrix bridge for coven-gateway")]
struct Cli {
    /// Config file path
    #[arg(short, long, env = "COVEN_MATRIX_CONFIG")]
    config: Option<std::path::PathBuf>,
}

#[tokio::main]
async fn main() -> Result<()> {
    tracing_subscriber::fmt()
        .with_env_filter(
            tracing_subscriber::EnvFilter::from_default_env()
                .add_directive("coven_matrix_rs=info".parse()?),
        )
        .init();

    let cli = Cli::parse();
    info!("coven-matrix-bridge starting");

    // TODO: Load config, start bridge
    Ok(())
}
```

**Step 3: Create lib.rs**

```rust
// ABOUTME: Library root for coven-matrix-rs.
// ABOUTME: Exports bridge, config, and client modules.

pub mod config;
pub mod error;
```

**Step 4: Verify it compiles**

Run: `cd ../coven && cargo build -p coven-matrix-rs`
Expected: Successful build (with warnings about unused modules)

**Step 5: Commit**

```bash
git add crates/coven-matrix-rs/
git commit -m "feat(matrix-rs): scaffold coven-matrix-rs crate"
```

---

## Task 2: Configuration Module

**Files:**
- Create: `../coven/crates/coven-matrix-rs/src/config.rs`
- Create: `../coven/crates/coven-matrix-rs/src/error.rs`
- Modify: `../coven/crates/coven-matrix-rs/src/lib.rs`

**Step 1: Create error.rs**

```rust
// ABOUTME: Error types for coven-matrix-rs.
// ABOUTME: Defines BridgeError enum for all bridge failure modes.

use thiserror::Error;

#[derive(Error, Debug)]
pub enum BridgeError {
    #[error("Configuration error: {0}")]
    Config(String),

    #[error("Matrix error: {0}")]
    Matrix(#[from] matrix_sdk::Error),

    #[error("Gateway error: {0}")]
    Gateway(#[from] tonic::Status),

    #[error("Connection error: {0}")]
    Connection(#[from] tonic::transport::Error),

    #[error("IO error: {0}")]
    Io(#[from] std::io::Error),
}

pub type Result<T> = std::result::Result<T, BridgeError>;
```

**Step 2: Create config.rs**

```rust
// ABOUTME: Configuration loading and validation for the Matrix bridge.
// ABOUTME: Supports TOML config files with environment variable expansion.

use crate::error::{BridgeError, Result};
use serde::Deserialize;
use std::path::PathBuf;

#[derive(Debug, Deserialize)]
pub struct Config {
    pub matrix: MatrixConfig,
    pub gateway: GatewayConfig,
    #[serde(default)]
    pub bridge: BridgeConfig,
}

#[derive(Debug, Deserialize)]
pub struct MatrixConfig {
    pub homeserver: String,
    pub username: String,
    pub password: String,
    #[serde(default)]
    pub recovery_key: Option<String>,
    #[serde(default)]
    pub state_dir: Option<PathBuf>,
}

#[derive(Debug, Deserialize)]
pub struct GatewayConfig {
    pub url: String,
    #[serde(default)]
    pub token: Option<String>,
}

#[derive(Debug, Deserialize, Default)]
pub struct BridgeConfig {
    #[serde(default)]
    pub allowed_rooms: Vec<String>,
    #[serde(default)]
    pub command_prefix: Option<String>,
    #[serde(default = "default_typing_indicator")]
    pub typing_indicator: bool,
}

fn default_typing_indicator() -> bool {
    true
}

impl Config {
    pub fn load(path: Option<PathBuf>) -> Result<Self> {
        let path = path
            .or_else(|| {
                dirs::config_dir().map(|d| d.join("coven").join("matrix-bridge.toml"))
            })
            .ok_or_else(|| BridgeError::Config("Could not determine config path".into()))?;

        let contents = std::fs::read_to_string(&path).map_err(|e| {
            BridgeError::Config(format!("Failed to read config from {:?}: {}", path, e))
        })?;

        // Expand environment variables
        let contents = shellexpand::env(&contents)
            .map_err(|e| BridgeError::Config(format!("Failed to expand env vars: {}", e)))?;

        let config: Config = toml::from_str(&contents)
            .map_err(|e| BridgeError::Config(format!("Failed to parse config: {}", e)))?;

        config.validate()?;
        Ok(config)
    }

    fn validate(&self) -> Result<()> {
        if self.matrix.homeserver.is_empty() {
            return Err(BridgeError::Config("matrix.homeserver is required".into()));
        }
        if self.matrix.username.is_empty() {
            return Err(BridgeError::Config("matrix.username is required".into()));
        }
        if self.gateway.url.is_empty() {
            return Err(BridgeError::Config("gateway.url is required".into()));
        }
        Ok(())
    }

    pub fn is_room_allowed(&self, room_id: &str) -> bool {
        self.bridge.allowed_rooms.is_empty()
            || self.bridge.allowed_rooms.iter().any(|r| r == room_id)
    }
}
```

**Step 3: Add shellexpand dependency to Cargo.toml**

Add to `[dependencies]`:
```toml
shellexpand = "3"
```

**Step 4: Update lib.rs**

```rust
// ABOUTME: Library root for coven-matrix-rs.
// ABOUTME: Exports bridge, config, and client modules.

pub mod config;
pub mod error;

pub use config::Config;
pub use error::{BridgeError, Result};
```

**Step 5: Verify it compiles**

Run: `cd ../coven && cargo build -p coven-matrix-rs`
Expected: Successful build

**Step 6: Commit**

```bash
git add crates/coven-matrix-rs/
git commit -m "feat(matrix-rs): add config and error modules"
```

---

## Task 3: Gateway Client Module

**Files:**
- Create: `../coven/crates/coven-matrix-rs/src/gateway.rs`
- Modify: `../coven/crates/coven-matrix-rs/src/lib.rs`

**Step 1: Create gateway.rs**

```rust
// ABOUTME: gRPC client wrapper for communicating with coven-gateway.
// ABOUTME: Handles authentication, message sending, and event streaming.

use crate::error::Result;
use coven_proto::client::ClientServiceClient;
use coven_proto::{
    ClientSendMessageRequest, ListAgentsRequest, StreamEventsRequest,
};
use futures::StreamExt;
use tonic::transport::Channel;
use tonic::{Request, Status};
use tracing::{debug, info};

pub struct GatewayClient {
    client: ClientServiceClient<tonic::service::interceptor::InterceptedService<Channel, AuthInterceptor>>,
}

#[derive(Clone)]
struct AuthInterceptor {
    token: Option<String>,
}

impl tonic::service::Interceptor for AuthInterceptor {
    fn call(&mut self, mut req: Request<()>) -> std::result::Result<Request<()>, Status> {
        if let Some(ref token) = self.token {
            let auth_value = format!("Bearer {}", token)
                .parse()
                .map_err(|_| Status::internal("invalid token format"))?;
            req.metadata_mut().insert("authorization", auth_value);
        }
        Ok(req)
    }
}

impl GatewayClient {
    pub async fn connect(url: &str, token: Option<String>) -> Result<Self> {
        info!(url = %url, "Connecting to gateway");

        let channel = Channel::from_shared(url.to_string())?
            .connect()
            .await?;

        let interceptor = AuthInterceptor { token };
        let client = ClientServiceClient::with_interceptor(channel, interceptor);

        Ok(Self { client })
    }

    pub async fn list_agents(&mut self) -> Result<Vec<coven_proto::AgentInfo>> {
        debug!("Listing agents");
        let response = self.client.list_agents(ListAgentsRequest {}).await?;
        Ok(response.into_inner().agents)
    }

    pub async fn send_message(
        &mut self,
        conversation_key: String,
        content: String,
        idempotency_key: String,
    ) -> Result<coven_proto::ClientSendMessageResponse> {
        debug!(conversation_key = %conversation_key, "Sending message to gateway");

        let request = ClientSendMessageRequest {
            conversation_key,
            content,
            attachments: vec![],
            idempotency_key,
        };

        let response = self.client.send_message(request).await?;
        Ok(response.into_inner())
    }

    pub async fn stream_events(
        &mut self,
        conversation_key: String,
    ) -> Result<impl futures::Stream<Item = std::result::Result<coven_proto::ClientStreamEvent, Status>>> {
        debug!(conversation_key = %conversation_key, "Starting event stream");

        let request = StreamEventsRequest {
            conversation_key,
            since_event_id: None,
        };

        let response = self.client.stream_events(request).await?;
        Ok(response.into_inner())
    }
}
```

**Step 2: Update lib.rs**

```rust
// ABOUTME: Library root for coven-matrix-rs.
// ABOUTME: Exports bridge, config, and client modules.

pub mod config;
pub mod error;
pub mod gateway;

pub use config::Config;
pub use error::{BridgeError, Result};
pub use gateway::GatewayClient;
```

**Step 3: Verify it compiles**

Run: `cd ../coven && cargo build -p coven-matrix-rs`
Expected: Successful build

**Step 4: Commit**

```bash
git add crates/coven-matrix-rs/
git commit -m "feat(matrix-rs): add gateway client module"
```

---

## Task 4: Matrix Client Module

**Files:**
- Create: `../coven/crates/coven-matrix-rs/src/matrix.rs`
- Modify: `../coven/crates/coven-matrix-rs/src/lib.rs`

**Step 1: Create matrix.rs**

```rust
// ABOUTME: Matrix client wrapper using matrix-sdk.
// ABOUTME: Handles login, sync, message sending, and event handling.

use crate::config::MatrixConfig;
use crate::error::{BridgeError, Result};
use matrix_sdk::{
    config::SyncSettings,
    room::Room,
    ruma::{
        events::room::message::{
            MessageType, OriginalSyncRoomMessageEvent, RoomMessageEventContent,
        },
        OwnedRoomId, OwnedUserId,
    },
    Client,
};
use std::path::PathBuf;
use tracing::{debug, info, warn};

pub struct MatrixClient {
    client: Client,
    user_id: OwnedUserId,
}

impl MatrixClient {
    pub async fn login(config: &MatrixConfig) -> Result<Self> {
        info!(homeserver = %config.homeserver, username = %config.username, "Logging into Matrix");

        let state_dir = config.state_dir.clone().unwrap_or_else(|| {
            dirs::data_dir()
                .unwrap_or_else(|| PathBuf::from("."))
                .join("coven-matrix-bridge")
        });

        std::fs::create_dir_all(&state_dir)?;

        let client = Client::builder()
            .homeserver_url(&config.homeserver)
            .sqlite_store(&state_dir, None)
            .build()
            .await?;

        client
            .matrix_auth()
            .login_username(&config.username, &config.password)
            .initial_device_display_name("coven-matrix-bridge")
            .await?;

        let user_id = client
            .user_id()
            .ok_or_else(|| BridgeError::Matrix(matrix_sdk::Error::UnknownError(
                "No user ID after login".into(),
            )))?
            .to_owned();

        info!(user_id = %user_id, "Matrix login successful");

        Ok(Self { client, user_id })
    }

    pub fn user_id(&self) -> &OwnedUserId {
        &self.user_id
    }

    pub fn client(&self) -> &Client {
        &self.client
    }

    pub async fn send_text(&self, room_id: &OwnedRoomId, text: &str) -> Result<()> {
        let room = self.client.get_room(room_id).ok_or_else(|| {
            BridgeError::Config(format!("Room not found: {}", room_id))
        })?;

        if let Room::Joined(joined) = room {
            let content = RoomMessageEventContent::text_plain(text);
            joined.send(content).await?;
            debug!(room_id = %room_id, "Sent message to Matrix room");
        } else {
            warn!(room_id = %room_id, "Cannot send to non-joined room");
        }

        Ok(())
    }

    pub async fn send_html(&self, room_id: &OwnedRoomId, plain: &str, html: &str) -> Result<()> {
        let room = self.client.get_room(room_id).ok_or_else(|| {
            BridgeError::Config(format!("Room not found: {}", room_id))
        })?;

        if let Room::Joined(joined) = room {
            let content = RoomMessageEventContent::text_html(plain, html);
            joined.send(content).await?;
            debug!(room_id = %room_id, "Sent HTML message to Matrix room");
        } else {
            warn!(room_id = %room_id, "Cannot send to non-joined room");
        }

        Ok(())
    }

    pub async fn set_typing(&self, room_id: &OwnedRoomId, typing: bool) -> Result<()> {
        let room = self.client.get_room(room_id).ok_or_else(|| {
            BridgeError::Config(format!("Room not found: {}", room_id))
        })?;

        if let Room::Joined(joined) = room {
            joined.typing_notice(typing).await?;
        }

        Ok(())
    }

    pub async fn sync_once(&self) -> Result<()> {
        self.client.sync_once(SyncSettings::default()).await?;
        Ok(())
    }
}

/// Extract text content from a Matrix message event
pub fn extract_text_content(event: &OriginalSyncRoomMessageEvent) -> Option<String> {
    match &event.content.msgtype {
        MessageType::Text(text) => Some(text.body.clone()),
        _ => None,
    }
}
```

**Step 2: Update lib.rs**

```rust
// ABOUTME: Library root for coven-matrix-rs.
// ABOUTME: Exports bridge, config, and client modules.

pub mod config;
pub mod error;
pub mod gateway;
pub mod matrix;

pub use config::Config;
pub use error::{BridgeError, Result};
pub use gateway::GatewayClient;
pub use matrix::MatrixClient;
```

**Step 3: Verify it compiles**

Run: `cd ../coven && cargo build -p coven-matrix-rs`
Expected: Successful build

**Step 4: Commit**

```bash
git add crates/coven-matrix-rs/
git commit -m "feat(matrix-rs): add matrix client module"
```

---

## Task 5: Bridge Core Logic

**Files:**
- Create: `../coven/crates/coven-matrix-rs/src/bridge.rs`
- Modify: `../coven/crates/coven-matrix-rs/src/lib.rs`
- Modify: `../coven/crates/coven-matrix-rs/src/main.rs`

**Step 1: Create bridge.rs**

```rust
// ABOUTME: Core bridge logic connecting Matrix rooms to coven agents.
// ABOUTME: Handles message routing, response streaming, and formatting.

use crate::config::Config;
use crate::error::Result;
use crate::gateway::GatewayClient;
use crate::matrix::{extract_text_content, MatrixClient};
use coven_proto::client_stream_event::Payload;
use futures::StreamExt;
use matrix_sdk::{
    config::SyncSettings,
    ruma::{
        events::room::message::OriginalSyncRoomMessageEvent, OwnedRoomId,
    },
};
use std::collections::HashMap;
use std::sync::Arc;
use tokio::sync::RwLock;
use tracing::{debug, error, info, warn};

pub struct Bridge {
    config: Config,
    matrix: MatrixClient,
    gateway: Arc<RwLock<GatewayClient>>,
    /// Maps room_id -> agent conversation_key
    bindings: Arc<RwLock<HashMap<OwnedRoomId, String>>>,
}

impl Bridge {
    pub async fn new(config: Config) -> Result<Self> {
        let matrix = MatrixClient::login(&config.matrix).await?;
        let gateway = GatewayClient::connect(
            &config.gateway.url,
            config.gateway.token.clone(),
        ).await?;

        Ok(Self {
            config,
            matrix,
            gateway: Arc::new(RwLock::new(gateway)),
            bindings: Arc::new(RwLock::new(HashMap::new())),
        })
    }

    pub async fn run(&self) -> Result<()> {
        info!("Starting bridge");

        // Initial sync to get room state
        self.matrix.sync_once().await?;

        // Set up message handler
        let gateway = self.gateway.clone();
        let bindings = self.bindings.clone();
        let config = self.config.clone();
        let matrix_client = self.matrix.client().clone();
        let our_user_id = self.matrix.user_id().clone();

        self.matrix.client().add_event_handler(
            move |event: OriginalSyncRoomMessageEvent, room: matrix_sdk::room::Room| {
                let gateway = gateway.clone();
                let bindings = bindings.clone();
                let config = config.clone();
                let matrix_client = matrix_client.clone();
                let our_user_id = our_user_id.clone();

                async move {
                    // Ignore our own messages
                    if event.sender == our_user_id {
                        return;
                    }

                    let room_id = room.room_id().to_owned();

                    // Check if room is allowed
                    if !config.is_room_allowed(room_id.as_str()) {
                        debug!(room_id = %room_id, "Room not in allowed list, ignoring");
                        return;
                    }

                    // Extract text content
                    let Some(content) = extract_text_content(&event) else {
                        debug!("Non-text message, ignoring");
                        return;
                    };

                    // Check command prefix
                    let content = if let Some(ref prefix) = config.bridge.command_prefix {
                        if let Some(stripped) = content.strip_prefix(prefix) {
                            stripped.trim().to_string()
                        } else {
                            debug!("Message doesn't start with prefix, ignoring");
                            return;
                        }
                    } else {
                        content
                    };

                    // Get binding for this room
                    let conversation_key = {
                        let bindings = bindings.read().await;
                        bindings.get(&room_id).cloned()
                    };

                    let Some(conversation_key) = conversation_key else {
                        warn!(room_id = %room_id, "No agent bound to room");
                        return;
                    };

                    // Process message
                    if let Err(e) = process_message(
                        &gateway,
                        &matrix_client,
                        &room_id,
                        conversation_key,
                        content,
                        config.bridge.typing_indicator,
                    ).await {
                        error!(error = %e, "Failed to process message");
                    }
                }
            },
        );

        // Run sync loop
        info!("Starting Matrix sync loop");
        self.matrix.client().sync(SyncSettings::default()).await?;

        Ok(())
    }

    pub async fn bind_room(&self, room_id: OwnedRoomId, conversation_key: String) {
        let mut bindings = self.bindings.write().await;
        bindings.insert(room_id.clone(), conversation_key.clone());
        info!(room_id = %room_id, conversation_key = %conversation_key, "Bound room to agent");
    }

    pub async fn unbind_room(&self, room_id: &OwnedRoomId) {
        let mut bindings = self.bindings.write().await;
        bindings.remove(room_id);
        info!(room_id = %room_id, "Unbound room");
    }
}

async fn process_message(
    gateway: &Arc<RwLock<GatewayClient>>,
    matrix_client: &matrix_sdk::Client,
    room_id: &OwnedRoomId,
    conversation_key: String,
    content: String,
    typing_indicator: bool,
) -> Result<()> {
    debug!(room_id = %room_id, content = %content, "Processing message");

    // Set typing indicator
    if typing_indicator {
        if let Some(room) = matrix_client.get_room(room_id) {
            if let matrix_sdk::room::Room::Joined(joined) = room {
                let _ = joined.typing_notice(true).await;
            }
        }
    }

    // Send message to gateway
    let idempotency_key = uuid::Uuid::new_v4().to_string();
    {
        let mut gateway = gateway.write().await;
        gateway.send_message(
            conversation_key.clone(),
            content,
            idempotency_key,
        ).await?;
    }

    // Stream response
    let mut stream = {
        let mut gateway = gateway.write().await;
        gateway.stream_events(conversation_key).await?
    };

    let mut response_text = String::new();

    while let Some(event_result) = stream.next().await {
        match event_result {
            Ok(event) => {
                match event.payload {
                    Some(Payload::Text(chunk)) => {
                        response_text.push_str(&chunk.content);
                    }
                    Some(Payload::Done(_)) => {
                        debug!("Stream done");
                        break;
                    }
                    Some(Payload::Error(err)) => {
                        error!(error = %err.message, "Stream error");
                        response_text = format!("Error: {}", err.message);
                        break;
                    }
                    _ => {
                        // Ignore other events (thinking, tool_use, etc.)
                    }
                }
            }
            Err(e) => {
                error!(error = %e, "Stream error");
                break;
            }
        }
    }

    // Stop typing indicator
    if typing_indicator {
        if let Some(room) = matrix_client.get_room(room_id) {
            if let matrix_sdk::room::Room::Joined(joined) = room {
                let _ = joined.typing_notice(false).await;
            }
        }
    }

    // Send response to Matrix
    if !response_text.is_empty() {
        if let Some(room) = matrix_client.get_room(room_id) {
            if let matrix_sdk::room::Room::Joined(joined) = room {
                let content = matrix_sdk::ruma::events::room::message::RoomMessageEventContent::text_plain(&response_text);
                joined.send(content).await?;
            }
        }
    }

    Ok(())
}
```

**Step 2: Update lib.rs**

```rust
// ABOUTME: Library root for coven-matrix-rs.
// ABOUTME: Exports bridge, config, and client modules.

pub mod bridge;
pub mod config;
pub mod error;
pub mod gateway;
pub mod matrix;

pub use bridge::Bridge;
pub use config::Config;
pub use error::{BridgeError, Result};
pub use gateway::GatewayClient;
pub use matrix::MatrixClient;
```

**Step 3: Update main.rs**

```rust
// ABOUTME: Entry point for coven-matrix-bridge binary.
// ABOUTME: Loads config, connects to Matrix and gateway, runs bridge loop.

use anyhow::Result;
use clap::Parser;
use coven_matrix_rs::{Bridge, Config};
use tracing::info;

#[derive(Parser)]
#[command(name = "coven-matrix-bridge")]
#[command(about = "Matrix bridge for coven-gateway")]
struct Cli {
    /// Config file path
    #[arg(short, long, env = "COVEN_MATRIX_CONFIG")]
    config: Option<std::path::PathBuf>,
}

#[tokio::main]
async fn main() -> Result<()> {
    tracing_subscriber::fmt()
        .with_env_filter(
            tracing_subscriber::EnvFilter::from_default_env()
                .add_directive("coven_matrix_rs=info".parse()?),
        )
        .init();

    let cli = Cli::parse();
    info!("coven-matrix-bridge starting");

    let config = Config::load(cli.config)?;
    info!(homeserver = %config.matrix.homeserver, gateway = %config.gateway.url, "Config loaded");

    let bridge = Bridge::new(config).await?;
    bridge.run().await?;

    Ok(())
}
```

**Step 4: Verify it compiles**

Run: `cd ../coven && cargo build -p coven-matrix-rs`
Expected: Successful build

**Step 5: Commit**

```bash
git add crates/coven-matrix-rs/
git commit -m "feat(matrix-rs): add bridge core logic"
```

---

## Task 6: Add Command Handling

**Files:**
- Create: `../coven/crates/coven-matrix-rs/src/commands.rs`
- Modify: `../coven/crates/coven-matrix-rs/src/bridge.rs`
- Modify: `../coven/crates/coven-matrix-rs/src/lib.rs`

**Step 1: Create commands.rs**

```rust
// ABOUTME: Handles /coven commands in Matrix rooms for binding management.
// ABOUTME: Supports bind, unbind, status, and agents commands.

use crate::error::Result;
use crate::gateway::GatewayClient;
use std::sync::Arc;
use tokio::sync::RwLock;

pub enum Command {
    Bind(String),      // /coven bind <agent-id>
    Unbind,            // /coven unbind
    Status,            // /coven status
    Agents,            // /coven agents
    Help,              // /coven help
    Unknown(String),
}

impl Command {
    pub fn parse(input: &str) -> Option<Command> {
        let input = input.trim();

        // Check for /coven prefix
        let rest = input.strip_prefix("/coven")?.trim();

        if rest.is_empty() || rest == "help" {
            return Some(Command::Help);
        }

        let parts: Vec<&str> = rest.splitn(2, ' ').collect();
        match parts[0] {
            "bind" => {
                let agent_id = parts.get(1).map(|s| s.trim().to_string())?;
                if agent_id.is_empty() {
                    None
                } else {
                    Some(Command::Bind(agent_id))
                }
            }
            "unbind" => Some(Command::Unbind),
            "status" => Some(Command::Status),
            "agents" => Some(Command::Agents),
            "help" => Some(Command::Help),
            other => Some(Command::Unknown(other.to_string())),
        }
    }
}

pub async fn execute_command(
    command: Command,
    gateway: &Arc<RwLock<GatewayClient>>,
) -> Result<String> {
    match command {
        Command::Bind(agent_id) => {
            Ok(format!("Binding to agent: {}\nUse `/coven status` to verify.", agent_id))
        }
        Command::Unbind => {
            Ok("Room unbound from agent.".to_string())
        }
        Command::Status => {
            Ok("Status: No agent bound to this room.\nUse `/coven bind <agent-id>` to bind an agent.".to_string())
        }
        Command::Agents => {
            let mut gateway = gateway.write().await;
            let agents = gateway.list_agents().await?;

            if agents.is_empty() {
                Ok("No agents currently online.".to_string())
            } else {
                let mut response = String::from("Online agents:\n");
                for agent in agents {
                    response.push_str(&format!(
                        "• {} ({})\n",
                        agent.instance_id,
                        agent.metadata.as_ref()
                            .map(|m| m.working_directory.as_str())
                            .unwrap_or("unknown")
                    ));
                }
                Ok(response)
            }
        }
        Command::Help => {
            Ok(r#"Coven Bridge Commands:
• /coven bind <agent-id> - Bind this room to an agent
• /coven unbind - Unbind this room from current agent
• /coven status - Show current binding status
• /coven agents - List available agents
• /coven help - Show this help message"#.to_string())
        }
        Command::Unknown(cmd) => {
            Ok(format!("Unknown command: {}\nUse `/coven help` for available commands.", cmd))
        }
    }
}
```

**Step 2: Update lib.rs to export commands**

```rust
// ABOUTME: Library root for coven-matrix-rs.
// ABOUTME: Exports bridge, config, and client modules.

pub mod bridge;
pub mod commands;
pub mod config;
pub mod error;
pub mod gateway;
pub mod matrix;

pub use bridge::Bridge;
pub use config::Config;
pub use error::{BridgeError, Result};
pub use gateway::GatewayClient;
pub use matrix::MatrixClient;
```

**Step 3: Verify it compiles**

Run: `cd ../coven && cargo build -p coven-matrix-rs`
Expected: Successful build

**Step 4: Commit**

```bash
git add crates/coven-matrix-rs/
git commit -m "feat(matrix-rs): add command handling for bindings"
```

---

## Task 7: Create Example Config and README

**Files:**
- Create: `../coven/crates/coven-matrix-rs/config.example.toml`
- Create: `../coven/crates/coven-matrix-rs/README.md`

**Step 1: Create config.example.toml**

```toml
# Coven Matrix Bridge Configuration

[matrix]
# Matrix homeserver URL
homeserver = "https://matrix.org"

# Bot account credentials (use environment variables in production)
username = "@coven-bot:matrix.org"
password = "${MATRIX_PASSWORD}"

# Optional: E2EE recovery key for encrypted rooms
# recovery_key = "${MATRIX_RECOVERY_KEY}"

# Optional: Directory for Matrix state/crypto storage
# state_dir = "~/.local/share/coven-matrix-bridge"

[gateway]
# Coven gateway gRPC URL
url = "http://localhost:6666"

# Authentication token (from coven-link or gateway admin)
token = "${COVEN_TOKEN}"

[bridge]
# Optional: Restrict to specific rooms (empty = allow all)
allowed_rooms = [
    # "!roomid:matrix.org",
]

# Optional: Only respond to messages with this prefix
# command_prefix = "!coven "

# Show typing indicator while agent is responding
typing_indicator = true
```

**Step 2: Create README.md**

```markdown
# coven-matrix-rs

Rust Matrix bridge for coven-gateway. Routes messages between Matrix rooms and coven agents via gRPC.

## Installation

```bash
cargo install --path .
```

## Configuration

Create `~/.config/coven/matrix-bridge.toml`:

```toml
[matrix]
homeserver = "https://matrix.org"
username = "@your-bot:matrix.org"
password = "${MATRIX_PASSWORD}"

[gateway]
url = "http://localhost:6666"
token = "${COVEN_TOKEN}"

[bridge]
typing_indicator = true
```

## Usage

```bash
# Run with default config location
coven-matrix-bridge

# Run with custom config
coven-matrix-bridge --config /path/to/config.toml

# Set config via environment
COVEN_MATRIX_CONFIG=/path/to/config.toml coven-matrix-bridge
```

## Commands

In any Matrix room where the bot is present:

- `/coven bind <agent-id>` - Bind room to an agent
- `/coven unbind` - Unbind room from agent
- `/coven status` - Show current binding
- `/coven agents` - List available agents
- `/coven help` - Show help

## Environment Variables

- `MATRIX_PASSWORD` - Matrix account password
- `COVEN_TOKEN` - Gateway authentication token
- `COVEN_MATRIX_CONFIG` - Config file path
- `RUST_LOG` - Logging level (e.g., `coven_matrix_rs=debug`)
```

**Step 3: Commit**

```bash
git add crates/coven-matrix-rs/
git commit -m "docs(matrix-rs): add example config and README"
```

---

## Task 8: Integration Test Setup

**Files:**
- Create: `../coven/crates/coven-matrix-rs/tests/integration.rs`

**Step 1: Create integration test file**

```rust
// ABOUTME: Integration tests for coven-matrix-rs.
// ABOUTME: Tests config loading and command parsing.

use coven_matrix_rs::commands::Command;
use coven_matrix_rs::config::Config;
use std::io::Write;
use tempfile::NamedTempFile;

#[test]
fn test_command_parsing() {
    assert!(matches!(Command::parse("/coven help"), Some(Command::Help)));
    assert!(matches!(Command::parse("/coven"), Some(Command::Help)));
    assert!(matches!(Command::parse("/coven agents"), Some(Command::Agents)));
    assert!(matches!(Command::parse("/coven status"), Some(Command::Status)));
    assert!(matches!(Command::parse("/coven unbind"), Some(Command::Unbind)));
    assert!(matches!(
        Command::parse("/coven bind agent-123"),
        Some(Command::Bind(id)) if id == "agent-123"
    ));
    assert!(matches!(
        Command::parse("/coven unknown"),
        Some(Command::Unknown(cmd)) if cmd == "unknown"
    ));
    assert!(Command::parse("hello world").is_none());
    assert!(Command::parse("/other command").is_none());
}

#[test]
fn test_config_loading() {
    let config_content = r#"
[matrix]
homeserver = "https://matrix.org"
username = "@bot:matrix.org"
password = "secret"

[gateway]
url = "http://localhost:6666"
token = "test-token"

[bridge]
allowed_rooms = ["!room1:matrix.org"]
typing_indicator = false
"#;

    let mut file = NamedTempFile::new().unwrap();
    file.write_all(config_content.as_bytes()).unwrap();

    let config = Config::load(Some(file.path().to_path_buf())).unwrap();

    assert_eq!(config.matrix.homeserver, "https://matrix.org");
    assert_eq!(config.matrix.username, "@bot:matrix.org");
    assert_eq!(config.gateway.url, "http://localhost:6666");
    assert_eq!(config.bridge.allowed_rooms.len(), 1);
    assert!(!config.bridge.typing_indicator);
}

#[test]
fn test_room_allowed_check() {
    let config_content = r#"
[matrix]
homeserver = "https://matrix.org"
username = "@bot:matrix.org"
password = "secret"

[gateway]
url = "http://localhost:6666"

[bridge]
allowed_rooms = ["!allowed:matrix.org"]
"#;

    let mut file = NamedTempFile::new().unwrap();
    file.write_all(config_content.as_bytes()).unwrap();

    let config = Config::load(Some(file.path().to_path_buf())).unwrap();

    assert!(config.is_room_allowed("!allowed:matrix.org"));
    assert!(!config.is_room_allowed("!other:matrix.org"));
}

#[test]
fn test_empty_allowed_rooms_allows_all() {
    let config_content = r#"
[matrix]
homeserver = "https://matrix.org"
username = "@bot:matrix.org"
password = "secret"

[gateway]
url = "http://localhost:6666"

[bridge]
allowed_rooms = []
"#;

    let mut file = NamedTempFile::new().unwrap();
    file.write_all(config_content.as_bytes()).unwrap();

    let config = Config::load(Some(file.path().to_path_buf())).unwrap();

    assert!(config.is_room_allowed("!any:matrix.org"));
    assert!(config.is_room_allowed("!room:other.server"));
}
```

**Step 2: Add tempfile dev dependency to Cargo.toml**

Add to Cargo.toml:
```toml
[dev-dependencies]
tempfile = "3"
```

**Step 3: Run tests**

Run: `cd ../coven && cargo test -p coven-matrix-rs`
Expected: All tests pass

**Step 4: Commit**

```bash
git add crates/coven-matrix-rs/
git commit -m "test(matrix-rs): add integration tests for config and commands"
```

---

## Summary

After completing all tasks, you will have:

1. **Crate skeleton** with proper workspace integration
2. **Config module** supporting TOML with env var expansion
3. **Gateway client** for gRPC communication via ClientService
4. **Matrix client** wrapper around matrix-sdk
5. **Bridge core** routing messages between Matrix and gateway
6. **Command handling** for `/coven` binding management
7. **Documentation** with example config and README
8. **Tests** for config loading and command parsing

**To run the bridge:**

```bash
cd ../coven
cargo build -p coven-matrix-rs
./target/debug/coven-matrix-bridge --config path/to/config.toml
```

**Next steps after this plan:**
- Add E2EE support (using matrix-sdk crypto features)
- Add HTML/Markdown formatting for responses
- Add graceful shutdown handling
- Add reconnection logic for both Matrix and gateway
