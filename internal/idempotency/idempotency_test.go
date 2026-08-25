package idempotency

import (
	"context"
	"github.com/vance1852/hazard-response-control-plane/internal/apperr"
	"github.com/vance1852/hazard-response-control-plane/internal/clock"
	"github.com/vance1852/hazard-response-control-plane/internal/identity"
	"testing"
	"time"
)

type memoryIdempotency struct {
	records map[string]Record
	next    int
}

func (m *memoryIdempotency) Begin(_ context.Context, r Record) (Record, bool, error) {
	key := r.ActorID + "|" + r.Method + "|" + r.Route + "|" + r.Key
	if old, ok := m.records[key]; ok {
		if string(old.Hash) != string(r.Hash) {
			return Record{}, false, apperr.Conflict("idempotency_key_reused", "different body")
		}
		return old, true, nil
	}
	m.next++
	r.ID = string(rune('a' + m.next))
	m.records[key] = r
	return r, false, nil
}
func (m *memoryIdempotency) Complete(_ context.Context, id string, code int, body []byte, now time.Time) error {
	for key, r := range m.records {
		if r.ID == id {
			r.Status = "completed"
			r.ResponseCode = code
			r.ResponseBody = body
			r.UpdatedAt = now
			m.records[key] = r
			return nil
		}
	}
	return apperr.NotFound("record_not_found", "missing")
}
func (m *memoryIdempotency) Fail(_ context.Context, id string, now time.Time) error {
	for key, r := range m.records {
		if r.ID == id {
			r.Status = "failed"
			r.UpdatedAt = now
			m.records[key] = r
			return nil
		}
	}
	return nil
}
func (m *memoryIdempotency) Expire(_ context.Context, now time.Time, limit int) (int, error) {
	n := 0
	for key, r := range m.records {
		if n >= limit {
			break
		}
		if !now.Before(r.ExpiresAt) {
			delete(m.records, key)
			n++
		}
	}
	return n, nil
}
func newIdem(t *testing.T) (*Service, *memoryIdempotency, *clock.Manual) {
	t.Helper()
	repo := &memoryIdempotency{records: map[string]Record{}}
	clk := clock.NewManual(time.Date(2026, 8, 24, 8, 0, 0, 0, time.UTC))
	svc, err := NewService(repo, clk, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return svc, repo, clk
}
func TestHashBodyIsStable(t *testing.T) {
	a, _ := HashBody(map[string]any{"name": "quake", "severity": 4})
	b, _ := HashBody(map[string]any{"name": "quake", "severity": 4})
	if string(a) != string(b) {
		t.Fatal("hash is not stable")
	}
}
func TestBeginAndCompleteLifecycle(t *testing.T) {
	svc, repo, _ := newIdem(t)
	actor := identity.Actor{UserID: "u"}
	hash, _ := HashBody(map[string]string{"x": "y"})
	record, done, err := svc.Begin(context.Background(), actor, "POST", "/v1/incidents", "key", hash)
	if err != nil || done || record.ID == "" {
		t.Fatalf("begin=%#v %v %v", record, done, err)
	}
	if err := svc.Complete(context.Background(), record, 201, []byte("{}")); err != nil {
		t.Fatal(err)
	}
	again, done, err := svc.Begin(context.Background(), actor, "POST", "/v1/incidents", "key", hash)
	if err != nil || !done || again.Status != "completed" {
		t.Fatalf("again=%#v %v %v", again, done, err)
	}
	_ = repo
}
func TestChangedBodyCannotReuseKey(t *testing.T) {
	svc, repo, _ := newIdem(t)
	actor := identity.Actor{UserID: "u"}
	first, _ := HashBody(map[string]int{"severity": 3})
	second, _ := HashBody(map[string]int{"severity": 5})
	_, _, err := svc.Begin(context.Background(), actor, "POST", "/v1/incidents", "key", first)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = svc.Begin(context.Background(), actor, "POST", "/v1/incidents", "key", second)
	if !apperr.IsKind(err, apperr.KindConflict) {
		t.Fatalf("error=%v", err)
	}
	_ = repo
}
func TestIdempotencyRequiresAuthenticatedActor(t *testing.T) {
	svc, _, _ := newIdem(t)
	_, _, err := svc.Begin(context.Background(), identity.Actor{}, "POST", "/v1", "key", []byte("x"))
	if !apperr.IsKind(err, apperr.KindUnauthenticated) {
		t.Fatalf("error=%v", err)
	}
}
func TestIdempotencyRequiresKey(t *testing.T) {
	svc, _, _ := newIdem(t)
	_, _, err := svc.Begin(context.Background(), identity.Actor{UserID: "u"}, "POST", "/v1", "", []byte("x"))
	if !apperr.IsKind(err, apperr.KindValidation) {
		t.Fatalf("error=%v", err)
	}
}
func TestFailureReleasesProcessingRecord(t *testing.T) {
	svc, repo, clk := newIdem(t)
	record, _, err := svc.Begin(context.Background(), identity.Actor{UserID: "u"}, "POST", "/v1", "key", []byte("x"))
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Fail(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	if status := repoRecordByID(repo, record.ID).Status; status != "failed" {
		t.Fatalf("status=%q want failed", status)
	}
	clk.Advance(2 * time.Hour)
	count, err := repo.Expire(context.Background(), clk.Now(), 10)
	if err != nil || count != 1 {
		t.Fatalf("count=%d err=%v", count, err)
	}
}

// TestFailOnlyReleasesOwnRecord guards against Fail using the caller identity
// (ActorID) instead of the request record id: when the wrong id is used, the
// own processing record stays stuck and blocks retries, while a sibling record
// for the same actor must remain untouched.
func TestFailOnlyReleasesOwnRecord(t *testing.T) {
	svc, repo, _ := newIdem(t)
	actor := identity.Actor{UserID: "u"}
	own, _, err := svc.Begin(context.Background(), actor, "POST", "/v1", "own", []byte("x"))
	if err != nil {
		t.Fatal(err)
	}
	sibling, _, err := svc.Begin(context.Background(), actor, "POST", "/v1", "sibling", []byte("x"))
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.Fail(context.Background(), own); err != nil {
		t.Fatal(err)
	}

	if status := repoRecordByID(repo, own.ID).Status; status != "failed" {
		t.Fatalf("own status=%q want failed", status)
	}
	if status := repoRecordByID(repo, sibling.ID).Status; status != "processing" {
		t.Fatalf("sibling status=%q want processing", status)
	}
}

func repoRecordByID(m *memoryIdempotency, id string) Record {
	for _, r := range m.records {
		if r.ID == id {
			return r
		}
	}
	return Record{Status: "missing"}
}
