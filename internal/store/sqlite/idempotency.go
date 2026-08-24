package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"github.com/vance1852/hazard-response-control-plane/internal/apperr"
	"github.com/vance1852/hazard-response-control-plane/internal/idempotency"
	"time"
)

func (s *Store) Begin(ctx context.Context, r idempotency.Record) (idempotency.Record, bool, error) {
	if r.ID == "" {
		id, e := newID("idem")
		if e != nil {
			return idempotency.Record{}, false, e
		}
		r.ID = id
	}
	_, e := s.db.ExecContext(ctx, `INSERT INTO idempotency_records(id,actor_id,method,route,idempotency_key,request_hash,status,expires_at,created_at,updated_at) VALUES(?,?,?,?,?,?,?, ?,?,?)`, r.ID, r.ActorID, r.Method, r.Route, r.Key, r.Hash, r.Status, formatTime(r.ExpiresAt), formatTime(r.UpdatedAt), formatTime(r.UpdatedAt))
	if e == nil {
		return r, false, nil
	}
	if !isUniqueViolation(e) {
		return idempotency.Record{}, false, e
	}
	var x idempotency.Record
	var hash []byte
	var exp, upd string
	var status string
	e = s.db.QueryRowContext(ctx, `SELECT id,actor_id,method,route,idempotency_key,request_hash,status,COALESCE(response_code,0),COALESCE(response_body,''),expires_at,updated_at FROM idempotency_records WHERE actor_id=? AND method=? AND route=? AND idempotency_key=?`, r.ActorID, r.Method, r.Route, r.Key).Scan(&x.ID, &x.ActorID, &x.Method, &x.Route, &x.Key, &hash, &status, &x.ResponseCode, &x.ResponseBody, &exp, &upd)
	if errors.Is(e, sql.ErrNoRows) {
		return idempotency.Record{}, false, apperr.Conflict("idempotency_race", "idempotency record conflicted")
	}
	if e != nil {
		return idempotency.Record{}, false, e
	}
	x.Hash = hash
	x.Status = status
	x.ExpiresAt, _ = parseTime(exp)
	x.UpdatedAt, _ = parseTime(upd)
	if x.Status == "completed" && x.ResponseBody == nil {
		x.ResponseBody = []byte{}
	}
	return x, true, nil
}
func (s *Store) Complete(ctx context.Context, id string, code int, body []byte, now time.Time) error {
	r, e := s.db.ExecContext(ctx, `UPDATE idempotency_records SET status='completed',response_code=?,response_body=?,updated_at=? WHERE id=? AND status='processing'`, code, body, formatTime(now), id)
	if e != nil {
		return e
	}
	n, _ := r.RowsAffected()
	if n != 1 {
		return apperr.Conflict("idempotency_state", "idempotency record is no longer processing")
	}
	return nil
}
func (s *Store) Fail(ctx context.Context, id string, now time.Time) error {
	_, e := s.db.ExecContext(ctx, `UPDATE idempotency_records SET status='failed',updated_at=? WHERE id=? AND status='processing'`, formatTime(now), id)
	return e
}
func (s *Store) Expire(ctx context.Context, now time.Time, limit int) (int, error) {
	r, e := s.db.ExecContext(ctx, `DELETE FROM idempotency_records WHERE expires_at<=? LIMIT ?`, formatTime(now), limit)
	if e != nil {
		return 0, e
	}
	n, _ := r.RowsAffected()
	return int(n), nil
}
