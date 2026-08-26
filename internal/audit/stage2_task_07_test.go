package audit

import("context";"testing";"github.com/vance1852/hazard-response-control-plane/internal/identity")
type stage2AuditRepo struct{filter Filter}
func(*stage2AuditRepo)Append(context.Context,Event)(Event,error){return Event{},nil}
func(r *stage2AuditRepo)Search(_ context.Context,f Filter)(Page,error){r.filter=f;return Page{},nil}
func TestStage2AuditorActorFilterReachesRepository(t *testing.T){
 repo:=&stage2AuditRepo{};svc,err:=NewService(repo);if err!=nil{t.Fatal(err)}
 _,err=svc.Search(context.Background(),identity.Actor{UserID:"auditor-7",Role:identity.RoleAuditor},Filter{ActorID:"field-operator-7",Action:"deployment.transition"});if err!=nil{t.Fatal(err)}
 if repo.filter.ActorID!="field-operator-7"{t.Fatalf("repository actor filter = %q",repo.filter.ActorID)}
}
