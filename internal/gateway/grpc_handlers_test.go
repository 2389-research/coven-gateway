// ABOUTME: Tests for CovenControl gRPC handler gap coverage in grpc.go.
// ABOUTME: Covers receiveRegistration, checkRecvError, maybeGrantLeaderRole, and related helpers.

package gateway

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/2389/coven-gateway/internal/agent"
	"github.com/2389/coven-gateway/internal/packs"
	"github.com/2389/coven-gateway/internal/store"
	pb "github.com/2389/coven-gateway/proto/coven"
)

// =============================================================================
// scriptedStream: a fakeAgentStream variant that plays back a sequence of messages.
// Re-uses fakeAgentStream from grpc_test.go for Send; Recv is overridden.
// =============================================================================

// scriptedAgentStream replays a sequence of messages then returns io.EOF.
type scriptedAgentStream struct {
	grpc.ServerStream
	ctx      context.Context
	mu       sync.Mutex
	sent     []*pb.ServerMessage
	messages []*pb.AgentMessage
	pos      int
}

func (s *scriptedAgentStream) Context() context.Context { return s.ctx }

func (s *scriptedAgentStream) Send(msg *pb.ServerMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = append(s.sent, msg)
	return nil
}

func (s *scriptedAgentStream) Recv() (*pb.AgentMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pos >= len(s.messages) {
		return nil, io.EOF
	}
	msg := s.messages[s.pos]
	s.pos++
	return msg, nil
}

// =============================================================================
// receiveRegistration
// =============================================================================

func TestReceiveRegistration_EOF(t *testing.T) {
	srv := newTestCovenControlServer(t)
	stream := &scriptedAgentStream{ctx: context.Background()}
	// No messages — Recv returns EOF immediately
	reg, clean, err := srv.receiveRegistration(stream)
	if reg != nil {
		t.Error("expected nil reg on EOF")
	}
	if !clean {
		t.Error("expected cleanDisconnect=true on EOF")
	}
	if err != nil {
		t.Errorf("expected nil error on EOF, got %v", err)
	}
}

func TestReceiveRegistration_NonRegisterFirstMessage(t *testing.T) {
	srv := newTestCovenControlServer(t)
	stream := &scriptedAgentStream{
		ctx: context.Background(),
		messages: []*pb.AgentMessage{
			// Send a heartbeat instead of Register
			{Payload: &pb.AgentMessage_Heartbeat{Heartbeat: &pb.Heartbeat{TimestampMs: 1}}},
		},
	}
	reg, clean, err := srv.receiveRegistration(stream)
	if reg != nil {
		t.Error("expected nil reg for non-register first message")
	}
	if clean {
		t.Error("expected cleanDisconnect=false for non-register")
	}
	if err == nil {
		t.Error("expected error for non-register first message")
	}
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", status.Code(err))
	}
}

func TestReceiveRegistration_MissingAgentID(t *testing.T) {
	srv := newTestCovenControlServer(t)
	stream := &scriptedAgentStream{
		ctx: context.Background(),
		messages: []*pb.AgentMessage{
			{Payload: &pb.AgentMessage_Register{Register: &pb.RegisterAgent{
				AgentId: "", // missing
				Name:    "test",
			}}},
		},
	}
	reg, clean, err := srv.receiveRegistration(stream)
	if reg != nil {
		t.Error("expected nil reg for missing agent_id")
	}
	if clean {
		t.Error("expected cleanDisconnect=false")
	}
	if err == nil {
		t.Error("expected error for missing agent_id")
	}
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", status.Code(err))
	}
}

func TestReceiveRegistration_Valid(t *testing.T) {
	srv := newTestCovenControlServer(t)
	stream := &scriptedAgentStream{
		ctx: context.Background(),
		messages: []*pb.AgentMessage{
			{Payload: &pb.AgentMessage_Register{Register: &pb.RegisterAgent{
				AgentId: "agent-1",
				Name:    "TestAgent",
			}}},
		},
	}
	reg, clean, err := srv.receiveRegistration(stream)
	if reg == nil {
		t.Fatal("expected non-nil reg")
	}
	if clean {
		t.Error("expected cleanDisconnect=false for valid registration")
	}
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if reg.GetAgentId() != "agent-1" {
		t.Errorf("agent_id = %q, want agent-1", reg.GetAgentId())
	}
}

// =============================================================================
// checkRecvError
// =============================================================================

func TestCheckRecvError_Nil(t *testing.T) {
	srv := newTestCovenControlServer(t)
	cont, err := srv.checkRecvError(nil, "agent-1")
	if !cont {
		t.Error("expected continue=true for nil error")
	}
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestCheckRecvError_EOF(t *testing.T) {
	srv := newTestCovenControlServer(t)
	cont, err := srv.checkRecvError(io.EOF, "agent-1")
	if cont {
		t.Error("expected continue=false for EOF")
	}
	if err != nil {
		t.Errorf("expected nil error for EOF, got %v", err)
	}
}

func TestCheckRecvError_Canceled(t *testing.T) {
	srv := newTestCovenControlServer(t)
	canceledErr := status.Error(codes.Canceled, "context canceled")
	cont, err := srv.checkRecvError(canceledErr, "agent-1")
	if cont {
		t.Error("expected continue=false for canceled")
	}
	if err != nil {
		t.Errorf("expected nil error for canceled, got %v", err)
	}
}

func TestCheckRecvError_OtherError(t *testing.T) {
	srv := newTestCovenControlServer(t)
	otherErr := status.Error(codes.Internal, "something broke")
	cont, err := srv.checkRecvError(otherErr, "agent-1")
	if cont {
		t.Error("expected continue=false for internal error")
	}
	if err == nil {
		t.Error("expected non-nil error for internal error")
	}
}

// =============================================================================
// dispatchMessage — unknown type arm
// =============================================================================

func TestDispatchMessage_UnknownType(t *testing.T) {
	srv := newTestCovenControlServer(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	conn := agent.NewConnection(agent.ConnectionParams{
		ID:     "agent-disp",
		Name:   "Test",
		Stream: &fakeAgentStream{ctx: context.Background()},
		Logger: logger,
	})
	stream := &fakeAgentStream{ctx: context.Background()}

	// An AgentMessage with nil payload hits the default arm
	msg := &pb.AgentMessage{}
	srv.dispatchMessage(stream, conn, msg) // should not panic
}

// =============================================================================
// maybeGrantLeaderRole
// =============================================================================

func TestMaybeGrantLeaderRole_NoPrincipalID(t *testing.T) {
	srv := newTestCovenControlServer(t)
	// Should return early without doing anything
	srv.maybeGrantLeaderRole(context.Background(), "", []string{"leader"})
}

func TestMaybeGrantLeaderRole_NoLeaderCapability(t *testing.T) {
	srv := newTestCovenControlServer(t)
	// No "leader" in caps — should return early
	srv.maybeGrantLeaderRole(context.Background(), "principal-1", []string{"chat", "code"})
}

func TestMaybeGrantLeaderRole_WithLeaderCapability(t *testing.T) {
	gw := newTestGateway(t)
	srv := &covenControlServer{gateway: gw, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}

	// Create principal in store first
	sqlStore := gw.store.(*store.SQLiteStore)
	ctx := context.Background()
	principal := &store.Principal{
		ID:          "leader-principal",
		Type:        store.PrincipalTypeAgent,
		PubkeyFP:    "aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111",
		DisplayName: "leader-agent",
		Status:      store.PrincipalStatusOnline,
	}
	if err := sqlStore.CreatePrincipal(ctx, principal); err != nil {
		t.Fatalf("create principal: %v", err)
	}

	// Should add leader role without error
	srv.maybeGrantLeaderRole(ctx, "leader-principal", []string{"chat", "leader"})

	// Verify role was added
	hasRole, err := sqlStore.HasRole(ctx, store.RoleSubjectPrincipal, "leader-principal", store.RoleLeader)
	if err != nil {
		t.Fatalf("HasRole: %v", err)
	}
	if !hasRole {
		t.Error("expected leader-principal to have leader role")
	}
}

// =============================================================================
// maybeUpdateBindingsForWorkspace
// =============================================================================

func TestMaybeUpdateBindingsForWorkspace_EmptyWorkspace(t *testing.T) {
	srv := newTestCovenControlServer(t)
	// Should return early without doing anything
	srv.maybeUpdateBindingsForWorkspace(context.Background(), "", "agent-1")
}

func TestMaybeUpdateBindingsForWorkspace_WithWorkspace(t *testing.T) {
	gw := newTestGateway(t)
	srv := &covenControlServer{gateway: gw, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	// Should succeed with no bindings to update (count=0)
	srv.maybeUpdateBindingsForWorkspace(context.Background(), "notes", "agent-new")
}

// =============================================================================
// extractRegistrationInfo
// =============================================================================

func TestExtractRegistrationInfo_NoMetadata(t *testing.T) {
	srv := newTestCovenControlServer(t)
	reg := &pb.RegisterAgent{AgentId: "a1", Name: "TestAgent"}
	info := srv.extractRegistrationInfo(context.Background(), reg)
	if info.instanceID == "" {
		t.Error("expected non-empty instanceID")
	}
	if len(info.workspaces) > 0 {
		t.Error("expected empty workspaces without metadata")
	}
}

func TestExtractRegistrationInfo_WithMetadata(t *testing.T) {
	srv := newTestCovenControlServer(t)
	reg := &pb.RegisterAgent{
		AgentId: "a1",
		Name:    "TestAgent",
		Metadata: &pb.AgentMetadata{
			Workspaces:       []string{"Code", "Personal"},
			WorkingDirectory: "/home/user/projects",
			Backend:          "claude",
		},
	}
	info := srv.extractRegistrationInfo(context.Background(), reg)
	if len(info.workspaces) != 2 {
		t.Errorf("workspaces = %v, want 2 items", info.workspaces)
	}
	if info.workingDir != "/home/user/projects" {
		t.Errorf("workingDir = %q, want '/home/user/projects'", info.workingDir)
	}
	if info.backend != "claude" {
		t.Errorf("backend = %q, want 'claude'", info.backend)
	}
}

// =============================================================================
// loadAgentSecrets
// =============================================================================

func TestLoadAgentSecrets_WithSQLiteStore(t *testing.T) {
	gw := newTestGateway(t)
	srv := &covenControlServer{gateway: gw, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	// Should return empty map for agent with no secrets
	secrets := srv.loadAgentSecrets(context.Background(), "agent-no-secrets")
	if secrets == nil {
		t.Error("expected non-nil secrets map")
	}
}

// =============================================================================
// handleExecutePackTool — packRouter nil branch
// =============================================================================

func TestHandleExecutePackTool_NilPackRouter(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	gw := newTestGateway(t)
	gw.packRouter = nil // force nil router
	srv := &covenControlServer{gateway: gw, logger: logger}

	stream := &fakeAgentStream{ctx: context.Background()}
	conn := agent.NewConnection(agent.ConnectionParams{
		ID:     "agent-exec",
		Name:   "Test",
		Stream: stream,
		Logger: logger,
	})

	req := &pb.ExecutePackTool{RequestId: "req-nil", ToolName: "some_tool"}
	srv.handleExecutePackTool(context.Background(), conn, req)

	// Wait briefly for the error to be sent (handleExecutePackTool is async).
	found := false
	for range 100 {
		for _, msg := range stream.sentMessages() {
			if r := msg.GetPackToolResult(); r != nil && r.GetRequestId() == "req-nil" {
				found = true
				if r.GetError() == "" {
					t.Error("expected error message in pack tool result for nil router")
				}
				break
			}
		}
		if found {
			break
		}
	}
	if !found {
		t.Error("expected PackToolResult to be sent for nil router")
	}
}

// =============================================================================
// sendPackToolError
// =============================================================================

func TestSendPackToolError_Success(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := &covenControlServer{
		gateway: newTestGateway(t),
		logger:  logger,
	}
	stream := &fakeAgentStream{ctx: context.Background()}
	conn := agent.NewConnection(agent.ConnectionParams{
		ID:     "agent-err",
		Name:   "Test",
		Stream: stream,
		Logger: logger,
	})

	srv.sendPackToolError(conn, "req-error", "something went wrong")

	msgs := stream.sentMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	result := msgs[0].GetPackToolResult()
	if result == nil {
		t.Fatal("expected PackToolResult, got nil")
	}
	if result.GetRequestId() != "req-error" {
		t.Errorf("request_id = %q, want req-error", result.GetRequestId())
	}
	if result.GetError() != "something went wrong" {
		t.Errorf("error = %q, want 'something went wrong'", result.GetError())
	}
}

// =============================================================================
// handleExecutePackTool — with registered pack
// =============================================================================

func TestHandleExecutePackTool_UnknownTool(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	gw := newTestGateway(t)

	// Set up a pack router with a known pack
	registry := packs.NewRegistry(logger)
	manifest := &pb.PackManifest{
		PackId:  "testpack",
		Version: "1.0",
		Tools:   []*pb.ToolDefinition{{Name: "known_tool"}},
	}
	if err := registry.RegisterPack("testpack", manifest); err != nil {
		t.Fatalf("RegisterPack: %v", err)
	}
	router := packs.NewRouter(packs.RouterConfig{
		Registry: registry,
		Logger:   logger,
		Timeout:  50, // very short timeout in ms
	})
	gw.packRouter = router

	srv := &covenControlServer{gateway: gw, logger: logger}
	stream := &fakeAgentStream{ctx: context.Background()}
	conn := agent.NewConnection(agent.ConnectionParams{
		ID:     "agent-unknown",
		Name:   "Test",
		Stream: stream,
		Logger: logger,
	})

	// unknown_tool is not in any pack — router should send back an error result quickly.
	req := &pb.ExecutePackTool{RequestId: "req-unk", ToolName: "unknown_tool"}
	srv.handleExecutePackTool(context.Background(), conn, req)

	// Poll up to 200 iterations for the async error result to arrive.
	found := false
	for range 200 {
		for _, msg := range stream.sentMessages() {
			if r := msg.GetPackToolResult(); r != nil && r.GetRequestId() == "req-unk" {
				if r.GetError() == "" {
					t.Error("expected non-empty error in PackToolResult for unknown tool")
				}
				found = true
				break
			}
		}
		if found {
			break
		}
	}
	if !found {
		t.Error("expected PackToolResult to be sent for unknown tool")
	}
}

// =============================================================================
// loadAgentSecrets — success arm with a seeded secret
// =============================================================================

func TestLoadAgentSecrets_WithSeededSecret(t *testing.T) {
	gw := newTestGateway(t)
	srv := &covenControlServer{gateway: gw, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	sqlStore := gw.store.(*store.SQLiteStore)
	ctx := context.Background()

	// Seed a global secret
	secret := &store.Secret{
		ID:    "secret-1",
		Key:   "API_KEY",
		Value: "supersecret",
	}
	if err := sqlStore.CreateSecret(ctx, secret); err != nil {
		t.Fatalf("CreateSecret: %v", err)
	}

	secrets := srv.loadAgentSecrets(ctx, "agent-with-secret")
	if secrets == nil {
		t.Fatal("expected non-nil secrets map")
	}
	if val, ok := secrets["API_KEY"]; !ok || val != "supersecret" {
		t.Errorf("secrets[API_KEY] = %q, want 'supersecret'", val)
	}
}

// =============================================================================
// maybeUpdateBindingsForWorkspace — count > 0 arm (workspace with a binding)
// =============================================================================

func TestMaybeUpdateBindingsForWorkspace_CountNonZero(t *testing.T) {
	gw := newTestGateway(t)
	srv := &covenControlServer{gateway: gw, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	ctx := context.Background()
	sqlStore := gw.store.(*store.SQLiteStore)

	// Create a principal whose ID ends with "_notes" so UpdateBindingsByWorkspace("notes") matches
	principal := &store.Principal{
		ID:          "m_notes",
		Type:        store.PrincipalTypeAgent,
		PubkeyFP:    "aaaa2222aaaa2222aaaa2222aaaa2222aaaa2222aaaa2222aaaa2222aaaa2222",
		DisplayName: "notes-agent",
		Status:      store.PrincipalStatusOnline,
	}
	if err := sqlStore.CreatePrincipal(ctx, principal); err != nil {
		t.Fatalf("CreatePrincipal: %v", err)
	}

	// Create a binding pointing to "m_notes"
	binding := &store.Binding{
		ID:        "bind-notes-1",
		Frontend:  "matrix",
		ChannelID: "#notes-channel",
		AgentID:   "m_notes",
	}
	if err := sqlStore.CreateBindingV2(ctx, binding); err != nil {
		t.Fatalf("CreateBindingV2: %v", err)
	}

	// Now create the new agent principal
	newPrincipal := &store.Principal{
		ID:          "magic_notes",
		Type:        store.PrincipalTypeAgent,
		PubkeyFP:    "bbbb3333bbbb3333bbbb3333bbbb3333bbbb3333bbbb3333bbbb3333bbbb3333",
		DisplayName: "magic-notes-agent",
		Status:      store.PrincipalStatusOnline,
	}
	if err := sqlStore.CreatePrincipal(ctx, newPrincipal); err != nil {
		t.Fatalf("CreatePrincipal (new): %v", err)
	}

	// Update bindings for "notes" workspace to point to new agent
	srv.maybeUpdateBindingsForWorkspace(ctx, "notes", "magic_notes")
	// No assertion needed — just verify it runs the count>0 log path without panic
}

// =============================================================================
// newTestCovenControlServer creates a covenControlServer backed by a test gateway.
// =============================================================================

func newTestCovenControlServer(t *testing.T) *covenControlServer {
	t.Helper()
	gw := newTestGateway(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return &covenControlServer{gateway: gw, logger: logger}
}
