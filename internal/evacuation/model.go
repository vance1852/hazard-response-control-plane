package evacuation

import (
	"fmt"
	"strings"
	"time"
)

type ShelterStatus string

const (
	ShelterAvailable  ShelterStatus = "available"
	ShelterRestricted ShelterStatus = "restricted"
	ShelterClosed     ShelterStatus = "closed"
)

func (s ShelterStatus) Valid() bool {
	return s == ShelterAvailable || s == ShelterRestricted || s == ShelterClosed
}

type Shelter struct {
	ID        string        `json:"id"`
	RegionID  string        `json:"region_id"`
	Code      string        `json:"code"`
	Name      string        `json:"name"`
	Capacity  int           `json:"capacity"`
	Reserved  int           `json:"reserved"`
	Status    ShelterStatus `json:"status"`
	Version   int64         `json:"version"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
}

func (s Shelter) AvailableCapacity() int { return s.Capacity - s.Reserved }

func (s Shelter) Validate() error {
	if strings.TrimSpace(s.RegionID) == "" {
		return fmt.Errorf("shelter region is required")
	}
	if strings.TrimSpace(s.Code) == "" || strings.TrimSpace(s.Name) == "" {
		return fmt.Errorf("shelter code and name are required")
	}
	if s.Capacity < 1 {
		return fmt.Errorf("shelter capacity must be positive")
	}
	if s.Reserved < 0 || s.Reserved > s.Capacity {
		return fmt.Errorf("shelter reserved capacity is invalid")
	}
	if !s.Status.Valid() {
		return fmt.Errorf("invalid shelter status %q", s.Status)
	}
	return nil
}

type PlanStatus string

const (
	PlanDraft     PlanStatus = "draft"
	PlanSubmitted PlanStatus = "submitted"
	PlanApproved  PlanStatus = "approved"
	PlanExecuting PlanStatus = "executing"
	PlanCompleted PlanStatus = "completed"
	PlanCancelled PlanStatus = "cancelled"
)

var planTransitions = map[PlanStatus]map[PlanStatus]bool{
	PlanDraft:     {PlanSubmitted: true, PlanCancelled: true},
	PlanSubmitted: {PlanApproved: true, PlanCancelled: true},
	PlanApproved:  {PlanExecuting: true, PlanCancelled: true},
	PlanExecuting: {PlanCompleted: true, PlanCancelled: true},
	PlanCompleted: {},
	PlanCancelled: {},
}

func (s PlanStatus) CanTransition(target PlanStatus) bool { return planTransitions[s][target] }

type Plan struct {
	ID           string     `json:"id"`
	IncidentID   string     `json:"incident_id"`
	ZoneID       string     `json:"zone_id"`
	ShelterID    string     `json:"shelter_id"`
	Name         string     `json:"name"`
	EvacueeCount int        `json:"evacuee_count"`
	Status       PlanStatus `json:"status"`
	DeadlineAt   time.Time  `json:"deadline_at"`
	ApprovedBy   string     `json:"approved_by,omitempty"`
	ApprovedAt   *time.Time `json:"approved_at,omitempty"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
	Version      int64      `json:"version"`
	CreatedBy    string     `json:"created_by"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

func (p Plan) Validate(now time.Time) error {
	if strings.TrimSpace(p.IncidentID) == "" || strings.TrimSpace(p.ZoneID) == "" || strings.TrimSpace(p.ShelterID) == "" {
		return fmt.Errorf("incident, zone, and shelter are required")
	}
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("plan name is required")
	}
	if p.EvacueeCount < 1 {
		return fmt.Errorf("evacuee count must be positive")
	}
	if !p.DeadlineAt.After(now) {
		return fmt.Errorf("plan deadline must be in the future")
	}
	return nil
}

type Step struct {
	ID              string     `json:"id"`
	PlanID          string     `json:"plan_id"`
	Order           int        `json:"order"`
	Instruction     string     `json:"instruction"`
	ResponsibleRole string     `json:"responsible_role"`
	ExpectedMinutes int        `json:"expected_minutes"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
	CompletedBy     string     `json:"completed_by,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

func (s Step) Validate() error {
	if s.Order < 1 {
		return fmt.Errorf("step order must be positive")
	}
	if strings.TrimSpace(s.Instruction) == "" {
		return fmt.Errorf("step instruction is required")
	}
	if s.ResponsibleRole != "commander" && s.ResponsibleRole != "field_operator" {
		return fmt.Errorf("invalid responsible role")
	}
	if s.ExpectedMinutes < 1 || s.ExpectedMinutes > 1440 {
		return fmt.Errorf("expected minutes must be between 1 and 1440")
	}
	return nil
}

type ReservationStatus string

const (
	ReservationActive   ReservationStatus = "active"
	ReservationReleased ReservationStatus = "released"
	ReservationConsumed ReservationStatus = "consumed"
)

type Reservation struct {
	ID         string            `json:"id"`
	ShelterID  string            `json:"shelter_id"`
	PlanID     string            `json:"plan_id"`
	People     int               `json:"people"`
	Status     ReservationStatus `json:"status"`
	CreatedAt  time.Time         `json:"created_at"`
	ReleasedAt *time.Time        `json:"released_at,omitempty"`
}

type ShelterFilter struct {
	RegionID        string
	MinimumCapacity int
	Status          ShelterStatus
	Offset          int
	Limit           int
}

type ShelterPage struct {
	Items []Shelter `json:"items"`
	Total int       `json:"total"`
}
