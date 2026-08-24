package idempotency
import("context";"testing";"time";"github.com/vance1852/hazard-response-control-plane/internal/clock")
type responseOwnerRepo struct{Repository;body []byte}
func(r *responseOwnerRepo)Complete(_ context.Context,_ string,_ int,body []byte,_ time.Time)error{r.body=body;return nil}
func TestStage2CompletionOwnsStoredResponseBody(t *testing.T){repo:=&responseOwnerRepo{};svc,_:=NewService(repo,clock.NewManual(time.Now().UTC()),time.Hour);body:=[]byte("{\"deployment\":\"created\"}");if err:=svc.Complete(context.Background(),Record{ID:"rec"},201,body);err!=nil{t.Fatal(err)};body[0]='X';if string(repo.body)!="{\"deployment\":\"created\"}"{t.Fatalf("stored response mutated to %q",repo.body)}}
