#!/bin/sh
# Rebuild the local development database "ac" with service/migrate (schema, ac_app_login, demo fixture).
# Needs the compose postgres service (docker compose --profile infra up -d postgres).
set -e
cd "$(dirname "$0")/../../.."
PG=${PG_CONTAINER:-ac-project-postgres-1}
docker exec "$PG" psql -U postgres -q -c "DROP DATABASE IF EXISTS ac WITH (FORCE)" -c "CREATE DATABASE ac"
cd service/migrate && DATABASE_URL=${OWNER_DATABASE_URL:-postgres://postgres:local@localhost:5432/ac?sslmode=disable} \
  APP_LOGIN_PASSWORD=local SEED_FIXTURE=../../docs/04-agentic-sdlc/fixture-contract.json \
  GOTOOLCHAIN=auto go run ./cmd/migrate up
