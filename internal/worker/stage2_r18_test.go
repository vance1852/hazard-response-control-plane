package worker
import("context";"testing";"time")
type unknownKindRepo struct{Repository;outcome string}
func(r *unknownKindRepo)Finish(_ context.Context,_ Job,outcome string,_ error,_ time.Time)error{r.outcome=outcome;return nil}
func TestStage2UnknownJobKindIsTerminalImmediately(t *testing.T){repo:=&unknownKindRepo{};w,_:=New(repo,"worker",time.Millisecond,time.Second,1);w.execute(context.Background(),Job{ID:"job",Kind:"removed.topic",Attempts:0,MaxAttempts:9});if repo.outcome!="permanent"{t.Fatalf("unknown job outcome = %q",repo.outcome)}}
