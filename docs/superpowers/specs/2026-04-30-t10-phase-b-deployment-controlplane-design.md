# T10 Phase B: NGINX Deployment Control Plane Baseline

Date: 2026-04-30

## Scope

T10 Phase B connects the Agent-local NGINX connector from Phase A to platform facts. The goal is to create and update deployment records, dispatch `deploy_nginx_certificate` jobs to a selected Agent, consume Agent progress/completion callbacks, write evidence, and update the certificate asset health state.

This phase does not implement the Agent executor that consumes the job and invokes `internal/agent/deploy/nginx`. That executor is a follow-up step.

## Deliverables

- `internal/domain/deploymentrecord/`
  - In-memory deployment record service.
  - Idempotent deployment creation by scope and idempotency key.
  - State transitions for dispatch, progress, completion, rollback, and conflict handling.
- `internal/deployment/`
  - `StartNGINXDeployment`.
  - `HandleAgentProgress`.
  - `HandleAgentComplete`.
  - Agent job payload construction for `deploy_nginx_certificate`.
- `internal/driver/agenttransport/`
  - Optional callback hook for progress and completion.
  - Existing Agent endpoints stay unchanged.
- `internal/app/controlplane/http/`
  - `POST /api/v1/certificate-assets/{assetId}/deploy`.
- `certificateasset.Service`
  - Minimal status updates for `active` and `deploy_failed`.
- Makefile and planning updates for the expanded `ci-task-T10`.

## Non-Goals

- Real Agent executor and local NGINX deployment invocation.
- Real certificate material delivery to Agent.
- PostgreSQL repositories.
- Frontend pages.
- Full audit event taxonomy.

## Domain Model

`DeploymentRecord` stores:

- `asset_id`
- `version_id`
- `target_id`
- `agent_id`
- `job_id`
- `operation_id`
- `idempotency_key`
- `status`
- `backup_ref`
- `verification_result`
- `rollback_result`
- `evidence_ref`
- `error_code`
- `error_message`
- `started_at`
- `finished_at`

Statuses match the Phase A connector:

```text
pending
prechecking
backing_up
installing
reloading
verifying
succeeded
failed
rolling_back
rolled_back
rollback_failed
```

Terminal states are:

```text
succeeded
failed
rolled_back
rollback_failed
```

## Control Plane Flow

`StartNGINXDeployment`:

1. Validate asset and version.
2. Validate target type is `nginx`.
3. Select the bound Agent.
4. Ensure the Agent can execute `deploy:nginx`.
5. Create or reuse a deployment record by idempotency key.
6. Build `deploy_nginx_certificate` payload.
7. Dispatch through `agenttransport.Service`.
8. Store `job_id` and `operation_id` on the deployment record.

## Agent Job Payload

The payload contains:

- `schema_version`
- `operation_id`
- `asset_id`
- `version_id`
- `target_id`
- `target_type`
- `certificate_ref`
- `certificate_path`
- `private_key_path`
- `config_path`
- `allowed_paths`
- `verify_host`
- `verify_port`
- `server_name`
- `expected_fingerprint`

`certificate_ref` remains a material reference. Phase B does not define how the Agent resolves it into PEM/private key material.

## Callback Flow

`HandleAgentProgress`:

- Locate record by `operation_id`.
- Map `started` to `prechecking`.
- Map `running` with `evidence.stage` to that deployment stage.
- Keep `heartbeating` as liveness information without moving the state backwards.
- Preserve latest evidence metadata.

`HandleAgentComplete`:

- Locate record by `operation_id`.
- If `result_status=succeeded`, mark record `succeeded`, write deployment evidence, and mark the asset `active`.
- If `result_status` is failure-like, map to `failed`, `rolled_back`, or `rollback_failed` using compensation and rollback evidence.
- Write deployment or rollback evidence.
- Mark the asset `deploy_failed` for failed, rolled_back, or rollback_failed results.

## Idempotency

- Control plane deployment trigger idempotency key is scoped by tenant/project/environment.
- Duplicate deployment requests return the existing deployment record and job.
- `operation_id = deployment:<deployment_id>`.
- Duplicate progress updates do not move state backwards.
- Duplicate completion with the same result is accepted.
- Duplicate completion with conflicting terminal result is rejected.

## Evidence

Evidence records are written through the existing audit evidence service.

Evidence type:

- `deployment` for successful deployment or ordinary deployment failure.
- `rollback` when rollback evidence is present.

Storage ref is a generated logical reference:

```text
evidence://deployment-records/<deployment_id>/<evidence_type>
```

The full evidence metadata remains JSON-like map data for now.

## HTTP API

`POST /api/v1/certificate-assets/{assetId}/deploy`

Request:

```json
{
  "version_id": "uuid",
  "target_id": "uuid",
  "idempotency_key": "string"
}
```

Response:

```json
{
  "request_id": "string",
  "status": "accepted",
  "deployment_id": "uuid",
  "job_id": "uuid"
}
```

## Tests

- `deploymentrecord` domain tests.
- `deployment` orchestration tests.
- HTTP deployment trigger integration test.
- Agent progress and completion callback integration test.

The existing `ci-task-T10` is expanded with `test-deployment` and the updated integration target.
