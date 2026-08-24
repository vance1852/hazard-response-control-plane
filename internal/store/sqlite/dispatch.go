package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/vance1852/hazard-response-control-plane/internal/apperr"
	"github.com/vance1852/hazard-response-control-plane/internal/audit"
	"github.com/vance1852/hazard-response-control-plane/internal/dispatch"
)

func (s *Store) CreateUnit(ctx context.Context, unit dispatch.Unit) (dispatch.Unit, error) {
	if unit.ID == "" {
		id, err := newID("unt")
		if err != nil {
			return dispatch.Unit{}, err
		}
		unit.ID = id
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO field_units(id, region_id, call_sign, unit_type, capability_json, status,
		operator_id, version, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?, NULLIF(?,''), ?, ?, ?)`, unit.ID,
		unit.RegionID, unit.CallSign, string(unit.Type), unit.CapabilitiesJSON, string(unit.Status), unit.OperatorID,
		unit.Version, formatTime(unit.CreatedAt), formatTime(unit.UpdatedAt))
	if err != nil {
		if isUniqueViolation(err) {
			return dispatch.Unit{}, apperr.Conflict("unit_exists", "unit call sign already exists in region")
		}
		return dispatch.Unit{}, fmt.Errorf("insert field unit: %w", err)
	}
	return unit, nil
}

func (s *Store) FindUnit(ctx context.Context, id string) (dispatch.Unit, error) {
	return scanUnit(s.db.QueryRowContext(ctx, unitSelect+` WHERE id = ?`, id))
}

const unitSelect = `SELECT id, region_id, call_sign, unit_type, capability_json, status, COALESCE(operator_id,''), version,
	created_at, updated_at FROM field_units`

func scanUnit(row rowScanner) (dispatch.Unit, error) {
	var unit dispatch.Unit
	var unitType, status, createdAt, updatedAt string
	if err := row.Scan(&unit.ID, &unit.RegionID, &unit.CallSign, &unitType, &unit.CapabilitiesJSON, &status,
		&unit.OperatorID, &unit.Version, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dispatch.Unit{}, apperr.NotFound("unit_not_found", "field unit was not found")
		}
		return dispatch.Unit{}, fmt.Errorf("scan field unit: %w", err)
	}
	unit.Type, unit.Status = dispatch.UnitType(unitType), dispatch.UnitStatus(status)
	var err error
	if unit.CreatedAt, err = parseTime(createdAt); err != nil {
		return dispatch.Unit{}, err
	}
	if unit.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return dispatch.Unit{}, err
	}
	return unit, nil
}

func (s *Store) CreateRequest(ctx context.Context, request dispatch.Request, event audit.Event) (dispatch.Request, error) {
	var err error
	if request.ID == "" {
		request.ID, err = newID("req")
		if err != nil {
			return dispatch.Request{}, err
		}
	}
	if event.ID == "" {
		event.ID, err = newID("aud")
		if err != nil {
			return dispatch.Request{}, err
		}
	}
	err = s.WithinTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO resource_requests(id, incident_id, zone_id, requested_type, required_capability,
			quantity, priority, status, needed_by, version, created_by, approved_by, created_at, updated_at)
			VALUES(?, ?, NULLIF(?,''), ?, ?, ?, ?, ?, ?, ?, ?, NULL, ?, ?)`, request.ID, request.IncidentID, request.ZoneID,
			string(request.RequestedType), request.RequiredCapability, request.Quantity, request.Priority, string(request.Status),
			formatTime(request.NeededBy), request.Version, request.CreatedBy, formatTime(request.CreatedAt), formatTime(request.UpdatedAt))
		if err != nil {
			return fmt.Errorf("insert resource request: %w", err)
		}
		return appendAudit(ctx, tx, event)
	})
	if err != nil {
		return dispatch.Request{}, err
	}
	return request, nil
}

func (s *Store) FindRequest(ctx context.Context, id string) (dispatch.Request, error) {
	return scanRequest(s.db.QueryRowContext(ctx, requestSelect+` WHERE id = ?`, id))
}

const requestSelect = `SELECT id, incident_id, COALESCE(zone_id,''), requested_type, required_capability, quantity, priority,
	status, needed_by, version, created_by, COALESCE(approved_by,''), created_at, updated_at FROM resource_requests`

func scanRequest(row rowScanner) (dispatch.Request, error) {
	var request dispatch.Request
	var unitType, status, neededBy, createdAt, updatedAt string
	if err := row.Scan(&request.ID, &request.IncidentID, &request.ZoneID, &unitType, &request.RequiredCapability,
		&request.Quantity, &request.Priority, &status, &neededBy, &request.Version, &request.CreatedBy, &request.ApprovedBy,
		&createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dispatch.Request{}, apperr.NotFound("request_not_found", "resource request was not found")
		}
		return dispatch.Request{}, fmt.Errorf("scan resource request: %w", err)
	}
	request.RequestedType, request.Status = dispatch.UnitType(unitType), dispatch.RequestStatus(status)
	var err error
	if request.NeededBy, err = parseTime(neededBy); err != nil {
		return dispatch.Request{}, err
	}
	if request.CreatedAt, err = parseTime(createdAt); err != nil {
		return dispatch.Request{}, err
	}
	if request.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return dispatch.Request{}, err
	}
	return request, nil
}

func (s *Store) ApproveRequest(ctx context.Context, id string, version int64, approver string, now time.Time, event audit.Event) (dispatch.Request, error) {
	if event.ID == "" {
		generated, err := newID("aud")
		if err != nil {
			return dispatch.Request{}, err
		}
		event.ID = generated
	}
	err := s.WithinTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `UPDATE resource_requests SET status='approved', approved_by=?, version=version+1, updated_at=? WHERE id=? AND version=? AND status='requested' AND needed_by>?`, approver, formatTime(now), id, version, formatTime(now))
		if err != nil {
			return err
		}
		count, err := rowsAffected(result, "approve resource request")
		if err != nil {
			return err
		}
		if count != 1 {
			return apperr.Conflict("version_conflict", "resource request changed, expired, or is not requested")
		}
		return appendAudit(ctx, tx, event)
	})
	if err != nil {
		return dispatch.Request{}, err
	}
	return s.FindRequest(ctx, id)
}

func (s *Store) AllocateUnit(ctx context.Context, requestID string, requestVersion int64, unitID string, unitVersion int64, actorID string, now time.Time, event audit.Event) (dispatch.Request, dispatch.Deployment, dispatch.Unit, error) {
	deploymentID, err := newID("dep")
	if err != nil {
		return dispatch.Request{}, dispatch.Deployment{}, dispatch.Unit{}, err
	}
	if event.ID == "" {
		event.ID, err = newID("aud")
		if err != nil {
			return dispatch.Request{}, dispatch.Deployment{}, dispatch.Unit{}, err
		}
	}
	deployment := dispatch.Deployment{ID: deploymentID, RequestID: requestID, UnitID: unitID, Status: dispatch.DeploymentAssigned, AssignedBy: actorID, Version: 1, CreatedAt: now, UpdatedAt: now}
	err = s.WithinTx(ctx, func(tx *sql.Tx) error {
		var request dispatch.Request
		var requestStatus string
		if err := tx.QueryRowContext(ctx, `SELECT incident_id, quantity, status, version FROM resource_requests WHERE id=?`, requestID).Scan(&request.IncidentID, &request.Quantity, &requestStatus, &request.Version); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return apperr.NotFound("request_not_found", "resource request was not found")
			}
			return err
		}
		if request.Version != requestVersion || (requestStatus != "approved" && requestStatus != "partially_allocated") {
			return apperr.Conflict("version_conflict", "resource request changed or is not allocatable")
		}
		result, err := tx.ExecContext(ctx, `UPDATE field_units SET status='deployed', version=version+1, updated_at=? WHERE id=? AND version=? AND status='available'`, formatTime(now), unitID, unitVersion)
		if err != nil {
			return err
		}
		count, err := rowsAffected(result, "claim field unit")
		if err != nil {
			return err
		}
		if count != 1 {
			return apperr.Conflict("unit_unavailable", "field unit changed or is no longer available")
		}
		var allocated int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM deployments WHERE request_id=? AND status IN ('assigned','acknowledged','en_route','on_scene')`, requestID).Scan(&allocated); err != nil {
			return err
		}
		newStatus := "partially_allocated"
		if allocated+1 >= request.Quantity {
			newStatus = "allocated"
		}
		result, err = tx.ExecContext(ctx, `UPDATE resource_requests SET status=?, version=version+1, updated_at=? WHERE id=? AND version=? AND status IN ('approved','partially_allocated')`, newStatus, formatTime(now), requestID, requestVersion)
		if err != nil {
			return err
		}
		count, err = rowsAffected(result, "update allocation count")
		if err != nil {
			return err
		}
		if count != 1 {
			return apperr.Conflict("version_conflict", "resource request changed during allocation")
		}
		deployment.IncidentID = request.IncidentID
		_, err = tx.ExecContext(ctx, `INSERT INTO deployments(id, request_id, unit_id, incident_id, status, assigned_by, version, created_at, updated_at) VALUES(?, ?, ?, ?, 'assigned', ?, 1, ?, ?)`, deployment.ID, requestID, unitID, request.IncidentID, actorID, formatTime(now), formatTime(now))
		if err != nil {
			if isUniqueViolation(err) {
				return apperr.Conflict("unit_unavailable", "field unit already has an active deployment")
			}
			return err
		}
		return appendAudit(ctx, tx, event)
	})
	if err != nil {
		return dispatch.Request{}, dispatch.Deployment{}, dispatch.Unit{}, err
	}
	request, err := s.FindRequest(ctx, requestID)
	if err != nil {
		return dispatch.Request{}, dispatch.Deployment{}, dispatch.Unit{}, err
	}
	unit, err := s.FindUnit(ctx, unitID)
	if err != nil {
		return dispatch.Request{}, dispatch.Deployment{}, dispatch.Unit{}, err
	}
	return request, deployment, unit, nil
}

func (s *Store) FindDeployment(ctx context.Context, id string) (dispatch.Deployment, error) {
	return scanDeployment(s.db.QueryRowContext(ctx, deploymentSelect+` WHERE id=?`, id))
}

const deploymentSelect = `SELECT id, request_id, unit_id, incident_id, status, assigned_by, acknowledged_at, arrived_at,
	released_at, version, created_at, updated_at FROM deployments`

func scanDeployment(row rowScanner) (dispatch.Deployment, error) {
	var item dispatch.Deployment
	var status, createdAt, updatedAt string
	var acknowledgedAt, arrivedAt, releasedAt sql.NullString
	if err := row.Scan(&item.ID, &item.RequestID, &item.UnitID, &item.IncidentID, &status, &item.AssignedBy,
		&acknowledgedAt, &arrivedAt, &releasedAt, &item.Version, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dispatch.Deployment{}, apperr.NotFound("deployment_not_found", "deployment was not found")
		}
		return dispatch.Deployment{}, fmt.Errorf("scan deployment: %w", err)
	}
	item.Status = dispatch.DeploymentStatus(status)
	var err error
	if item.CreatedAt, err = parseTime(createdAt); err != nil {
		return item, err
	}
	if item.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return item, err
	}
	if acknowledgedAt.Valid {
		value, e := parseTime(acknowledgedAt.String)
		if e != nil {
			return item, e
		}
		item.AcknowledgedAt = &value
	}
	if arrivedAt.Valid {
		value, e := parseTime(arrivedAt.String)
		if e != nil {
			return item, e
		}
		item.ArrivedAt = &value
	}
	if releasedAt.Valid {
		value, e := parseTime(releasedAt.String)
		if e != nil {
			return item, e
		}
		item.ReleasedAt = &value
	}
	return item, nil
}

func (s *Store) TransitionDeployment(ctx context.Context, id string, version int64, target dispatch.DeploymentStatus, actorID string, now time.Time, event audit.Event) (dispatch.Deployment, *dispatch.Unit, error) {
	if event.ID == "" {
		generated, err := newID("aud")
		if err != nil {
			return dispatch.Deployment{}, nil, err
		}
		event.ID = generated
	}
	var released bool
	var unitID, requestID string
	err := s.WithinTx(ctx, func(tx *sql.Tx) error {
		var status string
		if err := tx.QueryRowContext(ctx, `SELECT status, unit_id, request_id FROM deployments WHERE id=? AND version=?`, id, version).Scan(&status, &unitID, &requestID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return apperr.Conflict("version_conflict", "deployment changed concurrently")
			}
			return err
		}
		current := dispatch.DeploymentStatus(status)
		if !current.CanTransition(target) {
			return apperr.Conflict("invalid_transition", "deployment transition is not allowed")
		}
		acknowledged, arrived, releasedAt := any(nil), any(nil), any(nil)
		if target == dispatch.DeploymentAcknowledged {
			acknowledged = formatTime(now)
		}
		if target == dispatch.DeploymentOnScene {
			arrived = formatTime(now)
		}
		if target == dispatch.DeploymentReleased || target == dispatch.DeploymentCancelled {
			releasedAt, released = formatTime(now), true
		}
		result, err := tx.ExecContext(ctx, `UPDATE deployments SET status=?, acknowledged_at=COALESCE(?,acknowledged_at), arrived_at=COALESCE(?,arrived_at), released_at=COALESCE(?,released_at), version=version+1, updated_at=? WHERE id=? AND version=?`, string(target), acknowledged, arrived, releasedAt, formatTime(now), id, version)
		if err != nil {
			return err
		}
		count, err := rowsAffected(result, "transition deployment")
		if err != nil {
			return err
		}
		if count != 1 {
			return apperr.Conflict("version_conflict", "deployment changed concurrently")
		}
		if released {
			result, err = tx.ExecContext(ctx, `UPDATE field_units SET status='available', version=version+1, updated_at=? WHERE id=? AND status='deployed'`, formatTime(now), unitID)
			if err != nil {
				return err
			}
			count, err = rowsAffected(result, "release field unit")
			if err != nil {
				return err
			}
			if count != 1 {
				return apperr.Conflict("unit_state_conflict", "deployed unit cannot be released")
			}
			var active int
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM deployments WHERE request_id=? AND status IN ('assigned','acknowledged','en_route','on_scene') AND id<>?`, requestID, id).Scan(&active); err != nil {
				return err
			}
			if active == 0 {
				_, err = tx.ExecContext(ctx, `UPDATE resource_requests SET status='fulfilled', version=version+1, updated_at=? WHERE id=? AND status IN ('allocated','partially_allocated')`, formatTime(now), requestID)
				if err != nil {
					return err
				}
			}
		}
		return appendAudit(ctx, tx, event)
	})
	if err != nil {
		return dispatch.Deployment{}, nil, err
	}
	deployment, err := s.FindDeployment(ctx, id)
	if err != nil {
		return dispatch.Deployment{}, nil, err
	}
	if !released {
		return deployment, nil, nil
	}
	unit, err := s.FindUnit(ctx, unitID)
	return deployment, &unit, err
}

func (s *Store) ListRequests(ctx context.Context, filter dispatch.RequestFilter) (dispatch.RequestPage, error) {
	clauses, args := make([]string, 0, 3), make([]any, 0, 8)
	if filter.IncidentID != "" {
		clauses = append(clauses, `incident_id=?`)
		args = append(args, filter.IncidentID)
	}
	if filter.PriorityAtLeast > 0 {
		clauses = append(clauses, `priority>=?`)
		args = append(args, filter.PriorityAtLeast)
	}
	if len(filter.Statuses) > 0 {
		p := make([]string, len(filter.Statuses))
		for i, v := range filter.Statuses {
			p[i] = "?"
			args = append(args, string(v))
		}
		clauses = append(clauses, `status IN (`+strings.Join(p, ",")+`)`)
	}
	where := ""
	if len(clauses) > 0 {
		where = ` WHERE ` + strings.Join(clauses, ` AND `)
	}
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM resource_requests`+where, args...).Scan(&total); err != nil {
		return dispatch.RequestPage{}, err
	}
	queryArgs := append(append([]any(nil), args...), filter.Limit, filter.Offset)
	rows, err := s.db.QueryContext(ctx, requestSelect+where+` ORDER BY priority DESC, needed_by, id LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return dispatch.RequestPage{}, err
	}
	defer rows.Close()
	items := make([]dispatch.Request, 0, filter.Limit)
	for rows.Next() {
		item, err := scanRequest(rows)
		if err != nil {
			return dispatch.RequestPage{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return dispatch.RequestPage{}, err
	}
	return dispatch.RequestPage{Items: items, Total: total}, nil
}
