CREATE TABLE shelters (
    id TEXT PRIMARY KEY,
    region_id TEXT NOT NULL REFERENCES regions(id),
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    capacity INTEGER NOT NULL CHECK (capacity >= 0),
    reserved INTEGER NOT NULL DEFAULT 0 CHECK (reserved >= 0 AND reserved <= capacity),
    status TEXT NOT NULL CHECK (status IN ('available', 'restricted', 'closed')),
    version INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(region_id, code)
);

CREATE INDEX shelters_region_status_idx ON shelters(region_id, status, capacity, reserved);

CREATE TABLE evacuation_plans (
    id TEXT PRIMARY KEY,
    incident_id TEXT NOT NULL REFERENCES incidents(id),
    zone_id TEXT NOT NULL REFERENCES incident_zones(id),
    shelter_id TEXT NOT NULL REFERENCES shelters(id),
    name TEXT NOT NULL,
    evacuee_count INTEGER NOT NULL CHECK (evacuee_count > 0),
    status TEXT NOT NULL CHECK (status IN ('draft', 'submitted', 'approved', 'executing', 'completed', 'cancelled')),
    deadline_at TEXT NOT NULL,
    approved_by TEXT REFERENCES users(id),
    approved_at TEXT,
    completed_at TEXT,
    version INTEGER NOT NULL DEFAULT 1,
    created_by TEXT NOT NULL REFERENCES users(id),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(incident_id, zone_id, name)
);

CREATE INDEX evacuation_plans_incident_status_idx ON evacuation_plans(incident_id, status, deadline_at);

CREATE TABLE evacuation_steps (
    id TEXT PRIMARY KEY,
    plan_id TEXT NOT NULL REFERENCES evacuation_plans(id) ON DELETE CASCADE,
    step_order INTEGER NOT NULL CHECK (step_order > 0),
    instruction TEXT NOT NULL,
    responsible_role TEXT NOT NULL CHECK (responsible_role IN ('commander', 'field_operator')),
    expected_minutes INTEGER NOT NULL CHECK (expected_minutes > 0),
    completed_at TEXT,
    completed_by TEXT REFERENCES users(id),
    created_at TEXT NOT NULL,
    UNIQUE(plan_id, step_order)
);

CREATE TABLE shelter_reservations (
    id TEXT PRIMARY KEY,
    shelter_id TEXT NOT NULL REFERENCES shelters(id),
    plan_id TEXT NOT NULL REFERENCES evacuation_plans(id),
    people INTEGER NOT NULL CHECK (people > 0),
    status TEXT NOT NULL CHECK (status IN ('active', 'released', 'consumed')),
    created_at TEXT NOT NULL,
    released_at TEXT,
    UNIQUE(plan_id)
);

CREATE INDEX shelter_reservations_shelter_status_idx ON shelter_reservations(shelter_id, status);

CREATE TABLE field_units (
    id TEXT PRIMARY KEY,
    region_id TEXT NOT NULL REFERENCES regions(id),
    call_sign TEXT NOT NULL,
    unit_type TEXT NOT NULL CHECK (unit_type IN ('rescue', 'medical', 'engineering', 'survey', 'transport')),
    capability_json TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('available', 'deployed', 'offline')),
    operator_id TEXT REFERENCES users(id),
    version INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(region_id, call_sign)
);

CREATE INDEX field_units_region_type_status_idx ON field_units(region_id, unit_type, status);

CREATE TABLE resource_requests (
    id TEXT PRIMARY KEY,
    incident_id TEXT NOT NULL REFERENCES incidents(id),
    zone_id TEXT REFERENCES incident_zones(id),
    requested_type TEXT NOT NULL,
    required_capability TEXT NOT NULL,
    quantity INTEGER NOT NULL CHECK (quantity > 0),
    priority INTEGER NOT NULL CHECK (priority BETWEEN 1 AND 5),
    status TEXT NOT NULL CHECK (status IN ('requested', 'approved', 'allocated', 'partially_allocated', 'fulfilled', 'cancelled')),
    needed_by TEXT NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    created_by TEXT NOT NULL REFERENCES users(id),
    approved_by TEXT REFERENCES users(id),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX resource_requests_incident_status_idx ON resource_requests(incident_id, status, priority DESC, needed_by);

CREATE TABLE deployments (
    id TEXT PRIMARY KEY,
    request_id TEXT NOT NULL REFERENCES resource_requests(id),
    unit_id TEXT NOT NULL REFERENCES field_units(id),
    incident_id TEXT NOT NULL REFERENCES incidents(id),
    status TEXT NOT NULL CHECK (status IN ('assigned', 'acknowledged', 'en_route', 'on_scene', 'released', 'cancelled')),
    assigned_by TEXT NOT NULL REFERENCES users(id),
    acknowledged_at TEXT,
    arrived_at TEXT,
    released_at TEXT,
    version INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE UNIQUE INDEX deployments_active_unit_idx ON deployments(unit_id) WHERE status IN ('assigned', 'acknowledged', 'en_route', 'on_scene');
CREATE INDEX deployments_request_status_idx ON deployments(request_id, status);
