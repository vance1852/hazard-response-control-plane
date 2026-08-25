package idempotency
import("context";"testing";"time";"github.com/vance1852/hazard-response-control-plane/internal/clock")
type failureIDRepo struct{Repository;id string}
func(r *failureIDRepo)Fail(_ context.Context,id string,_ time.Time)error{r.id=id;return nil}
func TestStage2FailureReleasesExactIdempotencyRecord(t *testing.T){repo:=&failureIDRepo{};svc,_:=NewService(repo,clock.NewManual(time.Now().UTC()),time.Hour);if err:=svc.Fail(context.Background(),Record{ID:"record-16",ActorID:"actor-16",Status:"processing"});err!=nil{t.Fatal(err)};if repo.id!="record-16"{t.Fatalf("failed record id = %q",repo.id)}}
