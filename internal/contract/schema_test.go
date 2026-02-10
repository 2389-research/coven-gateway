// ABOUTME: Contract tests for database schema to detect breaking schema changes.
// ABOUTME: Validates that expected tables and columns exist in SQLite database.

package contract

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/2389/coven-gateway/internal/store"
)

// expectedSchema defines the contract for our database schema.
// If a table or column is removed or renamed, these tests will fail,
// catching breaking changes before they reach production.
var expectedSchema = map[string][]string{
	"threads": {
		"id", "frontend_name", "external_id",
		"agent_id", "created_at", "updated_at",
	},
	"messages": {
		"id", "thread_id", "sender",
		"content", "created_at",
	},
	"agent_state": {
		"agent_id", "state", "updated_at",
	},
	"channel_bindings": {
		"frontend", "channel_id", "agent_id",
		"created_at", "updated_at",
	},
	"principals": {
		"principal_id", "type", "pubkey_fingerprint",
		"display_name", "status", "created_at",
		"last_seen", "metadata_json",
	},
	"roles": {
		"subject_type", "subject_id", "role", "created_at",
	},
	"audit_log": {
		"audit_id", "actor_principal_id", "actor_member_id",
		"action", "target_type", "target_id", "ts", "detail_json",
	},
	"ledger_events": {
		"event_id", "conversation_key", "direction",
		"author", "timestamp", "type", "text",
		"raw_transport", "raw_payload_ref",
		"actor_principal_id", "actor_member_id",
	},
	"bindings": {
		"binding_id", "frontend", "channel_id",
		"agent_id", "created_at", "created_by",
	},
}

// setupTestDB creates a temporary SQLite database with the production schema.
func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "contract_test.db")

	// Use the store package to create the database with proper schema
	sqliteStore, err := store.NewSQLiteStore(dbPath)
	require.NoError(t, err, "failed to create SQLite store")

	// Get the underlying DB connection
	// We need to open a new connection since the store owns its connection
	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err, "failed to open database")

	t.Cleanup(func() {
		db.Close()
		sqliteStore.Close()
	})

	return db
}

// getTableColumns queries SQLite to get column names for a table.
func getTableColumns(ctx context.Context, db *sql.DB, tableName string) (map[string]bool, error) {
	query := fmt.Sprintf("PRAGMA table_info(%s)", tableName)
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("querying table info: %w", err)
	}
	defer rows.Close()

	columns := make(map[string]bool)
	for rows.Next() {
		var (
			cid       int
			name      string
			colType   string
			notNull   int
			dfltValue sql.NullString
			pk        int
		)
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dfltValue, &pk); err != nil {
			return nil, fmt.Errorf("scanning column info: %w", err)
		}
		columns[name] = true
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating columns: %w", err)
	}

	return columns, nil
}

// TestSchemaSurface verifies that all expected tables and columns exist
// in the database schema. This acts as a contract test to prevent
// accidental breaking changes to the database structure.
func TestSchemaSurface(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	for table, expectedCols := range expectedSchema {
		t.Run(table, func(t *testing.T) {
			actualCols, err := getTableColumns(ctx, db, table)
			if !assert.NoError(t, err, "failed to get columns for table %s", table) {
				return
			}

			// Table should have at least one column (means it exists)
			if !assert.NotEmpty(t, actualCols, "table %s should exist and have columns", table) {
				return
			}

			// Verify each expected column exists
			for _, col := range expectedCols {
				assert.True(t, actualCols[col],
					"column %s.%s should exist", table, col)
			}

			// Report any extra columns not in contract (informational, not failure)
			for col := range actualCols {
				found := slices.Contains(expectedCols, col)
				if !found {
					t.Logf("INFO: extra column %s.%s not in contract (consider adding)", table, col)
				}
			}
		})
	}
}

// TestTablesExist is a quick sanity check that all expected tables exist.
func TestTablesExist(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	// Query SQLite master table for all tables
	rows, err := db.QueryContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'")
	require.NoError(t, err, "failed to query tables")
	defer rows.Close()

	actualTables := make(map[string]bool)
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name), "failed to scan table name")
		actualTables[name] = true
	}
	require.NoError(t, rows.Err(), "error iterating tables")

	// Verify all expected tables exist
	for table := range expectedSchema {
		assert.True(t, actualTables[table], "table %s should exist", table)
	}
}

// TestSchemaHasIndexes verifies that critical indexes exist for performance.
func TestSchemaHasIndexes(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	// Expected indexes that are critical for query performance
	expectedIndexes := []string{
		"idx_threads_frontend_external",
		"idx_messages_thread_id",
		"idx_messages_thread_created",
		"idx_principals_status",
		"idx_principals_type",
		"idx_principals_pubkey",
		"idx_roles_subject",
		"idx_audit_ts",
		"idx_audit_actor",
		"idx_audit_target",
		"idx_ledger_conversation",
		"idx_ledger_actor",
		"idx_ledger_timestamp",
		"idx_bindings_frontend",
		"idx_bindings_agent",
	}

	// Query SQLite master for all indexes
	rows, err := db.QueryContext(ctx, "SELECT name FROM sqlite_master WHERE type='index' AND name NOT LIKE 'sqlite_%'")
	require.NoError(t, err, "failed to query indexes")
	defer rows.Close()

	actualIndexes := make(map[string]bool)
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name), "failed to scan index name")
		actualIndexes[name] = true
	}
	require.NoError(t, rows.Err(), "error iterating indexes")

	// Verify expected indexes exist
	for _, idx := range expectedIndexes {
		assert.True(t, actualIndexes[idx], "index %s should exist", idx)
	}
}
