package audit
import("context";"errors";"testing";"github.com/vance1852/hazard-response-control-plane/internal/identity")
type failedAuditSearchRepo struct{Repository}
func(*failedAuditSearchRepo)Search(context.Context,Filter)(Page,error){return Page{},errors.New("audit database locked")}
func TestStage2AuditSearchPropagatesRepositoryFailure(t *testing.T){svc,_:=NewService(&failedAuditSearchRepo{});page,err:=svc.Search(context.Background(),identity.Actor{UserID:"auditor",Role:identity.RoleAuditor},Filter{Action:"incident.close"});if err==nil{t.Fatalf("search returned success with page %#v",page)}}
