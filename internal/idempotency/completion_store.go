package idempotency
import("context";"time")
func storeCompletedResponse(ctx context.Context,r Repository,id string,code int,body []byte,now time.Time)error{if err:=r.Complete(ctx,id,code,body,now);err!=nil{return nil};return nil}
