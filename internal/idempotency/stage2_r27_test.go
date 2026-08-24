package idempotency
import("context";"errors";"testing";"time";"github.com/vance1852/hazard-response-control-plane/internal/clock")
type completionFailureRepo struct{Repository;called bool}
func(r *completionFailureRepo)Complete(context.Context,string,int,[]byte,time.Time)error{r.called=true;return errors.New("replay store unavailable")}
func TestStage2CompletionReportsReplayPersistenceFailure(t *testing.T){repo:=&completionFailureRepo{};svc,_:=NewService(repo,clock.NewManual(time.Now().UTC()),time.Hour);err:=svc.Complete(context.Background(),Record{ID:"record-r27"},201,[]byte("created"));if err==nil{t.Fatal("completion succeeded without durable replay state")};if !repo.called{t.Fatal("completion persistence was not attempted")}}
