GO ?= go
DOCKER_COMPOSE ?= docker compose
GOFMT_DIRS := cmd internal/platform internal/app internal/repository internal/domain/identity internal/domain/tenancy internal/domain/job internal/domain/resource internal/domain/domains internal/domain/dnscredentials internal/domain/issuer internal/domain/audit internal/domain/settings internal/application/command/auth internal/application/command/jobs internal/application/command/domains internal/application/command/caaccounts internal/application/command/settings internal/application/query/authcontext internal/application/query/audit internal/application/query/dashboard internal/application/query/domains internal/application/query/jobs internal/application/query/settings internal/scheduler api/openapi
GO_PACKAGES := ./cmd/... ./internal/platform/... ./internal/app/... ./internal/repository/... ./internal/domain/identity/... ./internal/domain/tenancy/... ./internal/domain/job/... ./internal/domain/resource/... ./internal/domain/domains/... ./internal/domain/dnscredentials/... ./internal/domain/issuer/... ./internal/domain/audit/... ./internal/domain/settings/... ./internal/application/command/auth/... ./internal/application/command/jobs/... ./internal/application/command/domains/... ./internal/application/command/caaccounts/... ./internal/application/command/settings/... ./internal/application/query/authcontext/... ./internal/application/query/audit/... ./internal/application/query/dashboard/... ./internal/application/query/domains/... ./internal/application/query/jobs/... ./internal/application/query/settings/... ./internal/scheduler/... ./api/openapi/...
LOCAL_CACHE_DIR := $(CURDIR)/.cache
GO_BUILD_CACHE := $(LOCAL_CACHE_DIR)/go-build
GO_TMP_DIR := $(LOCAL_CACHE_DIR)/go-tmp

.PHONY: prepare-go-env fmt fmt-check fmt-check-t05 lint lint-t05 test-unit test verify-ddl test-repository openapi-verify test-contracts test-auth-domain test-domain-governance test-audit-settings test-query test-integration-auth test-integration-domain-governance test-integration-audit test-integration-query test-scheduler test-integration-scheduler run-controlplane run-agent dev-deps-up dev-deps-down dev-deps-logs web-lint web-test web-build ci-task-T00 ci-task-T01 ci-task-T02 ci-task-T03 ci-task-T04 ci-task-T05 ci-task-T06 ci-task-T07 ci-task-T13

prepare-go-env:
	@mkdir -p $(GO_BUILD_CACHE) $(GO_TMP_DIR)

fmt:
	@files=$$(find $(GOFMT_DIRS) -name '*.go' -type f); \
	if [ -n "$$files" ]; then gofmt -w $$files; fi

fmt-check:
	@files=$$(find $(GOFMT_DIRS) -name '*.go' -type f); \
	test -z "$$(gofmt -l $$files)"

fmt-check-t05:
	@files=$$(find internal/domain/job internal/application/command/jobs internal/scheduler -name '*.go' -type f); \
	test -z "$$(gofmt -l $$files)"

lint: prepare-go-env
	@GOCACHE=$(GO_BUILD_CACHE) GOTMPDIR=$(GO_TMP_DIR) $(GO) vet $(GO_PACKAGES)

lint-t05: prepare-go-env
	@GOCACHE=$(GO_BUILD_CACHE) GOTMPDIR=$(GO_TMP_DIR) $(GO) vet ./internal/domain/job/... ./internal/application/command/jobs/... ./internal/scheduler/...

test-unit: prepare-go-env
	@GOCACHE=$(GO_BUILD_CACHE) GOTMPDIR=$(GO_TMP_DIR) $(GO) test $(GO_PACKAGES)

test: test-unit

verify-ddl:
	@bash ./scripts/verify_ddl.sh

test-repository: prepare-go-env
	@GOCACHE=$(GO_BUILD_CACHE) GOTMPDIR=$(GO_TMP_DIR) $(GO) test ./internal/repository/...

openapi-verify: prepare-go-env
	@GOCACHE=$(GO_BUILD_CACHE) GOTMPDIR=$(GO_TMP_DIR) $(GO) test ./api/openapi/... -run 'TestOpenAPI'

test-contracts: prepare-go-env
	@GOCACHE=$(GO_BUILD_CACHE) GOTMPDIR=$(GO_TMP_DIR) $(GO) test ./api/openapi/... -run 'TestContracts'

test-auth-domain: prepare-go-env
	@GOCACHE=$(GO_BUILD_CACHE) GOTMPDIR=$(GO_TMP_DIR) $(GO) test ./internal/domain/identity/... ./internal/domain/tenancy/...

test-domain-governance: prepare-go-env
	@GOCACHE=$(GO_BUILD_CACHE) GOTMPDIR=$(GO_TMP_DIR) $(GO) test ./internal/domain/domains/... ./internal/domain/dnscredentials/... ./internal/domain/issuer/... ./internal/application/command/domains/... ./internal/application/command/caaccounts/...

test-audit-settings: prepare-go-env
	@GOCACHE=$(GO_BUILD_CACHE) GOTMPDIR=$(GO_TMP_DIR) $(GO) test ./internal/domain/audit/... ./internal/domain/settings/... ./internal/application/command/settings/... ./internal/application/query/audit/...

test-query: prepare-go-env
	@GOCACHE=$(GO_BUILD_CACHE) GOTMPDIR=$(GO_TMP_DIR) $(GO) test ./internal/application/query/...

test-integration-auth: prepare-go-env
	@GOCACHE=$(GO_BUILD_CACHE) GOTMPDIR=$(GO_TMP_DIR) $(GO) test ./internal/app/controlplane/http/... -run 'TestAuth'

test-integration-domain-governance: prepare-go-env
	@GOCACHE=$(GO_BUILD_CACHE) GOTMPDIR=$(GO_TMP_DIR) $(GO) test ./internal/app/controlplane/http/... -run 'TestGovernance'

test-integration-audit: prepare-go-env
	@GOCACHE=$(GO_BUILD_CACHE) GOTMPDIR=$(GO_TMP_DIR) $(GO) test ./internal/app/controlplane/http/... -run 'TestAuditSettings'

test-integration-query: prepare-go-env
	@GOCACHE=$(GO_BUILD_CACHE) GOTMPDIR=$(GO_TMP_DIR) $(GO) test ./internal/app/controlplane/... -run 'TestQuery'

test-scheduler: prepare-go-env
	@GOCACHE=$(GO_BUILD_CACHE) GOTMPDIR=$(GO_TMP_DIR) $(GO) test ./internal/domain/job/... ./internal/application/command/jobs/... ./internal/scheduler/...

test-integration-scheduler: prepare-go-env
	@bash ./scripts/test_scheduler_integration.sh

run-controlplane: prepare-go-env
	@GOCACHE=$(GO_BUILD_CACHE) GOTMPDIR=$(GO_TMP_DIR) $(GO) run ./cmd/controlplane

run-agent: prepare-go-env
	@GOCACHE=$(GO_BUILD_CACHE) GOTMPDIR=$(GO_TMP_DIR) $(GO) run ./cmd/agent

dev-deps-up:
	@$(DOCKER_COMPOSE) up -d postgres redis

dev-deps-down:
	@$(DOCKER_COMPOSE) down

dev-deps-logs:
	@$(DOCKER_COMPOSE) logs -f postgres redis

web-lint:
	@cd web/console && npm run lint

web-test:
	@cd web/console && npm run test

web-build:
	@cd web/console && npm run build

ci-task-T00: fmt-check lint test-unit
	@$(DOCKER_COMPOSE) config >/dev/null

ci-task-T01: fmt-check test-repository verify-ddl

ci-task-T02: fmt-check openapi-verify test-contracts

ci-task-T03: fmt-check lint test-unit test-auth-domain test-integration-auth

ci-task-T04: fmt-check lint test-domain-governance test-integration-domain-governance

ci-task-T05: fmt-check-t05 lint-t05 test-scheduler test-integration-scheduler

ci-task-T06: fmt-check lint test-audit-settings test-integration-audit openapi-verify test-contracts

ci-task-T07: fmt-check lint test-query test-integration-query

ci-task-T13: web-lint web-test web-build
