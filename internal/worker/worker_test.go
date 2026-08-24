package worker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type memoryJobs struct {
	mu       sync.Mutex
	jobs     []Job
	attempts int
	outcomes []string
}

func (m *memoryJobs) Claim(_ context.Context, _ string, now time.Time, _ time.Duration, batch int) ([]Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []Job{}
	for i := range m.jobs {
		if len(out) >= batch {
			break
		}
		if m.jobs[i].Status == "pending" && !m.jobs[i].AvailableAt.After(now) {
			m.jobs[i].Status = "running"
			m.jobs[i].Attempts++
			out = append(out, m.jobs[i])
		}
	}
	return out, nil
}
func (m *memoryJobs) RecordAttempt(_ context.Context, j Job, _ string, _ time.Time) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.attempts++
	return int64(m.attempts), nil
}
func (m *memoryJobs) Finish(_ context.Context, j Job, outcome string, _ error, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.outcomes = append(m.outcomes, outcome)
	for i := range m.jobs {
		if m.jobs[i].ID == j.ID {
			m.jobs[i].Status = outcome
			m.jobs[i].AvailableAt = now
		}
	}
	return nil
}
func (m *memoryJobs) RecoverExpired(context.Context, time.Time) error { return nil }
func TestBackoffIsBounded(t *testing.T) {
	if Backoff(1) != 2*time.Second {
		t.Fatalf("backoff1=%v", Backoff(1))
	}
	if Backoff(20) > 5*time.Minute {
		t.Fatalf("backoff exceeded bound")
	}
}
func TestWorkerRetriesAndStops(t *testing.T) {
	repo := &memoryJobs{jobs: []Job{{ID: "job", Kind: "sync", Status: "pending", Attempts: 0, MaxAttempts: 2, AvailableAt: time.Now().UTC()}}}
	w, err := New(repo, "worker", time.Millisecond, 50*time.Millisecond, 4)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	w.Register("sync", func(context.Context, Job) error {
		calls++
		if calls == 1 {
			return errors.New("temporary")
		}
		return nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	go w.Run(ctx)
	time.Sleep(20 * time.Millisecond)
	cancel()
	w.Stop()
	if repo.attempts == 0 {
		t.Fatal("worker did not claim job")
	}
}
func TestWorkerUnknownKindIsPermanent(t *testing.T) {
	repo := &memoryJobs{jobs: []Job{{ID: "job", Kind: "unknown", Status: "pending", MaxAttempts: 1, AvailableAt: time.Now().UTC()}}}
	w, _ := New(repo, "worker", time.Millisecond, 50*time.Millisecond, 4)
	ctx, cancel := context.WithCancel(context.Background())
	go w.Run(ctx)
	time.Sleep(10 * time.Millisecond)
	cancel()
	w.Stop()
	found := false
	for _, outcome := range repo.outcomes {
		if outcome == "permanent" {
			found = true
		}
	}
	if !found {
		t.Fatal("unknown job was not permanently failed")
	}
}
