// ABOUTME: Mail pack provides inter-agent messaging tools.
// ABOUTME: Requires the "mail" capability.

package builtins

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/2389/coven-gateway/internal/packs"
	"github.com/2389/coven-gateway/internal/store"
	pb "github.com/2389/coven-gateway/proto/coven"
)

// MailPack creates the mail pack with inter-agent messaging tools.
func MailPack(s store.BuiltinStore) *packs.BuiltinPack {
	m := &mailHandlers{store: s}
	return &packs.BuiltinPack{
		ID: "builtin:mail",
		Tools: []*packs.BuiltinTool{
			{
				Definition: &pb.ToolDefinition{
					Name:                 "mail_send",
					Description:          "Send message to another agent",
					InputSchemaJson:      `{"type":"object","properties":{"to_agent_id":{"type":"string"},"subject":{"type":"string"},"content":{"type":"string"}},"required":["to_agent_id","subject","content"]}`,
					RequiredCapabilities: []string{"mail"},
				},
				Handler: m.Send,
			},
			{
				Definition: &pb.ToolDefinition{
					Name:                 "mail_inbox",
					Description:          "List received messages",
					InputSchemaJson:      `{"type":"object","properties":{"limit":{"type":"integer"},"unread_only":{"type":"boolean"}}}`,
					RequiredCapabilities: []string{"mail"},
				},
				Handler: m.Inbox,
			},
			{
				Definition: &pb.ToolDefinition{
					Name:                 "mail_read",
					Description:          "Read and mark message as read",
					InputSchemaJson:      `{"type":"object","properties":{"message_id":{"type":"string"}},"required":["message_id"]}`,
					RequiredCapabilities: []string{"mail"},
				},
				Handler: m.Read,
			},
		},
	}
}

type mailHandlers struct {
	store store.BuiltinStore
}

type mailSendInput struct {
	ToAgentID string `json:"to_agent_id"`
	Subject   string `json:"subject"`
	Content   string `json:"content"`
}

func (m *mailHandlers) Send(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error) {
	var in mailSendInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	if in.ToAgentID == "" {
		return nil, fmt.Errorf("to_agent_id is required")
	}
	if in.Subject == "" {
		return nil, fmt.Errorf("subject is required")
	}
	if in.Content == "" {
		return nil, fmt.Errorf("content is required")
	}

	mail := &store.AgentMail{
		FromAgentID: agentID,
		ToAgentID:   in.ToAgentID,
		Subject:     in.Subject,
		Content:     in.Content,
	}
	if err := m.store.SendMail(ctx, mail); err != nil {
		return nil, err
	}

	return json.Marshal(map[string]string{"id": mail.ID, "status": "sent"})
}

type mailInboxInput struct {
	Limit      int  `json:"limit"`
	UnreadOnly bool `json:"unread_only"`
}

func (m *mailHandlers) Inbox(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error) {
	var in mailInboxInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	limit := in.Limit
	if limit <= 0 {
		limit = 20
	}

	messages, err := m.store.ListInbox(ctx, agentID, in.UnreadOnly, limit)
	if err != nil {
		return nil, err
	}

	return json.Marshal(map[string]any{"messages": messages, "count": len(messages)})
}

type mailReadInput struct {
	MessageID string `json:"message_id"`
}

func (m *mailHandlers) Read(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error) {
	var in mailReadInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	if in.MessageID == "" {
		return nil, fmt.Errorf("message_id is required")
	}

	mail, err := m.store.GetMail(ctx, in.MessageID)
	if err != nil {
		return nil, err
	}

	// Verify the calling agent is the recipient
	if mail.ToAgentID != agentID {
		return nil, fmt.Errorf("message not found")
	}

	// Mark as read
	if err := m.store.MarkMailRead(ctx, in.MessageID); err != nil {
		return nil, err
	}

	return json.Marshal(mail)
}
