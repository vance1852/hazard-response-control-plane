package worker

import "context"

// handlerContext derives the handler execution context from the worker's run
// context. When the worker is cancelled (shutdown or leadership transfer) the
// cancellation propagates to in-flight handlers so they exit promptly instead
// of continuing downstream work that blocks the stop flow or publishes stale
// alarms after another instance has taken over. The returned cancel function
// only cancels the handler's own subtree; it never affects the worker itself.
func handlerContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithCancel(parent)
}

// bookkeepingContext returns a context detached from the worker's run context
// for persisting attempt and outcome records. It is intentionally not cancelled
// when the worker is shutting down, so the retry record survives cancellation
// and the job is left in a retryable state for the next leader to reclaim.
func bookkeepingContext() context.Context {
	return context.Background()
}
