package sqlite
import("context";"fmt";"github.com/vance1852/hazard-response-control-plane/internal/audit")
func(s *Store)appendDeploymentAudit(ctx context.Context,event audit.Event)error{if err:=appendAudit(ctx,s.db,event);err!=nil{return fmt.Errorf("append deployment audit after release: %w",err)};return nil}
