# T10 Phase A: NGINX Local Deployment Connector

Date: 2026-04-30

## Scope

T10 Phase A delivers the Agent-local NGINX deployment and verification baseline. It does not wire the control plane deployment record, audit log, asset refresh, or Agent job dispatch path yet.

The goal is to make the local side effect safe and testable before it is consumed by `deploy_nginx_certificate` and `verify_nginx_deployment` jobs.

## Deliverables

- `internal/agent/verify/nginx/`
  - Certificate probe abstraction.
  - Leaf certificate SHA-256 fingerprint verification.
  - Host, port, SNI, and expected fingerprint validation.
- `internal/agent/deploy/nginx/`
  - Deployment state machine:
    - `pending`
    - `prechecking`
    - `backing_up`
    - `installing`
    - `reloading`
    - `verifying`
    - `succeeded`
    - `failed`
    - `rolling_back`
    - `rolled_back`
    - `rollback_failed`
  - Path policy for `allowed_paths`.
  - Certificate/private key validation and matching.
  - File backup, atomic write, restore, and rollback.
  - `nginx -t`, reload, and post-reload verification orchestration.
  - Structured evidence summary for later `DeploymentRecord` and audit integration.

## Non-Goals

- Control plane `DeploymentRecord` persistence.
- Audit event persistence.
- Certificate asset status refresh.
- Agent job payload mapping.
- Real `systemctl reload nginx` integration test.
- Multi-node and multi-target orchestration.

## Architecture

The connector is split into deployment and verification packages.

`verify/nginx` owns certificate observation. It accepts a probe interface and compares the observed leaf fingerprint with the expected certificate fingerprint.

`deploy/nginx` owns the local state machine and compensation logic. It depends on:

- `FileStore` for file read, backup, atomic write, restore, and remove.
- `CommandRunner` for `nginx -t` and reload commands.
- `Verifier` for post-reload certificate verification.
- `PathPolicy` for write-scope enforcement.

The deployment package returns a structured `DeployResult`. It does not know about the control plane schema, job attempts, or audit storage.

## State Machine

Success path:

```text
pending -> prechecking -> backing_up -> installing -> reloading -> verifying -> succeeded
```

Failure and rollback:

```text
prechecking -> failed
backing_up -> failed
installing -> failed -> rolling_back -> rolled_back | rollback_failed
reloading -> failed -> rolling_back -> rolled_back | rollback_failed
verifying -> failed -> rolling_back -> rolled_back | rollback_failed
```

Rollback restores both certificate and private key backups. After restore, it runs `nginx -t` and reload again. If restore or post-restore reload fails, the final status is `rollback_failed`.

## Request Model

`DeployRequest` contains:

- `target_id`
- `certificate_pem`
- `private_key_pem`
- `certificate_path`
- `private_key_path`
- `config_path`
- `nginx_test_command`
- `reload_command`
- `allowed_paths`
- `verify_host`
- `verify_port`
- `server_name`
- `expected_fingerprint`

If `expected_fingerprint` is empty, the connector computes it from `certificate_pem`.

## Result Model

`DeployResult` contains:

- Final status.
- Active fingerprint.
- Backup references.
- Ordered step results.
- Rollback result.
- Evidence summary.

Evidence includes affected paths, backup references, config-test output, reload output, verification details, rollback details, and final active fingerprint.

## Validation Rules

- All target paths must be covered by `allowed_paths`.
- `certificate_path`, `private_key_path`, and `config_path` are required.
- Certificate and private key must parse and match.
- `verify_host` and `verify_port` are required.
- NGINX test and reload command definitions are required.
- Deployment success requires successful config test, reload, and observed certificate fingerprint match.

## Test Strategy

- `verify/nginx`
  - Fingerprint match.
  - Fingerprint mismatch.
  - Probe failure.
  - Invalid input.
- `deploy/nginx`
  - Full success path.
  - Precheck failure without rollback.
  - Install failure with successful rollback.
  - Reload failure with successful rollback.
  - Verify failure with successful rollback.
  - Rollback failure.
  - Allowed path rejection.
  - Certificate/private key mismatch.

The tests use in-memory fakes for filesystem, command execution, and certificate probing. No real NGINX process is required in Phase A.

## Gate

Add:

- `make test-nginx-deploy`
- `make test-integration-nginx-deploy`
- `make ci-task-T10`

`ci-task-T10` runs:

- `fmt-check`
- `lint`
- `test-nginx-deploy`
- `test-integration-nginx-deploy`
