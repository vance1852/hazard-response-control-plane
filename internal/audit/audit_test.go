package audit

import (
	"context"
	"github.com/vance1852/hazard-response-control-plane/internal/identity"
	"testing"
	"time"
)

type fakeAuditRepo struct{ events []Event }

func (r *fakeAuditRepo) Append(_ context.Context, e Event) (Event, error) {
	r.events = append(r.events, e)
	return e, nil
}
func (r *fakeAuditRepo) Search(_ context.Context, f Filter) (Page, error) {
	return Page{Events: r.events}, nil
}
func TestNewAuditEventMarshalsDetail(t *testing.T) {
	event, err := New("u", "incident.activate", "incident", "i", "req", OutcomeSucceeded, map[string]any{"severity": 4}, time.Now())
	if err != nil || len(event.Detail) == 0 {
		t.Fatalf("event=%#v err=%v", event, err)
	}
}
func TestAuditServiceRequiresPrivilegedRole(t *testing.T) {
	svc, _ := NewService(&fakeAuditRepo{})
	if _, err := svc.Search(context.Background(), identity.Actor{UserID: "u", Role: identity.RoleFieldOperator}, Filter{}); err == nil {
		t.Fatal("operator searched audit")
	}
}
func TestAuditServiceSearchesEvents(t *testing.T) {
	repo := &fakeAuditRepo{}
	event, _ := New("u", "x", "incident", "i", "req", OutcomeSucceeded, nil, time.Now())
	repo.events = []Event{event}
	svc, _ := NewService(repo)
	page, err := svc.Search(context.Background(), identity.Actor{UserID: "u", Role: identity.RoleAuditor}, Filter{Limit: 10})
	if err != nil || len(page.Events) != 1 {
		t.Fatalf("page=%#v err=%v", page, err)
	}
}
