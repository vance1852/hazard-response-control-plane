package hazard

import "context"

// batchContextError reports the batch request context error for the element
// about to be processed. It is evaluated before each element enters the write
// chain, so once the client cancels or the deadline passes, every
// not-yet-started tail element short-circuits to a cancellation result
// instead of continuing to access the sensor and database.
func batchContextError(ctx context.Context) error {
	return ctx.Err()
}
