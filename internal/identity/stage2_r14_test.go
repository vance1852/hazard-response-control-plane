package identity
import("context";"testing";"time";"golang.org/x/crypto/bcrypt";"github.com/vance1852/hazard-response-control-plane/internal/apperr";"github.com/vance1852/hazard-response-control-plane/internal/clock")
type loginPersistRepo struct{Repository; attempted bool}
func(*loginPersistRepo)FindUserByUsername(context.Context,string)(User,error){hash,_:=bcrypt.GenerateFromPassword([]byte("StrongPassword9"),bcrypt.MinCost);return User{ID:"u",Username:"commander",PasswordHash:string(hash),Role:RoleCommander,Active:true},nil}
func(r *loginPersistRepo)CreateSession(context.Context,Session)(Session,error){r.attempted=true;return Session{},apperr.Conflict("session_write_conflict","session ledger unavailable")}
func TestStage2LoginFailsWhenSessionCannotPersist(t *testing.T){repo:=&loginPersistRepo{};svc,_:=NewService(repo,clock.NewManual(time.Now().UTC()),time.Hour);result,err:=svc.Login(context.Background(),"commander","StrongPassword9");if err==nil{t.Fatalf("login returned token %q without a persisted session",result.Token)};if !repo.attempted{t.Fatal("session persistence was not attempted")}}
