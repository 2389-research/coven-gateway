// ABOUTME: Contract tests for gRPC service surface to detect breaking API changes.
// ABOUTME: Validates that expected services and methods exist in generated proto code.

package contract

import (
	"fmt"
	"slices"
	"testing"

	"github.com/2389/coven-gateway/proto/coven"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

// expectedServices defines the contract for our gRPC API surface.
// If a service or method is removed or renamed, these tests will fail,
// catching breaking changes before they reach production.
var expectedServices = map[string]struct {
	methods []string
	streams []string
}{
	"coven.CovenControl": {
		methods: []string{},
		streams: []string{"AgentStream"},
	},
	"coven.AdminService": {
		methods: []string{
			"ListBindings",
			"CreateBinding",
			"UpdateBinding",
			"DeleteBinding",
		},
		streams: []string{},
	},
	"coven.ClientService": {
		methods: []string{
			"GetEvents",
			"GetMe",
			"SendMessage",
		},
		streams: []string{},
	},
}

// TestProtoSurface verifies that all expected gRPC services and methods exist
// in the generated protobuf code. This acts as a contract test to prevent
// accidental breaking changes to the API surface.
func TestProtoSurface(t *testing.T) {
	// Build lookup maps from the actual ServiceDesc definitions
	serviceDescs := map[string]grpc.ServiceDesc{
		"coven.CovenControl":  coven.CovenControl_ServiceDesc,
		"coven.AdminService":  coven.AdminService_ServiceDesc,
		"coven.ClientService": coven.ClientService_ServiceDesc,
	}

	for serviceName, expected := range expectedServices {
		t.Run(serviceName, func(t *testing.T) {
			desc, exists := serviceDescs[serviceName]
			if !assert.True(t, exists, "service %s should be registered", serviceName) {
				return
			}

			// Verify service name matches
			assert.Equal(t, serviceName, desc.ServiceName, "service name should match")

			// Build method lookup from actual descriptor
			actualMethods := make(map[string]bool)
			for _, m := range desc.Methods {
				actualMethods[m.MethodName] = true
			}

			// Build stream lookup from actual descriptor
			actualStreams := make(map[string]bool)
			for _, s := range desc.Streams {
				actualStreams[s.StreamName] = true
			}

			// Verify expected methods exist
			for _, method := range expected.methods {
				fullName := fmt.Sprintf("/%s/%s", serviceName, method)
				assert.True(t, actualMethods[method],
					"method %s should exist in service %s", fullName, serviceName)
			}

			// Verify expected streams exist
			for _, stream := range expected.streams {
				fullName := fmt.Sprintf("/%s/%s", serviceName, stream)
				assert.True(t, actualStreams[stream],
					"stream %s should exist in service %s", fullName, serviceName)
			}

			// Report any extra methods not in contract (informational, not failure)
			for method := range actualMethods {
				found := slices.Contains(expected.methods, method)
				if !found {
					t.Logf("INFO: extra method %s/%s not in contract (consider adding)", serviceName, method)
				}
			}

			// Report any extra streams not in contract (informational, not failure)
			for stream := range actualStreams {
				found := slices.Contains(expected.streams, stream)
				if !found {
					t.Logf("INFO: extra stream %s/%s not in contract (consider adding)", serviceName, stream)
				}
			}
		})
	}
}

// TestServiceDescriptorsExist verifies that all ServiceDesc variables are exported
// and have the expected structure.
func TestServiceDescriptorsExist(t *testing.T) {
	tests := []struct {
		name        string
		desc        grpc.ServiceDesc
		serviceName string
	}{
		{"CovenControl", coven.CovenControl_ServiceDesc, "coven.CovenControl"},
		{"AdminService", coven.AdminService_ServiceDesc, "coven.AdminService"},
		{"ClientService", coven.ClientService_ServiceDesc, "coven.ClientService"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.serviceName, tt.desc.ServiceName, "ServiceName should match expected")
			assert.Equal(t, "coven.proto", tt.desc.Metadata, "Metadata should reference coven.proto")
			// HandlerType is intentionally (*ServerInterface)(nil) in gRPC, so we just verify
			// the service has either methods or streams defined
			hasEndpoints := len(tt.desc.Methods) > 0 || len(tt.desc.Streams) > 0
			assert.True(t, hasEndpoints, "service should have at least one method or stream")
		})
	}
}
