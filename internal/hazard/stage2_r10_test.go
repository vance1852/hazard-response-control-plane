package hazard

import (
 "context"
 "testing"
 "time"
 "github.com/vance1852/hazard-response-control-plane/internal/audit"
 "github.com/vance1852/hazard-response-control-plane/internal/clock"
 "github.com/vance1852/hazard-response-control-plane/internal/identity"
)
type closeDependencyRepo struct { Repository; transitioned bool }
func (r *closeDependencyRepo) FindIncident(context.Context,string)(Incident,error){return Incident{ID:"inc-close",RegionID:"region-a",Status:IncidentStabilizing,Version:4},nil}
func (r *closeDependencyRepo) CountOpenDependencies(context.Context,string)(int,int,error){return 0,1,nil}
func (r *closeDependencyRepo) TransitionIncident(context.Context,string,int64,IncidentStatus,string,*time.Time,audit.Event)(Incident,error){r.transitioned=true;return Incident{Status:IncidentClosed},nil}
func TestStage2IncidentCloseWaitsForActiveDeployment(t *testing.T){
 repo:=&closeDependencyRepo{};svc,err:=NewService(repo,clock.NewManual(time.Date(2026,8,24,8,0,0,0,time.UTC)));if err!=nil{t.Fatal(err)}
 _,err=svc.Transition(context.Background(),identity.Actor{UserID:"commander",RegionID:"region-a",Role:identity.RoleCommander},"inc-close",4,IncidentClosed,"field teams remain")
 if err==nil{t.Fatal("incident closed while a field deployment was active")}
 if repo.transitioned{t.Fatal("repository transition executed despite active deployment")}
}
