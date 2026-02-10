// ABOUTME: Tests for mail pack tool handlers.
// ABOUTME: Uses real SQLite store for integration testing.

package builtins

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/2389/coven-gateway/internal/store"
)

func TestMailSend(t *testing.T) {
	s := newTestStore(t)
	pack := MailPack(s)

	handler := findHandler(pack, "mail_send")
	if handler == nil {
		t.Fatal("mail_send handler not found")
	}

	input := `{"to_agent_id": "agent-2", "subject": "Hello", "content": "World"}`
	result, err := handler(context.Background(), "agent-1", json.RawMessage(input))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	var resp map[string]string
	if err := json.Unmarshal(result, &resp); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if resp["status"] != "sent" {
		t.Errorf("unexpected status: %s", resp["status"])
	}
	if resp["id"] == "" {
		t.Error("expected id in response")
	}
}

func TestMailSendValidation(t *testing.T) {
	s := newTestStore(t)
	pack := MailPack(s)

	handler := findHandler(pack, "mail_send")

	tests := []struct {
		name  string
		input string
	}{
		{"missing to_agent_id", `{"subject": "Hello", "content": "World"}`},
		{"missing subject", `{"to_agent_id": "agent-2", "content": "World"}`},
		{"missing content", `{"to_agent_id": "agent-2", "subject": "Hello"}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := handler(context.Background(), "agent-1", json.RawMessage(tc.input))
			if err == nil {
				t.Errorf("expected error for %s", tc.name)
			}
		})
	}
}

func TestMailInbox(t *testing.T) {
	s := newTestStore(t)
	pack := MailPack(s)

	// Send a mail first
	sendHandler := findHandler(pack, "mail_send")
	_, err := sendHandler(context.Background(), "agent-1", json.RawMessage(`{"to_agent_id": "agent-2", "subject": "Hello", "content": "World"}`))
	if err != nil {
		t.Fatalf("mail_send: %v", err)
	}

	// Check recipient's inbox
	inboxHandler := findHandler(pack, "mail_inbox")
	result, err := inboxHandler(context.Background(), "agent-2", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("mail_inbox: %v", err)
	}

	var resp map[string]any
	if err := json.Unmarshal(result, &resp); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if resp["count"].(float64) != 1 {
		t.Errorf("expected 1 message, got %v", resp["count"])
	}
}

func TestMailInboxIsolation(t *testing.T) {
	s := newTestStore(t)
	pack := MailPack(s)

	// Send mail to agent-2
	sendHandler := findHandler(pack, "mail_send")
	_, err := sendHandler(context.Background(), "agent-1", json.RawMessage(`{"to_agent_id": "agent-2", "subject": "Hello", "content": "World"}`))
	if err != nil {
		t.Fatalf("mail_send: %v", err)
	}

	// Agent-3's inbox should be empty
	inboxHandler := findHandler(pack, "mail_inbox")
	result, err := inboxHandler(context.Background(), "agent-3", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("mail_inbox: %v", err)
	}

	var resp map[string]any
	json.Unmarshal(result, &resp)
	if resp["count"].(float64) != 0 {
		t.Errorf("expected 0 messages for agent-3, got %v", resp["count"])
	}
}

func TestMailInboxUnreadOnly(t *testing.T) {
	s := newTestStore(t)
	pack := MailPack(s)

	// Send two mails
	sendHandler := findHandler(pack, "mail_send")
	result1, _ := sendHandler(context.Background(), "agent-1", json.RawMessage(`{"to_agent_id": "agent-2", "subject": "First", "content": "Message 1"}`))
	var sendResp map[string]string
	json.Unmarshal(result1, &sendResp)
	mailID := sendResp["id"]

	sendHandler(context.Background(), "agent-1", json.RawMessage(`{"to_agent_id": "agent-2", "subject": "Second", "content": "Message 2"}`))

	// Read the first mail
	readHandler := findHandler(pack, "mail_read")
	_, err := readHandler(context.Background(), "agent-2", json.RawMessage(`{"message_id": "`+mailID+`"}`))
	if err != nil {
		t.Fatalf("mail_read: %v", err)
	}

	// Unread only should show 1
	inboxHandler := findHandler(pack, "mail_inbox")
	result, err := inboxHandler(context.Background(), "agent-2", json.RawMessage(`{"unread_only": true}`))
	if err != nil {
		t.Fatalf("mail_inbox: %v", err)
	}

	var resp map[string]any
	json.Unmarshal(result, &resp)
	if resp["count"].(float64) != 1 {
		t.Errorf("expected 1 unread message, got %v", resp["count"])
	}
}

func TestMailInboxLimit(t *testing.T) {
	s := newTestStore(t)
	pack := MailPack(s)

	// Send multiple mails
	sendHandler := findHandler(pack, "mail_send")
	for range 5 {
		_, err := sendHandler(context.Background(), "agent-1", json.RawMessage(`{"to_agent_id": "agent-2", "subject": "Test", "content": "Message"}`))
		if err != nil {
			t.Fatalf("mail_send: %v", err)
		}
	}

	// Request with limit
	inboxHandler := findHandler(pack, "mail_inbox")
	result, err := inboxHandler(context.Background(), "agent-2", json.RawMessage(`{"limit": 3}`))
	if err != nil {
		t.Fatalf("mail_inbox: %v", err)
	}

	var resp map[string]any
	json.Unmarshal(result, &resp)
	if resp["count"].(float64) != 3 {
		t.Errorf("expected 3 messages, got %v", resp["count"])
	}
}

func TestMailRead(t *testing.T) {
	s := newTestStore(t)
	pack := MailPack(s)

	// Send a mail
	sendHandler := findHandler(pack, "mail_send")
	result, err := sendHandler(context.Background(), "agent-1", json.RawMessage(`{"to_agent_id": "agent-2", "subject": "Hello", "content": "World"}`))
	if err != nil {
		t.Fatalf("mail_send: %v", err)
	}
	var sendResp map[string]string
	json.Unmarshal(result, &sendResp)
	mailID := sendResp["id"]

	// Read the mail
	readHandler := findHandler(pack, "mail_read")
	result, err = readHandler(context.Background(), "agent-2", json.RawMessage(`{"message_id": "`+mailID+`"}`))
	if err != nil {
		t.Fatalf("mail_read: %v", err)
	}

	var mail store.AgentMail
	if err := json.Unmarshal(result, &mail); err != nil {
		t.Fatalf("unmarshal mail: %v", err)
	}
	if mail.Subject != "Hello" {
		t.Errorf("unexpected subject: %s", mail.Subject)
	}
	if mail.Content != "World" {
		t.Errorf("unexpected content: %s", mail.Content)
	}
	if mail.FromAgentID != "agent-1" {
		t.Errorf("unexpected from: %s", mail.FromAgentID)
	}
}

func TestMailReadAccessControl(t *testing.T) {
	s := newTestStore(t)
	pack := MailPack(s)

	// Send a mail to agent-2
	sendHandler := findHandler(pack, "mail_send")
	result, err := sendHandler(context.Background(), "agent-1", json.RawMessage(`{"to_agent_id": "agent-2", "subject": "Private", "content": "Secret"}`))
	if err != nil {
		t.Fatalf("mail_send: %v", err)
	}
	var sendResp map[string]string
	json.Unmarshal(result, &sendResp)
	mailID := sendResp["id"]

	// Agent-3 should not be able to read agent-2's mail
	readHandler := findHandler(pack, "mail_read")
	_, err = readHandler(context.Background(), "agent-3", json.RawMessage(`{"message_id": "`+mailID+`"}`))
	if err == nil {
		t.Error("expected error when agent-3 tries to read agent-2's mail")
	}

	// Agent-1 (the sender) should also not be able to read it through mail_read
	_, err = readHandler(context.Background(), "agent-1", json.RawMessage(`{"message_id": "`+mailID+`"}`))
	if err == nil {
		t.Error("expected error when sender tries to read recipient's mail")
	}

	// Agent-2 (the recipient) should be able to read it
	_, err = readHandler(context.Background(), "agent-2", json.RawMessage(`{"message_id": "`+mailID+`"}`))
	if err != nil {
		t.Fatalf("recipient should be able to read own mail: %v", err)
	}
}

func TestMailReadValidation(t *testing.T) {
	s := newTestStore(t)
	pack := MailPack(s)

	readHandler := findHandler(pack, "mail_read")
	_, err := readHandler(context.Background(), "agent-1", json.RawMessage(`{}`))
	if err == nil {
		t.Error("expected error for missing message_id")
	}
}
