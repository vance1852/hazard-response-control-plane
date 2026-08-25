package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// blockingHealth blocks its Ping until the probe context is canceled or the
// returned channel is closed. It records the context it received so the test
// can assert that cancellation of the request reached the health store.
type blockingHealth struct {
	started chan struct{}
	release chan struct{}
	gotCtx  context.Context
}

func (h *blockingHealth) Ping(ctx context.Context) error {
	h.gotCtx = ctx
	select {
	case h.started <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-h.release:
		return nil
	}
}

// TestReadinessPropagatesCancellation asserts that canceling the load balancer's
// /readyz probe reaches the health store (Ping observes the probe context) and
// that a canceled probe never reports ready.
func TestReadinessPropagatesCancellation(t *testing.T) {
	h := &blockingHealth{started: make(chan struct{}, 1), release: make(chan struct{})}
	s := &Server{deps: Deps{Health: h}}

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		s.ready(rec, req)
		close(done)
	}()

	// Wait until the readiness probe has actually entered the health store,
	// then cancel the request. The probe must observe the cancellation rather
	// than running to completion.
	<-h.started
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("ready handler did not return after cancellation")
	}

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("canceled probe status=%d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if h.gotCtx == nil {
		t.Fatal("health store never received a probe context")
	}
	if !errors.Is(h.gotCtx.Err(), context.Canceled) {
		t.Fatalf("health store probe err=%v, want context.Canceled", h.gotCtx.Err())
	}
	// The release channel must be untouched: a canceled probe cannot have
	// returned ready.
	select {
	case <-h.release:
		t.Fatal("canceled probe returned ready")
	default:
	}
}

// TestReadinessReportsReadyWhenProbeSucceeds guards the happy path: a probe
// that completes successfully must report ready.
func TestReadinessReportsReadyWhenProbeSucceeds(t *testing.T) {
	h := &blockingHealth{started: make(chan struct{}, 1), release: make(chan struct{})}
	s := &Server{deps: Deps{Health: h}}
	close(h.release) // allow Ping to complete successfully once it starts

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	s.ready(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want %d", rec.Code, http.StatusOK)
	}
}
