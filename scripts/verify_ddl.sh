#!/usr/bin/env bash

set -euo pipefail

PROJECT_NAME="${COMPOSE_PROJECT_NAME:-autocertx_verify_ddl_$$}"
COMPOSE=(docker compose -p "${PROJECT_NAME}")

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

tables=$("${COMPOSE[@]}" exec -T postgres psql -U autocertx -d autocertx -tAc "SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_type = 'BASE TABLE';")
test "${tables}" -ge 30

missing=$("${COMPOSE[@]}" exec -T postgres psql -U autocertx -d autocertx -tAc "WITH base_tables AS (SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' AND table_type = 'BASE TABLE'), field_presence AS (SELECT table_name, bool_or(column_name = 'id') AS has_id, bool_or(column_name = 'created_at') AS has_created_at, bool_or(column_name = 'updated_at') AS has_updated_at FROM information_schema.columns WHERE table_schema = 'public' GROUP BY table_name) SELECT count(*) FROM base_tables bt LEFT JOIN field_presence fp USING (table_name) WHERE NOT COALESCE(fp.has_id, false) OR NOT COALESCE(fp.has_created_at, false) OR NOT COALESCE(fp.has_updated_at, false);")
test "${missing}" -eq 0
