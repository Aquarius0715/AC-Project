#!/bin/sh
# Rebuild the local development database "ac" from docs/02-design/db/schema.sql and load the demo fixture.
# Needs the compose postgres service (docker compose --profile schema up -d postgres).
set -e
cd "$(dirname "$0")/../../.."
PG=${PG_CONTAINER:-ac-project-postgres-1}
docker exec "$PG" psql -U postgres -q -c "DROP DATABASE IF EXISTS ac WITH (FORCE)" -c "CREATE DATABASE ac"
docker exec -i "$PG" psql -U postgres -d ac -v ON_ERROR_STOP=1 -q < docs/02-design/db/schema.sql
docker exec "$PG" psql -U postgres -d ac -q -c "DO \$\$BEGIN IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname='ac_app_login') THEN CREATE ROLE ac_app_login LOGIN PASSWORD 'local' IN ROLE ac_app; END IF; END\$\$"
cd service/core && GOTOOLCHAIN=auto go run ./cmd/seed
