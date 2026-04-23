-- AutoCertX initial schema
-- Scope:
-- 1. Cover tables explicitly defined in doc/一期GA详细设计.md
-- 2. Supplement low-ambiguity must-have tables discovered in requirement/design gap analysis:
--    - auth_sessions
--    - api_keys
--    - system_settings
--    - tenant_quotas
--    - approval_requests / approval_steps / approval_records
--    - export_records
-- Target database: PostgreSQL 16+

BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$;

-- ============================================================================
-- 01. Tenant / identity / security
-- ============================================================================

CREATE TABLE tenants (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name varchar(128) NOT NULL,
    code varchar(64) NOT NULL,
    status varchar(32) NOT NULL CHECK (status IN ('active', 'suspended', 'disabled')),
    plan_code varchar(64),
    locale varchar(16) NOT NULL DEFAULT 'zh-CN',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uk_tenants_code UNIQUE (code)
);

CREATE TABLE projects (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    name varchar(128) NOT NULL,
    code varchar(64) NOT NULL,
    description text,
    status varchar(32) NOT NULL CHECK (status IN ('active', 'archived', 'disabled')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uk_projects_tenant_code UNIQUE (tenant_id, code)
);

CREATE TABLE environments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    project_id uuid NOT NULL REFERENCES projects(id),
    name varchar(128) NOT NULL,
    code varchar(64) NOT NULL,
    environment_type varchar(32) NOT NULL CHECK (environment_type IN ('dev', 'test', 'staging', 'prod')),
    status varchar(32) NOT NULL CHECK (status IN ('active', 'archived', 'disabled')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uk_environments_project_code UNIQUE (project_id, code)
);

CREATE TABLE users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid REFERENCES tenants(id),
    username varchar(64) NOT NULL,
    display_name varchar(128) NOT NULL,
    email varchar(255),
    phone varchar(32),
    locale varchar(16) NOT NULL DEFAULT 'zh-CN',
    status varchar(32) NOT NULL CHECK (status IN ('active', 'locked', 'disabled')),
    last_login_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uk_users_username UNIQUE (username)
);

CREATE TABLE user_credentials (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id),
    credential_type varchar(32) NOT NULL CHECK (credential_type IN ('password')),
    password_hash text NOT NULL,
    password_algo_version integer NOT NULL,
    must_change_password boolean NOT NULL DEFAULT false,
    password_updated_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uk_user_credentials_user_type UNIQUE (user_id, credential_type)
);

CREATE TABLE roles (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid REFERENCES tenants(id),
    role_code varchar(64) NOT NULL,
    role_name varchar(128) NOT NULL,
    scope_level varchar(32) NOT NULL CHECK (scope_level IN ('tenant', 'project', 'environment')),
    is_system boolean NOT NULL DEFAULT false,
    status varchar(32) NOT NULL CHECK (status IN ('active', 'disabled')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX uk_roles_tenant_code
    ON roles ((COALESCE(tenant_id, '00000000-0000-0000-0000-000000000000'::uuid)), role_code);

CREATE TABLE role_bindings (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    project_id uuid REFERENCES projects(id),
    environment_id uuid REFERENCES environments(id),
    user_id uuid NOT NULL REFERENCES users(id),
    role_id uuid NOT NULL REFERENCES roles(id),
    scope_type varchar(32) NOT NULL CHECK (scope_type IN ('tenant', 'project', 'environment')),
    scope_id uuid NOT NULL,
    status varchar(32) NOT NULL CHECK (status IN ('active', 'disabled')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uk_role_bindings_user_role_scope UNIQUE (user_id, role_id, scope_type, scope_id)
);

CREATE TABLE tenant_quotas (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    quota_code varchar(64) NOT NULL,
    limit_value bigint NOT NULL CHECK (limit_value >= 0),
    period_type varchar(32) NOT NULL CHECK (period_type IN ('total', 'daily', 'monthly')),
    warn_threshold_percent integer NOT NULL DEFAULT 80 CHECK (warn_threshold_percent BETWEEN 1 AND 100),
    status varchar(32) NOT NULL CHECK (status IN ('active', 'disabled')),
    effective_from timestamptz,
    effective_to timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uk_tenant_quotas_tenant_code_period UNIQUE (tenant_id, quota_code, period_type)
);

CREATE TABLE auth_sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid REFERENCES tenants(id),
    user_id uuid NOT NULL REFERENCES users(id),
    session_type varchar(32) NOT NULL CHECK (session_type IN ('web', 'api', 'refresh_token')),
    refresh_token_hash text NOT NULL,
    client_ip inet,
    user_agent text,
    status varchar(32) NOT NULL CHECK (status IN ('active', 'revoked', 'expired')),
    issued_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    last_seen_at timestamptz,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uk_auth_sessions_refresh_token_hash UNIQUE (refresh_token_hash)
);

CREATE TABLE api_keys (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    project_id uuid REFERENCES projects(id),
    environment_id uuid REFERENCES environments(id),
    owner_user_id uuid REFERENCES users(id),
    key_name varchar(128) NOT NULL,
    access_key_id varchar(64) NOT NULL,
    secret_hash text NOT NULL,
    scope_type varchar(32) NOT NULL CHECK (scope_type IN ('tenant', 'project', 'environment')),
    scopes_jsonb jsonb NOT NULL DEFAULT '{}'::jsonb,
    status varchar(32) NOT NULL CHECK (status IN ('active', 'disabled', 'revoked', 'expired')),
    description text,
    last_used_at timestamptz,
    expires_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uk_api_keys_access_key_id UNIQUE (access_key_id)
);

CREATE TABLE system_settings (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    project_id uuid REFERENCES projects(id),
    environment_id uuid REFERENCES environments(id),
    setting_scope varchar(32) NOT NULL CHECK (setting_scope IN ('tenant', 'project', 'environment')),
    setting_key varchar(128) NOT NULL,
    value_jsonb jsonb NOT NULL,
    status varchar(32) NOT NULL CHECK (status IN ('active', 'disabled')),
    description text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX uk_system_settings_scope_key
    ON system_settings (
        tenant_id,
        COALESCE(project_id, '00000000-0000-0000-0000-000000000000'::uuid),
        COALESCE(environment_id, '00000000-0000-0000-0000-000000000000'::uuid),
        setting_scope,
        setting_key
    );

-- ============================================================================
-- 02. Domain / CA / credential governance
-- ============================================================================

CREATE TABLE dns_credentials (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    project_id uuid NOT NULL REFERENCES projects(id),
    environment_id uuid NOT NULL REFERENCES environments(id),
    provider_type varchar(32) NOT NULL CHECK (provider_type IN ('alidns')),
    display_name varchar(128) NOT NULL,
    access_key_id varchar(128) NOT NULL,
    secret_envelope jsonb NOT NULL,
    scope_mode varchar(32) NOT NULL CHECK (scope_mode IN ('environment', 'domain')),
    status varchar(32) NOT NULL CHECK (status IN ('active', 'disabled', 'error', 'rotating')),
    last_verified_at timestamptz,
    last_rotated_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uk_dns_credentials_env_name UNIQUE (environment_id, display_name)
);

CREATE TABLE ca_accounts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    project_id uuid NOT NULL REFERENCES projects(id),
    environment_id uuid NOT NULL REFERENCES environments(id),
    provider_type varchar(32) NOT NULL CHECK (provider_type IN ('acme')),
    provider_name varchar(32) NOT NULL CHECK (provider_name IN ('letsencrypt')),
    display_name varchar(128) NOT NULL,
    directory_url text NOT NULL,
    account_kid text,
    account_key_secret_ref varchar(255) NOT NULL,
    status varchar(32) NOT NULL CHECK (status IN ('active', 'disabled', 'error', 'verifying')),
    capabilities_jsonb jsonb NOT NULL DEFAULT '{}'::jsonb,
    last_checked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uk_ca_accounts_env_name UNIQUE (environment_id, display_name)
);

CREATE TABLE domain_assets (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    project_id uuid NOT NULL REFERENCES projects(id),
    environment_id uuid NOT NULL REFERENCES environments(id),
    domain_name varchar(255) NOT NULL,
    domain_type varchar(32) NOT NULL CHECK (domain_type IN ('single', 'wildcard_root', 'san_member')),
    default_challenge_type varchar(32) NOT NULL CHECK (default_challenge_type IN ('http-01', 'dns-01')),
    default_dns_provider varchar(32),
    dns_credential_id uuid REFERENCES dns_credentials(id),
    allow_wildcard boolean NOT NULL DEFAULT false,
    status varchar(32) NOT NULL CHECK (status IN ('active', 'disabled', 'error')),
    last_validation_status varchar(32),
    last_validated_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uk_domain_assets_env_domain UNIQUE (environment_id, domain_name)
);

-- ============================================================================
-- 03. Assets / requests / workflows
-- ============================================================================

CREATE TABLE certificate_assets (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    project_id uuid NOT NULL REFERENCES projects(id),
    environment_id uuid NOT NULL REFERENCES environments(id),
    name varchar(128) NOT NULL,
    status varchar(32) NOT NULL CHECK (status IN ('active', 'expiring', 'renewing', 'deploy_failed', 'expired', 'revoked', 'orphaned')),
    current_version_id uuid,
    current_request_id uuid,
    expires_at timestamptz,
    managed_by varchar(32) NOT NULL CHECK (managed_by IN ('auto', 'manual', 'imported')),
    renewal_window_days integer NOT NULL CHECK (renewal_window_days >= 1),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uk_certificate_assets_env_name UNIQUE (environment_id, name)
);

CREATE TABLE certificate_requests (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    project_id uuid NOT NULL REFERENCES projects(id),
    environment_id uuid NOT NULL REFERENCES environments(id),
    request_type varchar(32) NOT NULL CHECK (request_type IN ('issue', 'renew')),
    request_source varchar(32) NOT NULL CHECK (request_source IN ('manual', 'scheduled')),
    asset_id uuid REFERENCES certificate_assets(id),
    ca_account_id uuid NOT NULL REFERENCES ca_accounts(id),
    algorithm varchar(32) NOT NULL CHECK (algorithm IN ('rsa')),
    certificate_type varchar(32) NOT NULL CHECK (certificate_type IN ('single', 'san', 'wildcard')),
    challenge_type varchar(32) NOT NULL CHECK (challenge_type IN ('http-01', 'dns-01')),
    common_name varchar(255) NOT NULL,
    status varchar(32) NOT NULL CHECK (status IN ('draft', 'submitted', 'accepted', 'running', 'completed', 'failed', 'cancelled')),
    requested_by uuid REFERENCES users(id),
    idempotency_key varchar(128) NOT NULL,
    reason text,
    submitted_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uk_certificate_requests_idempotency UNIQUE (idempotency_key)
);

CREATE TABLE certificate_request_domains (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    certificate_request_id uuid NOT NULL REFERENCES certificate_requests(id),
    domain_asset_id uuid NOT NULL REFERENCES domain_assets(id),
    relation_type varchar(32) NOT NULL CHECK (relation_type IN ('primary', 'san', 'wildcard')),
    sort_order integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uk_certificate_request_domains_request_domain UNIQUE (certificate_request_id, domain_asset_id, relation_type)
);

CREATE TABLE issue_workflows (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    project_id uuid NOT NULL REFERENCES projects(id),
    environment_id uuid NOT NULL REFERENCES environments(id),
    certificate_request_id uuid NOT NULL REFERENCES certificate_requests(id),
    ca_account_id uuid NOT NULL REFERENCES ca_accounts(id),
    workflow_type varchar(32) NOT NULL CHECK (workflow_type IN ('issue', 'renew')),
    status varchar(32) NOT NULL CHECK (
        status IN (
            'created', 'order_pending', 'challenge_pending', 'challenge_processing',
            'challenge_valid', 'finalizing', 'issued', 'deploy_pending', 'deploying',
            'deployed', 'partially_failed', 'failed', 'cancelled'
        )
    ),
    order_url text,
    finalize_url text,
    csr_ref varchar(255),
    certificate_ref varchar(255),
    last_error_code varchar(64),
    last_error_message text,
    started_at timestamptz,
    finished_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE workflow_challenges (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    issue_workflow_id uuid NOT NULL REFERENCES issue_workflows(id),
    domain_asset_id uuid REFERENCES domain_assets(id),
    challenge_type varchar(32) NOT NULL CHECK (challenge_type IN ('http-01', 'dns-01')),
    identifier varchar(255) NOT NULL,
    token text,
    key_authorization text,
    http_path text,
    dns_record_name varchar(255),
    dns_record_value text,
    status varchar(32) NOT NULL CHECK (
        status IN (
            'pending', 'presenting', 'presented', 'propagating', 'ready',
            'verifying', 'valid', 'invalid', 'cleanup_pending', 'cleaned', 'cleanup_failed'
        )
    ),
    presented_at timestamptz,
    validated_at timestamptz,
    cleaned_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE certificate_asset_domains (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    asset_id uuid NOT NULL REFERENCES certificate_assets(id),
    domain_asset_id uuid NOT NULL REFERENCES domain_assets(id),
    relation_type varchar(32) NOT NULL CHECK (relation_type IN ('primary', 'san', 'wildcard')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uk_certificate_asset_domains_asset_domain UNIQUE (asset_id, domain_asset_id, relation_type)
);

CREATE TABLE certificate_versions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    asset_id uuid NOT NULL REFERENCES certificate_assets(id),
    version_no integer NOT NULL CHECK (version_no >= 1),
    serial_number varchar(128) NOT NULL,
    subject_cn varchar(255) NOT NULL,
    sans_jsonb jsonb NOT NULL DEFAULT '[]'::jsonb,
    issuer_name varchar(255) NOT NULL,
    not_before timestamptz NOT NULL,
    not_after timestamptz NOT NULL,
    fingerprint_sha256 varchar(128) NOT NULL,
    pem_chain_ref varchar(255) NOT NULL,
    private_key_ref varchar(255),
    status varchar(32) NOT NULL CHECK (status IN ('issued', 'deploying', 'current', 'superseded', 'deploy_failed', 'revoked')),
    issued_at timestamptz NOT NULL,
    deployed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uk_certificate_versions_asset_version UNIQUE (asset_id, version_no),
    CONSTRAINT uk_certificate_versions_fingerprint UNIQUE (fingerprint_sha256)
);

ALTER TABLE certificate_assets
    ADD CONSTRAINT fk_certificate_assets_current_request
    FOREIGN KEY (current_request_id) REFERENCES certificate_requests(id);

ALTER TABLE certificate_assets
    ADD CONSTRAINT fk_certificate_assets_current_version
    FOREIGN KEY (current_version_id) REFERENCES certificate_versions(id);

CREATE UNIQUE INDEX uk_certificate_versions_asset_current
    ON certificate_versions (asset_id)
    WHERE status = 'current';

-- ============================================================================
-- 04. Agent / execution / jobs
-- ============================================================================

CREATE TABLE deployment_targets (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    project_id uuid NOT NULL REFERENCES projects(id),
    environment_id uuid NOT NULL REFERENCES environments(id),
    target_name varchar(128) NOT NULL,
    target_type varchar(32) NOT NULL CHECK (target_type IN ('nginx', 'tomcat-jsse-pkcs12')),
    agent_selector_jsonb jsonb NOT NULL DEFAULT '{}'::jsonb,
    service_name varchar(128) NOT NULL,
    node_hint varchar(255),
    config_path text NOT NULL,
    install_path_jsonb jsonb NOT NULL DEFAULT '{}'::jsonb,
    status varchar(32) NOT NULL CHECK (status IN ('active', 'disabled', 'error')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uk_deployment_targets_env_name UNIQUE (environment_id, target_name)
);

CREATE TABLE agents (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    project_id uuid NOT NULL REFERENCES projects(id),
    environment_id uuid NOT NULL REFERENCES environments(id),
    agent_name varchar(128) NOT NULL,
    hostname varchar(255) NOT NULL,
    ip_address inet NOT NULL,
    version varchar(32) NOT NULL,
    protocol_version integer NOT NULL,
    os varchar(32) NOT NULL,
    arch varchar(32) NOT NULL,
    labels_jsonb jsonb NOT NULL DEFAULT '{}'::jsonb,
    status varchar(32) NOT NULL CHECK (status IN ('registering', 'online', 'degraded', 'offline', 'disabled', 'incompatible')),
    last_seen_at timestamptz,
    cert_serial_no varchar(128),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uk_agents_env_name UNIQUE (environment_id, agent_name)
);

CREATE TABLE agent_capabilities (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id uuid NOT NULL REFERENCES agents(id),
    capability_code varchar(64) NOT NULL,
    capability_version varchar(32),
    status varchar(32) NOT NULL CHECK (status IN ('active', 'disabled')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uk_agent_capabilities_agent_code UNIQUE (agent_id, capability_code)
);

CREATE TABLE node_registration_tokens (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    project_id uuid NOT NULL REFERENCES projects(id),
    environment_id uuid NOT NULL REFERENCES environments(id),
    token_hash varchar(128) NOT NULL,
    issued_by uuid NOT NULL REFERENCES users(id),
    expires_at timestamptz NOT NULL,
    used_at timestamptz,
    used_by_agent_id uuid REFERENCES agents(id),
    status varchar(32) NOT NULL CHECK (status IN ('active', 'used', 'expired', 'revoked')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uk_node_registration_tokens_hash UNIQUE (token_hash)
);

CREATE TABLE jobs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    project_id uuid NOT NULL REFERENCES projects(id),
    environment_id uuid NOT NULL REFERENCES environments(id),
    job_type varchar(64) NOT NULL,
    aggregate_type varchar(64) NOT NULL,
    aggregate_id uuid NOT NULL,
    status varchar(32) NOT NULL CHECK (status IN ('pending', 'claimed', 'running', 'retry', 'succeeded', 'failed', 'cancelled', 'timed_out')),
    priority smallint NOT NULL DEFAULT 100,
    payload_jsonb jsonb NOT NULL DEFAULT '{}'::jsonb,
    idempotency_key varchar(128) NOT NULL,
    lease_owner varchar(128),
    lease_expire_at timestamptz,
    attempt_count integer NOT NULL DEFAULT 0,
    max_attempts integer NOT NULL DEFAULT 3,
    next_run_at timestamptz NOT NULL DEFAULT now(),
    last_error_code varchar(64),
    last_error_message text,
    version integer NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uk_jobs_idempotency UNIQUE (idempotency_key)
);

CREATE TABLE job_attempts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id uuid NOT NULL REFERENCES jobs(id),
    attempt_no integer NOT NULL CHECK (attempt_no >= 1),
    worker_id varchar(128) NOT NULL,
    agent_id uuid REFERENCES agents(id),
    started_at timestamptz NOT NULL DEFAULT now(),
    last_heartbeat_at timestamptz,
    finished_at timestamptz,
    result_status varchar(32) NOT NULL CHECK (result_status IN ('started', 'heartbeating', 'succeeded', 'failed', 'timed_out', 'abandoned')),
    error_code varchar(64),
    error_message text,
    evidence_ref varchar(255),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uk_job_attempts_job_attempt UNIQUE (job_id, attempt_no)
);

CREATE TABLE domain_validation_records (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    project_id uuid NOT NULL REFERENCES projects(id),
    environment_id uuid NOT NULL REFERENCES environments(id),
    domain_asset_id uuid NOT NULL REFERENCES domain_assets(id),
    validation_type varchar(32) NOT NULL CHECK (validation_type IN ('dns-01', 'http-01')),
    provider_type varchar(32) NOT NULL CHECK (provider_type IN ('alidns', 'agent-http01')),
    workflow_challenge_id uuid REFERENCES workflow_challenges(id),
    job_id uuid REFERENCES jobs(id),
    status varchar(32) NOT NULL CHECK (
        status IN (
            'pending', 'presenting', 'propagating', 'verifying',
            'valid', 'invalid', 'cleanup_pending', 'cleaned', 'cleanup_failed'
        )
    ),
    latency_ms integer,
    error_code varchar(64),
    error_message text,
    validated_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE dns_record_operations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    project_id uuid NOT NULL REFERENCES projects(id),
    environment_id uuid NOT NULL REFERENCES environments(id),
    domain_asset_id uuid NOT NULL REFERENCES domain_assets(id),
    validation_record_id uuid REFERENCES domain_validation_records(id),
    provider_type varchar(32) NOT NULL CHECK (provider_type IN ('alidns')),
    operation_type varchar(32) NOT NULL CHECK (operation_type IN ('create', 'update', 'delete', 'cleanup')),
    record_name varchar(255) NOT NULL,
    record_type varchar(16) NOT NULL CHECK (record_type IN ('TXT')),
    record_value_digest varchar(128) NOT NULL,
    ttl integer,
    status varchar(32) NOT NULL CHECK (status IN ('pending', 'executing', 'succeeded', 'failed')),
    error_code varchar(64),
    error_message text,
    executed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE deployment_bindings (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    project_id uuid NOT NULL REFERENCES projects(id),
    environment_id uuid NOT NULL REFERENCES environments(id),
    asset_id uuid NOT NULL REFERENCES certificate_assets(id),
    target_id uuid NOT NULL REFERENCES deployment_targets(id),
    install_policy_jsonb jsonb NOT NULL DEFAULT '{}'::jsonb,
    is_primary boolean NOT NULL DEFAULT false,
    status varchar(32) NOT NULL CHECK (status IN ('active', 'disabled', 'drifted')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uk_deployment_bindings_asset_target UNIQUE (asset_id, target_id)
);

CREATE TABLE deployment_records (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    project_id uuid NOT NULL REFERENCES projects(id),
    environment_id uuid NOT NULL REFERENCES environments(id),
    asset_id uuid NOT NULL REFERENCES certificate_assets(id),
    version_id uuid NOT NULL REFERENCES certificate_versions(id),
    target_id uuid NOT NULL REFERENCES deployment_targets(id),
    agent_id uuid NOT NULL REFERENCES agents(id),
    job_id uuid REFERENCES jobs(id),
    status varchar(32) NOT NULL CHECK (
        status IN (
            'pending', 'prechecking', 'backing_up', 'installing',
            'reloading', 'verifying', 'succeeded', 'failed',
            'rolling_back', 'rolled_back', 'rollback_failed'
        )
    ),
    backup_ref varchar(255),
    verification_result_jsonb jsonb,
    rollback_result_jsonb jsonb,
    error_code varchar(64),
    error_message text,
    started_at timestamptz,
    finished_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE discovery_records (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    project_id uuid NOT NULL REFERENCES projects(id),
    environment_id uuid NOT NULL REFERENCES environments(id),
    agent_id uuid NOT NULL REFERENCES agents(id),
    target_type varchar(32) NOT NULL CHECK (target_type IN ('nginx', 'tomcat-jsse-pkcs12')),
    target_id uuid REFERENCES deployment_targets(id),
    node_name varchar(255) NOT NULL,
    service_name varchar(128) NOT NULL,
    config_path text NOT NULL,
    certificate_path text,
    keystore_path text,
    fingerprint_sha256 varchar(128),
    subject_cn varchar(255),
    not_after timestamptz,
    match_asset_id uuid REFERENCES certificate_assets(id),
    status varchar(32) NOT NULL CHECK (status IN ('scanned', 'parsed', 'matched', 'unmanaged', 'invalid', 'ignored')),
    raw_detail_jsonb jsonb,
    scanned_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- ============================================================================
-- 05. Audit / evidence / notification
-- ============================================================================

CREATE TABLE audit_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    project_id uuid REFERENCES projects(id),
    environment_id uuid REFERENCES environments(id),
    actor_type varchar(32) NOT NULL CHECK (actor_type IN ('user', 'system', 'agent', 'api_key')),
    actor_id varchar(128) NOT NULL,
    action varchar(128) NOT NULL,
    resource_type varchar(64) NOT NULL,
    resource_id uuid,
    request_id varchar(64),
    trace_id varchar(64),
    detail_jsonb jsonb NOT NULL DEFAULT '{}'::jsonb,
    occurred_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE evidence_records (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    project_id uuid REFERENCES projects(id),
    environment_id uuid REFERENCES environments(id),
    resource_type varchar(64) NOT NULL,
    resource_id uuid NOT NULL,
    evidence_type varchar(64) NOT NULL CHECK (evidence_type IN ('challenge', 'deployment', 'rollback', 'discovery', 'log')),
    storage_ref varchar(255) NOT NULL,
    digest_sha256 varchar(128) NOT NULL,
    metadata_jsonb jsonb,
    status varchar(32) NOT NULL CHECK (status IN ('active', 'archived')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE webhook_endpoints (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    project_id uuid REFERENCES projects(id),
    environment_id uuid REFERENCES environments(id),
    name varchar(128) NOT NULL,
    endpoint_url text NOT NULL,
    secret_envelope jsonb,
    event_types_jsonb jsonb NOT NULL DEFAULT '[]'::jsonb,
    status varchar(32) NOT NULL CHECK (status IN ('active', 'disabled')),
    last_tested_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX uk_webhook_endpoints_tenant_env_name
    ON webhook_endpoints (
        tenant_id,
        COALESCE(environment_id, '00000000-0000-0000-0000-000000000000'::uuid),
        name
    );

CREATE TABLE notification_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    project_id uuid REFERENCES projects(id),
    environment_id uuid REFERENCES environments(id),
    webhook_endpoint_id uuid NOT NULL REFERENCES webhook_endpoints(id),
    event_type varchar(64) NOT NULL,
    resource_type varchar(64) NOT NULL,
    resource_id uuid NOT NULL,
    payload_jsonb jsonb NOT NULL DEFAULT '{}'::jsonb,
    status varchar(32) NOT NULL CHECK (status IN ('pending', 'delivering', 'succeeded', 'failed', 'retry', 'cancelled')),
    last_error_code varchar(64),
    last_error_message text,
    next_retry_at timestamptz,
    delivered_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE export_records (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    project_id uuid REFERENCES projects(id),
    environment_id uuid REFERENCES environments(id),
    export_no varchar(64) NOT NULL,
    export_type varchar(32) NOT NULL CHECK (export_type IN ('audit', 'evidence', 'report')),
    resource_type varchar(64),
    requested_by uuid REFERENCES users(id),
    format varchar(16) NOT NULL CHECK (format IN ('pdf', 'csv', 'json')),
    filters_jsonb jsonb NOT NULL DEFAULT '{}'::jsonb,
    status varchar(32) NOT NULL CHECK (status IN ('pending', 'running', 'succeeded', 'failed', 'expired')),
    storage_ref varchar(255),
    error_code varchar(64),
    error_message text,
    finished_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uk_export_records_export_no UNIQUE (export_no)
);

-- ============================================================================
-- 06. Approval workflow reserve
-- ============================================================================

CREATE TABLE approval_requests (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    project_id uuid REFERENCES projects(id),
    environment_id uuid REFERENCES environments(id),
    action_type varchar(64) NOT NULL,
    resource_type varchar(64) NOT NULL,
    resource_id uuid,
    initiator_user_id uuid NOT NULL REFERENCES users(id),
    status varchar(32) NOT NULL CHECK (status IN ('pending', 'in_review', 'approved', 'rejected', 'cancelled', 'expired')),
    rule_snapshot_jsonb jsonb NOT NULL DEFAULT '{}'::jsonb,
    context_jsonb jsonb NOT NULL DEFAULT '{}'::jsonb,
    current_step_no integer NOT NULL DEFAULT 0,
    due_at timestamptz,
    decided_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE approval_steps (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    approval_request_id uuid NOT NULL REFERENCES approval_requests(id),
    step_no integer NOT NULL CHECK (step_no >= 1),
    step_type varchar(32) NOT NULL CHECK (step_type IN ('serial', 'parallel', 'any')),
    approver_scope_jsonb jsonb NOT NULL DEFAULT '{}'::jsonb,
    status varchar(32) NOT NULL CHECK (status IN ('pending', 'in_progress', 'approved', 'rejected', 'skipped', 'expired')),
    started_at timestamptz,
    finished_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uk_approval_steps_request_step UNIQUE (approval_request_id, step_no)
);

CREATE TABLE approval_records (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    approval_request_id uuid NOT NULL REFERENCES approval_requests(id),
    approval_step_id uuid NOT NULL REFERENCES approval_steps(id),
    approver_user_id uuid NOT NULL REFERENCES users(id),
    decision varchar(32) NOT NULL CHECK (decision IN ('approved', 'rejected', 'abstained', 'timeout')),
    reason text,
    decided_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- ============================================================================
-- 07. Indexes
-- ============================================================================

CREATE INDEX idx_projects_tenant_status
    ON projects (tenant_id, status);

CREATE INDEX idx_environments_tenant_project
    ON environments (tenant_id, project_id);

CREATE INDEX idx_users_tenant_status
    ON users (tenant_id, status);

CREATE INDEX idx_auth_sessions_user_status
    ON auth_sessions (user_id, status, expires_at);

CREATE INDEX idx_api_keys_scope_status
    ON api_keys (tenant_id, scope_type, status, expires_at);

CREATE INDEX idx_tenant_quotas_tenant_status
    ON tenant_quotas (tenant_id, status);

CREATE INDEX idx_dns_credentials_provider
    ON dns_credentials (environment_id, provider_type, status);

CREATE INDEX idx_ca_accounts_provider
    ON ca_accounts (environment_id, provider_type, provider_name);

CREATE INDEX idx_domain_assets_dns_credential
    ON domain_assets (dns_credential_id);

CREATE INDEX idx_certificate_requests_asset
    ON certificate_requests (asset_id, status);

CREATE INDEX idx_issue_workflows_request
    ON issue_workflows (certificate_request_id, status);

CREATE INDEX idx_issue_workflows_ca_account
    ON issue_workflows (ca_account_id, status);

CREATE INDEX idx_workflow_challenges_workflow
    ON workflow_challenges (issue_workflow_id, status);

CREATE INDEX idx_workflow_challenges_domain
    ON workflow_challenges (domain_asset_id, challenge_type);

CREATE INDEX idx_domain_validation_records_domain_status
    ON domain_validation_records (domain_asset_id, status);

CREATE INDEX idx_domain_validation_records_job
    ON domain_validation_records (job_id);

CREATE INDEX idx_dns_record_operations_domain
    ON dns_record_operations (domain_asset_id, created_at DESC);

CREATE INDEX idx_dns_record_operations_validation
    ON dns_record_operations (validation_record_id);

CREATE INDEX idx_certificate_assets_status
    ON certificate_assets (environment_id, status, expires_at);

CREATE INDEX idx_certificate_versions_asset_status
    ON certificate_versions (asset_id, status);

CREATE INDEX idx_deployment_targets_env_type
    ON deployment_targets (environment_id, target_type, status);

CREATE INDEX idx_deployment_bindings_target_status
    ON deployment_bindings (target_id, status);

CREATE INDEX idx_agents_env_status
    ON agents (environment_id, status, last_seen_at DESC);

CREATE INDEX idx_node_registration_tokens_env_status
    ON node_registration_tokens (environment_id, status, expires_at);

CREATE INDEX idx_jobs_schedulable
    ON jobs (status, next_run_at, priority);

CREATE INDEX idx_jobs_lease
    ON jobs (status, lease_expire_at);

CREATE INDEX idx_jobs_aggregate
    ON jobs (aggregate_type, aggregate_id);

CREATE INDEX idx_job_attempts_agent
    ON job_attempts (agent_id, started_at DESC);

CREATE INDEX idx_deployment_records_asset
    ON deployment_records (asset_id, created_at DESC);

CREATE INDEX idx_deployment_records_target_status
    ON deployment_records (target_id, status, created_at DESC);

CREATE INDEX idx_discovery_records_env_status
    ON discovery_records (environment_id, status, scanned_at DESC);

CREATE INDEX idx_discovery_records_fingerprint
    ON discovery_records (fingerprint_sha256);

CREATE INDEX idx_audit_events_tenant_time
    ON audit_events (tenant_id, occurred_at DESC);

CREATE INDEX idx_audit_events_resource
    ON audit_events (resource_type, resource_id, occurred_at DESC);

CREATE INDEX idx_evidence_records_resource
    ON evidence_records (resource_type, resource_id);

CREATE INDEX idx_notification_events_status
    ON notification_events (status, next_retry_at);

CREATE INDEX idx_notification_events_webhook
    ON notification_events (webhook_endpoint_id, created_at DESC);

CREATE INDEX idx_approval_requests_resource
    ON approval_requests (resource_type, resource_id, status);

CREATE INDEX idx_approval_steps_request_status
    ON approval_steps (approval_request_id, status);

CREATE INDEX idx_export_records_tenant_status
    ON export_records (tenant_id, status, created_at DESC);

-- ============================================================================
-- 08. updated_at triggers
-- ============================================================================

DO $$
DECLARE
    tbl text;
BEGIN
    FOREACH tbl IN ARRAY ARRAY[
        'tenants',
        'projects',
        'environments',
        'users',
        'user_credentials',
        'roles',
        'role_bindings',
        'tenant_quotas',
        'auth_sessions',
        'api_keys',
        'system_settings',
        'dns_credentials',
        'ca_accounts',
        'domain_assets',
        'certificate_assets',
        'certificate_requests',
        'certificate_request_domains',
        'issue_workflows',
        'workflow_challenges',
        'certificate_asset_domains',
        'certificate_versions',
        'deployment_targets',
        'agents',
        'agent_capabilities',
        'node_registration_tokens',
        'jobs',
        'job_attempts',
        'domain_validation_records',
        'dns_record_operations',
        'deployment_bindings',
        'deployment_records',
        'discovery_records',
        'audit_events',
        'evidence_records',
        'webhook_endpoints',
        'notification_events',
        'export_records',
        'approval_requests',
        'approval_steps',
        'approval_records'
    ]
    LOOP
        EXECUTE format(
            'CREATE TRIGGER trg_%I_updated_at
             BEFORE UPDATE ON %I
             FOR EACH ROW
             EXECUTE FUNCTION set_updated_at()',
            tbl,
            tbl
        );
    END LOOP;
END
$$;

COMMIT;
