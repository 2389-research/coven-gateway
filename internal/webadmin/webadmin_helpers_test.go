// ABOUTME: Shared test helpers for webadmin tests requiring a real SQLite store.
// ABOUTME: Provides newTestAdminWithStore for handlers that type-assert *store.SQLiteStore.

package webadmin

import (
	"context"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/2389/coven-gateway/internal/store"
)

// newTestAdminWithStore creates an Admin backed by a real *store.SQLiteStore in t.TempDir().
// The store is closed via t.Cleanup. Handlers that call getSQLiteStore() (secrets, etc.)
// require this helper rather than newTestAdmin().
func newTestAdminWithStore(t *testing.T) *Admin {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	s, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	return NewWithConfig(NewConfig{
		Store: s,
	})
}

// createAdminUserWithPassword seeds an AdminUser with the given username and plaintext password
// into the admin's store. Returns the created user. Fatals on any error.
func createAdminUserWithPassword(t *testing.T, a *Admin, username, plaintext string) *store.AdminUser {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(plaintext), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	user := &store.AdminUser{
		ID:           "user-" + username,
		Username:     username,
		PasswordHash: string(hash),
		DisplayName:  username + " Display",
	}
	if err := a.store.CreateAdminUser(context.Background(), user); err != nil {
		t.Fatalf("CreateAdminUser: %v", err)
	}
	return user
}
