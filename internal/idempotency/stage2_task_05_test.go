package idempotency

import (
 "context"
 "testing"
 "time"
 "github.com/vance1852/hazard-response-control-plane/internal/clock"
 "github.com/vance1852/hazard-response-control-plane/internal/identity"
)

type stage2HashRepo struct{ record Record }
func (r *stage2HashRepo) Begin(_ context.Context,v Record)(Record,bool,error){v.ID="idem-5";r.record=v;return v,false,nil}
func (*stage2HashRepo) Complete(context.Context,string,int,[]byte,time.Time)error{return nil}
func (*stage2HashRepo) Fail(context.Context,string,time.Time)error{return nil}
func (*stage2HashRepo) Expire(context.Context,time.Time,int)(int,error){return 0,nil}
func TestStage2BeginOwnsRequestHash(t *testing.T){
 repo:=&stage2HashRepo{};svc,err:=NewService(repo,clock.NewManual(time.Now().UTC()),time.Hour);if err!=nil{t.Fatal(err)}
 requestHash:=[]byte{1,2,3,4};_,_,err=svc.Begin(context.Background(),identity.Actor{UserID:"operator-5"},"POST","/v1/incidents","retry-5",requestHash);if err!=nil{t.Fatal(err)}
 requestHash[0]=99
 if repo.record.Hash[0]!=1{t.Fatalf("persisted request hash changed through caller buffer: %v",repo.record.Hash)}
}
