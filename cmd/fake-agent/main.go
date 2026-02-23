// ABOUTME: Minimal fake agent for E2E testing — connects via gRPC, echoes messages with markdown.
// ABOUTME: Usage: fake-agent [-addr localhost:50051] [-name "Echo Agent"]
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/2389/coven-gateway/proto/coven"
)

func main() {
	addr := flag.String("addr", "localhost:50051", "gRPC server address")
	name := flag.String("name", "Echo Agent", "Agent display name")
	agentID := flag.String("id", "e2e-echo-agent", "Agent ID")
	flag.Parse()

	if err := run(*addr, *name, *agentID); err != nil {
		log.Fatal(err)
	}
}

func run(addr, name, agentID string) error {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer conn.Close()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	client := pb.NewCovenControlClient(conn)
	stream, err := client.AgentStream(ctx)
	if err != nil {
		return fmt.Errorf("failed to open stream: %w", err)
	}

	// Register
	if err := stream.Send(&pb.AgentMessage{
		Payload: &pb.AgentMessage_Register{
			Register: &pb.RegisterAgent{
				AgentId:      agentID,
				Name:         name,
				Capabilities: []string{"chat", "echo"},
				Metadata: &pb.AgentMetadata{
					WorkingDirectory: "/tmp/fake-agent",
					Hostname:         "e2e-test",
					Os:               "test",
					Backend:          "direct",
				},
			},
		},
	}); err != nil {
		return fmt.Errorf("failed to register: %w", err)
	}

	// Receive Welcome
	msg, err := stream.Recv()
	if err != nil {
		return fmt.Errorf("failed to receive welcome: %w", err)
	}
	welcome := msg.GetWelcome()
	if welcome == nil {
		return fmt.Errorf("expected welcome, got: %v", msg)
	}
	fmt.Fprintf(os.Stderr, "registered as %s (instance: %s)\n", welcome.AgentId, welcome.InstanceId)

	// Message loop
	for {
		msg, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			if ctx.Err() != nil {
				return nil // graceful shutdown
			}
			return fmt.Errorf("recv error: %w", err)
		}

		sm := msg.GetSendMessage()
		if sm == nil {
			continue
		}

		log.Printf("received message [%s]: %s", sm.RequestId, sm.Content)

		reply := echoReply(sm.Content)

		// Send text response with markdown
		if err := stream.Send(&pb.AgentMessage{
			Payload: &pb.AgentMessage_Response{
				Response: &pb.MessageResponse{
					RequestId: sm.RequestId,
					Event:     &pb.MessageResponse_Text{Text: reply},
				},
			},
		}); err != nil {
			log.Printf("send text error: %v", err)
			continue
		}

		// Small delay to simulate streaming
		time.Sleep(50 * time.Millisecond)

		// Send Done
		if err := stream.Send(&pb.AgentMessage{
			Payload: &pb.AgentMessage_Response{
				Response: &pb.MessageResponse{
					RequestId: sm.RequestId,
					Event:     &pb.MessageResponse_Done{Done: &pb.Done{FullResponse: reply}},
				},
			},
		}); err != nil {
			log.Printf("send done error: %v", err)
		}
	}
}

func echoReply(input string) string {
	lower := strings.ToLower(input)
	if strings.Contains(lower, "markdown") || strings.Contains(lower, "bullet") || strings.Contains(lower, "list") {
		return "Here is a **markdown** response:\n\n- First item\n- Second item with `code`\n- Third item\n\n> This is a blockquote.\n"
	}
	return fmt.Sprintf("Echo: **%s**\n\nI received your message and am responding with some *formatted* text.", input)
}
