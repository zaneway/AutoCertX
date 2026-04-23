# T07 Phase A Design

- Date: 2026-04-22
- Scope: `T07` query aggregation layer, phase A
- Inputs:
  - `doc/AI系统开发规划.md`
  - `doc/前端页面设计.md`
  - `doc/一期GA详细设计.md`

## Goal

Build the first stable page-oriented read model layer on top of completed `T03/T04/T05/T06` facts, without crossing into unfinished `T08/T09/T12` domains.

Phase A only implements query modules backed by already available facts:

- `dashboard`
- `jobs`
- `domains`
- `settings`

Phase A explicitly defers:

- `assets`
- `delivery`
- `discoveries`
- dashboard queries requiring real certificate asset or discovery facts

## Design Choice

Recommended approach: strict read-model phase split.

Why:

- It keeps `application/query` as a real boundary instead of a thin wrapper around command services.
- It avoids inventing placeholder facts for `certificate_assets`, `agent nodes`, and `discoveries`.
- It unlocks later `T14/T15` work with stable DTOs for the data that already exists.

## Package Boundaries

New packages:

- `internal/application/query/dashboard/`
- `internal/application/query/jobs/`
- `internal/application/query/domains/`
- `internal/application/query/settings/`

Existing packages reused:

- `internal/application/query/audit/`
- `internal/application/query/authcontext/`

Rules:

- Query services may depend on domain services and read repositories.
- Query services must not depend on HTTP handlers.
- Query services must not own write behavior.
- Repository remains responsible for fact retrieval, not page-specific shaping.

## Phase A Endpoints

Implement in this phase:

- `GET /api/v1/dashboard/summary`
- `GET /api/v1/dashboard/job-failures`
- `GET /api/v1/jobs`
- `GET /api/v1/jobs/{id}`
- `GET /api/v1/jobs/{id}/attempts`
- `GET /api/v1/domains`
- `GET /api/v1/domains/{id}`
- `GET /api/v1/domains/{id}/validation-records`
- `GET /api/v1/domains/{id}/txt-operations`
- `GET /api/v1/domains/{id}/certificate-assets`
- `GET /api/v1/dns-credentials`
- `GET /api/v1/ca-accounts`
- `GET /api/v1/ca-accounts/{id}`
- `GET /api/v1/ca-accounts/{id}/capabilities`
- `GET /api/v1/settings/webhooks`
- `GET /api/v1/settings/renewal-window`
- `GET /api/v1/settings/security`

Keep existing query packages and routes as-is in phase A:

- `GET /api/v1/audit-events`
- `GET /api/v1/audit-events/{id}`

## Deferred Items

Deferred to later tasks because facts are not ready yet:

- `GET /api/v1/dashboard/expiring-certificates`
  - depends on `certificate_assets` fact model
  - target task: `T08`
- `GET /api/v1/dashboard/discovery-anomalies`
  - depends on `discoveries` fact model
  - target task: `T12`
- `internal/application/query/assets/`
  - target task: `T08`
- `internal/application/query/delivery/`
  - target task: `T09`
- `internal/application/query/discoveries/`
  - target task: `T12`
- `AssetDetail / AgentDetail / DiscoveryDetail`
  - target tasks: `T08/T09/T12`

## Core Implementation Strategy

### Jobs Query

`query/jobs` gets its own reader interface. The reader must expose:

- list jobs
- get job
- list job attempts

Production PostgreSQL job store and in-memory job repository will both implement this read capability.

Important constraint:

- repository returns scheduling facts
- query layer shapes list/detail DTOs and filters

### Dashboard Query

Phase A dashboard summary aggregates only facts that already exist:

- domain count
- dns credential count
- CA account count
- webhook count
- failed job count

`job failures` is the only dashboard detail list implemented in phase A because it is backed by `T05`.

### Domains Query

`query/domains` shapes governance read models from existing domain services:

- domains
- validation records
- TXT operations
- related certificate refs
- DNS credentials
- CA accounts

### Settings Query

`query/settings` shapes read DTOs for:

- webhook endpoint list
- renewal window settings
- security settings

## Control Plane Integration

`internal/app/controlplane/wiring/build.go` must wire the new query services into HTTP deps.

HTTP layer must:

- use query services for GET routes
- keep POST/PUT on command services
- continue enforcing auth, scope resolution, and permission middleware before query execution

## Testing

Add `T07` tests in three layers:

- query package unit tests
- HTTP integration tests for new GET routes
- regression coverage to ensure `T03/T04/T05/T06` behavior is not broken

Planned gates:

- `go test ./internal/application/query/...`
- `go test ./internal/app/controlplane/http/...`
- `make test-integration-query`
- `make ci-task-T07`

## Documentation Updates

Update:

- `doc/AI系统开发规划.md`
  - mark `T07` as `进行中`
  - record Phase A scope and deferred list
- `README.md` if new query endpoints need a high-level note
- OpenAPI contract if route behavior or query parameters are expanded during implementation
