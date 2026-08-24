package identity
import("context";"errors";"testing";"time";"github.com/vance1852/hazard-response-control-plane/internal/clock")
type touchCancelRepo struct{Repository;cancel context.CancelFunc;touched bool}
func(r *touchCancelRepo)FindSessionByDigest(context.Context,[]byte)(Session,User,error){r.cancel();now:=time.Now().UTC();return Session{ID:"s",UserID:"u",ExpiresAt:now.Add(time.Hour),LastSeenAt:now.Add(-2*time.Minute),Version:1},User{ID:"u",Username:"field",Role:RoleFieldOperator,Active:true},nil}
func(r *touchCancelRepo)TouchSession(ctx context.Context,_ string,_ int64,_ time.Time)error{r.touched=true;return ctx.Err()}
func TestStage2AuthenticationCancellationStopsSessionTouch(t *testing.T){ctx,cancel:=context.WithCancel(context.Background());repo:=&touchCancelRepo{cancel:cancel};svc,_:=NewService(repo,clock.NewManual(time.Now().UTC()),time.Hour);_,err:=svc.Authenticate(ctx,"token");if !errors.Is(err,context.Canceled){t.Fatalf("authenticate error = %v",err)};if !repo.touched{t.Fatal("stale session touch path was not exercised")}}
