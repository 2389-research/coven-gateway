// ABOUTME: Matrix bridge core for fold-matrix
// ABOUTME: Handles Matrix client connection and message routing to gateway

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// Bridge connects Matrix to fold-gateway.
type Bridge struct {
	config  *Config
	matrix  *mautrix.Client
	gateway *GatewayClient
	logger  *slog.Logger

	// Track rooms we're actively processing to avoid duplicate handling
	processing sync.Map

	// ctx is the parent context for message processing goroutines
	ctx    context.Context
	cancel context.CancelFunc
}

// NewBridge creates a new Matrix bridge.
// The client is created but not logged in - call Login() before Run().
func NewBridge(cfg *Config, logger *slog.Logger) (*Bridge, error) {
	// Create Matrix client (not logged in yet)
	client, err := mautrix.NewClient(cfg.Matrix.Homeserver, "", "")
	if err != nil {
		return nil, fmt.Errorf("creating matrix client: %w", err)
	}

	// Create gateway client
	gateway := NewGatewayClient(cfg.Gateway.URL)

	return &Bridge{
		config:  cfg,
		matrix:  client,
		gateway: gateway,
		logger:  logger,
	}, nil
}

// Login authenticates with the Matrix homeserver using username/password.
func (b *Bridge) Login(ctx context.Context) error {
	b.logger.Info("logging in to matrix", "homeserver", b.config.Matrix.Homeserver, "username", b.config.Matrix.Username)

	resp, err := b.matrix.Login(ctx, &mautrix.ReqLogin{
		Type: mautrix.AuthTypePassword,
		Identifier: mautrix.UserIdentifier{
			Type: mautrix.IdentifierTypeUser,
			User: b.config.Matrix.Username,
		},
		Password:                 b.config.Matrix.Password,
		InitialDeviceDisplayName: "fold-matrix",
		StoreCredentials:         true,
	})
	if err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	b.logger.Info("logged in to matrix", "user_id", resp.UserID, "device_id", resp.DeviceID)
	return nil
}

// UserID returns the logged-in user's ID.
func (b *Bridge) UserID() string {
	return b.matrix.UserID.String()
}

// Run starts the bridge and blocks until context is cancelled.
func (b *Bridge) Run(ctx context.Context) error {
	b.logger.Info("starting matrix bridge",
		"homeserver", b.config.Matrix.Homeserver,
		"user_id", b.matrix.UserID,
		"gateway", b.config.Gateway.URL,
	)

	// Store context for message processing goroutines
	b.ctx, b.cancel = context.WithCancel(ctx)
	defer b.cancel()

	// Register event handler for messages
	syncer, ok := b.matrix.Syncer.(*mautrix.DefaultSyncer)
	if !ok {
		return fmt.Errorf("unexpected syncer type: %T", b.matrix.Syncer)
	}
	syncer.OnEventType(event.EventMessage, b.handleMessageEvent)

	// Start syncing
	b.logger.Info("connecting to matrix homeserver")

	syncErr := make(chan error, 1)
	go func() {
		syncErr <- b.matrix.SyncWithContext(b.ctx)
	}()

	b.logger.Info("matrix bridge running")

	// Wait for context cancellation or sync error
	select {
	case <-ctx.Done():
		b.logger.Info("shutting down matrix bridge")
		b.cancel()
		return nil
	case err := <-syncErr:
		return fmt.Errorf("matrix sync failed: %w", err)
	}
}

// handleMessageEvent processes incoming Matrix messages.
func (b *Bridge) handleMessageEvent(ctx context.Context, evt *event.Event) {
	// Ignore our own messages
	if evt.Sender == b.matrix.UserID {
		return
	}

	// Get message content
	content, ok := evt.Content.Parsed.(*event.MessageEventContent)
	if !ok {
		return
	}

	// Only handle text messages
	if content.MsgType != event.MsgText {
		return
	}

	roomID := evt.RoomID.String()
	msgBody := content.Body

	// Check allowed rooms
	if !b.isRoomAllowed(roomID) {
		b.logger.Debug("ignoring message from non-allowed room", "room", roomID)
		return
	}

	// Check command prefix
	if b.config.Bridge.CommandPrefix != "" {
		if !strings.HasPrefix(msgBody, b.config.Bridge.CommandPrefix) {
			return
		}
		msgBody = strings.TrimPrefix(msgBody, b.config.Bridge.CommandPrefix)
		msgBody = strings.TrimSpace(msgBody)
	}

	if msgBody == "" {
		return
	}

	// Check for /fold commands
	if strings.HasPrefix(msgBody, "/fold ") || msgBody == "/fold" {
		go b.handleFoldCommand(b.ctx, evt.RoomID, strings.TrimPrefix(msgBody, "/fold"))
		return
	}

	b.logger.Info("received message",
		"room", roomID,
		"sender", evt.Sender.String(),
		"content", truncate(msgBody, 50),
	)

	// Process message in goroutine to not block sync
	// Use bridge context for graceful shutdown support
	go b.processMessage(b.ctx, evt.RoomID, evt.Sender, msgBody)
}

// processMessage sends the message to gateway and streams response back.
func (b *Bridge) processMessage(ctx context.Context, roomID id.RoomID, sender id.UserID, content string) {
	roomStr := roomID.String()

	// Check if we're already processing a message in this room
	if _, loaded := b.processing.LoadOrStore(roomStr, true); loaded {
		b.logger.Debug("already processing message in room, dropping", "room", roomStr)
		return
	}
	defer b.processing.Delete(roomStr)

	// Send typing indicator
	if b.config.Bridge.TypingIndicator {
		b.setTyping(roomID, true)
		defer b.setTyping(roomID, false)
	}

	// Build gateway request
	req := SendRequest{
		Sender:    sender.String(),
		Content:   content,
		Frontend:  "matrix",
		ChannelID: roomStr,
	}

	// Accumulate response text
	var responseText strings.Builder

	// Send to gateway and handle streaming response
	fullResponse, err := b.gateway.SendMessage(ctx, req, func(evt SSEEvent) {
		switch evt.Type {
		case EventText:
			var data TextEventData
			if err := parseJSON(evt.Data, &data); err == nil {
				responseText.WriteString(data.Text)
			}
		case EventThinking:
			// Optionally show thinking indicator
			b.logger.Debug("agent thinking", "room", roomStr)
		case EventError:
			var data ErrorEventData
			if err := parseJSON(evt.Data, &data); err == nil {
				b.logger.Error("agent error", "room", roomStr, "error", data.Error)
			}
		}
	})

	if err != nil {
		b.logger.Error("gateway request failed", "room", roomStr, "error", err)
		b.sendMessage(roomID, fmt.Sprintf("Error: %v", err))
		return
	}

	// Use full response if available, otherwise accumulated text
	response := fullResponse
	if response == "" {
		response = responseText.String()
	}

	if response == "" {
		b.logger.Warn("empty response from agent", "room", roomStr)
		return
	}

	b.logger.Info("sending response",
		"room", roomStr,
		"length", len(response),
	)

	b.sendMessage(roomID, response)
}

// isRoomAllowed checks if the room is in the allowed list.
func (b *Bridge) isRoomAllowed(roomID string) bool {
	if len(b.config.Bridge.AllowedRooms) == 0 {
		return true // Allow all if no filter
	}

	for _, allowed := range b.config.Bridge.AllowedRooms {
		if allowed == roomID {
			return true
		}
	}
	return false
}

// typingTimeout is the duration the typing indicator shows (30 seconds).
const typingTimeout = 30 * time.Second

// networkTimeout is the timeout for Matrix API calls.
const networkTimeout = 10 * time.Second

// setTyping sends typing indicator to room.
func (b *Bridge) setTyping(roomID id.RoomID, typing bool) {
	var timeout time.Duration
	if typing {
		timeout = typingTimeout
	}
	// Use a timeout context to avoid hanging during shutdown or network issues
	ctx, cancel := context.WithTimeout(context.Background(), networkTimeout)
	defer cancel()
	_, err := b.matrix.UserTyping(ctx, roomID, typing, timeout)
	if err != nil {
		b.logger.Debug("failed to set typing indicator", "room", roomID.String(), "error", err)
	}
}

// sendMessage sends a markdown message to a room, converting to HTML for Matrix.
func (b *Bridge) sendMessage(roomID id.RoomID, text string) {
	// Use a longer timeout for sending messages (they can be large)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Convert markdown to HTML for Matrix formatted messages
	htmlBody := markdownToHTML(text)

	content := &event.MessageEventContent{
		MsgType:       event.MsgText,
		Body:          text, // Plain text fallback
		Format:        event.FormatHTML,
		FormattedBody: htmlBody,
	}

	_, err := b.matrix.SendMessageEvent(ctx, roomID, event.EventMessage, content)
	if err != nil {
		b.logger.Error("failed to send message", "room", roomID.String(), "error", err)
	}
}

// truncate shortens a string to the given max rune count, adding "..." if truncated.
func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

// parseJSON unmarshals JSON from a string into the given value.
func parseJSON(data string, v interface{}) error {
	return json.Unmarshal([]byte(data), v)
}

// markdownToHTML converts markdown text to HTML for Matrix messages.
func markdownToHTML(text string) string {
	var buf bytes.Buffer
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM, // GitHub Flavored Markdown (tables, strikethrough, etc.)
		),
		goldmark.WithRendererOptions(
			html.WithHardWraps(), // Convert newlines to <br>
		),
	)
	if err := md.Convert([]byte(text), &buf); err != nil {
		// On error, return escaped text
		return template.HTMLEscapeString(text)
	}
	return buf.String()
}

// handleFoldCommand processes /fold commands for binding management.
func (b *Bridge) handleFoldCommand(ctx context.Context, roomID id.RoomID, cmd string) {
	cmd = strings.TrimSpace(cmd)
	parts := strings.Fields(cmd)

	if len(parts) == 0 {
		b.sendMessage(roomID, "**Usage:** `/fold <bind|unbind|status|agents>`")
		return
	}

	switch parts[0] {
	case "bind":
		if len(parts) < 2 {
			b.sendMessage(roomID, "**Usage:** `/fold bind <instance-id>`\n\nUse `/fold agents` to list available agents.")
			return
		}
		b.handleBind(ctx, roomID, parts[1])
	case "unbind":
		b.handleUnbind(ctx, roomID)
	case "status":
		b.handleStatus(ctx, roomID)
	case "agents":
		b.handleListAgents(ctx, roomID)
	default:
		b.sendMessage(roomID, fmt.Sprintf("Unknown command: `%s`\n\nAvailable commands: `bind`, `unbind`, `status`, `agents`", parts[0]))
	}
}

// handleBind binds this room to an agent by instance ID.
func (b *Bridge) handleBind(ctx context.Context, roomID id.RoomID, instanceID string) {
	b.logger.Info("bind command", "room", roomID.String(), "instance_id", instanceID)

	resp, err := b.gateway.CreateBinding(ctx, "matrix", roomID.String(), instanceID)
	if err != nil {
		b.logger.Error("failed to create binding", "room", roomID.String(), "error", err)
		b.sendMessage(roomID, fmt.Sprintf("Failed to bind: %v", err))
		return
	}

	var msg string
	if resp.ReboundFrom != nil {
		msg = fmt.Sprintf("Rebound from **%s** to **%s**\n\nWorking directory: `%s`",
			*resp.ReboundFrom, resp.AgentName, resp.WorkingDir)
	} else {
		msg = fmt.Sprintf("Bound to **%s**\n\nWorking directory: `%s`",
			resp.AgentName, resp.WorkingDir)
	}

	b.sendMessage(roomID, msg)
}

// handleUnbind removes the binding from this room.
func (b *Bridge) handleUnbind(ctx context.Context, roomID id.RoomID) {
	b.logger.Info("unbind command", "room", roomID.String())

	err := b.gateway.DeleteBinding(ctx, "matrix", roomID.String())
	if err != nil {
		b.logger.Error("failed to delete binding", "room", roomID.String(), "error", err)
		b.sendMessage(roomID, fmt.Sprintf("Failed to unbind: %v", err))
		return
	}

	b.sendMessage(roomID, "Unbound from agent. Use `/fold bind <instance-id>` to bind to a new agent.")
}

// handleStatus shows the current binding status for this room.
func (b *Bridge) handleStatus(ctx context.Context, roomID id.RoomID) {
	b.logger.Info("status command", "room", roomID.String())

	binding, err := b.gateway.GetBinding(ctx, "matrix", roomID.String())
	if err != nil {
		b.logger.Error("failed to get binding", "room", roomID.String(), "error", err)
		b.sendMessage(roomID, fmt.Sprintf("Failed to get status: %v", err))
		return
	}

	if binding == nil {
		b.sendMessage(roomID, "This room is not bound to any agent.\n\nUse `/fold agents` to list available agents, then `/fold bind <instance-id>` to bind.")
		return
	}

	status := "offline"
	if binding.Online {
		status = "online"
	}

	msg := fmt.Sprintf("**Agent:** %s (%s)\n**Working directory:** `%s`",
		binding.AgentName, status, binding.WorkingDir)

	b.sendMessage(roomID, msg)
}

// handleListAgents lists all online agents.
func (b *Bridge) handleListAgents(ctx context.Context, roomID id.RoomID) {
	b.logger.Info("agents command", "room", roomID.String())

	agents, err := b.gateway.ListAgents(ctx)
	if err != nil {
		b.logger.Error("failed to list agents", "room", roomID.String(), "error", err)
		b.sendMessage(roomID, fmt.Sprintf("Failed to list agents: %v", err))
		return
	}

	if len(agents) == 0 {
		b.sendMessage(roomID, "No agents online.")
		return
	}

	var sb strings.Builder
	sb.WriteString("**Online agents:**\n\n")

	for _, agent := range agents {
		sb.WriteString(fmt.Sprintf("- `%s` - **%s** (`%s`)\n",
			agent.InstanceID, agent.Name, agent.WorkingDir))
	}

	sb.WriteString("\nUse `/fold bind <instance-id>` to bind this room to an agent.")

	b.sendMessage(roomID, sb.String())
}
