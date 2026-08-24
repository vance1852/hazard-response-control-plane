package sqlite

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/vance1852/hazard-response-control-plane/internal/apperr"
	"github.com/vance1852/hazard-response-control-plane/internal/audit"
	"github.com/vance1852/hazard-response-control-plane/internal/hazard"
)

func (s *Store) CreateRegion(ctx context.Context, region hazard.Region) (hazard.Region, error) {
	if region.ID == "" {
		id, err := newID("reg")
		if err != nil {
			return hazard.Region{}, err
		}
		region.ID = id
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO regions(id, code, name, timezone, active, created_at, updated_at)
		VALUES(?, ?, ?, ?, ?, ?, ?)`, region.ID, region.Code, region.Name, region.Timezone, region.Active,
		formatTime(region.CreatedAt), formatTime(region.UpdatedAt))
	if err != nil {
		if isUniqueViolation(err) {
			return hazard.Region{}, apperr.Conflict("region_exists", "region code already exists")
		}
		return hazard.Region{}, fmt.Errorf("insert region: %w", err)
	}
	return region, nil
}

func (s *Store) FindRegion(ctx context.Context, id string) (hazard.Region, error) {
	var region hazard.Region
	var createdAt, updatedAt string
	err := s.db.QueryRowContext(ctx, `SELECT id, code, name, timezone, active, created_at, updated_at FROM regions WHERE id = ?`, id).
		Scan(&region.ID, &region.Code, &region.Name, &region.Timezone, &region.Active, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return hazard.Region{}, apperr.NotFound("region_not_found", "region was not found")
	}
	if err != nil {
		return hazard.Region{}, fmt.Errorf("scan region: %w", err)
	}
	if region.CreatedAt, err = parseTime(createdAt); err != nil {
		return hazard.Region{}, err
	}
	if region.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return hazard.Region{}, err
	}
	return region, nil
}

func (s *Store) CreateSensor(ctx context.Context, sensor hazard.Sensor) (hazard.Sensor, error) {
	if sensor.ID == "" {
		id, err := newID("sen")
		if err != nil {
			return hazard.Sensor{}, err
		}
		sensor.ID = id
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO sensors(id, region_id, station_code, name, kind, latitude, longitude,
		active, version, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, sensor.ID, sensor.RegionID,
		sensor.StationCode, sensor.Name, string(sensor.Kind), sensor.Latitude, sensor.Longitude, sensor.Active,
		sensor.Version, formatTime(sensor.CreatedAt), formatTime(sensor.UpdatedAt))
	if err != nil {
		if isUniqueViolation(err) {
			return hazard.Sensor{}, apperr.Conflict("sensor_exists", "station code already exists in region")
		}
		return hazard.Sensor{}, fmt.Errorf("insert sensor: %w", err)
	}
	return sensor, nil
}

func (s *Store) FindSensor(ctx context.Context, id string) (hazard.Sensor, error) {
	return scanSensor(s.db.QueryRowContext(ctx, `SELECT id, region_id, station_code, name, kind, latitude, longitude,
		active, version, created_at, updated_at FROM sensors WHERE id = ?`, id))
}

func scanSensor(row rowScanner) (hazard.Sensor, error) {
	var sensor hazard.Sensor
	var kind, createdAt, updatedAt string
	if err := row.Scan(&sensor.ID, &sensor.RegionID, &sensor.StationCode, &sensor.Name, &kind, &sensor.Latitude,
		&sensor.Longitude, &sensor.Active, &sensor.Version, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return hazard.Sensor{}, apperr.NotFound("sensor_not_found", "sensor was not found")
		}
		return hazard.Sensor{}, fmt.Errorf("scan sensor: %w", err)
	}
	sensor.Kind = hazard.SensorKind(kind)
	var err error
	if sensor.CreatedAt, err = parseTime(createdAt); err != nil {
		return hazard.Sensor{}, err
	}
	if sensor.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return hazard.Sensor{}, err
	}
	return sensor, nil
}

func (s *Store) InsertObservation(ctx context.Context, observation hazard.Observation) (hazard.Observation, bool, error) {
	if observation.ID == "" {
		id, err := newID("obs")
		if err != nil {
			return hazard.Observation{}, false, err
		}
		observation.ID = id
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO observations(id, sensor_id, source_sequence, observed_at, metric, value,
		unit, quality, payload_json, created_by, created_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		observation.ID, observation.SensorID, observation.SourceSequence, formatTime(observation.ObservedAt), observation.Metric,
		observation.Value, observation.Unit, string(observation.Quality), observation.PayloadJSON, observation.CreatedBy,
		formatTime(observation.CreatedAt))
	if err == nil {
		return observation, false, nil
	}
	if !isUniqueViolation(err) {
		return hazard.Observation{}, false, fmt.Errorf("insert observation: %w", err)
	}
	existing, findErr := s.FindObservationBySequence(ctx, observation.SensorID, observation.SourceSequence)
	if findErr != nil {
		return hazard.Observation{}, false, errors.Join(err, findErr)
	}
	if existing.Metric != observation.Metric || existing.Unit != observation.Unit || existing.Value != observation.Value ||
		!existing.ObservedAt.Equal(observation.ObservedAt) || existing.PayloadJSON != observation.PayloadJSON {
		return hazard.Observation{}, false, apperr.Conflict("observation_sequence_reused", "source sequence was reused with different observation data")
	}
	return existing, true, nil
}

func (s *Store) FindObservationBySequence(ctx context.Context, sensorID, sequence string) (hazard.Observation, error) {
	var observation hazard.Observation
	var observedAt, createdAt, quality string
	err := s.db.QueryRowContext(ctx, `SELECT id, sensor_id, source_sequence, observed_at, metric, value, unit, quality,
		payload_json, created_by, created_at FROM observations WHERE sensor_id = ? AND source_sequence = ?`, sensorID, sequence).
		Scan(&observation.ID, &observation.SensorID, &observation.SourceSequence, &observedAt, &observation.Metric,
			&observation.Value, &observation.Unit, &quality, &observation.PayloadJSON, &observation.CreatedBy, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return hazard.Observation{}, apperr.NotFound("observation_not_found", "observation was not found")
	}
	if err != nil {
		return hazard.Observation{}, fmt.Errorf("scan observation: %w", err)
	}
	observation.Quality = hazard.ObservationQuality(quality)
	if observation.ObservedAt, err = parseTime(observedAt); err != nil {
		return hazard.Observation{}, err
	}
	if observation.CreatedAt, err = parseTime(createdAt); err != nil {
		return hazard.Observation{}, err
	}
	return observation, nil
}

func (s *Store) ActivateIncident(ctx context.Context, record hazard.ActivationRecord) (hazard.Incident, []hazard.Zone, error) {
	incident := record.Incident
	if incident.ID == "" {
		id, err := newID("inc")
		if err != nil {
			return hazard.Incident{}, nil, err
		}
		incident.ID = id
	}
	zones := make([]hazard.Zone, len(record.Zones))
	copy(zones, record.Zones)
	for index := range zones {
		if zones[index].ID == "" {
			id, err := newID("zon")
			if err != nil {
				return hazard.Incident{}, nil, err
			}
			zones[index].ID = id
		}
		zones[index].IncidentID = incident.ID
	}
	if record.Audit.ID == "" {
		id, err := newID("aud")
		if err != nil {
			return hazard.Incident{}, nil, err
		}
		record.Audit.ID = id
	}
	outboxID, err := newID("out")
	if err != nil {
		return hazard.Incident{}, nil, err
	}
	err = s.WithinTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO incidents(id, region_id, external_ref, hazard_type, title, severity,
			status, command_level, summary, occurred_at, activated_at, closed_at, version, created_by, created_at, updated_at)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, ?, ?, ?, ?)`, incident.ID, incident.RegionID,
			incident.ExternalRef, string(incident.HazardType), incident.Title, incident.Severity, string(incident.Status),
			string(incident.CommandLevel), incident.Summary, formatTime(incident.OccurredAt), nullableTime(incident.ActivatedAt),
			incident.Version, incident.CreatedBy, formatTime(incident.CreatedAt), formatTime(incident.UpdatedAt))
		if err != nil {
			if isUniqueViolation(err) {
				return apperr.Conflict("incident_exists", "incident external reference already exists in region")
			}
			return fmt.Errorf("insert incident: %w", err)
		}
		for _, zone := range zones {
			if _, err := tx.ExecContext(ctx, `INSERT INTO incident_zones(id, incident_id, region_id, name, risk_level,
				population, geometry_json, created_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?)`, zone.ID, incident.ID, zone.RegionID,
				zone.Name, zone.RiskLevel, zone.Population, zone.GeometryJSON, formatTime(zone.CreatedAt)); err != nil {
				return fmt.Errorf("insert incident zone %q: %w", zone.Name, err)
			}
		}
		if err := appendAudit(ctx, tx, record.Audit); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return hazard.Incident{}, nil, err
	}
	if err := s.appendActivationOutbox(ctx, outboxID, incident.ID, record); err != nil {
		return hazard.Incident{}, nil, err
	}
	return incident, zones, nil
}

func (s *Store) FindIncident(ctx context.Context, id string) (hazard.Incident, error) {
	return scanIncident(s.db.QueryRowContext(ctx, incidentSelect+` WHERE id = ?`, id))
}

const incidentSelect = `SELECT id, region_id, external_ref, hazard_type, title, severity, status, command_level,
	summary, occurred_at, activated_at, closed_at, version, created_by, created_at, updated_at FROM incidents`

func scanIncident(row rowScanner) (hazard.Incident, error) {
	var incident hazard.Incident
	var hazardType, status, level, occurredAt, createdAt, updatedAt string
	var activatedAt, closedAt sql.NullString
	if err := row.Scan(&incident.ID, &incident.RegionID, &incident.ExternalRef, &hazardType, &incident.Title,
		&incident.Severity, &status, &level, &incident.Summary, &occurredAt, &activatedAt, &closedAt,
		&incident.Version, &incident.CreatedBy, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return hazard.Incident{}, apperr.NotFound("incident_not_found", "incident was not found")
		}
		return hazard.Incident{}, fmt.Errorf("scan incident: %w", err)
	}
	incident.HazardType, incident.Status, incident.CommandLevel = hazard.HazardType(hazardType), hazard.IncidentStatus(status), hazard.CommandLevel(level)
	var err error
	if incident.OccurredAt, err = parseTime(occurredAt); err != nil {
		return hazard.Incident{}, err
	}
	if incident.CreatedAt, err = parseTime(createdAt); err != nil {
		return hazard.Incident{}, err
	}
	if incident.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return hazard.Incident{}, err
	}
	if activatedAt.Valid {
		value, parseErr := parseTime(activatedAt.String)
		if parseErr != nil {
			return hazard.Incident{}, parseErr
		}
		incident.ActivatedAt = &value
	}
	if closedAt.Valid {
		value, parseErr := parseTime(closedAt.String)
		if parseErr != nil {
			return hazard.Incident{}, parseErr
		}
		incident.ClosedAt = &value
	}
	return incident, nil
}

func (s *Store) ListIncidentZones(ctx context.Context, incidentID string) ([]hazard.Zone, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, incident_id, region_id, name, risk_level, population, geometry_json,
		created_at FROM incident_zones WHERE incident_id = ? ORDER BY risk_level DESC, name`, incidentID)
	if err != nil {
		return nil, fmt.Errorf("query incident zones: %w", err)
	}
	defer rows.Close()
	result := make([]hazard.Zone, 0)
	for rows.Next() {
		var zone hazard.Zone
		var createdAt string
		if err := rows.Scan(&zone.ID, &zone.IncidentID, &zone.RegionID, &zone.Name, &zone.RiskLevel, &zone.Population,
			&zone.GeometryJSON, &createdAt); err != nil {
			return nil, fmt.Errorf("scan incident zone: %w", err)
		}
		if zone.CreatedAt, err = parseTime(createdAt); err != nil {
			return nil, err
		}
		result = append(result, zone)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate incident zones: %w", err)
	}
	return result, nil
}

func (s *Store) TransitionIncident(ctx context.Context, id string, version int64, target hazard.IncidentStatus, summary string, closedAt *time.Time, event audit.Event) (hazard.Incident, error) {
	if event.ID == "" {
		generated, err := newID("aud")
		if err != nil {
			return hazard.Incident{}, err
		}
		event.ID = generated
	}
	updatedAt := event.CreatedAt
	err := s.WithinTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `UPDATE incidents SET status = ?, summary = CASE WHEN ? = '' THEN summary ELSE ? END,
			closed_at = ?, version = version + 1, updated_at = ? WHERE id = ? AND version = ?`, string(target), summary,
			summary, nullableTime(closedAt), formatTime(updatedAt), id, version)
		if err != nil {
			return fmt.Errorf("update incident state: %w", err)
		}
		count, err := rowsAffected(result, "update incident state")
		if err != nil {
			return err
		}
		if count != 1 {
			return apperr.Conflict("version_conflict", "incident changed concurrently")
		}
		return appendAudit(ctx, tx, event)
	})
	if err != nil {
		return hazard.Incident{}, err
	}
	return s.FindIncident(ctx, id)
}

func (s *Store) CountOpenDependencies(ctx context.Context, incidentID string) (int, int, error) {
	var plans, deployments int
	err := s.db.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM evacuation_plans WHERE incident_id = ? AND status IN ('submitted','approved','executing')),
		(SELECT COUNT(*) FROM deployments WHERE incident_id = ? AND status IN ('assigned','acknowledged','en_route','on_scene'))`, incidentID, incidentID).
		Scan(&plans, &deployments)
	if err != nil {
		return 0, 0, fmt.Errorf("count incident dependencies: %w", err)
	}
	return plans, deployments, nil
}

func (s *Store) ListIncidents(ctx context.Context, filter hazard.IncidentFilter) (hazard.IncidentPage, error) {
	where, args, err := incidentWhere(filter)
	if err != nil {
		return hazard.IncidentPage{}, err
	}
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM incidents`+where, args...).Scan(&total); err != nil {
		return hazard.IncidentPage{}, fmt.Errorf("count incidents: %w", err)
	}
	queryWhere, queryArgs := where, append([]any(nil), args...)
	if filter.Cursor != "" {
		updatedAt, id, err := decodeIncidentCursor(filter.Cursor)
		if err != nil {
			return hazard.IncidentPage{}, apperr.Validation("invalid_cursor", "incident cursor is invalid")
		}
		if queryWhere == "" {
			queryWhere = ` WHERE `
		} else {
			queryWhere += ` AND `
		}
		queryWhere += `(updated_at < ? OR (updated_at = ? AND id < ?))`
		queryArgs = append(queryArgs, updatedAt, updatedAt, id)
	}
	queryArgs = append(queryArgs, filter.Limit+1)
	rows, err := s.db.QueryContext(ctx, incidentSelect+queryWhere+` ORDER BY updated_at DESC, id DESC LIMIT ?`, queryArgs...)
	if err != nil {
		return hazard.IncidentPage{}, fmt.Errorf("query incidents: %w", err)
	}
	defer rows.Close()
	items := make([]hazard.Incident, 0, filter.Limit+1)
	for rows.Next() {
		item, err := scanIncident(rows)
		if err != nil {
			return hazard.IncidentPage{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return hazard.IncidentPage{}, fmt.Errorf("iterate incidents: %w", err)
	}
	page := hazard.IncidentPage{Items: items, Total: total}
	if len(items) > filter.Limit {
		last := items[filter.Limit-1]
		page.Items = items[:filter.Limit]
		page.NextCursor = encodeIncidentCursor(last.UpdatedAt, last.ID)
	}
	return page, nil
}

func incidentWhere(filter hazard.IncidentFilter) (string, []any, error) {
	clauses := make([]string, 0, 3)
	args := make([]any, 0, 8)
	if filter.RegionID != "" {
		clauses = append(clauses, `region_id = ?`)
		args = append(args, filter.RegionID)
	}
	if filter.Hazard != "" {
		clauses = append(clauses, `hazard_type = ?`)
		args = append(args, string(filter.Hazard))
	}
	if filter.MinSeverity > 0 {
		clauses = append(clauses, `severity >= ?`)
		args = append(args, filter.MinSeverity)
	}
	if len(filter.Statuses) > 0 {
		placeholders := make([]string, len(filter.Statuses))
		for index, status := range filter.Statuses {
			placeholders[index] = "?"
			args = append(args, string(status))
		}
		clauses = append(clauses, `status IN (`+strings.Join(placeholders, ",")+`)`)
	}
	if len(clauses) == 0 {
		return "", args, nil
	}
	return ` WHERE ` + strings.Join(clauses, ` AND `), args, nil
}

func encodeIncidentCursor(updatedAt time.Time, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(formatTime(updatedAt) + "|" + id))
}

func decodeIncidentCursor(cursor string) (string, string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return "", "", err
	}
	parts := strings.SplitN(string(raw), "|", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid cursor")
	}
	if _, err := parseTime(parts[0]); err != nil {
		return "", "", err
	}
	if parts[1] == "" {
		return "", "", fmt.Errorf("missing cursor id")
	}
	return parts[0], parts[1], nil
}
