#!/bin/sh
# Rebuild a local database with service/migrate (schema, ac_app_login, demo fixture).
#   sh scripts/resetdb.sh            development database "ac" (local API, worker, BFF)
#   DB=ac_test sh scripts/resetdb.sh  test database used by go test (never shared with running services)
# Needs the compose postgres service (docker compose --profile infra up -d postgres), or PG_ADMIN_URL (an admin
# connection to any PostgreSQL 16, as in CI).
set -e
cd "$(dirname "$0")/../../.."
PG=${PG_CONTAINER:-ac-project-postgres-1}
DB=${DB:-ac}
if [ -n "${PG_ADMIN_URL:-}" ]; then # CI: a PostgreSQL service reached directly (.github/workflows/ci.yml)
  psql "$PG_ADMIN_URL" -q -c "DROP DATABASE IF EXISTS $DB WITH (FORCE)" -c "CREATE DATABASE $DB"
else
  docker exec "$PG" psql -U postgres -q -c "DROP DATABASE IF EXISTS $DB WITH (FORCE)" -c "CREATE DATABASE $DB"
fi
cd service/api && DATABASE_URL=${OWNER_DATABASE_URL:-postgres://postgres:local@localhost:5432/$DB?sslmode=disable} \
  APP_LOGIN_PASSWORD=local SEED_FIXTURE=../../docs/04-agentic-sdlc/fixture-contract.json \
  GOTOOLCHAIN=auto go run ./cmd/migrate up
