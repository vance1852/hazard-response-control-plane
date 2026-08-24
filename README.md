# Hazard Response Control Plane

Hazard Response Control Plane coordinates earthquake, rainstorm, landslide, debris-flow, and flood response across regional command teams. It is a backend operational system with two linked flows: verified observations can activate incident zones, and incident zones can progress through evacuation plans and scarce field-unit dispatch.

## Runtime

- Go 1.22, SQLite with WAL, foreign keys, busy timeout, and embedded versioned migrations.
- `cmd/server` exposes the JSON API on `:8080` by default.
- Opaque server-side bearer sessions are bcrypt-backed and revocable. Roles are `commander`, `field_operator`, and `auditor`.
- Durable jobs have leases, bounded retries, attempt history, and restart recovery. Audit and outbox records are transactional with operational state changes.

## Main API paths

`POST /v1/auth/login`, `POST /v1/auth/logout`, and `POST /v1/users` manage identity. `POST /v1/regions`, `POST /v1/sensors`, `POST /v1/observations`, `POST /v1/incidents`, and `GET /v1/incidents` implement observation and incident operations. `POST /v1/shelters` and the `/v1/evacuation-plans` state endpoints implement capacity-safe evacuation. `POST /v1/units`, `/v1/resource-requests`, and `/v1/deployments` implement field dispatch. `GET /v1/audit-events` exposes filtered cursor-ordered audit evidence to commanders and auditors. `/healthz` is local liveness and `/readyz` checks the database.

## Local commands

```text
go mod download
go test ./... -count=1
go test -race ./... -count=1
go vet ./...
go build ./...
go run ./cmd/server
```

Set `HAZARD_BOOTSTRAP_PASSWORD` to create the first commander. `HAZARD_DATABASE_PATH` selects the SQLite file and `HAZARD_MIGRATION_DIR` can point at a migration directory for container or deployment layouts. See `.env.example` for all configuration values.

## Docker

The root `Dockerfile` builds the real `./cmd/server` entry point with Go 1.22 and copies the real `migrations/` directory. Build both target images with:

```text
docker build --platform linux/amd64 -t hazard-response-control-plane:amd64 .
docker build --platform linux/arm64 -t hazard-response-control-plane:arm64 .
```

The default entry uses `/app/hazardd` and stores the database under `/tmp` (mount a persistent volume for production deployments).
