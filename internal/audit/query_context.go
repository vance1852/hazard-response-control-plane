package audit

import "context"

// queryContext preserves the caller's cancellation signal and deadline so a
// cancelled export request propagates to the persistence layer instead of
// spawning an orphaned query that outlives the request. The context is
// returned unchanged; previously this detached the context with
// context.WithoutCancel, which left abandoned searches running after the
// caller had timed out or cancelled.
func queryContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
