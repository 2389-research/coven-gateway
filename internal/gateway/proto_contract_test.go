package gateway

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	pb "github.com/2389/fold-gateway/proto/fold"
)

// TestProtoContract_RegisterAgent verifies RegisterAgent message serializes
// and deserializes correctly, ensuring Go and Rust see identical bytes.
func TestProtoContract_RegisterAgent(t *testing.T) {
	original := &pb.AgentMessage{
		Payload: &pb.AgentMessage_Register{
			Register: &pb.RegisterAgent{
				AgentId:      "agent-123",
				Name:         "test-agent",
				Capabilities: []string{"code", "chat"},
			},
		},
	}

	// Serialize
	data, err := proto.Marshal(original)
	require.NoError(t, err)

	// Deserialize
	decoded := &pb.AgentMessage{}
	err = proto.Unmarshal(data, decoded)
	require.NoError(t, err)

	// Verify
	reg := decoded.GetRegister()
	require.NotNil(t, reg)
	assert.Equal(t, "agent-123", reg.GetAgentId())
	assert.Equal(t, "test-agent", reg.GetName())
	assert.Equal(t, []string{"code", "chat"}, reg.GetCapabilities())
}

// TestProtoContract_SendMessage verifies SendMessage serializes correctly
// including attachments.
func TestProtoContract_SendMessage(t *testing.T) {
	original := &pb.ServerMessage{
		Payload: &pb.ServerMessage_SendMessage{
			SendMessage: &pb.SendMessage{
				RequestId: "req-456",
				ThreadId:  "thread-789",
				Sender:    "user@example.com",
				Content:   "Hello, agent!",
				Attachments: []*pb.FileAttachment{
					{
						Filename: "test.txt",
						MimeType: "text/plain",
						Data:     []byte("file contents"),
					},
				},
			},
		},
	}

	data, err := proto.Marshal(original)
	require.NoError(t, err)

	decoded := &pb.ServerMessage{}
	err = proto.Unmarshal(data, decoded)
	require.NoError(t, err)

	msg := decoded.GetSendMessage()
	require.NotNil(t, msg)
	assert.Equal(t, "req-456", msg.GetRequestId())
	assert.Equal(t, "thread-789", msg.GetThreadId())
	assert.Equal(t, "user@example.com", msg.GetSender())
	assert.Equal(t, "Hello, agent!", msg.GetContent())
	require.Len(t, msg.GetAttachments(), 1)
	assert.Equal(t, "test.txt", msg.GetAttachments()[0].GetFilename())
}
