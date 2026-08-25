package worker

import (
 "context"
 "testing"
 "time"
)

type stage2ContextRepo struct{}
func (stage2ContextRepo) Claim(context.Context,string,time.Time,time.Duration,int)([]Job,error){return nil,nil}
func (stage2ContextRepo) RecordAttempt(context.Context,Job,string,time.Time)(int64,error){return 1,nil}
func (stage2ContextRepo) Finish(context.Context,Job,string,error,time.Time) error{return nil}
func (stage2ContextRepo) RecoverExpired(context.Context,time.Time) error{return nil}

func TestStage2CancelledRunContextReachesHandler(t *testing.T) {
 w, err := New(stage2ContextRepo{}, "worker-3", time.Millisecond, time.Second, 1)
 if err != nil { t.Fatal(err) }
 seen := make(chan error, 1)
 w.Register("incident.notification", func(ctx context.Context, _ Job) error { seen <- ctx.Err(); return ctx.Err() })
 ctx, cancel := context.WithCancel(context.Background()); cancel()
 w.execute(ctx, Job{ID: "job-3", Kind: "incident.notification", MaxAttempts: 3})
 if got := <-seen; got != context.Canceled { t.Fatalf("handler context error = %v, want context.Canceled", got) }
}
