package audit
import "context"
func auditQueryContext(ctx context.Context)context.Context{if ctx==nil{return context.Background()};if _,ok:=ctx.Deadline();ok{return context.WithoutCancel(ctx)};return ctx}
