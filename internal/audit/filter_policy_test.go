package audit

import (
	"context"
	"testing"

	"github.com/vance1852/hazard-response-control-plane/internal/identity"
)

// capturingRepo records the Filter it received from the service so the
// service-to-repository filter propagation can be asserted.
type capturingRepo struct{ received Filter }

func (r *capturingRepo) Append(_ context.Context, e Event) (Event, error) {
	return e, nil
}
func (r *capturingRepo) Search(_ context.Context, f Filter) (Page, error) {
	r.received = f
	return Page{}, nil
}

func TestAuditSearchPreservesAuditorActorID(t *testing.T) {
	repo := &capturingRepo{}
	svc, _ := NewService(repo)

	const wantActor = "field-operator-42"
	const wantAction = "deployment.create"
	filter := Filter{ActorID: wantActor, Action: wantAction, Limit: 10}

	if _, err := svc.Search(context.Background(), identity.Actor{UserID: "aud1", Role: identity.RoleAuditor}, filter); err != nil {
		t.Fatalf("search: %v", err)
	}
	if repo.received.ActorID != wantActor {
		t.Fatalf("actor_id not propagated to repository: got %q want %q", repo.received.ActorID, wantActor)
	}
	if repo.received.Action != wantAction {
		t.Fatalf("action not propagated to repository: got %q want %q", repo.received.Action, wantAction)
	}
}

func TestAuditSearchScopesCommanderToSelf(t *testing.T) {
	repo := &capturingRepo{}
	svc, _ := NewService(repo)

	filter := Filter{ActorID: "someone-else", Action: "deployment.create", Limit: 10}
	if _, err := svc.Search(context.Background(), identity.Actor{UserID: "cmd-1", Role: identity.RoleCommander}, filter); err != nil {
		t.Fatalf("search: %v", err)
	}
	if repo.received.ActorID != "cmd-1" {
		t.Fatalf("commander actor_id not scoped to self: got %q want %q", repo.received.ActorID, "cmd-1")
	}
	if repo.received.Action != "deployment.create" {
		t.Fatalf("action dropped for commander: got %q", repo.received.Action)
	}
}
