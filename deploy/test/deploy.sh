#!/usr/bin/env bash
# Deploys the shared test environment (DEC-93, DEC-94). A tag test<major>.<minor>.<patch>r<n> runs the workflow
# .github/workflows/deploy-test.yml on the lab server's self-hosted runner: it checks the tag out, waits until the
# tag's commit is on main and its CI passed (deploy/test/ci_gate.py), and runs "deploy.sh deploy TAG" from that
# checkout, so the build and deploy logs show on the Actions tab.
#
# A deployment builds the tag's images one at a time, with the tag as the image tag (the images of the last KEEP
# deployed tags stay), and starts the compose project ac-test: compose.yaml with deploy/test/compose.test.yaml on top
# and the server's environment file ~/ac-test/env (deploy/test/setup.sh writes it). The data stays from one deployment
# to the next, except when the migrations changed: version 1 is edited in place before the first release and never
# applied again to a database that has it, so the data is then built again from the seed. A deployment that fails
# brings back the tag before it.
#
#   deploy.sh deploy TAG   deploys the tag (the workflow; by hand, from a checkout that has the tag)
#   deploy.sh boot         after a reboot: the stack again, then the API services once Keycloak answers, as they load
#                          its token keys at start (IR419); the unit ac-test-boot runs it
#   deploy.sh status       the deployed tag, the last failures and the containers
#   deploy.sh reset        the data again from the seed, on the deployed tag
#
# The tree is AC_TEST_REPO (the workflow's checkout), else the one the last deployment used. Everything runs inside
# main, which bash reads whole before it starts. The steps check their own results (|| return 1): bash ignores set -e
# inside a function called as a condition.
set -euo pipefail

ROOT=${AC_TEST_ROOT:-$HOME/ac-test}
SETTINGS=$ROOT/env
STATE=$ROOT/state
REPO=${AC_TEST_REPO:-$(cat "$STATE/repo" 2>/dev/null || echo "$ROOT/repo")}
PROJECT=ac-test
KEEP=3
API_SERVICES=(gateway identity-api equipment-api maintenance-api billing-api energy-api)
MIGRATIONS=service/api/internal/migrations

log() { printf '%s %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*"; }

on_actions() { [ "${GITHUB_ACTIONS:-}" = true ]; }

state() { cat "$STATE/$1" 2>/dev/null || true; }

# one value of the environment file, without its quotes; the file is never sourced (the broker password is the
# person's own and may hold any character)
setting() { sed -n "s/^$1=//p" "$SETTINGS" | tail -n 1 | sed -e "s/^'\(.*\)'$/\1/" -e 's/^"\(.*\)"$/\1/'; }

# compose for the tag $1, on the tree
compose() {
  local tag=$1
  shift
  IMAGE_TAG=$tag docker compose -p "$PROJECT" --progress plain --env-file "$SETTINGS" \
    -f "$REPO/compose.yaml" -f "$REPO/deploy/test/compose.test.yaml" --profile full --profile iot "$@"
}

checkout() { git -C "$REPO" checkout --quiet --force --detach "refs/tags/$1"; }

# a step of the deployment: a group of its own on the Actions tab, and the step a failure names
STEP=
step() {
  end_step
  STEP=$1
  if on_actions; then
    echo "::group::$1"
  else
    log "$1"
  fi
}
end_step() {
  if [ -n "$STEP" ] && on_actions; then
    echo "::endgroup::"
  fi
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
    log "ready: $url"
  done
}

# the images of the tag, one at a time: the server serves other projects too, and the four Next.js builds at once need
# several GB
build() {
  local tag=$1 services service total i=0
  services=$(compose "$tag" config --format json | python3 -c '
import json, sys
seen = set()
for name, service in sorted(json.load(sys.stdin)["services"].items()):
    if "build" in service and service.get("image") not in seen:
        seen.add(service.get("image"))
        print(name)
') || return 1
  total=$(wc -w <<<"$services" | tr -d ' ')
  for service in $services; do
    i=$((i + 1))
    step "building the images ($i/$total): $service"
    compose "$tag" build "$service" || return 1
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

# checks out the tag $1, builds and starts it; $2 is 1 when the data is built again from the seed
deploy() {
  local tag=$1 fresh=$2
  step "checking out $tag"
  checkout "$tag" || return 1
  step "writing the realm and the broker logins"
  prepare || return 1
  build "$tag" || return 1
  if [ "$fresh" = 1 ]; then
    step "removing the data (the migrations changed)"
    compose "$tag" down --volumes --remove-orphans || return 1
  fi
  step "starting the stack"
  compose "$tag" up -d --remove-orphans || return 1
  step "waiting for the apps and Keycloak"
  ready 900 || return 1
  step "loading the mock fleet's history"
  compose "$tag" run --rm mock-history || return 1 # the mock fleet's 30 days up to the scenario clock
  end_step
  STEP=
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

# deploys the tag $1: from the seed again when the migrations changed since the deployed tag; back to that tag when it
# fails. A deployment that works leaves the tree and this script for the boot unit.
release() {
  local tag=$1 schema fresh=0 previous
  if ! git -C "$REPO" rev-parse --quiet --verify "refs/tags/$tag" >/dev/null; then
    log "$tag is not a tag of $REPO"
    return 1
  fi
  schema=$(git -C "$REPO" rev-parse "refs/tags/$tag:$MIGRATIONS")
  previous=$(state deployed)
  if [ -n "$previous" ] && [ "$(state schema)" != "$schema" ]; then
    fresh=1
    log "$tag: the migrations changed since $previous; the data is built again from the seed"
  fi
  log "deploying $tag ($(git -C "$REPO" rev-parse --short "refs/tags/$tag^{commit}")) after ${previous:-nothing}"
  if deploy "$tag" "$fresh"; then # each step returns its own failure
    printf '%s\n' "$tag" >"$STATE/deployed"
    printf '%s\n' "$schema" >"$STATE/schema"
    printf '%s\n' "$REPO" >"$STATE/repo"
    install -m 700 "$REPO/deploy/test/deploy.sh" "$ROOT/deploy.sh"
    prune "$tag"
    log "$tag is live: $(setting CUSTOMER_WEB_URL)"
    return 0
  fi
  end_step
  printf '%s\n' "$tag" >>"$STATE/failures"
  if on_actions; then
    echo "::error::$tag failed while $STEP${previous:+; back to $previous}"
  fi
  log "$tag failed while $STEP"
  if [ -n "$previous" ]; then
    log "back to $previous"
    { checkout "$previous" && prepare && compose "$previous" up -d --remove-orphans; } ||
      log "$previous did not start again"
  fi
  return 1
}

# after a reboot: the stack again, then the API services once Keycloak answers
boot() {
  local deployed
  deployed=$(state deployed)
  if [ -z "$deployed" ]; then
    return 0
  fi
  log "after the boot: starting $deployed"
  checkout "$deployed" || return 1
  compose "$deployed" up -d --remove-orphans || return 1
  ready 900 || return 1
  compose "$deployed" restart "${API_SERVICES[@]}" || return 1
}

status() {
  echo "deployed: $(state deployed) (since $(stat -c %y "$STATE/deployed" 2>/dev/null || echo never)), from $REPO"
  if [ -s "$STATE/failures" ]; then
    echo "failed: $(tail -n 5 "$STATE/failures" | tr '\n' ' ')"
  fi
  if [ -n "$(state deployed)" ]; then
    compose "$(state deployed)" ps --format 'table {{.Service}}\t{{.Status}}'
  fi
}

main() {
  install -d -m 700 "$STATE"
  exec 9>"$STATE/lock"
  if ! flock -w 7200 9; then # one deployment at a time; the boot unit waits for one in progress
    log "another deployment held the lock for two hours"
    return 1
  fi
  case "${1:-}" in
  deploy)
    if [ -z "${2:-}" ]; then
      echo "usage: deploy.sh deploy TAG" >&2
      return 2
    fi
    git -C "$REPO" fetch --quiet --force --tags origin 2>/dev/null || true # the workflow's checkout has them already
    release "$2"
    ;;
  boot) boot ;;
  status) status ;;
  reset)
    if [ -z "$(state deployed)" ]; then
      echo "nothing is deployed yet" >&2
      return 1
    fi
    log "$(state deployed): the data again from the seed"
    deploy "$(state deployed)" 1
    ;;
  *)
    echo "usage: deploy.sh deploy TAG | boot | status | reset" >&2
    return 2
    ;;
  esac
}

main "$@"
exit
