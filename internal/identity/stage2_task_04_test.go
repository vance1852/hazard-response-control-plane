package identity

import (
 "context"
 "testing"
 "time"
 "github.com/vance1852/hazard-response-control-plane/internal/clock"
)

type stage2LogoutRepo struct{ revoked string }
func (r *stage2LogoutRepo) CreateUser(context.Context,User)(User,error){return User{},nil}
func (r *stage2LogoutRepo) FindUserByID(context.Context,string)(User,error){return User{},nil}
func (r *stage2LogoutRepo) FindUserByUsername(context.Context,string)(User,error){return User{},nil}
func (r *stage2LogoutRepo) CreateSession(context.Context,Session)(Session,error){return Session{},nil}
func (r *stage2LogoutRepo) FindSessionByDigest(context.Context,[]byte)(Session,User,error){return Session{},User{},nil}
func (r *stage2LogoutRepo) TouchSession(context.Context,string,int64,time.Time) error{return nil}
func (r *stage2LogoutRepo) RevokeSession(_ context.Context,id string,_ int64,_ time.Time) error{r.revoked=id;return nil}
func (r *stage2LogoutRepo) RevokeExpired(context.Context,time.Time,int)(int,error){return 0,nil}

func TestStage2LogoutRevokesAuthenticatedSession(t *testing.T){
 repo:=&stage2LogoutRepo{}; svc,err:=NewService(repo,clock.NewManual(time.Now().UTC()),time.Hour); if err!=nil{t.Fatal(err)}
 actor:=Actor{UserID:"user-4",SessionID:"session-4",Role:RoleCommander}
 if err:=svc.Logout(context.Background(),actor);err!=nil{t.Fatal(err)}
 if repo.revoked!=actor.SessionID{t.Fatalf("revoked session = %q, want %q",repo.revoked,actor.SessionID)}
}
