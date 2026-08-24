package sqlite

import (
 "context"
 "fmt"

 "github.com/vance1852/hazard-response-control-plane/internal/hazard"
)

func (s *Store) appendActivationOutbox(ctx context.Context, outboxID, incidentID string, record hazard.ActivationRecord) error {
 _, err := s.db.ExecContext(ctx, `INSERT INTO outbox_events(id, topic, aggregate_type, aggregate_id, payload_json,
  status, attempts, available_at, created_at) VALUES(?, ?, 'incident', ?, ?, 'pending', 0, ?, ?)`, outboxID,
  record.Topic, incidentID, record.Payload, formatTime(record.Now), formatTime(record.Now))
 if err != nil {
  return fmt.Errorf("insert incident outbox event: %w", err)
 }
 return nil
}
