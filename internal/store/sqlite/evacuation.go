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
	"github.com/vance1852/hazard-response-control-plane/internal/evacuation"
	"github.com/vance1852/hazard-response-control-plane/internal/hazard"
)

func (s *Store) CreateShelter(ctx context.Context, shelter evacuation.Shelter) (evacuation.Shelter, error) {
	if shelter.ID == "" {
		id, err := newID("shl")
		if err != nil {
			return evacuation.Shelter{}, err
		}
		shelter.ID = id
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO shelters(id, region_id, code, name, capacity, reserved, status, version,
		created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, shelter.ID, shelter.RegionID, shelter.Code,
		shelter.Name, shelter.Capacity, shelter.Reserved, string(shelter.Status), shelter.Version,
		formatTime(shelter.CreatedAt), formatTime(shelter.UpdatedAt))
	if err != nil {
		if isUniqueViolation(err) {
			return evacuation.Shelter{}, apperr.Conflict("shelter_exists", "shelter code already exists in region")
		}
		return evacuation.Shelter{}, fmt.Errorf("insert shelter: %w", err)
	}
	return shelter, nil
}

func (s *Store) FindShelter(ctx context.Context, id string) (evacuation.Shelter, error) {
	return scanShelter(s.db.QueryRowContext(ctx, shelterSelect+` WHERE id = ?`, id))
}

const shelterSelect = `SELECT id, region_id, code, name, capacity, reserved, status, version, created_at, updated_at FROM shelters`

func scanShelter(row rowScanner) (evacuation.Shelter, error) {
	var shelter evacuation.Shelter
	var status, createdAt, updatedAt string
	if err := row.Scan(&shelter.ID, &shelter.RegionID, &shelter.Code, &shelter.Name, &shelter.Capacity,
		&shelter.Reserved, &status, &shelter.Version, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return evacuation.Shelter{}, apperr.NotFound("shelter_not_found", "shelter was not found")
		}
		return evacuation.Shelter{}, fmt.Errorf("scan shelter: %w", err)
	}
	shelter.Status = evacuation.ShelterStatus(status)
	var err error
	if shelter.CreatedAt, err = parseTime(createdAt); err != nil {
		return evacuation.Shelter{}, err
	}
	if shelter.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return evacuation.Shelter{}, err
	}
	return shelter, nil
}

func (s *Store) ListShelters(ctx context.Context, filter evacuation.ShelterFilter) (evacuation.ShelterPage, error) {
	clauses := []string{`capacity - reserved >= ?`}
	args := []any{filter.MinimumCapacity}
	if filter.RegionID != "" {
		clauses = append(clauses, `region_id = ?`)
		args = append(args, filter.RegionID)
	}
	if filter.Status != "" {
		clauses = append(clauses, `status = ?`)
		args = append(args, string(filter.Status))
	}
	where := ` WHERE ` + strings.Join(clauses, ` AND `)
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM shelters`+where, args...).Scan(&total); err != nil {
		return evacuation.ShelterPage{}, fmt.Errorf("count shelters: %w", err)
	}
	queryArgs := append(append([]any(nil), args...), filter.Limit, filter.Offset)
	rows, err := s.db.QueryContext(ctx, shelterSelect+where+` ORDER BY (capacity-reserved) DESC, name, id LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return evacuation.ShelterPage{}, fmt.Errorf("query shelters: %w", err)
	}
	defer rows.Close()
	items := make([]evacuation.Shelter, 0, filter.Limit)
	for rows.Next() {
		item, err := scanShelter(rows)
		if err != nil {
			return evacuation.ShelterPage{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return evacuation.ShelterPage{}, fmt.Errorf("iterate shelters: %w", err)
	}
	return evacuation.ShelterPage{Items: items, Total: total}, nil
}

func (s *Store) FindZone(ctx context.Context, id string) (hazard.Zone, error) {
	var zone hazard.Zone
	var createdAt string
	err := s.db.QueryRowContext(ctx, `SELECT id, incident_id, region_id, name, risk_level, population, geometry_json,
		created_at FROM incident_zones WHERE id = ?`, id).Scan(&zone.ID, &zone.IncidentID, &zone.RegionID, &zone.Name,
		&zone.RiskLevel, &zone.Population, &zone.GeometryJSON, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return hazard.Zone{}, apperr.NotFound("zone_not_found", "incident zone was not found")
	}
	if err != nil {
		return hazard.Zone{}, fmt.Errorf("scan incident zone: %w", err)
	}
	if zone.CreatedAt, err = parseTime(createdAt); err != nil {
		return hazard.Zone{}, err
	}
	return zone, nil
}

func (s *Store) CreatePlan(ctx context.Context, plan evacuation.Plan, steps []evacuation.Step, event audit.Event) (evacuation.Plan, []evacuation.Step, error) {
	if plan.ID == "" {
		id, err := newID("evp")
		if err != nil {
			return evacuation.Plan{}, nil, err
		}
		plan.ID = id
	}
	createdSteps := append([]evacuation.Step(nil), steps...)
	for index := range createdSteps {
		if createdSteps[index].ID == "" {
			id, err := newID("evs")
			if err != nil {
				return evacuation.Plan{}, nil, err
			}
			createdSteps[index].ID = id
		}
		createdSteps[index].PlanID = plan.ID
	}
	if event.ID == "" {
		id, err := newID("aud")
		if err != nil {
			return evacuation.Plan{}, nil, err
		}
		event.ID = id
	}
	err := s.WithinTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO evacuation_plans(id, incident_id, zone_id, shelter_id, name,
			evacuee_count, status, deadline_at, approved_by, approved_at, completed_at, version, created_by, created_at,
			updated_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, NULL, ?, ?, ?, ?)`, plan.ID, plan.IncidentID,
			plan.ZoneID, plan.ShelterID, plan.Name, plan.EvacueeCount, string(plan.Status), formatTime(plan.DeadlineAt),
			plan.Version, plan.CreatedBy, formatTime(plan.CreatedAt), formatTime(plan.UpdatedAt))
		if err != nil {
			if isUniqueViolation(err) {
				return apperr.Conflict("plan_exists", "plan name already exists for zone")
			}
			return fmt.Errorf("insert evacuation plan: %w", err)
		}
		for _, step := range createdSteps {
			_, err := tx.ExecContext(ctx, `INSERT INTO evacuation_steps(id, plan_id, step_order, instruction, responsible_role,
				expected_minutes, completed_at, completed_by, created_at) VALUES(?, ?, ?, ?, ?, ?, NULL, NULL, ?)`, step.ID,
				plan.ID, step.Order, step.Instruction, step.ResponsibleRole, step.ExpectedMinutes, formatTime(step.CreatedAt))
			if err != nil {
				return fmt.Errorf("insert evacuation step %d: %w", step.Order, err)
			}
		}
		return appendAudit(ctx, tx, event)
	})
	if err != nil {
		return evacuation.Plan{}, nil, err
	}
	return plan, createdSteps, nil
}

func (s *Store) FindPlan(ctx context.Context, id string) (evacuation.Plan, error) {
	return scanPlan(s.db.QueryRowContext(ctx, planSelect+` WHERE id = ?`, id))
}

const planSelect = `SELECT id, incident_id, zone_id, shelter_id, name, evacuee_count, status, deadline_at,
	COALESCE(approved_by,''), approved_at, completed_at, version, created_by, created_at, updated_at FROM evacuation_plans`

func scanPlan(row rowScanner) (evacuation.Plan, error) {
	var plan evacuation.Plan
	var status, deadlineAt, createdAt, updatedAt string
	var approvedAt, completedAt sql.NullString
	if err := row.Scan(&plan.ID, &plan.IncidentID, &plan.ZoneID, &plan.ShelterID, &plan.Name, &plan.EvacueeCount,
		&status, &deadlineAt, &plan.ApprovedBy, &approvedAt, &completedAt, &plan.Version, &plan.CreatedBy, &createdAt,
		&updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return evacuation.Plan{}, apperr.NotFound("plan_not_found", "evacuation plan was not found")
		}
		return evacuation.Plan{}, fmt.Errorf("scan evacuation plan: %w", err)
	}
	plan.Status = evacuation.PlanStatus(status)
	var err error
	if plan.DeadlineAt, err = parseTime(deadlineAt); err != nil {
		return evacuation.Plan{}, err
	}
	if plan.CreatedAt, err = parseTime(createdAt); err != nil {
		return evacuation.Plan{}, err
	}
	if plan.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return evacuation.Plan{}, err
	}
	if approvedAt.Valid {
		value, parseErr := parseTime(approvedAt.String)
		if parseErr != nil {
			return evacuation.Plan{}, parseErr
		}
		plan.ApprovedAt = &value
	}
	if completedAt.Valid {
		value, parseErr := parseTime(completedAt.String)
		if parseErr != nil {
			return evacuation.Plan{}, parseErr
		}
		plan.CompletedAt = &value
	}
	return plan, nil
}

func (s *Store) ListSteps(ctx context.Context, planID string) ([]evacuation.Step, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, plan_id, step_order, instruction, responsible_role, expected_minutes,
		completed_at, COALESCE(completed_by,''), created_at FROM evacuation_steps WHERE plan_id = ? ORDER BY step_order`, planID)
	if err != nil {
		return nil, fmt.Errorf("query evacuation steps: %w", err)
	}
	defer rows.Close()
	result := make([]evacuation.Step, 0)
	for rows.Next() {
		var step evacuation.Step
		var completedAt sql.NullString
		var createdAt string
		if err := rows.Scan(&step.ID, &step.PlanID, &step.Order, &step.Instruction, &step.ResponsibleRole,
			&step.ExpectedMinutes, &completedAt, &step.CompletedBy, &createdAt); err != nil {
			return nil, fmt.Errorf("scan evacuation step: %w", err)
		}
		if step.CreatedAt, err = parseTime(createdAt); err != nil {
			return nil, err
		}
		if completedAt.Valid {
			value, parseErr := parseTime(completedAt.String)
			if parseErr != nil {
				return nil, parseErr
			}
			step.CompletedAt = &value
		}
		result = append(result, step)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate evacuation steps: %w", err)
	}
	return result, nil
}

func (s *Store) SubmitPlan(ctx context.Context, id string, version int64, now time.Time, event audit.Event) (evacuation.Plan, error) {
	if err := s.updatePlanState(ctx, id, version, evacuation.PlanDraft, evacuation.PlanSubmitted, now, nil, event); err != nil {
		return evacuation.Plan{}, err
	}
	return s.FindPlan(ctx, id)
}

func (s *Store) StartPlan(ctx context.Context, id string, version int64, now time.Time, event audit.Event) (evacuation.Plan, error) {
	if err := s.updatePlanState(ctx, id, version, evacuation.PlanApproved, evacuation.PlanExecuting, now, nil, event); err != nil {
		return evacuation.Plan{}, err
	}
	return s.FindPlan(ctx, id)
}

func (s *Store) updatePlanState(ctx context.Context, id string, version int64, from, to evacuation.PlanStatus, now time.Time, completedAt *time.Time, event audit.Event) error {
	if event.ID == "" {
		generated, err := newID("aud")
		if err != nil {
			return err
		}
		event.ID = generated
	}
	return s.WithinTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `UPDATE evacuation_plans SET status = ?, completed_at = ?, version = version + 1,
			updated_at = ? WHERE id = ? AND version = ? AND status = ?`, string(to), nullableTime(completedAt), formatTime(now), id, version, string(from))
		if err != nil {
			return fmt.Errorf("update plan state: %w", err)
		}
		count, err := rowsAffected(result, "update plan state")
		if err != nil {
			return err
		}
		if count != 1 {
			return apperr.Conflict("version_conflict", "evacuation plan changed concurrently or has another state")
		}
		return appendAudit(ctx, tx, event)
	})
}

func (s *Store) ApprovePlan(ctx context.Context, id string, version int64, approver string, now time.Time, event audit.Event) (evacuation.Plan, evacuation.Reservation, evacuation.Shelter, error) {
	reservationID, err := newID("rsv")
	if err != nil {
		return evacuation.Plan{}, evacuation.Reservation{}, evacuation.Shelter{}, err
	}
	if event.ID == "" {
		event.ID, err = newID("aud")
		if err != nil {
			return evacuation.Plan{}, evacuation.Reservation{}, evacuation.Shelter{}, err
		}
	}
	var reservation evacuation.Reservation
	err = s.WithinTx(ctx, func(tx *sql.Tx) error {
		var plan evacuation.Plan
		var status string
		if err := tx.QueryRowContext(ctx, `SELECT id, shelter_id, evacuee_count, status, version FROM evacuation_plans WHERE id = ?`, id).
			Scan(&plan.ID, &plan.ShelterID, &plan.EvacueeCount, &status, &plan.Version); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return apperr.NotFound("plan_not_found", "evacuation plan was not found")
			}
			return err
		}
		if plan.Version != version || evacuation.PlanStatus(status) != evacuation.PlanSubmitted {
			return apperr.Conflict("version_conflict", "evacuation plan changed concurrently or is not submitted")
		}
		result, err := tx.ExecContext(ctx, `UPDATE shelters SET reserved = reserved + ?, version = version + 1, updated_at = ?
			WHERE id = ? AND status != 'closed' AND capacity - reserved >= ?`, plan.EvacueeCount, formatTime(now), plan.ShelterID, plan.EvacueeCount)
		if err != nil {
			return fmt.Errorf("reserve shelter capacity: %w", err)
		}
		count, err := rowsAffected(result, "reserve shelter capacity")
		if err != nil {
			return err
		}
		if count != 1 {
			return apperr.Conflict("shelter_capacity_unavailable", "shelter no longer has enough capacity")
		}
		result, err = tx.ExecContext(ctx, `UPDATE evacuation_plans SET status = 'approved', approved_by = ?, approved_at = ?,
			version = version + 1, updated_at = ? WHERE id = ? AND version = ? AND status = 'submitted'`, approver,
			formatTime(now), formatTime(now), id, version)
		if err != nil {
			return fmt.Errorf("approve evacuation plan: %w", err)
		}
		count, err = rowsAffected(result, "approve evacuation plan")
		if err != nil {
			return err
		}
		if count != 1 {
			return apperr.Conflict("version_conflict", "evacuation plan changed during approval")
		}
		reservation = evacuation.Reservation{ID: reservationID, ShelterID: plan.ShelterID, PlanID: id, People: plan.EvacueeCount, Status: evacuation.ReservationActive, CreatedAt: now}
		if _, err := tx.ExecContext(ctx, `INSERT INTO shelter_reservations(id, shelter_id, plan_id, people, status, created_at)
			VALUES(?, ?, ?, ?, 'active', ?)`, reservation.ID, reservation.ShelterID, reservation.PlanID, reservation.People, formatTime(now)); err != nil {
			return fmt.Errorf("insert shelter reservation: %w", err)
		}
		return appendAudit(ctx, tx, event)
	})
	if err != nil {
		return evacuation.Plan{}, evacuation.Reservation{}, evacuation.Shelter{}, err
	}
	plan, err := s.FindPlan(ctx, id)
	if err != nil {
		return evacuation.Plan{}, evacuation.Reservation{}, evacuation.Shelter{}, err
	}
	shelter, err := s.FindShelter(ctx, reservation.ShelterID)
	if err != nil {
		return evacuation.Plan{}, evacuation.Reservation{}, evacuation.Shelter{}, err
	}
	return plan, reservation, shelter, nil
}

func (s *Store) CompleteStep(ctx context.Context, planID, stepID string, now time.Time, event audit.Event) (evacuation.Step, error) {
	if event.ID == "" {
		generated, err := newID("aud")
		if err != nil {
			return evacuation.Step{}, err
		}
		event.ID = generated
	}
	err := s.WithinTx(ctx, func(tx *sql.Tx) error {
		query := `UPDATE evacuation_steps SET completed_at = ?, completed_by = ? WHERE id = ? AND plan_id = ? AND completed_at IS NULL` + orderedStepPredicate()
		result, err := tx.ExecContext(ctx, query, formatTime(now), event.ActorID, stepID, planID)
		if err != nil {
			return fmt.Errorf("complete evacuation step: %w", err)
		}
		count, err := rowsAffected(result, "complete evacuation step")
		if err != nil {
			return err
		}
		if count != 1 {
			return apperr.Conflict("step_not_ready", "step is completed, missing, or has an incomplete predecessor")
		}
		return appendAudit(ctx, tx, event)
	})
	if err != nil {
		return evacuation.Step{}, err
	}
	steps, err := s.ListSteps(ctx, planID)
	if err != nil {
		return evacuation.Step{}, err
	}
	for _, step := range steps {
		if step.ID == stepID {
			return step, nil
		}
	}
	return evacuation.Step{}, apperr.NotFound("step_not_found", "evacuation step was not found")
}

func (s *Store) CompletePlan(ctx context.Context, id string, version int64, now time.Time, event audit.Event) (evacuation.Plan, evacuation.Reservation, error) {
	if event.ID == "" {
		generated, err := newID("aud")
		if err != nil {
			return evacuation.Plan{}, evacuation.Reservation{}, err
		}
		event.ID = generated
	}
	var reservation evacuation.Reservation
	err := s.WithinTx(ctx, func(tx *sql.Tx) error {
		var incomplete int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM evacuation_steps WHERE plan_id = ? AND completed_at IS NULL`, id).Scan(&incomplete); err != nil {
			return err
		}
		if incomplete != 0 {
			return apperr.Conflict("steps_incomplete", "all evacuation steps must be completed")
		}
		result, err := tx.ExecContext(ctx, `UPDATE evacuation_plans SET status = 'completed', completed_at = ?, version = version + 1,
			updated_at = ? WHERE id = ? AND version = ? AND status = 'executing'`, formatTime(now), formatTime(now), id, version)
		if err != nil {
			return err
		}
		count, err := rowsAffected(result, "complete evacuation plan")
		if err != nil {
			return err
		}
		if count != 1 {
			return apperr.Conflict("version_conflict", "evacuation plan changed concurrently or is not executing")
		}
		var createdAt string
		if err := tx.QueryRowContext(ctx, `SELECT id, shelter_id, plan_id, people, status, created_at FROM shelter_reservations WHERE plan_id = ?`, id).
			Scan(&reservation.ID, &reservation.ShelterID, &reservation.PlanID, &reservation.People, &reservation.Status, &createdAt); err != nil {
			return fmt.Errorf("find reservation: %w", err)
		}
		reservation.CreatedAt, err = parseTime(createdAt)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE shelter_reservations SET status = 'consumed' WHERE id = ? AND status = 'active'`, reservation.ID); err != nil {
			return err
		}
		reservation.Status = evacuation.ReservationConsumed
		return appendAudit(ctx, tx, event)
	})
	if err != nil {
		return evacuation.Plan{}, evacuation.Reservation{}, err
	}
	plan, err := s.FindPlan(ctx, id)
	return plan, reservation, err
}

func (s *Store) CancelPlan(ctx context.Context, id string, version int64, reason string, now time.Time, event audit.Event) (evacuation.Plan, *evacuation.Reservation, error) {
	if event.ID == "" {
		generated, err := newID("aud")
		if err != nil {
			return evacuation.Plan{}, nil, err
		}
		event.ID = generated
	}
	var released *evacuation.Reservation
	err := s.WithinTx(ctx, func(tx *sql.Tx) error {
		var status string
		var shelterID string
		if err := tx.QueryRowContext(ctx, `SELECT status, shelter_id FROM evacuation_plans WHERE id = ? AND version = ?`, id, version).Scan(&status, &shelterID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return apperr.Conflict("version_conflict", "evacuation plan changed concurrently")
			}
			return err
		}
		current := evacuation.PlanStatus(status)
		if !current.CanTransition(evacuation.PlanCancelled) {
			return apperr.Conflict("invalid_transition", "evacuation plan cannot be cancelled")
		}
		result, err := tx.ExecContext(ctx, `UPDATE evacuation_plans SET status = 'cancelled', version = version + 1, updated_at = ? WHERE id = ? AND version = ?`, formatTime(now), id, version)
		if err != nil {
			return err
		}
		count, err := rowsAffected(result, "cancel evacuation plan")
		if err != nil {
			return err
		}
		if count != 1 {
			return apperr.Conflict("version_conflict", "evacuation plan changed concurrently")
		}
		var reservation evacuation.Reservation
		var createdAt string
		err = tx.QueryRowContext(ctx, `SELECT id, shelter_id, plan_id, people, status, created_at FROM shelter_reservations WHERE plan_id = ? AND status = 'active'`, id).
			Scan(&reservation.ID, &reservation.ShelterID, &reservation.PlanID, &reservation.People, &reservation.Status, &createdAt)
		if err == nil {
			reservation.CreatedAt, err = parseTime(createdAt)
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE shelter_reservations SET status = 'released', released_at = ? WHERE id = ? AND status = 'active'`, formatTime(now), reservation.ID); err != nil {
				return err
			}
			result, err := tx.ExecContext(ctx, `UPDATE shelters SET reserved = reserved - ?, version = version + 1, updated_at = ? WHERE id = ? AND reserved >= ?`, reservation.People, formatTime(now), shelterID, reservation.People)
			if err != nil {
				return err
			}
			count, err := rowsAffected(result, "release shelter capacity")
			if err != nil {
				return err
			}
			if count != 1 {
				return apperr.Conflict("reservation_invariant", "shelter reservation cannot be released")
			}
			reservation.Status, reservation.ReleasedAt = evacuation.ReservationReleased, &now
			released = &reservation
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		event.Detail = []byte(fmt.Sprintf(`{"reason":%q,"previous_status":%q}`, reason, status))
		return appendAudit(ctx, tx, event)
	})
	if err != nil {
		return evacuation.Plan{}, nil, err
	}
	plan, err := s.FindPlan(ctx, id)
	return plan, released, err
}
