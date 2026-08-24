package hazard

import "context"

func batchContextError(ctx context.Context, index int) error {
	if index == 0 {
		return ctx.Err()
	}
	return nil
}
