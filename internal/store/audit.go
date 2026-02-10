// ABOUTME: Audit log entity and store methods for tracking administrative actions
// ABOUTME: Records who did what to which resource for compliance and debugging

package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// AuditAction represents an auditable action.
type AuditAction string

const (
	AuditApprovePrincipal AuditAction = "approve_principal"
	AuditRevokePrincipal  AuditAction = "revoke_principal"
	AuditGrantCapability  AuditAction = "grant_capability"
	AuditRevokeCapability AuditAction = "revoke_capability"
	AuditCreateBinding    AuditAction = "create_binding"
	AuditUpdateBinding    AuditAction = "update_binding"
	AuditDeleteBinding    AuditAction = "delete_binding"
	AuditCreateToken      AuditAction = "create_token"
	AuditCreatePrincipal  AuditAction = "create_principal"
	AuditDeletePrincipal  AuditAction = "delete_principal"
)

// ValidAuditActions lists all valid audit actions.
var ValidAuditActions = []AuditAction{
	AuditApprovePrincipal,
	AuditRevokePrincipal,
	AuditGrantCapability,
	AuditRevokeCapability,
	AuditCreateBinding,
	AuditUpdateBinding,
	AuditDeleteBinding,
	AuditCreateToken,
	AuditCreatePrincipal,
	AuditDeletePrincipal,
}

// AuditEntry represents a single audit log entry.
type AuditEntry struct {
	ID               string         // UUID v4
	ActorPrincipalID string         // who performed the action
	ActorMemberID    *string        // associated member (nil in v1)
	Action           AuditAction    // what action was performed
	TargetType       string         // "principal", "capability", "binding"
	TargetID         string         // ID of the affected resource
	Timestamp        time.Time      // when it happened
	Detail           map[string]any // additional context (max 64KB JSON)
}

// AuditFilter specifies filtering options for listing audit entries.
type AuditFilter struct {
	Since            *time.Time   // entries after this time
	Until            *time.Time   // entries before this time
	ActorPrincipalID *string      // filter by actor
	Action           *AuditAction // filter by action type
	TargetType       *string      // filter by target type
	TargetID         *string      // filter by target ID
	Limit            int          // max results (default 100, max 1000)
}

// AppendAuditLog appends a new entry to the audit log.
// Generates ID and Timestamp if not set.
func (s *SQLiteStore) AppendAuditLog(ctx context.Context, e *AuditEntry) error {
	// Generate ID if not set
	if e.ID == "" {
		e.ID = uuid.New().String()
	}

	// Generate timestamp if not set
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now().UTC()
	}

	var detailJSON *string
	if e.Detail != nil {
		data, err := json.Marshal(e.Detail)
		if err != nil {
			return fmt.Errorf("marshaling audit detail: %w", err)
		}
		str := string(data)
		detailJSON = &str
	}

	query := `
		INSERT INTO audit_log (audit_id, actor_principal_id, actor_member_id, action, target_type, target_id, ts, detail_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := s.db.ExecContext(ctx, query,
		e.ID,
		e.ActorPrincipalID,
		e.ActorMemberID,
		e.Action,
		e.TargetType,
		e.TargetID,
		e.Timestamp.UTC().Format(time.RFC3339),
		detailJSON,
	)
	if err != nil {
		return fmt.Errorf("inserting audit entry: %w", err)
	}

	s.logger.Debug("appended audit log",
		"id", e.ID,
		"actor", e.ActorPrincipalID,
		"action", e.Action,
		"target", e.TargetType+"/"+e.TargetID,
	)
	return nil
}

// normalizeAuditLimit applies default (100) and cap (1000) to audit limit.
func normalizeAuditLimit(limit int) int {
	switch {
	case limit <= 0:
		return 100
	case limit > 1000:
		return 1000
	default:
		return limit
	}
}

// auditQueryArgs builds the query arguments from an AuditFilter.
type auditQueryArgs struct {
	sinceStr  *string
	untilStr  *string
	actionStr *string
}

// buildAuditQueryArgs converts filter time/action fields to query args.
func buildAuditQueryArgs(f AuditFilter) auditQueryArgs {
	var args auditQueryArgs
	if f.Since != nil {
		s := f.Since.UTC().Format(time.RFC3339)
		args.sinceStr = &s
	}
	if f.Until != nil {
		s := f.Until.UTC().Format(time.RFC3339)
		args.untilStr = &s
	}
	if f.Action != nil {
		a := string(*f.Action)
		args.actionStr = &a
	}
	return args
}

// scanAuditEntry scans a row into an AuditEntry.
func scanAuditEntry(scanner interface{ Scan(dest ...any) error }) (AuditEntry, error) {
	var e AuditEntry
	var actionStr, tsStr string
	var detailJSON *string

	if err := scanner.Scan(
		&e.ID,
		&e.ActorPrincipalID,
		&e.ActorMemberID,
		&actionStr,
		&e.TargetType,
		&e.TargetID,
		&tsStr,
		&detailJSON,
	); err != nil {
		return e, fmt.Errorf("scanning audit entry: %w", err)
	}

	e.Action = AuditAction(actionStr)
	var err error
	e.Timestamp, err = time.Parse(time.RFC3339, tsStr)
	if err != nil {
		return e, fmt.Errorf("parsing timestamp: %w", err)
	}

	if detailJSON != nil {
		if err := json.Unmarshal([]byte(*detailJSON), &e.Detail); err != nil {
			return e, fmt.Errorf("unmarshaling detail: %w", err)
		}
	}
	return e, nil
}

const auditLogQuery = `
	SELECT audit_id, actor_principal_id, actor_member_id, action, target_type, target_id, ts, detail_json
	FROM audit_log
	WHERE (? IS NULL OR ts >= ?)
	  AND (? IS NULL OR ts <= ?)
	  AND (? IS NULL OR actor_principal_id = ?)
	  AND (? IS NULL OR action = ?)
	  AND (? IS NULL OR target_type = ?)
	  AND (? IS NULL OR target_id = ?)
	ORDER BY ts DESC
	LIMIT ?
`

// ListAuditLog returns audit entries matching the filter criteria.
// Results are returned newest first (DESC by timestamp).
func (s *SQLiteStore) ListAuditLog(ctx context.Context, f AuditFilter) ([]AuditEntry, error) {
	limit := normalizeAuditLimit(f.Limit)
	args := buildAuditQueryArgs(f)

	rows, err := s.db.QueryContext(ctx, auditLogQuery,
		args.sinceStr, args.sinceStr,
		args.untilStr, args.untilStr,
		f.ActorPrincipalID, f.ActorPrincipalID,
		args.actionStr, args.actionStr,
		f.TargetType, f.TargetType,
		f.TargetID, f.TargetID,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("querying audit log: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var entries []AuditEntry
	for rows.Next() {
		e, err := scanAuditEntry(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating audit entries: %w", err)
	}

	if entries == nil {
		entries = []AuditEntry{}
	}
	return entries, nil
}
