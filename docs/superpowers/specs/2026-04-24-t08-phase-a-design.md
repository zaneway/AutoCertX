# T08 Phase A Design

- Date: 2026-04-24
- Scope: `T08` ACME issuance workflow, phase A
- Inputs:
  - `doc/AI系统开发规划.md`
  - `doc/需求说明书.md`
  - `doc/一期GA详细设计.md`
  - `api/openapi/openapi.json`

## Goal

Build the first executable issuance workflow baseline for `T08` without coupling the business core to real `Let's Encrypt`, `AliDNS`, or Agent-side challenge execution.

Phase A establishes:

- `CertificateRequest` request facts
- `IssueWorkflow` main state machine
- `WorkflowChallenge` sub-state machine
- minimal `CertificateAsset` and asset version facts
- scheduler-driven orchestration through `start_issue_workflow` and `continue_issue_workflow`
- replaceable fake adapters for ACME, DNS-01, and HTTP-01 presentation

Phase A explicitly does not deliver:

- real `Let's Encrypt` integration
- real `AliDNS` integration
- real Agent-side `HTTP-01` presentation and cleanup
- deployment, verification, rollback, or renewal scan execution

## Design Choice

Recommended approach: domain-first minimal closed loop with job-driven orchestration.

Why:

- It fixes the long-lived business boundaries before introducing unstable external systems.
- It keeps workflow truth in durable domain facts instead of job payloads.
- It reuses `T05` scheduler semantics for retry and lease management.
- It lets later real integrations replace adapters without redesigning the workflow core.

## Scope And Boundaries

Phase A covers one stable issuance slice:

- create `CertificateRequest`
- create one `IssueWorkflow` from the request
- create one or more `WorkflowChallenge` rows from fake ACME authorization data
- drive challenge presentation and verification to completion
- finalize issuance and persist one asset version

Phase A only drives the following workflow statuses:

- `created`
- `order_pending`
- `challenge_pending`
- `challenge_processing`
- `challenge_valid`
- `finalizing`
- `issued`
- `failed`
- `cancelled`

The deployment-related workflow statuses remain reserved for later phases:

- `deploy_pending`
- `deploying`
- `deployed`
- `partially_failed`

## Package Boundaries

New packages:

- `internal/domain/certificaterequest/`
- `internal/domain/certificateasset/`
- `internal/domain/issueworkflow/`
- `internal/workflow/`
- `internal/driver/acme/`
- `internal/driver/dns/`

Existing packages reused:

- `internal/domain/domains/`
- `internal/domain/issuer/`
- `internal/domain/job/`
- `internal/application/command/jobs/`
- `internal/scheduler/`

Rules:

- domain packages own durable facts and legal state transitions
- `internal/workflow` owns orchestration and cross-service sequencing
- `internal/driver/acme` and `internal/driver/dns` expose replaceable interfaces only
- no business service may depend directly on raw ACME client or DNS SDK types
- job payloads may point to workflow facts but must not become the source of truth

## Domain Model

### CertificateRequest

Purpose:

- capture the user-visible issuance or renewal request
- enforce idempotency and request-level validation

Phase A required fields:

- scope: tenant, project, environment
- `request_type`
- `request_source`
- `asset_id` for renewal paths
- `ca_account_id`
- `certificate_type`
- `challenge_type`
- `common_name`
- `idempotency_key`
- related domain references

Lifecycle used in phase A:

- `submitted`
- `accepted`
- `running`
- `completed`
- `failed`
- `cancelled`

### IssueWorkflow

Purpose:

- persist the issuance execution truth independent of jobs and handlers

Phase A required fields:

- `certificate_request_id`
- `ca_account_id`
- `workflow_type`
- `status`
- `order_url`
- `finalize_url`
- `csr_ref`
- `certificate_ref`
- `last_error_code`
- `last_error_message`
- `started_at`
- `finished_at`

### WorkflowChallenge

Purpose:

- represent one identifier-specific challenge execution
- keep per-challenge material and cleanup status

Phase A required fields:

- `issue_workflow_id`
- `domain_asset_id`
- `challenge_type`
- `identifier`
- `token`
- `key_authorization`
- `http_path`
- `dns_record_name`
- `dns_record_value`
- `status`
- `presented_at`
- `validated_at`
- `cleaned_at`

Lifecycle used in phase A:

- `pending`
- `presenting`
- `presented`
- `propagating`
- `ready`
- `verifying`
- `valid`
- `invalid`
- `cleanup_pending`
- `cleaned`
- `cleanup_failed`

### CertificateAsset

Purpose:

- anchor long-lived certificate governance facts
- separate stable asset identity from per-issuance versions

Phase A baseline only:

- create asset on first issuance if absent
- create new version on issuance success
- renewal must append a version instead of overwriting the previous one

## State Progression

Three state lines cooperate but stay independent.

### Request State

- `submitted` when request is stored
- `accepted` when the initial workflow job is scheduled
- `running` when workflow execution begins
- `completed` when an issued certificate has been persisted into an asset version
- `failed` when workflow reaches terminal failure

### Workflow State

- `created -> order_pending`
- `order_pending -> challenge_pending`
- `challenge_pending -> challenge_processing`
- `challenge_processing -> challenge_valid`
- `challenge_valid -> finalizing`
- `finalizing -> issued`
- `order_pending/challenge_processing/finalizing -> failed`
- `created/challenge_pending -> cancelled`

### Challenge State

- `pending -> presenting`
- `presenting -> presented`
- `presented -> propagating`
- `propagating -> ready`
- `ready -> verifying`
- `verifying -> valid`
- `verifying -> invalid`
- `invalid -> cleanup_pending -> cleaned|cleanup_failed`

## Orchestration Model

`internal/workflow` exposes the workflow coordination surface.

Core use cases:

- `SubmitRequest`
- `StartWorkflow`
- `ContinueWorkflow`
- `CompleteIssuedWorkflow`
- `FailWorkflow`

Responsibilities:

- validate request inputs against `domains` and `issuer`
- create request and workflow facts transactionally where possible
- schedule platform jobs through `T05`
- reload durable workflow state before every stage transition
- translate adapter errors into retryable, compensatable, or terminal failures

## Job Design

Phase A uses only the already frozen platform job types:

- `start_issue_workflow`
- `continue_issue_workflow`

### start_issue_workflow

Purpose:

- bootstrap exactly one workflow from one request

Shape:

- `aggregate_type`: `certificate_request`
- `aggregate_id`: `request_id`
- `idempotency_key`: `issuewf:start:<request_id>`

Responsibilities:

- load request facts
- create or fetch the unique workflow
- invoke fake ACME order creation
- create workflow challenge facts
- move workflow to `challenge_pending`
- enqueue the first `continue_issue_workflow`

### continue_issue_workflow

Purpose:

- advance one workflow from its current durable state

Shape:

- `aggregate_type`: `issue_workflow`
- `aggregate_id`: `workflow_id`
- `idempotency_key`: `issuewf:continue:<workflow_id>:<entered_status>`

Suggested payload:

```json
{
  "request_id": "uuid",
  "workflow_id": "uuid",
  "expected_status": "challenge_pending",
  "trigger": "manual|scheduled"
}
```

Rules:

- payload is advisory, not authoritative
- worker always reloads request, workflow, and challenges from storage
- one execution should cross at most one stable stage boundary
- waiting for propagation or verify completion should use scheduler retry instead of busy looping

## Adapter Strategy

Phase A adapters are fake but contract-stable.

### ACME Adapter

Must expose:

- `CreateOrder`
- `NotifyChallengeReady`
- `PollAuthorization`
- `FinalizeOrder`
- `DownloadCertificate`

### DNS Adapter

Must expose:

- `PresentTXT`
- `CleanupTXT`
- `CheckPropagation`

### HTTP-01 Presenter

Phase A keeps the abstraction but uses a fake presenter.

Must expose:

- `PresentHTTP01`
- `CleanupHTTP01`

Later phases will route this through real Agent transport.

## Failure Handling

Failures are grouped into three classes.

### Terminal Validation Failure

Examples:

- wildcard request with `http-01`
- missing domain facts
- missing or disabled CA account

Behavior:

- reject request creation or mark request and workflow failed immediately
- no retry

### Retryable External Failure

Examples:

- ACME temporary failure
- DNS propagation not ready
- transient adapter timeout

Behavior:

- keep workflow in the current stage
- update `last_error_code` and `last_error_message`
- complete job as retryable and let `T05` backoff re-drive it

### Compensatable Failure

Examples:

- challenge was already presented and later verification failed

Behavior:

- move challenge to `cleanup_pending`
- attempt best-effort cleanup in the same execution
- persist `cleaned` or `cleanup_failed`
- mark workflow and request failed

Special rule:

- if certificate finalize succeeded but asset version persistence failed, keep workflow at `issued` and retry the post-issuance persistence path until asset version creation succeeds

## HTTP Surface For Phase A

Phase A needs only the minimal write entry points already described by OpenAPI:

- `POST /api/v1/certificate-assets/requests`
- `POST /api/v1/certificate-assets/{assetId}/renew`

Expected behavior:

- both return `202 Accepted`
- both create or reuse idempotent platform jobs
- request validation maps onto the existing API error conventions

Read APIs for asset detail, versions, deployments, and discoveries remain stubbed or query-backed by later tasks unless the minimum asset baseline is enough to support them safely.

## Testing

Add tests in four layers.

### Domain Tests

Cover:

- request validation
- idempotency constraints
- legal and illegal workflow transitions
- legal and illegal challenge transitions
- asset version append semantics on renewal

### Workflow Tests

Cover:

- start workflow bootstrap
- continue workflow stage-by-stage progression
- fake `http-01` and fake `dns-01` branches
- retry on transient adapter failure
- cleanup on challenge verification failure
- persistence retry after successful finalize

### HTTP Integration Tests

Cover:

- `POST /api/v1/certificate-assets/requests`
- `POST /api/v1/certificate-assets/{assetId}/renew`
- accepted response shape and job id emission
- validation and conflict error mapping

### Scheduler Integration Tests

Cover:

- `start_issue_workflow` creates workflow and challenges once
- repeated `continue_issue_workflow` executions remain idempotent
- retryable failures eventually converge under fake adapters

## Gate Plan

Add Makefile coverage for `T08`.

Suggested targets:

- `test-issuance`
- `test-integration-acme`
- `ci-task-T08`

Suggested `ci-task-T08` composition:

- `fmt-check`
- `lint`
- `test-issuance`
- `test-integration-acme`
- `openapi-verify`
- `test-contracts`

## Phase B Backlog

Record the real integration work explicitly as the next follow-up set.

Phase B:

- real `Let's Encrypt` adapter
- real `AliDNS` adapter
- real Agent-side `HTTP-01` presentation and cleanup
- staging environment E2E tests
- propagation polling policy and retry tuning
- failure injection and operational observability for external calls

## Implementation Order

Recommended sequence:

1. implement `certificaterequest`, `issueworkflow`, and `certificateasset` domain packages
2. add fake adapters in `internal/driver/acme` and `internal/driver/dns`
3. implement orchestration in `internal/workflow`
4. wire `start_issue_workflow` and `continue_issue_workflow` through `T05` jobs
5. expose minimal HTTP write endpoints and integration tests
6. add `T08` Makefile gates and update planning docs
