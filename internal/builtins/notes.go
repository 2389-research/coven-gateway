// ABOUTME: Notes pack provides key-value storage for agents.
// ABOUTME: Requires the "notes" capability.

package builtins

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/2389/coven-gateway/internal/packs"
	"github.com/2389/coven-gateway/internal/store"
	pb "github.com/2389/coven-gateway/proto/coven"
)

// NotesPack creates the notes pack with key-value storage tools.
func NotesPack(s store.BuiltinStore) *packs.BuiltinPack {
	n := &notesHandlers{store: s}
	return &packs.BuiltinPack{
		ID: "builtin:notes",
		Tools: []*packs.BuiltinTool{
			{
				Definition: &pb.ToolDefinition{
					Name:                 "note_set",
					Description:          "Store a note",
					InputSchemaJson:      `{"type":"object","properties":{"key":{"type":"string"},"value":{"type":"string"}},"required":["key","value"]}`,
					RequiredCapabilities: []string{"notes"},
				},
				Handler: n.Set,
			},
			{
				Definition: &pb.ToolDefinition{
					Name:                 "note_get",
					Description:          "Retrieve a note",
					InputSchemaJson:      `{"type":"object","properties":{"key":{"type":"string"}},"required":["key"]}`,
					RequiredCapabilities: []string{"notes"},
				},
				Handler: n.Get,
			},
			{
				Definition: &pb.ToolDefinition{
					Name:                 "note_list",
					Description:          "List all note keys",
					InputSchemaJson:      `{"type":"object","properties":{}}`,
					RequiredCapabilities: []string{"notes"},
				},
				Handler: n.List,
			},
			{
				Definition: &pb.ToolDefinition{
					Name:                 "note_delete",
					Description:          "Delete a note",
					InputSchemaJson:      `{"type":"object","properties":{"key":{"type":"string"}},"required":["key"]}`,
					RequiredCapabilities: []string{"notes"},
				},
				Handler: n.Delete,
			},
		},
	}
}

type notesHandlers struct {
	store store.BuiltinStore
}

type noteSetInput struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func (n *notesHandlers) Set(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error) {
	var in noteSetInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	note := &store.AgentNote{
		AgentID: agentID,
		Key:     in.Key,
		Value:   in.Value,
	}
	if err := n.store.SetNote(ctx, note); err != nil {
		return nil, err
	}

	return json.Marshal(map[string]string{"key": in.Key, "status": "saved"})
}

type noteGetInput struct {
	Key string `json:"key"`
}

func (n *notesHandlers) Get(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error) {
	var in noteGetInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	note, err := n.store.GetNote(ctx, agentID, in.Key)
	if err != nil {
		return nil, err
	}

	return json.Marshal(map[string]string{"key": note.Key, "value": note.Value})
}

func (n *notesHandlers) List(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error) {
	notes, err := n.store.ListNotes(ctx, agentID)
	if err != nil {
		return nil, err
	}

	keys := make([]string, len(notes))
	for i, note := range notes {
		keys[i] = note.Key
	}

	return json.Marshal(map[string]any{"keys": keys, "count": len(keys)})
}

type noteDeleteInput struct {
	Key string `json:"key"`
}

func (n *notesHandlers) Delete(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error) {
	var in noteDeleteInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	if err := n.store.DeleteNote(ctx, agentID, in.Key); err != nil {
		return nil, err
	}

	return json.Marshal(map[string]string{"key": in.Key, "status": "deleted"})
}
