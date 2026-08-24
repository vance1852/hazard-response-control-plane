CREATE TABLE regions (
    id TEXT PRIMARY KEY,
    code TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    timezone TEXT NOT NULL,
    active INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE users (
    id TEXT PRIMARY KEY,
    region_id TEXT REFERENCES regions(id),
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    display_name TEXT NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('commander', 'field_operator', 'auditor')),
    active INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
    version INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX users_region_role_idx ON users(region_id, role, active);

CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_digest BLOB NOT NULL UNIQUE,
    expires_at TEXT NOT NULL,
    revoked_at TEXT,
    last_seen_at TEXT NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL
);

CREATE INDEX sessions_user_active_idx ON sessions(user_id, expires_at, revoked_at);

CREATE TABLE sensors (
    id TEXT PRIMARY KEY,
    region_id TEXT NOT NULL REFERENCES regions(id),
    station_code TEXT NOT NULL,
    name TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('seismic', 'rainfall', 'slope', 'river')),
    latitude REAL NOT NULL,
    longitude REAL NOT NULL,
    active INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
    version INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(region_id, station_code)
);

CREATE INDEX sensors_region_kind_idx ON sensors(region_id, kind, active);

CREATE TABLE observations (
    id TEXT PRIMARY KEY,
    sensor_id TEXT NOT NULL REFERENCES sensors(id),
    source_sequence TEXT NOT NULL,
    observed_at TEXT NOT NULL,
    metric TEXT NOT NULL,
    value REAL NOT NULL,
    unit TEXT NOT NULL,
    quality TEXT NOT NULL CHECK (quality IN ('verified', 'provisional', 'rejected')),
    payload_json TEXT NOT NULL,
    created_by TEXT NOT NULL REFERENCES users(id),
    created_at TEXT NOT NULL,
    UNIQUE(sensor_id, source_sequence)
);

CREATE INDEX observations_sensor_time_idx ON observations(sensor_id, observed_at DESC);
CREATE INDEX observations_metric_time_idx ON observations(metric, observed_at DESC);

CREATE TABLE incidents (
    id TEXT PRIMARY KEY,
    region_id TEXT NOT NULL REFERENCES regions(id),
    external_ref TEXT NOT NULL,
    hazard_type TEXT NOT NULL CHECK (hazard_type IN ('earthquake', 'rainstorm', 'landslide', 'debris_flow', 'flood')),
    title TEXT NOT NULL,
    severity INTEGER NOT NULL CHECK (severity BETWEEN 1 AND 5),
    status TEXT NOT NULL CHECK (status IN ('monitoring', 'active', 'stabilizing', 'closed', 'cancelled')),
    command_level TEXT NOT NULL CHECK (command_level IN ('local', 'regional', 'joint')),
    summary TEXT NOT NULL,
    occurred_at TEXT NOT NULL,
    activated_at TEXT,
    closed_at TEXT,
    version INTEGER NOT NULL DEFAULT 1,
    created_by TEXT NOT NULL REFERENCES users(id),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(region_id, external_ref)
);

CREATE INDEX incidents_region_status_idx ON incidents(region_id, status, severity DESC, occurred_at DESC);

CREATE TABLE incident_zones (
    id TEXT PRIMARY KEY,
    incident_id TEXT NOT NULL REFERENCES incidents(id) ON DELETE CASCADE,
    region_id TEXT NOT NULL REFERENCES regions(id),
    name TEXT NOT NULL,
    risk_level INTEGER NOT NULL CHECK (risk_level BETWEEN 1 AND 5),
    population INTEGER NOT NULL CHECK (population >= 0),
    geometry_json TEXT NOT NULL,
    created_at TEXT NOT NULL,
    UNIQUE(incident_id, name)
);

CREATE INDEX incident_zones_incident_risk_idx ON incident_zones(incident_id, risk_level DESC);
