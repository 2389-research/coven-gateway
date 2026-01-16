package store

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// setupTestStore creates a temporary SQLite store for testing.
func setupTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewSQLiteStore(dbPath)
	require.NoError(t, err)

	t.Cleanup(func() {
		store.Close()
	})

	return store
}
