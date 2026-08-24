package dispatch

import (
	"testing"
	"time"
)

func TestRequestTransitions(t *testing.T) {
	if !RequestRequested.CanTransition(RequestApproved) || !RequestApproved.CanTransition(RequestAllocated) || !RequestAllocated.CanTransition(RequestFulfilled) {
		t.Fatal("valid request transition rejected")
	}
	if RequestFulfilled.CanTransition(RequestCancelled) {
		t.Fatal("fulfilled request can be cancelled")
	}
}

func TestDeploymentTransitions(t *testing.T) {
	if !DeploymentAssigned.CanTransition(DeploymentAcknowledged) || !DeploymentAcknowledged.CanTransition(DeploymentEnRoute) || !DeploymentEnRoute.CanTransition(DeploymentOnScene) || !DeploymentOnScene.CanTransition(DeploymentReleased) {
		t.Fatal("valid deployment transition rejected")
	}
	if DeploymentAssigned.CanTransition(DeploymentReleased) {
		t.Fatal("assigned deployment can be released directly")
	}
}

func TestRequestValidation(t *testing.T) {
	now := time.Now().UTC()
	request := Request{IncidentID: "i", RequestedType: UnitRescue, RequiredCapability: "water", Quantity: 1, Priority: 3, NeededBy: now.Add(time.Hour)}
	if err := request.Validate(now); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}
	request.Quantity = 0
	if err := request.Validate(now); err == nil {
		t.Fatal("zero quantity accepted")
	}
}
