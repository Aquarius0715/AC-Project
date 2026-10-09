# Go tasks for the service module (Go "Organizing a Go module" server layout: cmd/ commands, internal/ packages;
# backend Go design §2, §10). Run from service/api. DB tests need the compose postgres service
# (docker compose --profile infra up -d postgres) and use their own database ac_test (IR171).
GO ?= GOTOOLCHAIN=auto go

gen:
	$(GO) run ./cmd/gen/ops

seed:
	$(GO) run ./cmd/seed

# unit tests next to the code (no database)
test-unit:
	$(GO) vet ./...
	$(GO) test -count=1 $$($(GO) list ./... | grep -v /test/)

# the integration suites outgrew go test's default 10 minute limit (the split-service run takes about ten minutes)
TEST_TIMEOUT ?= 30m

# DB integration tests of the HTTP API (test/integration, black box through internal/server)
test-integration: testdb-exists
	$(GO) test -count=1 -timeout $(TEST_TIMEOUT) ./test/...

# the integration suite against the split services: one server per domain on its own database role behind the
# gateway (IR194); any cross-domain table access fails
test-cluster: testdb-exists
	AC_TEST_CLUSTER=1 $(GO) test -count=1 -timeout $(TEST_TIMEOUT) ./test/...

# everything; ac_test is created when missing (AC_TEST_DATABASE_URL / AC_TEST_OWNER_DATABASE_URL override it)
test: testdb-exists
	$(GO) vet ./...
	$(GO) test -count=1 -timeout $(TEST_TIMEOUT) ./...

# swagger builds the API description from the swag annotations of the operation handlers (IR220): swag formats the
# annotations, writes Swagger 2.0 JSON that the gateway embeds (GET /swagger.json, Swagger UI at /docs) and the copy
# in the design docs. scripts/swag_annotate.py --check lists handlers without annotations.
swagger:
	python3 scripts/swag_annotate.py --check
	$(GO) tool swag fmt -d ./internal,./cmd
	$(GO) tool swag init -g cmd/gateway/main.go -d . --parseInternal --overridesFile .swaggo -o internal/gateway/swagger --outputTypes json
	mkdir -p ../../docs/02-design/api && cp internal/gateway/swagger/swagger.json ../../docs/02-design/api/swagger.json

# migrations copies the design schema into the first migration (until the first release freezes it)
migrations:
	cp ../../docs/02-design/db/schema.sql internal/migrations/000001_init.up.sql

# resetdb rebuilds the development database "ac"; testdb rebuilds the test database "ac_test" (tests accumulate rows)
resetdb:
	sh scripts/resetdb.sh

testdb:
	DB=ac_test sh scripts/resetdb.sh

testdb-exists:
	@docker exec $${PG_CONTAINER:-ac-project-postgres-1} psql -U postgres -tAc "SELECT 1 FROM pg_database WHERE datname = 'ac_test'" | grep -q 1 || DB=ac_test sh scripts/resetdb.sh

test-fresh: testdb test

# coverage of internal/... from every test package (unit + integration), merged into coverage.out
cover: testdb-exists
	rm -rf .cover && mkdir -p .cover
	for p in $$($(GO) list -f '{{if or .TestGoFiles .XTestGoFiles}}{{.ImportPath}}{{end}}' ./...); do \
		$(GO) test -count=1 -coverpkg=./internal/... -coverprofile=.cover/$$(echo $$p | tr / _).out $$p >.cover/last.log 2>&1 || { cat .cover/last.log; exit 1; }; done
	python3 scripts/covermerge.py coverage.out .cover/*.out

.PHONY: swagger gen seed test test-unit test-integration test-cluster cover migrations resetdb testdb testdb-exists test-fresh
