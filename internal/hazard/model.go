package hazard

import (
	"fmt"
	"strings"
	"time"
)

type HazardType string

const (
	Earthquake HazardType = "earthquake"
	Rainstorm  HazardType = "rainstorm"
	Landslide  HazardType = "landslide"
	DebrisFlow HazardType = "debris_flow"
	Flood      HazardType = "flood"
)

func (h HazardType) Valid() bool {
	switch h {
	case Earthquake, Rainstorm, Landslide, DebrisFlow, Flood:
		return true
	default:
		return false
	}
}

type IncidentStatus string

const (
	IncidentMonitoring  IncidentStatus = "monitoring"
	IncidentActive      IncidentStatus = "active"
	IncidentStabilizing IncidentStatus = "stabilizing"
	IncidentClosed      IncidentStatus = "closed"
	IncidentCancelled   IncidentStatus = "cancelled"
)

var incidentTransitions = map[IncidentStatus]map[IncidentStatus]bool{
	IncidentMonitoring:  {IncidentActive: true, IncidentCancelled: true},
	IncidentActive:      {IncidentStabilizing: true, IncidentCancelled: true},
	IncidentStabilizing: {IncidentActive: true, IncidentClosed: true},
	IncidentClosed:      {},
	IncidentCancelled:   {},
}

func (s IncidentStatus) CanTransition(to IncidentStatus) bool { return incidentTransitions[s][to] }

type CommandLevel string

const (
	CommandLocal    CommandLevel = "local"
	CommandRegional CommandLevel = "regional"
	CommandJoint    CommandLevel = "joint"
)

func (c CommandLevel) Valid() bool {
	return c == CommandLocal || c == CommandRegional || c == CommandJoint
}

type Region struct {
	ID        string    `json:"id"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	Timezone  string    `json:"timezone"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (r Region) Validate() error {
	if strings.TrimSpace(r.Code) == "" {
		return fmt.Errorf("region code is required")
	}
	if strings.TrimSpace(r.Name) == "" {
		return fmt.Errorf("region name is required")
	}
	if _, err := time.LoadLocation(r.Timezone); err != nil {
		return fmt.Errorf("invalid timezone %q: %w", r.Timezone, err)
	}
	return nil
}

type SensorKind string

const (
	SensorSeismic  SensorKind = "seismic"
	SensorRainfall SensorKind = "rainfall"
	SensorSlope    SensorKind = "slope"
	SensorRiver    SensorKind = "river"
)

func (k SensorKind) Valid() bool {
	return k == SensorSeismic || k == SensorRainfall || k == SensorSlope || k == SensorRiver
}

type Sensor struct {
	ID          string     `json:"id"`
	RegionID    string     `json:"region_id"`
	StationCode string     `json:"station_code"`
	Name        string     `json:"name"`
	Kind        SensorKind `json:"kind"`
	Latitude    float64    `json:"latitude"`
	Longitude   float64    `json:"longitude"`
	Active      bool       `json:"active"`
	Version     int64      `json:"version"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func (s Sensor) Validate() error {
	if strings.TrimSpace(s.RegionID) == "" {
		return fmt.Errorf("sensor region is required")
	}
	if strings.TrimSpace(s.StationCode) == "" {
		return fmt.Errorf("station code is required")
	}
	if strings.TrimSpace(s.Name) == "" {
		return fmt.Errorf("sensor name is required")
	}
	if !s.Kind.Valid() {
		return fmt.Errorf("invalid sensor kind %q", s.Kind)
	}
	if s.Latitude < -90 || s.Latitude > 90 {
		return fmt.Errorf("latitude must be between -90 and 90")
	}
	if s.Longitude < -180 || s.Longitude > 180 {
		return fmt.Errorf("longitude must be between -180 and 180")
	}
	return nil
}

type ObservationQuality string

const (
	QualityVerified    ObservationQuality = "verified"
	QualityProvisional ObservationQuality = "provisional"
	QualityRejected    ObservationQuality = "rejected"
)

func (q ObservationQuality) Valid() bool {
	return q == QualityVerified || q == QualityProvisional || q == QualityRejected
}

type Observation struct {
	ID             string             `json:"id"`
	SensorID       string             `json:"sensor_id"`
	SourceSequence string             `json:"source_sequence"`
	ObservedAt     time.Time          `json:"observed_at"`
	Metric         string             `json:"metric"`
	Value          float64            `json:"value"`
	Unit           string             `json:"unit"`
	Quality        ObservationQuality `json:"quality"`
	PayloadJSON    string             `json:"payload_json"`
	CreatedBy      string             `json:"created_by"`
	CreatedAt      time.Time          `json:"created_at"`
}

func (o Observation) Validate(now time.Time) error {
	if strings.TrimSpace(o.SensorID) == "" {
		return fmt.Errorf("sensor is required")
	}
	if strings.TrimSpace(o.SourceSequence) == "" {
		return fmt.Errorf("source sequence is required")
	}
	if o.ObservedAt.IsZero() {
		return fmt.Errorf("observed time is required")
	}
	if o.ObservedAt.After(now.Add(5 * time.Minute)) {
		return fmt.Errorf("observed time is too far in the future")
	}
	if strings.TrimSpace(o.Metric) == "" || strings.TrimSpace(o.Unit) == "" {
		return fmt.Errorf("metric and unit are required")
	}
	if !o.Quality.Valid() {
		return fmt.Errorf("invalid observation quality %q", o.Quality)
	}
	if !strings.HasPrefix(strings.TrimSpace(o.PayloadJSON), "{") {
		return fmt.Errorf("payload must be a JSON object")
	}
	return nil
}

type Zone struct {
	ID           string    `json:"id"`
	IncidentID   string    `json:"incident_id"`
	RegionID     string    `json:"region_id"`
	Name         string    `json:"name"`
	RiskLevel    int       `json:"risk_level"`
	Population   int       `json:"population"`
	GeometryJSON string    `json:"geometry_json"`
	CreatedAt    time.Time `json:"created_at"`
}

func (z Zone) Validate() error {
	if strings.TrimSpace(z.Name) == "" {
		return fmt.Errorf("zone name is required")
	}
	if z.RiskLevel < 1 || z.RiskLevel > 5 {
		return fmt.Errorf("zone risk level must be between 1 and 5")
	}
	if z.Population < 0 {
		return fmt.Errorf("zone population cannot be negative")
	}
	if !strings.HasPrefix(strings.TrimSpace(z.GeometryJSON), "{") {
		return fmt.Errorf("zone geometry must be a JSON object")
	}
	return nil
}

type Incident struct {
	ID           string         `json:"id"`
	RegionID     string         `json:"region_id"`
	ExternalRef  string         `json:"external_ref"`
	HazardType   HazardType     `json:"hazard_type"`
	Title        string         `json:"title"`
	Severity     int            `json:"severity"`
	Status       IncidentStatus `json:"status"`
	CommandLevel CommandLevel   `json:"command_level"`
	Summary      string         `json:"summary"`
	OccurredAt   time.Time      `json:"occurred_at"`
	ActivatedAt  *time.Time     `json:"activated_at,omitempty"`
	ClosedAt     *time.Time     `json:"closed_at,omitempty"`
	Version      int64          `json:"version"`
	CreatedBy    string         `json:"created_by"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

func (i Incident) Validate(now time.Time) error {
	if strings.TrimSpace(i.RegionID) == "" {
		return fmt.Errorf("incident region is required")
	}
	if strings.TrimSpace(i.ExternalRef) == "" {
		return fmt.Errorf("external reference is required")
	}
	if !i.HazardType.Valid() {
		return fmt.Errorf("invalid hazard type %q", i.HazardType)
	}
	if strings.TrimSpace(i.Title) == "" {
		return fmt.Errorf("incident title is required")
	}
	if i.Severity < 1 || i.Severity > 5 {
		return fmt.Errorf("severity must be between 1 and 5")
	}
	if !i.CommandLevel.Valid() {
		return fmt.Errorf("invalid command level %q", i.CommandLevel)
	}
	if i.OccurredAt.IsZero() || i.OccurredAt.After(now.Add(5*time.Minute)) {
		return fmt.Errorf("incident occurrence time is invalid")
	}
	return nil
}

type IncidentFilter struct {
	RegionID    string
	Statuses    []IncidentStatus
	Hazard      HazardType
	MinSeverity int
	Cursor      string
	Limit       int
}

type IncidentPage struct {
	Items      []Incident `json:"items"`
	NextCursor string     `json:"next_cursor,omitempty"`
	Total      int        `json:"total"`
}
