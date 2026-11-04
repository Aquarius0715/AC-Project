#!/usr/bin/env bash
# Deploys the shared test environment from git tags (DEC-93). The lab server runs it every minute from a systemd timer
# (deploy/test/setup.sh installs both, with the environment file ~/ac-test/env).
#
# A new tag test<major>.<minor>.<patch>r<n> on a commit of main whose CI passed is checked out, built one image at a
# time with the tag as the image tag (the images of the last KEEP deployed tags stay), and started as the compose
# project ac-test: compose.yaml with deploy/test/compose.test.yaml on top. The newest such tag wins, so going back is a
# new tag on the older commit. The data stays from one deployment to the next, except when the migrations changed:
# version 1 is edited in place before the first release and never applied again to a database that has it, so the
# data is then built again from the seed. A deployment that fails leaves the one before running, and a tag is tried
# ATTEMPTS times at most. After a reboot it starts the stack and restarts the API services once Keycloak answers: they
# load its token keys at start (IR419). With GITHUB_DEPLOY_TOKEN in the environment file, each tag is a deployment of
# the environment "test" on GitHub, with its status at each step (deploy/test/github.py, IR630). A tag that changes
# these scripts is deployed by its own scripts.
#
#   deploy.sh              one check (what the timer runs)
#   deploy.sh status       the deployed tag, the last failures and the containers
#   deploy.sh reset        the data again from the seed, on the deployed tag
#   deploy.sh deploy TAG   that tag now, without the main and CI checks (for the person at the server)
#
# Everything runs inside main, which bash reads whole before it starts: a checkout that changes this file does not
# change the run in progress. The steps check their own results (|| return 1): bash ignores set -e inside a function
# called as a condition.
set -euo pipefail

ROOT=${AC_TEST_ROOT:-$HOME/ac-test}
REPO=$ROOT/repo
SETTINGS=$ROOT/env
STATE=$ROOT/state
PROJECT=ac-test
TAG_RE='^test[0-9]+[.][0-9]+[.][0-9]+r[0-9]+$'
KEEP=3
ATTEMPTS=3
API_SERVICES=(gateway identity-api equipment-api maintenance-api billing-api energy-api)
MIGRATIONS=service/api/internal/migrations

log() { printf '%s %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*"; }

state() { cat "$STATE/$1" 2>/dev/null || true; }

boot_id() { cat /proc/sys/kernel/random/boot_id 2>/dev/null || echo unknown; }

# one value of the environment file, without its quotes; the file is never sourced (the broker password is the
# person's own and may hold any character)
setting() { sed -n "s/^$1=//p" "$SETTINGS" | tail -n 1 | sed -e "s/^'\(.*\)'$/\1/" -e 's/^"\(.*\)"$/\1/'; }

# compose for the tag $1, on the checked-out tree
compose() {
  local tag=$1
  shift
  IMAGE_TAG=$tag docker compose -p "$PROJECT" --env-file "$SETTINGS" \
    -f "$REPO/compose.yaml" -f "$REPO/deploy/test/compose.test.yaml" --profile full --profile iot "$@"
}

checkout() { git -C "$REPO" checkout --quiet --force --detach "refs/tags/$1"; }

# owner/name of the repository on GitHub (GITHUB_REPOSITORY overrides it, and GITHUB_API_URL the API, for tests)
slug() {
  if [ -n "${GITHUB_REPOSITORY:-}" ]; then
    echo "$GITHUB_REPOSITORY"
  else
    git -C "$REPO" remote get-url origin | sed -E 's#^(https://github\.com/|git@github\.com:)##; s#\.git$##'
  fi
}

# the tag $1's deployment status $2 on GitHub, described by $3, when the environment file has GITHUB_DEPLOY_TOKEN and
# the checked-out tree has the reporter; a report that fails is logged and never stops the deployment
report() {
  if [ -z "$(setting GITHUB_DEPLOY_TOKEN)" ] || [ ! -f "$REPO/deploy/test/github.py" ]; then
    return 0
  fi
  python3 "$REPO/deploy/test/github.py" "$SETTINGS" "$(slug)" "$STATE" "$@" 2>/dev/null ||
    log "GitHub: the status $2 of $1 was not sent"
}

# the CI workflow's result for the commit $1 on GitHub's public API: success, pending (not finished, or the API out of
# reach) or the conclusion that failed
ci_result() {
  curl -fsS --max-time 20 -H 'Accept: application/vnd.github+json' \
    "${GITHUB_API_URL:-https://api.github.com}/repos/$(slug)/actions/runs?head_sha=$1&per_page=50" 2>/dev/null |
    python3 -c '
import json, sys
runs = [r for r in json.load(sys.stdin).get("workflow_runs", []) if r.get("name") == "CI"]
if any(r.get("conclusion") == "success" for r in runs):
    print("success")
elif runs and all(r.get("status") == "completed" for r in runs):
    print(runs[0].get("conclusion") or "failure")
else:
    print("pending")
' 2>/dev/null || echo pending
}

# the apps and the identity provider answer on the addresses people open, within $1 seconds
ready() {
  local deadline=$((SECONDS + $1)) url
  for url in "$(setting KEYCLOAK_URL)/realms/ac/.well-known/openid-configuration" \
    "$(setting CUSTOMER_WEB_URL)/login" "$(setting PARTNER_WEB_URL)/login" \
    "$(setting TECHNICIAN_WEB_URL)/login" "$(setting ADMIN_WEB_URL)/login"; do
    until [ "$(curl -s -o /dev/null --max-time 10 -w '%{http_code}' "$url")" = 200 ]; do
      if [ "$SECONDS" -ge "$deadline" ]; then
        log "not ready: $url"
        return 1
      fi
      sleep 5
    done
  done
}

# the images of the tag, one at a time: the server serves other projects too, and the four Next.js builds at once need
# several GB
build() {
  local tag=$1 services service
  services=$(compose "$tag" config --format json | python3 -c '
import json, sys
seen = set()
for name, service in sorted(json.load(sys.stdin)["services"].items()):
    if "build" in service and service.get("image") not in seen:
        seen.add(service.get("image"))
        print(name)
') || return 1
  local total i=0
  total=$(wc -w <<<"$services" | tr -d ' ')
  for service in $services; do
    i=$((i + 1))
    step "$tag" "building the images ($i/$total)"
    compose "$tag" build --quiet "$service" || return 1
  done
}

# the realm with this environment's URLs and client secrets, and the stack's own broker logins
prepare() {
  local keys result=0
  python3 "$REPO/deploy/test/realm.py" "$REPO/docker/keycloak/realm-ac.json" "$ROOT/realm-ac.json" "$SETTINGS" ||
    return 1
  keys=$(mktemp) || return 1
  grep -E '^MQTT_(DEVICE_SIM|IOT_BRIDGE)_KEY=' "$SETTINGS" >"$keys" &&
    ENV_FILE=$keys sh "$REPO/docker/mosquitto/passwd.sh" >/dev/null || result=1
  rm -f "$keys"
  return "$result"
}

# the step of the tag $1's deployment now ($2): in the log, on GitHub, and in a failure's description
STEP=
step() {
  STEP=$2
  report "$1" in_progress "$2"
}

# checks out the tag $1, builds and starts it; $2 is 1 when the data is built again from the seed
deploy() {
  local tag=$1 fresh=$2
  step "$tag" "checking out"
  checkout "$tag" || return 1
  step "$tag" "writing the realm and the broker logins"
  prepare || return 1
  build "$tag" || return 1
  if [ "$fresh" = 1 ]; then
    step "$tag" "removing the data (the migrations changed)"
    compose "$tag" down --volumes --remove-orphans || return 1
  fi
  step "$tag" "starting the stack"
  compose "$tag" up -d --remove-orphans || return 1
  step "$tag" "waiting for the apps and Keycloak"
  ready 900 || return 1
  step "$tag" "loading the mock fleet's history"
  compose "$tag" run --rm mock-history >/dev/null || return 1 # the mock fleet's 30 days up to the scenario clock
}

# keeps the images of the last KEEP deployed tags, for a quick way back
prune() {
  local keep image
  printf '%s\n' "$1" >>"$STATE/history"
  keep=$(tail -n "$KEEP" "$STATE/history")
  for image in $(docker image ls --format '{{.Repository}}:{{.Tag}}'); do
    case $image in
    ac-*:test*) grep -qxF "${image##*:}" <<<"$keep" || docker image rm "$image" >/dev/null 2>&1 || true ;;
    esac
  done
}

status() {
  echo "deployed: $(state deployed) (since $(stat -c %y "$STATE/deployed" 2>/dev/null || echo never))"
  if [ -n "$(setting GITHUB_DEPLOY_TOKEN)" ]; then
    echo "GitHub: each deployment shows under the repository's Deployments (environment test)"
  else
    echo "GitHub: not reported (no GITHUB_DEPLOY_TOKEN in $SETTINGS)"
  fi
  if [ -s "$STATE/failures" ]; then
    echo "failed tries: $(tail -n 5 "$STATE/failures" | tr '\n' ' ')"
  fi
  if [ -n "$(state deployed)" ]; then
    compose "$(state deployed)" ps --format 'table {{.Service}}\t{{.Status}}'
  fi
}

# after a reboot: the stack again, then the API services once Keycloak answers
after_boot() {
  local deployed boot
  deployed=$(state deployed)
  boot=$(boot_id)
  if [ -z "$deployed" ] || [ "$(state boot)" = "$boot" ]; then
    return 0
  fi
  log "first check since boot: starting $deployed"
  checkout "$deployed" || return 1
  compose "$deployed" up -d --remove-orphans || return 1
  ready 900 || return 1
  compose "$deployed" restart "${API_SERVICES[@]}" || return 1
  printf '%s\n' "$boot" >"$STATE/boot"
}

# one check: the newest test tag, when it is new, on main and through CI
check() {
  local tag sha tries result
  git -C "$REPO" fetch --quiet --force --prune --prune-tags --tags origin
  tag=$(git -C "$REPO" tag -l 'test*' --sort=-v:refname | grep -E "$TAG_RE" | head -n 1 || true)
  if [ -z "$tag" ] || [ "$tag" = "$(state deployed)" ]; then
    return 0
  fi
  tries=$(grep -cxF "$tag" "$STATE/failures" 2>/dev/null || true)
  if [ "${tries:-0}" -ge "$ATTEMPTS" ]; then
    return 0
  fi
  sha=$(git -C "$REPO" rev-parse "refs/tags/$tag^{commit}")
  if ! git -C "$REPO" merge-base --is-ancestor "$sha" origin/main; then
    log "$tag: ${sha:0:8} is not on main; not deployed"
    report "$tag" error "not deployed: ${sha:0:8} is not on main"
    printf '%s\n' "$tag" "$tag" "$tag" >>"$STATE/failures"
    return 1
  fi
  result=$(ci_result "$sha")
  if [ "$result" = pending ]; then
    if [ "$(state waiting)" != "$tag" ]; then
      log "$tag: waiting for CI on ${sha:0:8}"
      report "$tag" queued "waiting for CI on ${sha:0:8}"
      printf '%s\n' "$tag" >"$STATE/waiting"
    fi
    return 0
  fi
  if [ "$result" != success ]; then
    log "$tag: CI on ${sha:0:8} ended $result; not deployed"
    report "$tag" error "not deployed: CI on ${sha:0:8} ended $result"
    printf '%s\n' "$tag" "$tag" "$tag" >>"$STATE/failures"
    return 1
  fi
  # a tag that changes these scripts is deployed by its own (the new script reads main whole, so this one is replaced)
  if [ -z "${AC_TEST_REEXEC:-}" ] && ! git -C "$REPO" diff --quiet HEAD "refs/tags/$tag" -- deploy/test; then
    log "$tag: going on with its own deploy scripts"
    checkout "$tag" || return 1
    AC_TEST_REEXEC=1 exec bash "$REPO/deploy/test/deploy.sh"
  fi
  release "$tag" "$sha"
}

# deploys the tag $1 (commit $2): from the seed again when the migrations changed since the deployed tag; back to that
# tag when it fails
release() {
  local tag=$1 sha=$2 tries schema fresh=0 previous
  tries=$(grep -cxF "$tag" "$STATE/failures" 2>/dev/null || true)
  schema=$(git -C "$REPO" rev-parse "refs/tags/$tag:$MIGRATIONS")
  previous=$(state deployed)
  if [ -n "$previous" ] && [ "$(state schema)" != "$schema" ]; then
    fresh=1
    log "$tag: the migrations changed since $previous; the data is built again from the seed"
  fi
  log "deploying $tag (${sha:0:8})"
  if deploy "$tag" "$fresh"; then # each step returns its own failure
    printf '%s\n' "$tag" >"$STATE/deployed"
    printf '%s\n' "$schema" >"$STATE/schema"
    boot_id >"$STATE/boot"
    prune "$tag"
    log "$tag is live: $(setting CUSTOMER_WEB_URL)"
    report "$tag" success "live"
    return 0
  fi
  printf '%s\n' "$tag" >>"$STATE/failures"
  log "$tag failed while $STEP (try $((${tries:-0} + 1)) of $ATTEMPTS)"
  if [ -n "$previous" ]; then
    report "$tag" failure "failed while $STEP (try $((${tries:-0} + 1)) of $ATTEMPTS); back to $previous"
    log "back to $previous"
    { checkout "$previous" && prepare && compose "$previous" up -d --remove-orphans; } ||
      log "$previous did not start again"
  else
    report "$tag" failure "failed while $STEP (try $((${tries:-0} + 1)) of $ATTEMPTS)"
  fi
  return 1
}

main() {
  install -d -m 700 "$STATE"
  exec 9>"$STATE/lock"
  if ! flock -n 9; then
    return 0 # a deployment is running
  fi
  case "${1:-}" in
  status) status ;;
  reset)
    if [ -z "$(state deployed)" ]; then
      echo "nothing is deployed yet" >&2
      return 1
    fi
    log "$(state deployed): the data again from the seed"
    if ! deploy "$(state deployed)" 1; then # each step returns its own failure
      report "$(state deployed)" failure "the reset failed while $STEP"
      return 1
    fi
    report "$(state deployed)" success "live, the data built again from the seed"
    ;;
  deploy)
    if [ -z "${2:-}" ]; then
      echo "usage: deploy.sh deploy TAG" >&2
      return 2
    fi
    git -C "$REPO" fetch --quiet --force --tags origin
    release "$2" "$(git -C "$REPO" rev-parse "refs/tags/$2^{commit}")"
    ;;
  "")
    after_boot
    check
    ;;
  *)
    echo "usage: deploy.sh [status|reset|deploy TAG]" >&2
    return 2
    ;;
  esac
}

main "$@"
exit
