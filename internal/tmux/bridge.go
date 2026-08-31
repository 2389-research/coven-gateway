// ABOUTME: Bridge connects a discovered tmux Claude session to coven-gateway via gRPC.
// ABOUTME: Each bridge instance registers one agent, forwards messages, and streams responses.

package tmux

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/2389/coven-gateway/proto/coven"
)

// Bridge connects a single tmux session to the gateway as a coven agent.
type Bridge struct {
	session Session
	gateway string
	logger  *slog.Logger

	paneIO  *PaneIO
	tracker *ResponseTracker

	conn   *grpc.ClientConn
	stream pb.CovenControl_AgentStreamClient

	mu         sync.Mutex
	instanceID string
	running    bool
}

// NewBridge creates a bridge for the given session.
func NewBridge(session Session, gatewayAddr string, logger *slog.Logger) *Bridge {
	if logger == nil {
		logger = slog.Default()
	}
	return &Bridge{
		session: session,
		gateway: gatewayAddr,
		logger: logger.With(
			"agent_id", session.AgentID(),
			"session", session.SessionName,
			"pane", session.PaneID,
		),
	}
}

// Run connects to the gateway, registers as an agent, and enters the message loop.
// Blocks until the context is cancelled or an unrecoverable error occurs.
// Retries registration with backoff if the agent ID is already registered
// (e.g. gateway hasn't detected a previous disconnect yet).
func (b *Bridge) Run(ctx context.Context) error {
	b.mu.Lock()
	if b.running {
		b.mu.Unlock()
		return errors.New("bridge already running")
	}
	b.running = true
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		b.running = false
		b.mu.Unlock()
	}()

	// Set up pane I/O.
	b.paneIO = NewPaneIO(b.session.PaneID, b.logger)

	// Start pipe-pane for real-time output capture.
	if _, err := b.paneIO.StartPipePane(ctx); err != nil {
		return fmt.Errorf("start pipe-pane: %w", err)
	}
	defer func() { _ = b.paneIO.StopPipePane(context.Background()) }() //nolint:contextcheck // cleanup must use Background; parent ctx may be cancelled

	// Connect and register with retry for AlreadyExists.
	if err := b.connectWithRetry(ctx); err != nil {
		return err
	}
	defer b.conn.Close()

	// Enter message loop.
	return b.messageLoop(ctx)
}

// connectWithRetry handles the gRPC connect + register cycle.
// Retries with exponential backoff when the gateway rejects with AlreadyExists
// (previous adapter's agent hasn't been cleaned up yet).
func (b *Bridge) connectWithRetry(ctx context.Context) error {
	backoff := 2 * time.Second
	const maxBackoff = 30 * time.Second
	const maxRetries = 10

	for attempt := range maxRetries {
		err := b.connectAndRegister(ctx)
		if err == nil {
			return nil
		}

		// Only retry on AlreadyExists errors.
		if !strings.Contains(err.Error(), "already registered") {
			return err
		}

		if attempt < maxRetries-1 {
			b.logger.Info("agent already registered, retrying",
				"attempt", attempt+1,
				"backoff", backoff,
			)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
			backoff = min(backoff*2, maxBackoff)
		}
	}
	return fmt.Errorf("registration failed after %d retries", maxRetries)
}

// connectAndRegister opens a gRPC stream and registers. Closes the connection on failure.
func (b *Bridge) connectAndRegister(ctx context.Context) error {
	conn, err := grpc.NewClient(b.gateway, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("grpc dial: %w", err)
	}

	client := pb.NewCovenControlClient(conn)
	stream, err := client.AgentStream(ctx)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("open agent stream: %w", err)
	}

	if err := b.registerOnStream(stream); err != nil {
		_ = conn.Close()
		return err
	}

	b.conn = conn
	b.stream = stream
	return nil
}

// registerOnStream sends RegisterAgent and waits for Welcome on the given stream.
func (b *Bridge) registerOnStream(stream pb.CovenControl_AgentStreamClient) error {
	hostname, _ := hostName()
	reg := &pb.AgentMessage{
		Payload: &pb.AgentMessage_Register{
			Register: &pb.RegisterAgent{
				AgentId:      b.session.AgentID(),
				Name:         b.session.DisplayName(),
				Capabilities: []string{"chat", "code"},
				Metadata: &pb.AgentMetadata{
					WorkingDirectory: b.session.WorkingDir,
					Hostname:         hostname,
					Os:               osName(),
					Backend:          "tmux",
				},
			},
		},
	}

	if err := stream.Send(reg); err != nil {
		return fmt.Errorf("send register: %w", err)
	}

	msg, err := stream.Recv()
	if err != nil {
		return fmt.Errorf("recv welcome: %w", err)
	}

	welcome := msg.GetWelcome()
	if welcome == nil {
		// Check for registration error.
		if regErr := msg.GetRegistrationError(); regErr != nil {
			return fmt.Errorf("registration rejected: %s", regErr.Reason)
		}
		return fmt.Errorf("expected welcome, got: %v", msg)
	}

	b.instanceID = welcome.InstanceId
	b.logger.Info("registered with gateway",
		"instance_id", welcome.InstanceId,
		"agent_id", welcome.AgentId,
	)
	return nil
}

// messageLoop handles incoming messages from the gateway.
func (b *Bridge) messageLoop(ctx context.Context) error {
	// Start heartbeat goroutine.
	heartbeatCtx, heartbeatCancel := context.WithCancel(ctx)
	defer heartbeatCancel()
	go b.heartbeatLoop(heartbeatCtx)

	for {
		msg, err := b.stream.Recv()
		if errors.Is(err, io.EOF) {
			b.logger.Info("gateway closed stream")
			return nil
		}
		if err != nil {
			if ctx.Err() != nil {
				return nil // graceful shutdown
			}
			return fmt.Errorf("recv error: %w", err)
		}

		switch payload := msg.GetPayload().(type) {
		case *pb.ServerMessage_SendMessage:
			go b.handleSendMessage(ctx, payload.SendMessage)

		case *pb.ServerMessage_Shutdown:
			b.logger.Info("gateway requested shutdown", "reason", payload.Shutdown.Reason)
			return nil

		default:
			b.logger.Debug("unhandled server message", "type", fmt.Sprintf("%T", payload))
		}
	}
}

// handleSendMessage forwards a message to the Claude session and streams the response back.
func (b *Bridge) handleSendMessage(ctx context.Context, sm *pb.SendMessage) {
	requestID := sm.RequestId
	content := sm.Content
	logger := b.logger.With("request_id", requestID)

	logger.Info("forwarding message to tmux", "content_len", len(content))

	// Send "thinking" event.
	if err := b.sendThinking(requestID, "Processing..."); err != nil {
		logger.Error("send thinking event", "err", err)
	}

	// Set up response tracker to stream text chunks back.
	// Pass the input text so the tracker can filter the echo.
	tracker := NewResponseTracker(logger, content, func(state ResponseState, text string) {
		if state == StateResponding {
			if err := b.sendText(requestID, text); err != nil {
				logger.Error("send text event", "err", err)
			}
		}
	})
	b.tracker = tracker

	// Capture current pipe offset BEFORE sending input.
	// This ensures we only process output that appears after our input.
	startOffset := b.paneIO.CurrentPipeSize()
	logger.Debug("pipe start offset", "offset", startOffset)

	// Send input to the Claude session.
	if err := b.paneIO.SendInput(ctx, content); err != nil {
		logger.Error("send input to tmux", "err", err)
		_ = b.sendError(requestID, fmt.Sprintf("failed to send input: %v", err))
		return
	}

	// Wait for response with timeout.
	respCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	fullText, err := WaitForResponse(respCtx, tracker, b.paneIO, startOffset, 100*time.Millisecond)
	if err != nil {
		logger.Error("wait for response", "err", err)
		_ = b.sendError(requestID, fmt.Sprintf("response timeout or error: %v", err))
		return
	}

	// Send done event with full response.
	if err := b.sendDone(requestID, fullText); err != nil {
		logger.Error("send done event", "err", err)
	}

	logger.Info("response complete", "len", len(fullText))
}

// sendThinking sends a thinking event.
func (b *Bridge) sendThinking(requestID, text string) error {
	return b.stream.Send(&pb.AgentMessage{
		Payload: &pb.AgentMessage_Response{
			Response: &pb.MessageResponse{
				RequestId: requestID,
				Event:     &pb.MessageResponse_Thinking{Thinking: text},
			},
		},
	})
}

// sendText sends a text chunk event.
func (b *Bridge) sendText(requestID, text string) error {
	return b.stream.Send(&pb.AgentMessage{
		Payload: &pb.AgentMessage_Response{
			Response: &pb.MessageResponse{
				RequestId: requestID,
				Event:     &pb.MessageResponse_Text{Text: text},
			},
		},
	})
}

// sendDone sends a done event with the full response.
func (b *Bridge) sendDone(requestID, fullResponse string) error {
	return b.stream.Send(&pb.AgentMessage{
		Payload: &pb.AgentMessage_Response{
			Response: &pb.MessageResponse{
				RequestId: requestID,
				Event:     &pb.MessageResponse_Done{Done: &pb.Done{FullResponse: fullResponse}},
			},
		},
	})
}

// sendError sends an error event.
func (b *Bridge) sendError(requestID, message string) error {
	return b.stream.Send(&pb.AgentMessage{
		Payload: &pb.AgentMessage_Response{
			Response: &pb.MessageResponse{
				RequestId: requestID,
				Event:     &pb.MessageResponse_Error{Error: message},
			},
		},
	})
}

// heartbeatLoop sends periodic heartbeats to the gateway.
func (b *Bridge) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := b.stream.Send(&pb.AgentMessage{
				Payload: &pb.AgentMessage_Heartbeat{
					Heartbeat: &pb.Heartbeat{
						TimestampMs: time.Now().UnixMilli(),
					},
				},
			}); err != nil {
				b.logger.Warn("heartbeat failed", "err", err)
				return
			}
		}
	}
}

// Status returns the current bridge state for status logging.
func (b *Bridge) Status() string {
	b.mu.Lock()
	running := b.running
	b.mu.Unlock()

	if !running {
		return "stopped"
	}
	if b.tracker != nil {
		return b.tracker.State().String()
	}
	return "connected"
}
