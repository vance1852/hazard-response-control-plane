package worker

import("context";"testing";"time")

type ackOrderRepo struct{Repository;finishes []string}
func(r *ackOrderRepo)RecordAttempt(context.Context,Job,string,time.Time)(int64,error){return 1,nil}
func(r *ackOrderRepo)Finish(_ context.Context,_ Job,outcome string,_ error,_ time.Time)error{r.finishes=append(r.finishes,outcome);return nil}
func TestStage2WorkerAcknowledgesOnlyAfterHandlerCompletes(t *testing.T){
 repo:=&ackOrderRepo{};worker,err:=New(repo,"worker-r21",time.Millisecond,time.Second,1);if err!=nil{t.Fatal(err)}
 handlerRan:=false
 worker.Register("incident.publish",func(context.Context,Job)error{handlerRan=true;if len(repo.finishes)!=0{t.Fatalf("job finished before handler completed: %v",repo.finishes)};return nil})
 worker.execute(context.Background(),Job{ID:"job-r21",Kind:"incident.publish",MaxAttempts:3})
 if !handlerRan{t.Fatal("registered handler did not run")};if len(repo.finishes)!=1||repo.finishes[0]!="succeeded"{t.Fatalf("finish sequence = %v",repo.finishes)}
}
