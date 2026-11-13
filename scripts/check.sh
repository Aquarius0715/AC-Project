#!/bin/sh
# The fast checks of .github/workflows/ci.yml, to run before a push: web ESLint (no warnings), typecheck and Vitest; Go
# gofmt, vet and golangci-lint; generated files current; the design document validators. Each is judged by its exit code.
# The Go DB suites run with `make test-all` in service/api.
#   sh scripts/check.sh              the independent checks run at the same time (about a third of the time)
#   CHECK_SERIAL=1 sh scripts/check.sh   one after the other, for a machine short of memory (type-aware ESLint is heavy)
#
# Only the generated-files check runs alone, first: it rewrites Go sources and generated TypeScript that the others read.
# The rest are jobs that share nothing they write: one per web workspace (its ESLint, then its typecheck, because both
# run `next typegen` into the app's own .next), one for the E2E and load projects with knip, one for Vitest, one for Go,
# one for the documents. They are the same commands as before, split the way CI's web-lint matrix splits them. A job's
# output goes to its own log, which is printed when it fails. The exit status is 0 only when every job passed; never
# pipe this script into another command, or its status is lost.
# shellcheck disable=SC2086 # gen_paths is a list of paths, split on purpose
set -eu
cd "$(dirname "$0")/.."
step() { printf '\n== %s\n' "$1"; }

gen_paths="service/api service/web/shared/lib docs/02-design/api"
gen_state() { git diff -- $gen_paths | shasum; git status --porcelain -- $gen_paths | shasum; }
step "generated files are current (catalog, routes, contracts, Swagger)"
before=$(gen_state)
(cd service/api && GOTOOLCHAIN=auto go run ./cmd/gen/ops >/dev/null && make swagger >/dev/null 2>&1) || { echo "generation failed: run make swagger in service/api"; exit 1; }
[ "$before" = "$(gen_state)" ] || { echo "regenerated files changed: review and stage them"; git status --short -- $gen_paths; exit 1; }

logs=$(mktemp -d)
trap 'rm -rf "$logs"' EXIT INT TERM
count=0

# job NAME COMMAND: runs COMMAND with sh -c in the background (in the foreground with CHECK_SERIAL=1); its output goes to
# $logs/NAME.log and its exit status to $logs/NAME.rc
job() {
  count=$((count + 1))
  if [ "${CHECK_SERIAL:-}" = 1 ]; then
    step "$1"
    run_job "$1" "$2"
  else
    run_job "$1" "$2" &
  fi
}
run_job() {
  start=$(date +%s)
  if sh -c "$2" >"$logs/$1.log" 2>&1; then rc=0; else rc=$?; fi
  echo "$rc $(($(date +%s) - start))" >"$logs/$1.rc"
}

web="cd service/web"
# the apps' lint and typecheck both run `next typegen` into the app's own .next: one job each, one after the other
for app in customer partner technician admin portal; do
  job "web $app" "$web && npm run lint --workspace=$app && npm run typecheck --workspace=$app"
done
job "web shared" "$web && npm run lint --workspace=shared && npm run typecheck --workspace=shared"
job "web vitest" "$web && npm test"   # as CI: each workspace with its own vitest.config (shared: jsdom, 30 s per test)
job "web e2e, load, knip" "$web && npx eslint --max-warnings=0 -c e2e/eslint.config.mjs e2e && npx eslint --max-warnings=0 -c load/eslint.config.mjs load \
  && npx tsc -p e2e/tsconfig.json && npx tsc -p load/tsconfig.json && npm run knip"
job "go" "cd service/api && { test -z \"\$(gofmt -l .)\" || { gofmt -l .; exit 1; }; } && GOTOOLCHAIN=auto go vet ./... && make lint"
# one command per line: under set -e a failure on the left of && does not stop the script (IR505); the contract check
# uses the workspace's TypeScript, as CI's validator job does
job "docs" "sh docker/mosquitto/passwd.test.sh && python3 docs/tools/validate_documents.py --tsc service/web/node_modules/typescript/lib/tsc.js \
  && python3 docs/tools/check_review_regressions.py"

[ "${CHECK_SERIAL:-}" = 1 ] || { step "waiting for $count jobs"; wait; }

failed=0
printf '\n'
# a job killed before it wrote its status (out of memory, a signal) leaves no .rc file: that is a failure, not a pass
reported=$(find "$logs" -name '*.rc' | wc -l)
if [ "$reported" -ne "$count" ]; then
  printf 'FAILED  %s of %s jobs reported no exit status\n' "$((count - reported))" "$count"
  failed=1
fi
for rc in "$logs"/*.rc; do
  name=$(basename "$rc" .rc)
  read -r status seconds <"$rc"
  if [ "$status" = 0 ]; then
    printf 'ok      %4ss  %s\n' "$seconds" "$name"
  else
    printf 'FAILED  %4ss  %s (exit %s)\n' "$seconds" "$name" "$status"
    failed=1
  fi
done
if [ "$failed" = 1 ]; then
  for rc in "$logs"/*.rc; do
    name=$(basename "$rc" .rc)
    read -r status _ <"$rc"
    [ "$status" = 0 ] || { printf '\n----- %s (last 60 lines)\n' "$name"; tail -n 60 "$logs/$name.log"; }
  done
  exit 1
fi
printf '\nall checks passed\n'
