// ABOUTME: Tests for secrets store functionality
// ABOUTME: Covers CRUD operations and effective secrets resolution

package store

import (
	"context"
	"testing"
)

func TestCreateSecret(t *testing.T) {
	store := setupTestStore(t)

	ctx := context.Background()

	// Test creating a global secret
	global := &Secret{
		Key:   "API_KEY",
		Value: "global-key-123",
	}
	if err := store.CreateSecret(ctx, global); err != nil {
		t.Fatalf("CreateSecret (global) failed: %v", err)
	}
	if global.ID == "" {
		t.Error("expected ID to be set")
	}

	// Test creating an agent-specific secret
	agentID := "agent-1"
	agentSecret := &Secret{
		Key:     "API_KEY",
		Value:   "agent-specific-key",
		AgentID: &agentID,
	}
	if err := store.CreateSecret(ctx, agentSecret); err != nil {
		t.Fatalf("CreateSecret (agent-specific) failed: %v", err)
	}

	// Test duplicate key for same scope fails
	duplicate := &Secret{
		Key:   "API_KEY",
		Value: "another-global",
	}
	if err := store.CreateSecret(ctx, duplicate); err == nil {
		t.Error("expected error for duplicate global key")
	}

	// Test duplicate key for same agent fails
	duplicateAgent := &Secret{
		Key:     "API_KEY",
		Value:   "another-agent-key",
		AgentID: &agentID,
	}
	if err := store.CreateSecret(ctx, duplicateAgent); err == nil {
		t.Error("expected error for duplicate agent-specific key")
	}

	// Test same key for different agent succeeds
	agent2 := "agent-2"
	agent2Secret := &Secret{
		Key:     "API_KEY",
		Value:   "agent2-key",
		AgentID: &agent2,
	}
	if err := store.CreateSecret(ctx, agent2Secret); err != nil {
		t.Fatalf("CreateSecret (agent-2) should succeed: %v", err)
	}
}

func TestGetSecret(t *testing.T) {
	store := setupTestStore(t)

	ctx := context.Background()

	// Create a secret
	secret := &Secret{
		Key:   "DB_PASSWORD",
		Value: "super-secret",
	}
	if err := store.CreateSecret(ctx, secret); err != nil {
		t.Fatalf("CreateSecret failed: %v", err)
	}

	// Retrieve it
	retrieved, err := store.GetSecret(ctx, secret.ID)
	if err != nil {
		t.Fatalf("GetSecret failed: %v", err)
	}

	if retrieved.Key != secret.Key {
		t.Errorf("expected key %q, got %q", secret.Key, retrieved.Key)
	}
	if retrieved.Value != secret.Value {
		t.Errorf("expected value %q, got %q", secret.Value, retrieved.Value)
	}

	// Test not found
	_, err = store.GetSecret(ctx, "nonexistent-id")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestUpdateSecret(t *testing.T) {
	store := setupTestStore(t)

	ctx := context.Background()

	// Create a secret
	secret := &Secret{
		Key:   "REFRESH_TOKEN",
		Value: "original-token",
	}
	if err := store.CreateSecret(ctx, secret); err != nil {
		t.Fatalf("CreateSecret failed: %v", err)
	}

	// Update it
	secret.Value = "new-token"
	if err := store.UpdateSecret(ctx, secret); err != nil {
		t.Fatalf("UpdateSecret failed: %v", err)
	}

	// Verify update
	retrieved, err := store.GetSecret(ctx, secret.ID)
	if err != nil {
		t.Fatalf("GetSecret failed: %v", err)
	}
	if retrieved.Value != "new-token" {
		t.Errorf("expected value %q, got %q", "new-token", retrieved.Value)
	}

	// Test update nonexistent
	nonexistent := &Secret{ID: "nonexistent", Value: "whatever"}
	if err := store.UpdateSecret(ctx, nonexistent); err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestDeleteSecret(t *testing.T) {
	store := setupTestStore(t)

	ctx := context.Background()

	// Create a secret
	secret := &Secret{
		Key:   "TEMP_SECRET",
		Value: "delete-me",
	}
	if err := store.CreateSecret(ctx, secret); err != nil {
		t.Fatalf("CreateSecret failed: %v", err)
	}

	// Delete it
	if err := store.DeleteSecret(ctx, secret.ID); err != nil {
		t.Fatalf("DeleteSecret failed: %v", err)
	}

	// Verify deletion
	_, err := store.GetSecret(ctx, secret.ID)
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}

	// Test delete nonexistent
	if err := store.DeleteSecret(ctx, "nonexistent"); err != ErrNotFound {
		t.Errorf("expected ErrNotFound for nonexistent secret, got %v", err)
	}
}

func TestListAllSecrets(t *testing.T) {
	store := setupTestStore(t)

	ctx := context.Background()

	// Create multiple secrets
	agentID := "agent-1"
	secrets := []*Secret{
		{Key: "KEY_A", Value: "global-a"},
		{Key: "KEY_B", Value: "global-b"},
		{Key: "KEY_A", Value: "agent-a", AgentID: &agentID},
	}

	for _, s := range secrets {
		if err := store.CreateSecret(ctx, s); err != nil {
			t.Fatalf("CreateSecret failed: %v", err)
		}
	}

	// List all
	all, err := store.ListAllSecrets(ctx)
	if err != nil {
		t.Fatalf("ListAllSecrets failed: %v", err)
	}

	if len(all) != 3 {
		t.Errorf("expected 3 secrets, got %d", len(all))
	}
}

func TestGetEffectiveSecrets(t *testing.T) {
	store := setupTestStore(t)

	ctx := context.Background()

	// Set up test data:
	// Global: API_KEY=global-key, DB_HOST=localhost
	// Agent-1: API_KEY=agent1-key (override)
	// Agent-2: (no overrides, gets globals)

	agent1 := "agent-1"
	secrets := []*Secret{
		{Key: "API_KEY", Value: "global-key"},
		{Key: "DB_HOST", Value: "localhost"},
		{Key: "API_KEY", Value: "agent1-key", AgentID: &agent1},
	}

	for _, s := range secrets {
		if err := store.CreateSecret(ctx, s); err != nil {
			t.Fatalf("CreateSecret failed: %v", err)
		}
	}

	// Test agent-1: should get override for API_KEY, global for DB_HOST
	effective1, err := store.GetEffectiveSecrets(ctx, "agent-1")
	if err != nil {
		t.Fatalf("GetEffectiveSecrets (agent-1) failed: %v", err)
	}

	if effective1["API_KEY"] != "agent1-key" {
		t.Errorf("agent-1 API_KEY: expected %q, got %q", "agent1-key", effective1["API_KEY"])
	}
	if effective1["DB_HOST"] != "localhost" {
		t.Errorf("agent-1 DB_HOST: expected %q, got %q", "localhost", effective1["DB_HOST"])
	}

	// Test agent-2: should get all globals
	effective2, err := store.GetEffectiveSecrets(ctx, "agent-2")
	if err != nil {
		t.Fatalf("GetEffectiveSecrets (agent-2) failed: %v", err)
	}

	if effective2["API_KEY"] != "global-key" {
		t.Errorf("agent-2 API_KEY: expected %q, got %q", "global-key", effective2["API_KEY"])
	}
	if effective2["DB_HOST"] != "localhost" {
		t.Errorf("agent-2 DB_HOST: expected %q, got %q", "localhost", effective2["DB_HOST"])
	}

	// Test agent with no secrets defined
	effective3, err := store.GetEffectiveSecrets(ctx, "agent-3")
	if err != nil {
		t.Fatalf("GetEffectiveSecrets (agent-3) failed: %v", err)
	}

	// Should still get globals
	if effective3["API_KEY"] != "global-key" {
		t.Errorf("agent-3 API_KEY: expected %q, got %q", "global-key", effective3["API_KEY"])
	}
}

func TestGetEffectiveSecretsEmpty(t *testing.T) {
	store := setupTestStore(t)

	ctx := context.Background()

	// No secrets defined, should return empty map
	effective, err := store.GetEffectiveSecrets(ctx, "any-agent")
	if err != nil {
		t.Fatalf("GetEffectiveSecrets failed: %v", err)
	}

	if len(effective) != 0 {
		t.Errorf("expected empty map, got %d entries", len(effective))
	}
}
