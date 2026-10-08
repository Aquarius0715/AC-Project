# Go service tasks for service/api, core, worker and migrate (backend Go design §10; go.work in service/). Integration tests need: docker compose --profile infra --profile schema up -d postgres migrate
GO ?= GOTOOLCHAIN=auto go

gen:
	cd ../core && $(GO) run ./cmd/gen/ops

seed:
	cd ../core && $(GO) run ./cmd/seed

test:
	cd .. && $(GO) vet ./core/... ./api/... ./worker/... ./migrate/...
	cd .. && $(GO) test ./core/... ./api/... ./worker/... ./migrate/...

# resetdb rebuilds the local database with service/migrate and reseeds (tests accumulate rows between runs)
# migrations copies the design schema into the first migration (until the first release freezes it)
migrations:
	cp ../../docs/02-design/db/schema.sql ../migrate/migrations/000001_init.up.sql

resetdb:
	sh tools/resetdb.sh

test-fresh: resetdb test

cover:
	rm -rf .cover && mkdir -p .cover
	for p in $$($(GO) list -f '{{if .TestGoFiles}}{{.ImportPath}}{{end}}' ./internal/... github.com/pradita/ac-project/service/core/...); do \
		$(GO) test -count=1 -coverpkg=./internal/...,github.com/pradita/ac-project/service/core/... -coverprofile=.cover/$$(echo $$p | tr / _).out $$p >.cover/last.log 2>&1 || { cat .cover/last.log; exit 1; }; done
	python3 tools/covermerge.py coverage.out .cover/*.out

.PHONY: gen seed test cover migrations resetdb test-fresh
