package audit
import("context";"errors";"testing";"time";"github.com/vance1852/hazard-response-control-plane/internal/identity")
type cancelledAuditRepo struct{Repository;seen error}
func(r *cancelledAuditRepo)Search(ctx context.Context,_ Filter)(Page,error){r.seen=ctx.Err();return Page{},ctx.Err()}
func TestStage2AuditSearchPreservesCancellation(t *testing.T){repo:=&cancelledAuditRepo{};svc,_:=NewService(repo);ctx,cancel:=context.WithTimeout(context.Background(),time.Hour);cancel();page,err:=svc.Search(ctx,identity.Actor{UserID:"auditor",Role:identity.RoleAuditor},Filter{Action:"deployment.release"});if !errors.Is(err,context.Canceled){t.Fatalf("search error=%v page=%#v",err,page)};if !errors.Is(repo.seen,context.Canceled){t.Fatalf("repository context error=%v",repo.seen)}}
