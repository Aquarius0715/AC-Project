# Backend tasks (backend Go design §10). Integration tests need: docker compose --profile schema up -d postgres db-schema
GO ?= GOTOOLCHAIN=auto go

gen:
	$(GO) run ./cmd/gen/ops

seed:
	$(GO) run ./cmd/seed

test:
	$(GO) vet ./...
	$(GO) test ./...

# resetdb rebuilds the local database from schema.sql and reseeds (tests accumulate rows between runs)
resetdb:
	sh tools/resetdb.sh

test-fresh: resetdb test

cover:
	rm -rf .cover && mkdir -p .cover
	for p in $$($(GO) list -f '{{if .TestGoFiles}}{{.ImportPath}}{{end}}' ./internal/...); do \
		$(GO) test -count=1 -coverpkg=./internal/... -coverprofile=.cover/$$(echo $$p | tr / _).out $$p >.cover/last.log 2>&1 || { cat .cover/last.log; exit 1; }; done
	python3 tools/covermerge.py coverage.out .cover/*.out

.PHONY: gen seed test cover resetdb test-fresh
