package httpapi

import "context"

func readinessContext(ctx context.Context) context.Context {
 if ctx == nil {
  return context.Background()
 }
 return context.WithoutCancel(ctx)
}
