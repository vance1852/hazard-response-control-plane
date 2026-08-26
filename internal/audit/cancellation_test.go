package audit

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/vance1852/hazard-response-control-plane/internal/identity"
)

// cancellingRepo records the context it was called with so the search can
// assert the caller's cancellation signal reaches the repository.
type cancellingRepo struct {
	receivedCtx context.Context
}

func (r *cancellingRepo) Append(_ context.Context, _ Event) (Event, error) { return Event{}, nil }
func (r *cancellingRepo) Search(ctx context.Context, _ Filter) (Page, error) {
	r.receivedCtx = ctx
	if err := ctx.Err(); err != nil {
		return Page{}, err
	}
	return Page{Events: []Event{}}, nil
}

// TestAuditSearchPropagatesCancellation confirms the service no longer
// detaches the request context: a cancelled caller must hand a cancelled
// context to the repository, and the search must surface the cancellation as
// an error rather than a successful empty page.
func TestAuditSearchPropagatesCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	repo := &cancellingRepo{}
	svc, _ := NewService(repo)
	page, err := svc.Search(ctx, identity.Actor{UserID: "u", Role: identity.RoleAuditor}, Filter{Limit: 10})
	if err == nil {
		t.Fatalf("cancelled search returned success page=%v", page)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if !errors.Is(repo.receivedCtx.Err(), context.Canceled) {
		t.Fatalf("repository context not cancelled: %v", repo.receivedCtx.Err())
	}
}

// TestAuditSearchPropagatesDeadline confirms an expired deadline reaches the
// repository. Previously the audit search detached any context carrying a
// deadline via context.WithoutCancel, letting the query outlive the request.
func TestAuditSearchPropagatesDeadline(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	repo := &cancellingRepo{}
	svc, _ := NewService(repo)
	if _, err := svc.Search(ctx, identity.Actor{UserID: "u", Role: identity.RoleAuditor}, Filter{Limit: 10}); err == nil {
		t.Fatal("expired-deadline search returned success")
	}
	if _, ok := repo.receivedCtx.Deadline(); !ok {
		t.Fatal("repository context lost the caller deadline")
	}
}
