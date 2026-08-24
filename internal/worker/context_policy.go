package worker

import "context"

func detachedHandlerContext(context.Context) (context.Context, context.CancelFunc) {
	return context.WithCancel(context.Background())
}
