#!/usr/bin/env bash

set -euo pipefail

PROJECT_NAME="${COMPOSE_PROJECT_NAME:-autocertx_test_scheduler_$$}"
COMPOSE=(docker compose -p "${PROJECT_NAME}")
POSTGRES_DSN="${AUTOCERTX_POSTGRES_URL:-postgres://autocertx:autocertx@localhost:5432/autocertx?sslmode=disable}"

cleanup() {
  "${COMPOSE[@]}" down -v >/dev/null 2>&1 || true
}

trap cleanup EXIT

"${COMPOSE[@]}" up -d postgres >/dev/null

for _ in $(seq 1 30); do
  if "${COMPOSE[@]}" exec -T postgres pg_isready -U autocertx -d autocertx >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

"${COMPOSE[@]}" exec -T postgres pg_isready -U autocertx -d autocertx >/dev/null
"${COMPOSE[@]}" exec -T postgres psql -U autocertx -d autocertx -v ON_ERROR_STOP=1 -c "DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public;" >/dev/null
cat sql/001_init_schema.sql | "${COMPOSE[@]}" exec -T postgres psql -U autocertx -d autocertx -v ON_ERROR_STOP=1 >/dev/null

GOCACHE="${GOCACHE:-$(pwd)/.cache/go-build}" \
GOTMPDIR="${GOTMPDIR:-$(pwd)/.cache/go-tmp}" \
AUTOCERTX_POSTGRES_URL="${POSTGRES_DSN}" \
go test ./internal/repository/postgres/jobstore/... -count=1
