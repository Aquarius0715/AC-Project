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

# a fresh test database runs each suite in about a minute; one that kept every earlier run's rows took 12–20 minutes
TEST_TIMEOUT ?= 30m

# DB integration tests of the HTTP API (test/integration, black box through internal/server) on a fresh ac_test
test-integration: testdb
	$(GO) test -count=1 -timeout $(TEST_TIMEOUT) ./test/...

# the integration suite against the split services: one server per domain on its own database role behind the
# gateway (IR194); any cross-domain table access fails. Its own fresh database ac_test_cluster, so it can run beside
# make test (make test-all).
test-cluster: testdb-cluster test-cluster-run

# everything on a fresh ac_test (AC_TEST_DATABASE_URL / AC_TEST_OWNER_DATABASE_URL override it for the package tests)
test: testdb test-run

test-run:
	$(GO) vet ./...
	$(GO) test -count=1 -timeout $(TEST_TIMEOUT) ./...

test-cluster-run:
	AC_TEST_DB=ac_test_cluster AC_TEST_CLUSTER=1 $(GO) test -count=1 -timeout $(TEST_TIMEOUT) ./test/...

# both suites at once, each on its own database (about the time of the slower one). The databases are rebuilt one
# after the other first: their migrations update the cluster-wide login roles. The full logs stay in .test-all/.
test-all: testdb testdb-cluster
	@mkdir -p .test-all; \
	$(MAKE) test-run > .test-all/test.log 2>&1 & a=$$!; \
	$(MAKE) test-cluster-run > .test-all/test-cluster.log 2>&1 & b=$$!; \
	wait $$a; ra=$$?; wait $$b; rb=$$?; \
	grep -E '^(FAIL|--- FAIL|panic)' .test-all/test.log; echo "make test: exit $$ra"; \
	grep -E '^(FAIL|--- FAIL|panic)' .test-all/test-cluster.log; echo "make test-cluster: exit $$rb"; \
	[ $$ra -eq 0 ] && [ $$rb -eq 0 ]

# swagger builds the API description from the swag annotations of the operation handlers (IR220, REST routes IR222):
# swag formats the annotations, writes Swagger 2.0 JSON that the gateway embeds (GET /swagger.json, Swagger UI at
# /docs) and the copy in the design docs. scripts/swag_annotate.py --check (fed the mounted routes and parameters by
# cmd/gen/restdoc) fails when an annotation block or a route stub is missing or stale.
swagger:
	$(GO) run ./cmd/gen/restdoc | python3 scripts/swag_annotate.py --routes - --check
	$(GO) tool swag fmt -d ./internal,./cmd
	$(GO) tool swag init -g cmd/gateway/main.go -d . --parseInternal --overridesFile .swaggo -o internal/gateway/swagger --outputTypes json
	mkdir -p ../../docs/02-design/api && cp internal/gateway/swagger/swagger.json ../../docs/02-design/api/swagger.json

# migrations copies the design schema into the first migration (until the first release freezes it)
migrations:
	cp ../../docs/02-design/db/schema.sql internal/migrations/000001_init.up.sql

# resetdb rebuilds the development database "ac"; testdb / testdb-cluster rebuild the test databases (the suites
# leave their rows behind, so every suite run starts from a fresh one)
resetdb:
	sh scripts/resetdb.sh

testdb:
	DB=ac_test sh scripts/resetdb.sh

testdb-cluster:
	DB=ac_test_cluster sh scripts/resetdb.sh

testdb-exists:
	@docker exec $${PG_CONTAINER:-ac-project-postgres-1} psql -U postgres -tAc "SELECT 1 FROM pg_database WHERE datname = 'ac_test'" | grep -q 1 || DB=ac_test sh scripts/resetdb.sh

test-fresh: test

# coverage of internal/... from every test package (unit + integration), merged into coverage.out
cover: testdb-exists
	rm -rf .cover && mkdir -p .cover
	for p in $$($(GO) list -f '{{if or .TestGoFiles .XTestGoFiles}}{{.ImportPath}}{{end}}' ./...); do \
		$(GO) test -count=1 -coverpkg=./internal/... -coverprofile=.cover/$$(echo $$p | tr / _).out $$p >.cover/last.log 2>&1 || { cat .cover/last.log; exit 1; }; done
	python3 scripts/covermerge.py coverage.out .cover/*.out

.PHONY: swagger gen seed test test-run test-unit test-integration test-cluster test-cluster-run test-all cover migrations resetdb testdb testdb-cluster testdb-exists test-fresh
