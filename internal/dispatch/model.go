package dispatch

import (
	"fmt"
	"strings"
	"time"
)

type UnitType string

const (
	UnitRescue      UnitType = "rescue"
	UnitMedical     UnitType = "medical"
	UnitEngineering UnitType = "engineering"
	UnitSurvey      UnitType = "survey"
	UnitTransport   UnitType = "transport"
)

func (t UnitType) Valid() bool {
	return t == UnitRescue || t == UnitMedical || t == UnitEngineering || t == UnitSurvey || t == UnitTransport
}

type UnitStatus string

const (
	UnitAvailable UnitStatus = "available"
	UnitDeployed  UnitStatus = "deployed"
	UnitOffline   UnitStatus = "offline"
)

type Unit struct {
	ID               string     `json:"id"`
	RegionID         string     `json:"region_id"`
	CallSign         string     `json:"call_sign"`
	Type             UnitType   `json:"unit_type"`
	CapabilitiesJSON string     `json:"capabilities_json"`
	Status           UnitStatus `json:"status"`
	OperatorID       string     `json:"operator_id,omitempty"`
	Version          int64      `json:"version"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

func (u Unit) Validate() error {
	if strings.TrimSpace(u.RegionID) == "" {
		return fmt.Errorf("unit region is required")
	}
	if strings.TrimSpace(u.CallSign) == "" {
		return fmt.Errorf("unit call sign is required")
	}
	if !u.Type.Valid() {
		return fmt.Errorf("invalid unit type %q", u.Type)
	}
	if !strings.HasPrefix(strings.TrimSpace(u.CapabilitiesJSON), "[") {
		return fmt.Errorf("unit capabilities must be a JSON array")
	}
	return nil
}

type RequestStatus string

const (
	RequestRequested          RequestStatus = "requested"
	RequestApproved           RequestStatus = "approved"
	RequestAllocated          RequestStatus = "allocated"
	RequestPartiallyAllocated RequestStatus = "partially_allocated"
	RequestFulfilled          RequestStatus = "fulfilled"
	RequestCancelled          RequestStatus = "cancelled"
)

var requestTransitions = map[RequestStatus]map[RequestStatus]bool{
	RequestRequested:          {RequestApproved: true, RequestCancelled: true},
	RequestApproved:           {RequestAllocated: true, RequestPartiallyAllocated: true, RequestCancelled: true},
	RequestPartiallyAllocated: {RequestAllocated: true, RequestFulfilled: true, RequestCancelled: true},
	RequestAllocated:          {RequestFulfilled: true, RequestCancelled: true},
	RequestFulfilled:          {}, RequestCancelled: {},
}

func (s RequestStatus) CanTransition(target RequestStatus) bool { return requestTransitions[s][target] }

type Request struct {
	ID                 string        `json:"id"`
	IncidentID         string        `json:"incident_id"`
	ZoneID             string        `json:"zone_id,omitempty"`
	RequestedType      UnitType      `json:"requested_type"`
	RequiredCapability string        `json:"required_capability"`
	Quantity           int           `json:"quantity"`
	Priority           int           `json:"priority"`
	Status             RequestStatus `json:"status"`
	NeededBy           time.Time     `json:"needed_by"`
	Version            int64         `json:"version"`
	CreatedBy          string        `json:"created_by"`
	ApprovedBy         string        `json:"approved_by,omitempty"`
	CreatedAt          time.Time     `json:"created_at"`
	UpdatedAt          time.Time     `json:"updated_at"`
}

func (r Request) Validate(now time.Time) error {
	if strings.TrimSpace(r.IncidentID) == "" {
		return fmt.Errorf("incident is required")
	}
	if !r.RequestedType.Valid() {
		return fmt.Errorf("invalid requested unit type")
	}
	if strings.TrimSpace(r.RequiredCapability) == "" {
		return fmt.Errorf("required capability is required")
	}
	if r.Quantity < 1 || r.Quantity > 100 {
		return fmt.Errorf("quantity must be between 1 and 100")
	}
	if r.Priority < 1 || r.Priority > 5 {
		return fmt.Errorf("priority must be between 1 and 5")
	}
	if !r.NeededBy.After(now) {
		return fmt.Errorf("needed-by time must be in the future")
	}
	return nil
}

type DeploymentStatus string

const (
	DeploymentAssigned     DeploymentStatus = "assigned"
	DeploymentAcknowledged DeploymentStatus = "acknowledged"
	DeploymentEnRoute      DeploymentStatus = "en_route"
	DeploymentOnScene      DeploymentStatus = "on_scene"
	DeploymentReleased     DeploymentStatus = "released"
	DeploymentCancelled    DeploymentStatus = "cancelled"
)

var deploymentTransitions = map[DeploymentStatus]map[DeploymentStatus]bool{
	DeploymentAssigned:     {DeploymentAcknowledged: true, DeploymentCancelled: true},
	DeploymentAcknowledged: {DeploymentEnRoute: true, DeploymentCancelled: true},
	DeploymentEnRoute:      {DeploymentOnScene: true, DeploymentCancelled: true},
	DeploymentOnScene:      {DeploymentReleased: true},
	DeploymentReleased:     {}, DeploymentCancelled: {},
}

func (s DeploymentStatus) CanTransition(target DeploymentStatus) bool {
	return deploymentTransitions[s][target]
}

type Deployment struct {
	ID             string           `json:"id"`
	RequestID      string           `json:"request_id"`
	UnitID         string           `json:"unit_id"`
	IncidentID     string           `json:"incident_id"`
	Status         DeploymentStatus `json:"status"`
	AssignedBy     string           `json:"assigned_by"`
	AcknowledgedAt *time.Time       `json:"acknowledged_at,omitempty"`
	ArrivedAt      *time.Time       `json:"arrived_at,omitempty"`
	ReleasedAt     *time.Time       `json:"released_at,omitempty"`
	Version        int64            `json:"version"`
	CreatedAt      time.Time        `json:"created_at"`
	UpdatedAt      time.Time        `json:"updated_at"`
}

type RequestFilter struct {
	IncidentID      string
	Statuses        []RequestStatus
	PriorityAtLeast int
	Offset          int
	Limit           int
}

type RequestPage struct {
	Items []Request `json:"items"`
	Total int       `json:"total"`
}
