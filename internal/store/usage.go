// ABOUTME: SQLite implementation for token usage tracking
// ABOUTME: Stores and retrieves LLM token consumption data for analytics

package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// SaveUsage stores a token usage record.
func (s *SQLiteStore) SaveUsage(ctx context.Context, usage *TokenUsage) error {
	query := `
		INSERT INTO message_usage (
			id, thread_id, message_id, request_id, agent_id,
			input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, thinking_tokens,
			created_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := s.db.ExecContext(ctx, query,
		usage.ID,
		usage.ThreadID,
		nullString(usage.MessageID),
		usage.RequestID,
		usage.AgentID,
		usage.InputTokens,
		usage.OutputTokens,
		usage.CacheReadTokens,
		usage.CacheWriteTokens,
		usage.ThinkingTokens,
		usage.CreatedAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("inserting usage: %w", err)
	}

	s.logger.Debug("saved token usage",
		"id", usage.ID,
		"thread_id", usage.ThreadID,
		"agent_id", usage.AgentID,
		"input_tokens", usage.InputTokens,
		"output_tokens", usage.OutputTokens,
	)
	return nil
}

// LinkUsageToMessage updates a usage record with the final message ID.
func (s *SQLiteStore) LinkUsageToMessage(ctx context.Context, requestID, messageID string) error {
	query := `UPDATE message_usage SET message_id = ? WHERE request_id = ?`

	result, err := s.db.ExecContext(ctx, query, messageID, requestID)
	if err != nil {
		return fmt.Errorf("linking usage to message: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("getting rows affected: %w", err)
	}

	s.logger.Debug("linked usage to message",
		"request_id", requestID,
		"message_id", messageID,
		"rows_affected", rowsAffected,
	)
	return nil
}

// GetThreadUsage retrieves all usage records for a thread.
func (s *SQLiteStore) GetThreadUsage(ctx context.Context, threadID string) ([]*TokenUsage, error) {
	query := `
		SELECT id, thread_id, message_id, request_id, agent_id,
		       input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, thinking_tokens,
		       created_at
		FROM message_usage
		WHERE thread_id = ?
		ORDER BY created_at ASC
	`

	rows, err := s.db.QueryContext(ctx, query, threadID)
	if err != nil {
		return nil, fmt.Errorf("querying thread usage: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var usages []*TokenUsage
	for rows.Next() {
		usage, err := scanUsage(rows)
		if err != nil {
			return nil, err
		}
		usages = append(usages, usage)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating usage rows: %w", err)
	}

	return usages, nil
}

// GetUsageStats returns aggregated usage statistics with optional filters.
func (s *SQLiteStore) GetUsageStats(ctx context.Context, filter UsageFilter) (*UsageStats, error) {
	query := `
		SELECT
			COALESCE(SUM(input_tokens), 0) as total_input,
			COALESCE(SUM(output_tokens), 0) as total_output,
			COALESCE(SUM(cache_read_tokens), 0) as total_cache_read,
			COALESCE(SUM(cache_write_tokens), 0) as total_cache_write,
			COALESCE(SUM(thinking_tokens), 0) as total_thinking,
			COUNT(*) as request_count
		FROM message_usage
		WHERE 1=1
	`
	args := []any{}

	if filter.AgentID != nil {
		query += " AND agent_id = ?"
		args = append(args, *filter.AgentID)
	}
	if filter.Since != nil {
		query += " AND created_at >= ?"
		args = append(args, filter.Since.UTC().Format(time.RFC3339))
	}
	if filter.Until != nil {
		query += " AND created_at < ?"
		args = append(args, filter.Until.UTC().Format(time.RFC3339))
	}

	var stats UsageStats
	err := s.db.QueryRowContext(ctx, query, args...).Scan(
		&stats.TotalInput,
		&stats.TotalOutput,
		&stats.TotalCacheRead,
		&stats.TotalCacheWrite,
		&stats.TotalThinking,
		&stats.RequestCount,
	)
	if err != nil {
		return nil, fmt.Errorf("querying usage stats: %w", err)
	}

	// Calculate total tokens (input + output, excluding cache)
	stats.TotalTokens = stats.TotalInput + stats.TotalOutput + stats.TotalThinking

	return &stats, nil
}

// scanUsage scans a single usage row into a TokenUsage struct.
func scanUsage(rows *sql.Rows) (*TokenUsage, error) {
	var usage TokenUsage
	var messageID sql.NullString
	var createdAtStr string

	err := rows.Scan(
		&usage.ID,
		&usage.ThreadID,
		&messageID,
		&usage.RequestID,
		&usage.AgentID,
		&usage.InputTokens,
		&usage.OutputTokens,
		&usage.CacheReadTokens,
		&usage.CacheWriteTokens,
		&usage.ThinkingTokens,
		&createdAtStr,
	)
	if err != nil {
		return nil, fmt.Errorf("scanning usage row: %w", err)
	}

	if messageID.Valid {
		usage.MessageID = messageID.String
	}

	usage.CreatedAt, err = time.Parse(time.RFC3339, createdAtStr)
	if err != nil {
		return nil, fmt.Errorf("parsing created_at: %w", err)
	}

	return &usage, nil
}

// Ensure SQLiteStore implements UsageStore interface.
var _ UsageStore = (*SQLiteStore)(nil)
