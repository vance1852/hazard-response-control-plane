package dispatch
import("context";"testing";"time";"github.com/vance1852/hazard-response-control-plane/internal/audit";"github.com/vance1852/hazard-response-control-plane/internal/clock";"github.com/vance1852/hazard-response-control-plane/internal/identity")
type deploymentOwnerRepo struct{Repository; transitioned bool}
func(*deploymentOwnerRepo)FindDeployment(context.Context,string)(Deployment,error){return Deployment{ID:"dep",UnitID:"unit",Status:DeploymentAssigned,Version:1},nil}
func(*deploymentOwnerRepo)FindUnit(context.Context,string)(Unit,error){return Unit{ID:"unit",OperatorID:"operator-b",Status:UnitDeployed},nil}
func(r *deploymentOwnerRepo)TransitionDeployment(context.Context,string,int64,DeploymentStatus,string,time.Time,audit.Event)(Deployment,*Unit,error){r.transitioned=true;return Deployment{Status:DeploymentAcknowledged},nil,nil}
func TestStage2FieldOperatorCannotAdvanceAnotherUnit(t *testing.T){repo:=&deploymentOwnerRepo{};svc,_:=NewService(repo,clock.NewManual(time.Now().UTC()));_,_,err:=svc.TransitionDeployment(context.Background(),identity.Actor{UserID:"operator-a",Role:identity.RoleFieldOperator},"dep",1,DeploymentAcknowledged);if err==nil{t.Fatal("operator advanced another unit deployment")};if repo.transitioned{t.Fatal("ownership failure reached repository transition")}}
