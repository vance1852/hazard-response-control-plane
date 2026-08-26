package sqlite

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/vance1852/hazard-response-control-plane/internal/apperr"
	"github.com/vance1852/hazard-response-control-plane/internal/audit"
)

func (s *Store) Append(ctx context.Context, event audit.Event) (audit.Event, error) {
	if event.ID == "" {
		id, err := newID("aud")
		if err != nil {
			return audit.Event{}, err
		}
		event.ID = id
	}
	if err := appendAudit(ctx, s.db, event); err != nil {
		return audit.Event{}, err
	}
	return event, nil
}

func appendAudit(ctx context.Context, query Querier, event audit.Event) error {
	_, err := query.ExecContext(ctx, `INSERT INTO audit_events(id, actor_id, action, object_type, object_id, request_id,
		outcome, detail_json, created_at) VALUES(?, NULLIF(?,''), ?, ?, ?, ?, ?, ?, ?)`, event.ID, event.ActorID,
		event.Action, event.ObjectType, event.ObjectID, event.RequestID, string(event.Outcome), string(event.Detail), formatTime(event.CreatedAt))
	if err != nil {
		return fmt.Errorf("insert audit event: %w", err)
	}
	return nil
}

func (s *Store) Search(ctx context.Context, filter audit.Filter) (audit.Page, error) {
	if err := ctx.Err(); err != nil {
		return audit.Page{}, fmt.Errorf("query audit events: %w", err)
	}
	if filter.Limit == 0 {
		filter.Limit = 50
	}
	if filter.Limit < 1 || filter.Limit > 200 {
		return audit.Page{}, apperr.Validation("invalid_limit", "audit limit must be between 1 and 200")
	}
	clauses := make([]string, 0, 8)
	args := make([]any, 0, 10)
	if filter.ActorID != "" {
		clauses = append(clauses, `actor_id = ?`)
		args = append(args, filter.ActorID)
	}
	if filter.ObjectType != "" {
		clauses = append(clauses, `object_type = ?`)
		args = append(args, filter.ObjectType)
	}
	if filter.ObjectID != "" {
		clauses = append(clauses, `object_id = ?`)
		args = append(args, filter.ObjectID)
	}
	if filter.Action != "" {
		clauses = append(clauses, `action = ?`)
		args = append(args, filter.Action)
	}
	if !filter.After.IsZero() {
		clauses = append(clauses, `created_at >= ?`)
		args = append(args, formatTime(filter.After))
	}
	if !filter.Before.IsZero() {
		clauses = append(clauses, `created_at < ?`)
		args = append(args, formatTime(filter.Before))
	}
	if filter.Cursor != "" {
		createdAt, id, err := decodeAuditCursor(filter.Cursor)
		if err != nil {
			return audit.Page{}, apperr.Validation("invalid_cursor", "audit cursor is invalid")
		}
		clauses = append(clauses, `(created_at < ? OR (created_at = ? AND id < ?))`)
		args = append(args, createdAt, createdAt, id)
	}
	where := ""
	if len(clauses) > 0 {
		where = ` WHERE ` + strings.Join(clauses, ` AND `)
	}
	args = append(args, filter.Limit+1)
	rows, err := s.db.QueryContext(ctx, `SELECT id, COALESCE(actor_id,''), action, object_type, object_id, request_id,
		outcome, detail_json, created_at FROM audit_events`+where+` ORDER BY created_at DESC, id DESC LIMIT ?`, args...)
	if err != nil {
		return audit.Page{}, fmt.Errorf("query audit events: %w", err)
	}
	defer rows.Close()
	events := make([]audit.Event, 0, filter.Limit+1)
	for rows.Next() {
		var event audit.Event
		var outcome, detail, createdAt string
		if err := rows.Scan(&event.ID, &event.ActorID, &event.Action, &event.ObjectType, &event.ObjectID,
			&event.RequestID, &outcome, &detail, &createdAt); err != nil {
			return audit.Page{}, fmt.Errorf("scan audit event: %w", err)
		}
		event.Outcome, event.Detail = audit.Outcome(outcome), []byte(detail)
		if event.CreatedAt, err = parseTime(createdAt); err != nil {
			return audit.Page{}, err
		}
		events = append(events, event)
		if err := ctx.Err(); err != nil {
			return audit.Page{}, fmt.Errorf("iterate audit events: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		return audit.Page{}, fmt.Errorf("iterate audit events: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return audit.Page{}, fmt.Errorf("query audit events: %w", err)
	}
	page := audit.Page{Events: events}
	if len(events) > filter.Limit {
		last := events[filter.Limit-1]
		page.Events = events[:filter.Limit]
		page.NextCursor = encodeAuditCursor(last.CreatedAt.String(), last.ID)
	}
	return page, nil
}

func encodeAuditCursor(createdAt, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(createdAt + "|" + id))
}

func decodeAuditCursor(cursor string) (string, string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return "", "", err
	}
	parts := strings.SplitN(string(raw), "|", 2)
	if len(parts) != 2 || parts[1] == "" {
		return "", "", errors.New("invalid audit cursor")
	}
	parsed, err := parseTime(parts[0])
	if err != nil {
		return "", "", err
	}
	return formatTime(parsed), parts[1], nil
}

var _ audit.Repository = (*Store)(nil)
var _ Querier = (*sql.Tx)(nil)
