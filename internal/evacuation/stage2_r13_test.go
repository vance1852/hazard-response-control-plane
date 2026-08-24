package evacuation
import("context";"errors";"testing";"time";"github.com/vance1852/hazard-response-control-plane/internal/audit";"github.com/vance1852/hazard-response-control-plane/internal/clock";"github.com/vance1852/hazard-response-control-plane/internal/identity")
type cancelledStartRepo struct{Repository; started bool}
func(*cancelledStartRepo)FindPlan(context.Context,string)(Plan,error){return Plan{ID:"p",Status:PlanApproved,Version:2,DeadlineAt:time.Now().Add(time.Hour)},nil}
func(r *cancelledStartRepo)StartPlan(ctx context.Context,_ string,_ int64,_ time.Time,_ audit.Event)(Plan,error){r.started=true;if err:=ctx.Err();err!=nil{return Plan{},err};return Plan{Status:PlanExecuting},nil}
func TestStage2CancelledPlanStartDoesNotPersist(t *testing.T){repo:=&cancelledStartRepo{};svc,_:=NewService(repo,clock.NewManual(time.Now().UTC()));ctx,cancel:=context.WithCancel(context.Background());cancel();_,err:=svc.Start(ctx,identity.Actor{UserID:"field",Role:identity.RoleFieldOperator},"p",2);if !errors.Is(err,context.Canceled){t.Fatalf("start error = %v",err)};if repo.started{t.Fatal("cancelled plan start reached persistence")}}
