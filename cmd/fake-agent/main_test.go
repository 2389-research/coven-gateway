// ABOUTME: Tests for fake-agent's auth metadata helper.
// ABOUTME: Verifies -token attaches a Bearer authorization header and empty token attaches nothing.

package main

import (
	"context"
	"testing"

	"google.golang.org/grpc/metadata"
)

func TestWithBearerAppendsAuthorization(t *testing.T) {
	ctx := withBearer(context.Background(), "tok123")
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		t.Fatal("expected outgoing metadata")
	}
	got := md.Get("authorization")
	if len(got) != 1 || got[0] != "Bearer tok123" {
		t.Fatalf("authorization = %v, want [Bearer tok123]", got)
	}
}

func TestWithBearerEmptyTokenAddsNoMetadata(t *testing.T) {
	ctx := withBearer(context.Background(), "")
	if _, ok := metadata.FromOutgoingContext(ctx); ok {
		t.Fatal("expected no outgoing metadata for empty token")
	}
}
