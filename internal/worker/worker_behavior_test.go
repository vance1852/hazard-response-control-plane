package worker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type scriptedRepository struct {
	mu        sync.Mutex
	job       Job
	claims    int
	finished  []string
	recovered int
}

func (r *scriptedRepository) Claim(_ context.Context, _ string, now time.Time, _ time.Duration, _ int) ([]Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.job.Status != "pending" || r.job.AvailableAt.After(now) {
		return nil, nil
	}
	r.job.Status = "running"
	r.job.Attempts++
	r.claims++
	return []Job{r.job}, nil
}
func (r *scriptedRepository) RecordAttempt(context.Context, Job, string, time.Time) (int64, error) {
	return 1, nil
}
func (r *scriptedRepository) Finish(_ context.Context, _ Job, outcome string, _ error, _ time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.finished = append(r.finished, outcome)
	r.job.Status = outcome
	return nil
}
func (r *scriptedRepository) RecoverExpired(context.Context, time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.recovered++
	return nil
}
func TestWorkerHandlerReceivesContextAndPayload(t *testing.T) {
	repo := &scriptedRepository{job: Job{ID: "j", Kind: "sync", Payload: `{"incident":"i"}`, Status: "pending", MaxAttempts: 1, AvailableAt: time.Now().UTC()}}
	w, err := New(repo, "w", time.Millisecond, 20*time.Millisecond, 1)
	if err != nil {
		t.Fatal(err)
	}
	seen := make(chan string, 1)
	w.Register("sync", func(ctx context.Context, j Job) error {
		if ctx == nil {
			return errors.New("nil context")
		}
		seen <- j.Payload
		return nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	go w.Run(ctx)
	select {
	case payload := <-seen:
		if payload != repo.job.Payload {
			t.Fatalf("payload=%s", payload)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("handler not called")
	}
	cancel()
	w.Stop()
	if len(repo.finished) != 1 || repo.finished[0] != "succeeded" {
		t.Fatalf("finished=%v", repo.finished)
	}
}
// TestWorkerCancellationPropagatesToHandler ensures that when the worker run
// context is cancelled (shutdown or leadership transfer) an in-flight handler
// observes the cancellation through its own context and exits promptly instead
// of continuing downstream delivery. The job is left retryable and the attempt
// is recorded so the next leader can re-attempt.
func TestWorkerCancellationPropagatesToHandler(t *testing.T) {
	repo := &scriptedRepository{job: Job{ID: "j", Kind: "sync", Status: "pending", Attempts: 0, MaxAttempts: 3, AvailableAt: time.Now().UTC()}}
	w, err := New(repo, "w", time.Millisecond, 20*time.Millisecond, 1)
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{}, 1)
	canceled := make(chan struct{}, 1)
	w.Register("sync", func(ctx context.Context, _ Job) error {
		started <- struct{}{}
		defer close(canceled)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
			return errors.New("handler was not cancelled")
		}
	})
	runCtx, cancel := context.WithCancel(context.Background())
	go w.Run(runCtx)
	<-started
	cancel() // simulate shutdown / leadership transfer
	select {
	case <-canceled:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not observe worker cancellation")
	}
	w.Stop()
	if len(repo.finished) == 0 || repo.finished[0] != "retryable" {
		t.Fatalf("cancelled job must stay retryable, got finished=%v", repo.finished)
	}
	if repo.job.Attempts == 0 {
		t.Fatal("attempt record was not preserved")
	}
}

func TestWorkerCancellationStopsPolling(t *testing.T) {
	repo := &scriptedRepository{job: Job{ID: "j", Kind: "sync", Status: "pending", MaxAttempts: 1, AvailableAt: time.Now().Add(time.Hour)}}
	w, _ := New(repo, "w", time.Millisecond, 20*time.Millisecond, 1)
	ctx, cancel := context.WithCancel(context.Background())
	go w.Run(ctx)
	time.Sleep(10 * time.Millisecond)
	cancel()
	w.Stop()
	before := repo.recovered
	time.Sleep(5 * time.Millisecond)
	if repo.recovered > before+1 {
		t.Fatalf("worker kept polling after stop: %d", repo.recovered)
	}
}
func TestBackoffHandlesNonPositiveAttempt(t *testing.T) {
	if Backoff(0) <= 0 || Backoff(-1) <= 0 {
		t.Fatal("non-positive attempt produced invalid delay")
	}
}
