#!/bin/sh
# Rebuild a local database with service/migrate (schema, ac_app_login, demo fixture).
#   sh scripts/resetdb.sh            development database "ac" (local API, worker, BFF)
#   DB=ac_test sh scripts/resetdb.sh  test database used by go test (never shared with running services)
# Needs the compose postgres service (docker compose --profile infra up -d postgres).
set -e
cd "$(dirname "$0")/../../.."
PG=${PG_CONTAINER:-ac-project-postgres-1}
DB=${DB:-ac}
docker exec "$PG" psql -U postgres -q -c "DROP DATABASE IF EXISTS $DB WITH (FORCE)" -c "CREATE DATABASE $DB"
cd service/api && DATABASE_URL=${OWNER_DATABASE_URL:-postgres://postgres:local@localhost:5432/$DB?sslmode=disable} \
  APP_LOGIN_PASSWORD=local SEED_FIXTURE=../../docs/04-agentic-sdlc/fixture-contract.json \
  GOTOOLCHAIN=auto go run ./cmd/migrate up
