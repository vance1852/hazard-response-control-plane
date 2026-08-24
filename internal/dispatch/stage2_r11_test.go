package dispatch

import(
 "context"
 "encoding/json"
 "errors"
 "testing"
 "time"
 "github.com/vance1852/hazard-response-control-plane/internal/audit"
 "github.com/vance1852/hazard-response-control-plane/internal/clock"
 "github.com/vance1852/hazard-response-control-plane/internal/hazard"
 "github.com/vance1852/hazard-response-control-plane/internal/identity"
)
type cancelledAllocationRepo struct{Repository; allocated bool}
func(*cancelledAllocationRepo)FindRequest(context.Context,string)(Request,error){return Request{ID:"req",IncidentID:"inc",RequestedType:UnitMedical,RequiredCapability:"triage",Quantity:1,Status:RequestApproved,NeededBy:time.Now().Add(time.Hour),Version:1},nil}
func(*cancelledAllocationRepo)FindUnit(context.Context,string)(Unit,error){b,_:=json.Marshal([]string{"triage"});return Unit{ID:"unit",RegionID:"r",Type:UnitMedical,Status:UnitAvailable,CapabilitiesJSON:string(b),Version:1},nil}
func(*cancelledAllocationRepo)FindIncident(context.Context,string)(hazard.Incident,error){return hazard.Incident{ID:"inc",RegionID:"r",Status:hazard.IncidentActive},nil}
func(r *cancelledAllocationRepo)AllocateUnit(ctx context.Context,_ string,_ int64,_ string,_ int64,_ string,_ time.Time,_ audit.Event)(Request,Deployment,Unit,error){r.allocated=true;if err:=ctx.Err();err!=nil{return Request{},Deployment{},Unit{},err};return Request{},Deployment{},Unit{},nil}
func TestStage2CancelledAllocationDoesNotReachTransaction(t *testing.T){repo:=&cancelledAllocationRepo{};svc,_:=NewService(repo,clock.NewManual(time.Now().UTC()));ctx,cancel:=context.WithCancel(context.Background());cancel();_,_,_,err:=svc.Allocate(ctx,identity.Actor{UserID:"c",Role:identity.RoleCommander},"req",1,"unit",1);if !errors.Is(err,context.Canceled){t.Fatalf("allocation error = %v",err)};if repo.allocated{t.Fatal("cancelled allocation reached transaction")}}
