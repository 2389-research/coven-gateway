// Package agent manages connections to coven-agent instances.
//
// # Overview
//
// The agent package handles the lifecycle of connected AI agents, including
// registration, message routing, heartbeat monitoring, and graceful disconnection.
//
// # Manager
//
// The Manager tracks all connected agents:
//
//	mgr := agent.NewManager(store, logger)
//
// Key operations:
//
//   - Register(conn): Add a new agent connection
//   - Unregister(agentID): Remove an agent connection
//   - SendMessage(ctx, req): Route a message to an agent
//   - ListAgents(): Get all connected agents
//   - GetAgent(id): Get a specific agent by ID
//
// # Connection
//
// Connection represents a single agent's bidirectional gRPC stream:
//
//	type Connection struct {
//	    ID           string
//	    Name         string
//	    Capabilities []string
//	    stream       pb.CovenControl_AgentStreamServer
//	    pending      map[string]chan *pb.MessageResponse
//	}
//
// Key operations:
//
//   - Send(msg): Send a ServerMessage to the agent
//   - HandleMessage(msg): Process an incoming AgentMessage
//   - Close(): Gracefully close the connection
//
// # Request/Response Correlation
//
// When sending a message to an agent, the manager:
//
//  1. Generates a unique request_id
//  2. Creates a response channel
//  3. Sends the message via the agent's gRPC stream
//  4. Waits for response events on the channel
//  5. Correlates responses by request_id
//
// The Connection maintains a map of pending requests to channels:
//
//	pending map[string]chan *pb.MessageResponse
//
// When a response event arrives, it's routed to the correct channel.
//
// # Heartbeat Monitoring
//
// Agents send periodic heartbeats to indicate they're alive:
//
//	Heartbeat interval: 30s (configurable)
//	Timeout: 90s (configurable)
//
// If an agent misses heartbeats beyond the timeout, it's marked as disconnected.
//
// # Reconnection Grace Period
//
// When an agent disconnects unexpectedly:
//
//  1. Connection is marked as disconnected
//  2. Grace period starts (default: 5 minutes)
//  3. If agent reconnects within grace period, state is restored
//  4. If grace period expires, pending requests are failed
//
// # Agent Metadata
//
// Agents provide metadata during registration:
//
//   - working_directory: Agent's working directory
//   - git_info: Repository name, branch, commit
//   - hostname: Machine hostname
//   - os: Operating system
//   - backend: LLM backend (e.g., "claude")
//   - workspaces: Available workspace paths
//   - protocol_features: Supported protocol capabilities
//
// # Thread Safety
//
// Both Manager and Connection are thread-safe. They use mutexes to protect
// concurrent access to the agent map and pending request channels.
package agent
