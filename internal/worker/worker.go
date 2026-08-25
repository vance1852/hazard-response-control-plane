package worker

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"
)

type Job struct {
	ID, Kind, AggregateType, AggregateID, Payload, Status, LeaseOwner string
	Attempts, MaxAttempts                                             int
	AvailableAt, LeaseUntil                                           time.Time
	LastError                                                         string
}
type Repository interface {
	Claim(context.Context, string, time.Time, time.Duration, int) ([]Job, error)
	RecordAttempt(context.Context, Job, string, time.Time) (int64, error)
	Finish(context.Context, Job, string, error, time.Time) error
	RecoverExpired(context.Context, time.Time) error
}
type Handler func(context.Context, Job) error
type Worker struct {
	repo        Repository
	handlers    map[string]Handler
	poll, lease time.Duration
	batch       int
	id          string
	now         func() time.Time
	wg          sync.WaitGroup
}

func New(repo Repository, id string, poll, lease time.Duration, batch int) (*Worker, error) {
	if repo == nil || id == "" || poll <= 0 || lease <= poll || batch < 1 {
		return nil, fmt.Errorf("invalid worker configuration")
	}
	return &Worker{repo: repo, id: id, poll: poll, lease: lease, batch: batch, handlers: map[string]Handler{}, now: func() time.Time { return time.Now().UTC() }}, nil
}
func (w *Worker) Register(kind string, h Handler) { w.handlers[kind] = h }
func (w *Worker) Run(ctx context.Context) {
	w.wg.Add(1)
	defer w.wg.Done()
	ticker := time.NewTicker(w.poll)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.tick(ctx)
		}
	}
}
func (w *Worker) Stop() { w.wg.Wait() }
func (w *Worker) tick(ctx context.Context) {
	now := w.now()
	_ = w.repo.RecoverExpired(ctx, now)
	jobs, e := w.repo.Claim(ctx, w.id, now, w.lease, w.batch)
	if e != nil {
		return
	}
	for _, job := range jobs {
		if ctx.Err() != nil {
			// Worker run context was cancelled: stop dispatching new handlers.
			// Already-running handlers observe the cancellation through their
			// own context and exit promptly, leaving their jobs retryable.
			break
		}
		w.execute(ctx, job)
	}
}
func (w *Worker) execute(ctx context.Context, job Job) {
	// Persist attempt records on a detached context so the retry trail
	// survives even when the worker run context is cancelled mid-handler.
	recordCtx := bookkeepingContext()
	handler, ok := w.handlers[job.Kind]
	if !ok {
		_ = w.repo.Finish(recordCtx, job, "permanent", fmt.Errorf("no handler for %s", job.Kind), w.now())
		return
	}
	_, _ = w.repo.RecordAttempt(recordCtx, job, w.id, w.now())
	// Derive the handler context from the worker run context so that a
	// shutdown or leadership-transfer cancellation propagates to in-flight
	// handlers. They can observe ctx.Done() and return promptly instead of
	// blocking the stop flow or publishing stale alarms after leadership has
	// moved to another instance.
	handlerCtx, release := handlerContext(ctx)
	defer release()
	err := handler(handlerCtx, job)
	if err == nil {
		_ = w.repo.Finish(recordCtx, job, "succeeded", nil, w.now())
		return
	}
	// Cancellation (or an expired deadline) is never promoted to a permanent
	// failure: the job stays retryable so the next leader can re-attempt
	// delivery, and the attempt record above is preserved.
	canceled := errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
	outcome := "retryable"
	if !canceled && job.Attempts+1 >= job.MaxAttempts {
		outcome = "permanent"
	}
	_ = w.repo.Finish(recordCtx, job, outcome, err, w.now())
}
func Backoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	seconds := math.Min(300, math.Pow(2, float64(attempt)))
	return time.Duration(seconds) * time.Second
}

type SQLRepository struct{ DB *sql.DB }

func (r SQLRepository) Claim(ctx context.Context, workerID string, now time.Time, lease time.Duration, batch int) ([]Job, error) {
	tx, e := r.DB.BeginTx(ctx, nil)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	rows, e := tx.QueryContext(ctx, `SELECT id,kind,aggregate_type,aggregate_id,payload_json,status,attempts,max_attempts,available_at,COALESCE(lease_until,''),COALESCE(last_error,'') FROM jobs WHERE (status IN ('pending','retry_wait') OR (status='running' AND lease_until<=?)) AND available_at<=? ORDER BY available_at,id LIMIT ?`, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), batch)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Job{}
	for rows.Next() {
		var j Job
		var a, l string
		if e := rows.Scan(&j.ID, &j.Kind, &j.AggregateType, &j.AggregateID, &j.Payload, &j.Status, &j.Attempts, &j.MaxAttempts, &a, &l, &j.LastError); e != nil {
			return nil, e
		}
		j.AvailableAt, _ = time.Parse(time.RFC3339Nano, a)
		if l != "" {
			j.LeaseUntil, _ = time.Parse(time.RFC3339Nano, l)
		}
		out = append(out, j)
	}
	for _, j := range out {
		_, e = tx.ExecContext(ctx, `UPDATE jobs SET status='running',lease_owner=?,lease_until=?,attempts=attempts+1,version=version+1,updated_at=? WHERE id=?`, workerID, now.Add(lease).Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), j.ID)
		if e != nil {
			return nil, e
		}
	}
	if e = tx.Commit(); e != nil {
		return nil, e
	}
	return out, nil
}
func (r SQLRepository) RecordAttempt(ctx context.Context, j Job, worker string, now time.Time) (int64, error) {
	id, _ := newJobID()
	res, e := r.DB.ExecContext(ctx, `INSERT INTO job_attempts(id,job_id,attempt_number,worker_id,started_at) VALUES(?,?,?,?,?)`, id, j.ID, j.Attempts+1, worker, now.Format(time.RFC3339Nano))
	if e != nil {
		return 0, e
	}
	return res.LastInsertId()
}
func (r SQLRepository) Finish(ctx context.Context, j Job, outcome string, cause error, now time.Time) error {
	status := "succeeded"
	if outcome == "retryable" {
		status = "retry_wait"
	}
	if outcome == "permanent" {
		status = "permanently_failed"
	}
	next := now
	if status == "retry_wait" {
		next = now.Add(Backoff(j.Attempts))
	}
	_, e := r.DB.ExecContext(ctx, `UPDATE jobs SET status=?,available_at=?,lease_owner=NULL,lease_until=NULL,last_error=?,updated_at=? WHERE id=?`, status, next.Format(time.RFC3339Nano), errorString(cause), now.Format(time.RFC3339Nano), j.ID)
	return e
}
func (r SQLRepository) RecoverExpired(ctx context.Context, now time.Time) error {
	_, e := r.DB.ExecContext(ctx, `UPDATE jobs SET status='retry_wait',lease_owner=NULL,lease_until=NULL,available_at=?,updated_at=? WHERE status='running' AND lease_until<=?`, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	return e
}
func errorString(e error) string {
	if e == nil {
		return ""
	}
	return e.Error()
}
func newJobID() (string, error) { return fmt.Sprintf("jobattempt_%d", time.Now().UnixNano()), nil }
