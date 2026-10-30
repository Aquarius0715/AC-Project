#!/bin/sh
# The fast checks of .github/workflows/ci.yml, to run before a push: web ESLint (no warnings), typecheck and Vitest; Go
# gofmt, vet and staticcheck; generated files current; the design document validators. Each is judged by its exit code; the first failure
# stops the script. The Go DB suites run with `make test-all` in service/api.
#   sh scripts/check.sh
set -eu
cd "$(dirname "$0")/.."
step() { printf '\n== %s\n' "$1"; }

step "web lint";      (cd service/web && npm run lint >/dev/null)
step "web typecheck"; (cd service/web && npm run typecheck >/dev/null)
step "web vitest";    (cd service/web && npm test >/dev/null)   # as CI: each workspace with its own vitest.config (shared: jsdom, 30 s per test)
step "go gofmt";      (cd service/api && test -z "$(gofmt -l .)" || { gofmt -l .; exit 1; })
step "go vet";        (cd service/api && GOTOOLCHAIN=auto go vet ./...)
step "staticcheck";   (cd service/api && GOTOOLCHAIN=go1.26.10 go run honnef.co/go/tools/cmd/staticcheck@v0.8.2 ./...)
gen_paths="service/api service/web/shared/lib docs/02-design/api"
gen_state() { git diff -- $gen_paths | shasum; git status --porcelain -- $gen_paths | shasum; }
step "generated files are current (catalog, routes, contracts, Swagger)"
before=$(gen_state)
(cd service/api && GOTOOLCHAIN=auto go run ./cmd/gen/ops >/dev/null && make swagger >/dev/null 2>&1) || { echo "generation failed: run make swagger in service/api"; exit 1; }
[ "$before" = "$(gen_state)" ] || { echo "regenerated files changed: review and stage them"; git status --short -- $gen_paths; exit 1; }
step "docs";         python3 docs/tools/validate_documents.py >/dev/null && python3 docs/tools/check_review_regressions.py >/dev/null
printf '\nall checks passed\n'
