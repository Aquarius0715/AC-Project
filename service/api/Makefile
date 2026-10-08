# Go service tasks for service/api, core, worker and migrate (backend Go design §10; go.work in service/). Integration tests need the compose postgres service (docker compose --profile infra up -d postgres); they use the database ac_test
GO ?= GOTOOLCHAIN=auto go

gen:
	cd ../core && $(GO) run ./cmd/gen/ops

seed:
	cd ../core && $(GO) run ./cmd/seed

# go test uses its own database ac_test (AC_TEST_DATABASE_URL / AC_TEST_OWNER_DATABASE_URL override it), so a running
# local API or worker on the development database "ac" never races the tests
test: testdb-exists
	cd .. && $(GO) vet ./core/... ./api/... ./worker/... ./migrate/...
	cd .. && $(GO) test -count=1 ./core/... ./api/... ./worker/... ./migrate/...

# migrations copies the design schema into the first migration (until the first release freezes it)
migrations:
	cp ../../docs/02-design/db/schema.sql ../migrate/migrations/000001_init.up.sql

# resetdb rebuilds the development database "ac"; testdb rebuilds the test database "ac_test" (tests accumulate rows)
resetdb:
	sh tools/resetdb.sh

testdb:
	DB=ac_test sh tools/resetdb.sh

testdb-exists:
	@docker exec $${PG_CONTAINER:-ac-project-postgres-1} psql -U postgres -tAc "SELECT 1 FROM pg_database WHERE datname = 'ac_test'" | grep -q 1 || DB=ac_test sh tools/resetdb.sh

test-fresh: testdb test

cover: testdb-exists
	rm -rf .cover && mkdir -p .cover
	for p in $$($(GO) list -f '{{if .TestGoFiles}}{{.ImportPath}}{{end}}' ./internal/... github.com/pradita/ac-project/service/core/...); do \
		$(GO) test -count=1 -coverpkg=./internal/...,github.com/pradita/ac-project/service/core/... -coverprofile=.cover/$$(echo $$p | tr / _).out $$p >.cover/last.log 2>&1 || { cat .cover/last.log; exit 1; }; done
	python3 tools/covermerge.py coverage.out .cover/*.out

.PHONY: gen seed test cover migrations resetdb testdb testdb-exists test-fresh
