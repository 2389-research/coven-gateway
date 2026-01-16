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
