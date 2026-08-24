package identity
import("context";"errors";"testing";"time";"github.com/vance1852/hazard-response-control-plane/internal/clock")
type touchFailureRepo struct{Repository;touched bool}
func(*touchFailureRepo)FindSessionByDigest(context.Context,[]byte)(Session,User,error){return Session{ID:"session-r22",UserID:"user-r22",ExpiresAt:time.Date(2026,8,24,14,0,0,0,time.UTC),LastSeenAt:time.Date(2026,8,24,10,0,0,0,time.UTC),Version:4},User{ID:"user-r22",Username:"field-r22",Role:RoleFieldOperator,Active:true},nil}
func(r *touchFailureRepo)TouchSession(context.Context,string,int64,time.Time)error{r.touched=true;return errors.New("session database unavailable")}
func TestStage2AuthenticationRejectsSessionTouchInfrastructureFailure(t *testing.T){repo:=&touchFailureRepo{};svc,err:=NewService(repo,clock.NewManual(time.Date(2026,8,24,12,0,0,0,time.UTC)),time.Hour);if err!=nil{t.Fatal(err)};actor,err:=svc.Authenticate(context.Background(),"valid-token-r22");if err==nil{t.Fatalf("authentication returned actor after failed session touch: %#v",actor)};if !repo.touched{t.Fatal("session touch was not attempted")};if actor.UserID!=""{t.Fatalf("failed authentication leaked principal %#v",actor)}}
