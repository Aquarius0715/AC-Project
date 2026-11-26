#!/usr/bin/env bash
# Deploys the shared test environment (DEC-93, DEC-94). A tag test<major>.<minor>.<patch>r<n> runs the workflow
# .github/workflows/deploy-test.yml on the lab server's self-hosted runner: it checks the tag out, waits until the
# tag's commit is on main and its CI passed (deploy/test/ci_gate.py), and runs "deploy.sh deploy TAG" from that
# checkout, so the build and deploy logs show on the Actions tab.
#
# A deployment builds the tag's images one at a time, with the tag as the image tag (the images of the last KEEP
# deployed tags stay; untagged images go and the build cache is trimmed to CACHE_LIMIT), and starts the compose
# project ac-test: compose.yaml with deploy/test/compose.test.yaml and deploy/test/compose.live.yaml on top and the
# server's environment file ~/ac-test/env (deploy/test/setup.sh writes it). The data are the fixture's accounts and
# the real devices at customer-a, on the real clock (DEC-96). They stay from one deployment to the next: a new
# migration is applied to them by the stack's migrate service (ALTERs, IR785), and the data is built again from the
# seed only when a migration the deployed tag had was edited or removed, or compose.live.yaml changed. A deployment
# that fails brings back the tag before it (a migration it applied stays: they only add).
#
# Without downtime (DEC-111, IR855): after a tag with deploy/test/compose.rolling.yaml, a deployment starts the
# services that are not replaced one by one (the stores, Keycloak, migrate, the workers, the IoT bridge), then replaces
# the API services and the apps one container at a time in ROLLING's order: the new container starts next to the old
# one, the old one stops once the new one is healthy, and the apps' ports stay open on web-edge. The whole stack
# restarts instead when the data is built again from the seed, when a new migration says it breaks the code before it
# (-- migrate:breaking), and at the first deployment with the file.
#
#   deploy.sh deploy TAG   deploys the tag (the workflow; by hand, from a checkout that has the tag)
#   deploy.sh boot         after a reboot: the realm and the broker logins again, the stack, then the API services
#                          once Keycloak answers, as they load its token keys at start (IR419); the unit ac-test-boot
#                          runs it
#   deploy.sh status       the deployed tag, the last failures and the containers
#   deploy.sh reset        the data again from the seed, on the deployed tag
#
# The tree is AC_TEST_REPO (the workflow's checkout), else the one the last deployment used. Everything runs inside
# main, which bash reads whole before it starts. The steps check their own results (|| return 1): bash ignores set -e
# inside a function called as a condition.
set -euo pipefail

ROOT=${AC_TEST_ROOT:-$HOME/ac-test}
SETTINGS=${AC_TEST_ENV:-$ROOT/env} # the workflow passes the Environment test's file (IR733); the boot unit and by hand: the server's
STATE=$ROOT/state
REPO=${AC_TEST_REPO:-$(cat "$STATE/repo" 2>/dev/null || echo "$ROOT/repo")}
PROJECT=ac-test
KEEP=3
# the build cache BuildKit may keep after a deployment (IR725): unbounded, it filled the lab server's disk with the
# nightly E2E builds; 20 GB keeps the layers of the next builds warm
CACHE_LIMIT=20GB
API_SERVICES=(gateway identity-api equipment-api maintenance-api billing-api energy-api)
MIGRATIONS=service/api/internal/migrations
LIVE=deploy/test/compose.live.yaml # this environment's data (DEC-96); a tag before it has none
LEFT_OUT=mock-fleet                # the profile compose.live.yaml moves the mock fleet's services to
MULTIHOST=deploy/test/compose.multihost.yaml # more than one address (IR651); a tag before it has none
STRIPE=stripe                      # the profile of the Stripe CLI, which forwards test-mode webhooks to the gateway
ROLLING_FILE=deploy/test/compose.rolling.yaml # replacing one container at a time (IR855); a tag before it has none
EDGE=$ROOT/web-edge                # web-edge's nginx configuration, outside the checkout (its path changes with the runner)
# the stack's broker logins (docker/mosquitto/passwd.sh), outside the checkout: the tree is a runner's workspace, and a
# CI job checking it out again put the committed file back, which the broker read at its next start and then refused
# the bridge (2026-11-24)
BROKER_LOGINS=$ROOT/mosquitto
# replaced one container at a time, in this order: the domain services before the gateway that calls them, the gateway
# before the apps; the apps are behind web-edge
ROLLING=(identity-api equipment-api maintenance-api billing-api energy-api gateway
  customer-web-api partner-web-api technician-web-api admin-web-api portal-web)
WEB_APPS=(customer-web-api partner-web-api technician-web-api admin-web-api portal-web)
# the services web-edge sends requests to: the apps, and the gateway for the iOS apps (/v1/ only, DEC-112)
EDGE_SERVICES=("${WEB_APPS[@]}" gateway)
BREAKING_MARKER='-- migrate:breaking' # a migration the code before it cannot run on (internal/migrate, IR855)

log() { printf '%s %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*"; }

on_actions() { [ "${GITHUB_ACTIONS:-}" = true ]; }

state() { cat "$STATE/$1" 2>/dev/null || true; }

# one value of the environment file, without its quotes; the file is never sourced (the broker password is the
# person's own and may hold any character)
setting() { sed -n "s/^$1=//p" "$SETTINGS" | tail -n 1 | sed -e "s/^'\(.*\)'$/\1/" -e 's/^"\(.*\)"$/\1/'; }

# the issuers the APIs accept when Keycloak answers on several addresses: the tokens of a sign-in name the address it
# started on, so Keycloak's own URL and the same URL on each host of PUBLIC_HOSTS (IR651)
issuers() {
  local url list host
  url=$(setting KEYCLOAK_URL)
  list="$url/realms/ac"
  for host in $(setting PUBLIC_HOSTS | tr ',' ' '); do
    list+=",$(printf '%s' "$url" | sed -E "s#^(https?://)[^:/]+#\1$host#")/realms/ac"
  done
  printf '%s' "$list"
}

# Stripe test mode (DEC-68, IR828): on when the environment file sets STRIPE_SECRET_KEY. The lab server is not reachable
# from the internet, so Stripe's events come through the Stripe CLI of the stripe profile, which forwards them to the
# gateway; the billing service then talks to api.stripe.com, and the customer app's Pay opens Stripe Checkout, unless
# the file sets STRIPE_API_BASE or PAYMENT_PROVIDER itself.
stripe_on() { [ -n "$(setting STRIPE_SECRET_KEY)" ]; }

# a key without its webhook secret would take payments nothing confirms: billing refuses to start, so say why first
stripe_settings() {
  if stripe_on && [ -z "$(setting STRIPE_WEBHOOK_SECRET)" ]; then
    log "STRIPE_SECRET_KEY is set without STRIPE_WEBHOOK_SECRET: take it from the Stripe CLI (listen --print-secret)"
    return 1
  fi
}

# compose for the tag $1, on the tree, with this environment's data on the real clock. When the environment file lists
# PUBLIC_HOSTS the stack answers on each of those addresses (IR651): Keycloak takes its host from the request behind an
# nginx that checks it, and the APIs accept the tokens of a sign-in on any of those addresses.
compose() {
  local tag=$1 files=(-f "$REPO/compose.yaml" -f "$REPO/deploy/test/compose.test.yaml")
  local env=(IMAGE_TAG="$tag" DEMO_CLOCK_START=now WEB_EDGE_DIR="$EDGE" MOSQUITTO_PASSWD_DIR="$BROKER_LOGINS")
  local profiles=(--profile full --profile iot)
  shift
  if stripe_on; then
    profiles+=(--profile "$STRIPE")
    [ -n "$(setting STRIPE_API_BASE)" ] || env+=(STRIPE_API_BASE=https://api.stripe.com)
    [ -n "$(setting PAYMENT_PROVIDER)" ] || env+=(PAYMENT_PROVIDER=stripe)
  fi
  if [ -f "$REPO/$LIVE" ]; then
    files+=(-f "$REPO/$LIVE")
  fi
  if [ -n "$(setting PUBLIC_HOSTS)" ] && [ -f "$REPO/$MULTIHOST" ]; then
    files+=(-f "$REPO/$MULTIHOST")
    env+=(API_OIDC_ISSUER="$(issuers)")
  fi
  if [ -f "$REPO/$ROLLING_FILE" ]; then
    files+=(-f "$REPO/$ROLLING_FILE")
  fi
  env "${env[@]}" docker compose -p "$PROJECT" --progress plain --env-file "$SETTINGS" \
    "${files[@]}" "${profiles[@]}" "$@"
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

# the portal's address (IR791): the environment file's, or, in a file from before the portal, the customer app's host
# on PORTAL_WEB_PORT (3204 by default, as compose.test.yaml and compose.rolling.yaml publish it; IR857)
portal_url() {
  local url port
  url=$(setting PORTAL_WEB_URL)
  if [ -z "$url" ]; then
    port=$(setting PORTAL_WEB_PORT)
    url=$(setting CUSTOMER_WEB_URL | sed -E "s#^(https?://[^:/]+)(:[0-9]+)?#\1:${port:-3204}#")
  fi
  printf '%s' "$url"
}

# the apps and the identity provider answer on the addresses people open, within $1 seconds
ready() {
  local deadline=$((SECONDS + $1)) url
  for url in "$(setting KEYCLOAK_URL)/realms/ac/.well-known/openid-configuration" \
    "$(setting CUSTOMER_WEB_URL)/login" "$(setting PARTNER_WEB_URL)/login" \
    "$(setting TECHNICIAN_WEB_URL)/login" "$(setting ADMIN_WEB_URL)/login" "$(portal_url)"; do
    # /login of an app answers 307 to the identity provider (IR790): up is any answer below 400
    until code=$(curl -s -o /dev/null --max-time 10 -w '%{http_code}' "$url") && [ "$code" -ge 200 ] && [ "$code" -lt 400 ]; do
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

# the realm with this environment's URLs and client secrets, and the stack's own broker logins in BROKER_LOGINS (a tag
# before it writes them into the tree, where its compose.yaml mounts them from). LOGINS_CHANGED is 1 when the logins
# were written again: the running broker read the file before (broker_logins).
LOGINS_CHANGED=0
prepare() {
  local keys result=0 before
  python3 "$REPO/deploy/test/realm.py" "$REPO/docker/keycloak/realm-ac.json" "$ROOT/realm-ac.json" "$SETTINGS" ||
    return 1
  before=$(cat "$BROKER_LOGINS/passwd" 2>/dev/null || true)
  keys=$(mktemp) || return 1
  grep -E '^MQTT_(DEVICE_SIM|IOT_BRIDGE)_KEY=' "$SETTINGS" >"$keys" &&
    ENV_FILE=$keys MOSQUITTO_PASSWD_DIR=$BROKER_LOGINS sh "$REPO/docker/mosquitto/passwd.sh" >/dev/null || result=1
  rm -f "$keys"
  if [ "$(cat "$BROKER_LOGINS/passwd" 2>/dev/null || true)" != "$before" ]; then
    LOGINS_CHANGED=1
  fi
  return "$result"
}

# the broker restarts when prepare wrote its logins again: it reads them only at its start, and Docker started it at
# the boot before this script, or a deployment found it running; the bridge and the simulated modules connect again
broker_logins() {
  local tag=$1
  [ "$LOGINS_CHANGED" = 1 ] || return 0
  [ -n "$(containers mosquitto)" ] || return 0
  log "the broker logins changed: restarting the broker"
  compose "$tag" restart mosquitto || return 1
  LOGINS_CHANGED=0
}

# removes the containers of the services this environment leaves out (the mock fleet's, and the Stripe CLI when Stripe
# is off): compose keeps a container whose service left the active profiles, even when it removes orphans
leave_out() {
  local tag=$1 all active service out=()
  all=$(compose "$tag" --profile "$LEFT_OUT" --profile "$STRIPE" config --services) || return 1
  active=$(compose "$tag" config --services) || return 1
  for service in $all; do
    grep -qxF "$service" <<<"$active" || out+=("$service")
  done
  if [ "${#out[@]}" -gt 0 ]; then
    compose "$tag" rm --stop --force "${out[@]}" || return 1
  fi
}

# checks out the tag $1, builds and starts it; $2 is 1 when the data is built again from the seed, $3 is 1 when the
# services are replaced one container at a time
deploy() {
  local tag=$1 fresh=$2 rolling=${3:-0}
  step "checking out $tag"
  stripe_settings || return 1
  checkout "$tag" || return 1
  step "writing the realm and the broker logins"
  prepare || return 1
  build "$tag" || return 1
  step "removing the services this environment leaves out"
  leave_out "$tag" || return 1
  if [ "$fresh" = 1 ]; then
    step "removing the data"
    compose "$tag" down --volumes --remove-orphans || return 1
  fi
  start "$tag" "$rolling" || return 1
  broker_logins "$tag" || return 1
  step "waiting for the apps and Keycloak"
  ready 900 || return 1
  end_step
  STEP=
}

# starts the tag $1 from its checkout: the whole stack at once, or, when $2 is 1, the services that are not replaced
# and then each of ROLLING one container at a time (IR855)
start() {
  local tag=$1 rolling=$2 others
  edge_files "$tag" || return 1
  if [ "$rolling" != 1 ]; then
    step "starting the stack"
    compose "$tag" up -d --remove-orphans || return 1
    edge_reload || return 1
    return 0
  fi
  others=$(compose "$tag" config --services | grep -vxF -f <(printf '%s\n' "${ROLLING[@]}")) || return 1
  step "starting the services that are not replaced one by one (the stores, migrate, the workers)"
  # --no-deps: the replaced services stay as they are; the started ones still wait for each other (migrate first)
  # shellcheck disable=SC2086 # one word per service
  compose "$tag" up -d --remove-orphans --no-deps $others || return 1
  edge_reload || return 1
  local service
  for service in "${ROLLING[@]}"; do
    step "replacing $service"
    roll "$tag" "$service" || return 1
  done
}

# the containers of the service $1, by ID
containers() {
  docker ps -aq --no-trunc --filter "label=com.docker.compose.project=$PROJECT" \
    --filter "label=com.docker.compose.service=$1"
}

# whether the service $1 is behind web-edge (an app, or the gateway)
is_app() { grep -qxF -e "$1" <<<"$(printf '%s\n' "${EDGE_SERVICES[@]}")"; }

# web-edge's configuration for the tag $1: its nginx file, and the upstreams by service name. A tag without
# compose.rolling.yaml has no web-edge.
edge_files() {
  [ -f "$REPO/$ROLLING_FILE" ] || return 0
  install -d -m 755 "$EDGE" || return 1
  install -m 644 "$REPO/deploy/test/web-edge.conf" "$EDGE/edge.conf" || return 1
  upstreams
}

# writes the upstreams: each app by its service name, which Docker's DNS gives every container of; with $1 and the
# container IDs $2, the app $1 by those containers alone (the old ones while the new one starts, then the new one while
# the old ones stop), so no request reaches a container that is not ready or about to stop
upstreams() {
  local app id servers port
  for app in "${EDGE_SERVICES[@]}"; do
    port=3000
    [ "$app" = gateway ] && port=8080
    servers="server $app:$port resolve; "
    if [ "$app" = "${1:-}" ]; then
      servers=
      for id in $2; do
        servers+="server $(docker inspect -f '{{.Name}}' "$id" | sed 's#^/##'):$port resolve; "
      done
    fi
    printf 'upstream %s-pool { zone %s-pool 64k; %s}\n' "$app" "$app" "$servers"
  done >"$EDGE/00-upstreams.conf.new" || return 1
  mv "$EDGE/00-upstreams.conf.new" "$EDGE/00-upstreams.conf"
}

# web-edge reads its configuration again; the requests in flight finish on the old worker processes, which this waits
# for (up to a minute), so that the containers they reach may stop. Nothing to do without web-edge.
edge_reload() {
  local id deadline=$((SECONDS + 60))
  id=$(containers web-edge | head -n 1)
  [ -n "$id" ] || return 0
  docker exec "$id" nginx -t -q || return 1
  docker exec "$id" nginx -s reload || return 1
  sleep 1
  # grep reads all of ps (no -q): under pipefail an early exit would fail ps on SIGPIPE
  while docker exec "$id" ps | grep -F 'worker process is shutting down' >/dev/null; do
    if [ "$SECONDS" -ge "$deadline" ]; then
      log "web-edge: old worker processes still serve after a minute; going on"
      return 0
    fi
    sleep 1
  done
}

# whether every container of $1 (IDs, one per line) is healthy within $2 seconds
healthy() {
  local deadline=$((SECONDS + $2)) id state
  for id in $1; do
    until state=$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$id") &&
      { [ "$state" = healthy ] || [ "$state" = running ]; }; do
      if [ "$state" = unhealthy ] || [ "$state" = exited ] || [ "$SECONDS" -ge "$deadline" ]; then
        log "not healthy ($state): $(docker inspect -f '{{.Name}}' "$id")"
        docker logs --tail 40 "$id" 2>&1 || true
        return 1
      fi
      sleep 2
    done
  done
}

# replaces the containers of the service $1 for the tag $2 one at a time: the new container starts next to the old
# ones, and they stop when it is healthy (a Go service finishes its requests in flight; an app's are sent to the new
# container first). A service whose containers already run this configuration is left alone (the way back to a tag
# that did not change it); a new container that is not healthy is removed and the old ones keep serving.
roll() {
  local tag=$1 service=$2 want old new count
  want=$(compose "$tag" config --hash "$service" | awk '{ print $2 }') || return 1
  old=$(containers "$service")
  if [ -n "$old" ] && [ "$(xargs docker inspect -f '{{index .Config.Labels "com.docker.compose.config-hash"}}' <<<"$old" |
    sort -u)" = "$want" ]; then
    log "$service: already this configuration"
    return 0
  fi
  count=$(grep -c . <<<"$old")
  if [ -n "$old" ] && is_app "$service"; then
    { upstreams "$service" "$old" && edge_reload; } || return 1
  fi
  compose "$tag" up -d --no-deps --no-recreate --scale "$service=$((count + 1))" "$service" || return 1
  new=$(containers "$service" | grep -vxF -f <(printf '%s\n' "$old"))
  if [ -z "$new" ] || ! healthy "$new" 300; then
    log "$service: the new container is not healthy; the old one keeps serving"
    [ -z "$new" ] || xargs docker rm -f <<<"$new" >/dev/null
    is_app "$service" && upstreams && edge_reload
    return 1
  fi
  if [ -n "$old" ]; then
    if is_app "$service"; then
      { upstreams "$service" "$new" && edge_reload; } || return 1
    fi
    { xargs docker stop -t 30 <<<"$old" >/dev/null && xargs docker rm <<<"$old" >/dev/null; } || return 1
  fi
  if is_app "$service"; then
    { upstreams && edge_reload; } || return 1
  fi
  log "$service: replaced by $(docker inspect -f '{{.Name}}' "$new" | sed 's#^/##')"
}

# whether the tag $2 can follow the deployed tag $1 one container at a time: $1 has compose.rolling.yaml (web-edge holds
# the ports) and no migration $2 adds says it breaks the code before it
rolls_after() {
  local path
  [ -n "$1" ] && git -C "$REPO" rev-parse --quiet --verify "refs/tags/$1:$ROLLING_FILE" >/dev/null || return 1
  while read -r path; do
    case $path in *.sql) ;; *) continue ;; esac
    git -C "$REPO" rev-parse --quiet --verify "refs/tags/$1:$path" >/dev/null && continue
    if grep -qF -e "$BREAKING_MARKER" <<<"$(git -C "$REPO" show "refs/tags/$2:$path")"; then
      log "$path says it breaks the code before it: the whole stack restarts"
      return 1
    fi
  done < <(git -C "$REPO" ls-tree -r --name-only "refs/tags/$2" -- "$MIGRATIONS")
  return 0
}

# keeps the images of the last KEEP deployed tags, for a quick way back; then removes the untagged images (the
# replaced builds) and trims the build cache to CACHE_LIMIT (IR725). The host's untagged images that no container uses
# and the build cache go; tagged images, of this project or another, and every volume stay. A clean-up that fails
# does not fail the deployment
prune() {
  local keep image
  printf '%s\n' "$1" >>"$STATE/history"
  keep=$(tail -n "$KEEP" "$STATE/history")
  for image in $(docker image ls --format '{{.Repository}}:{{.Tag}}'); do
    case $image in
    ac-*:test*) grep -qxF "${image##*:}" <<<"$keep" || docker image rm "$image" >/dev/null 2>&1 || true ;;
    esac
  done
  docker image prune -f >/dev/null 2>&1 || true
  docker builder prune -f --max-used-space "$CACHE_LIMIT" >/dev/null 2>&1 || true
}

# this environment's data settings at the tag $1 (compose.live.yaml)
data_of() {
  git -C "$REPO" rev-parse --quiet --verify "refs/tags/$1:$LIVE" || echo none
}

# whether every migration of the tag $1 is, byte for byte, in the tag $2: a new migration is applied to the data, but a
# migration that was applied and then edited or removed cannot be, so the data must be built again (IR785)
migrations_kept() {
  local blob path
  while read -r _ _ blob path; do
    case $path in *.sql) ;; *) continue ;; esac # the Go file next to them is no migration
    [ "$(git -C "$REPO" rev-parse --quiet --verify "refs/tags/$2:$path" || true)" = "$blob" ] || return 1
  done < <(git -C "$REPO" ls-tree -r "refs/tags/$1" -- "$MIGRATIONS")
  return 0
}

# deploys the tag $1: from the seed again when what the data are built from changed since the deployed tag; back to
# that tag when it fails. A deployment that works leaves the tree and this script for the boot unit.
release() {
  local tag=$1 data fresh=0 previous rolling=0 back=0
  if ! git -C "$REPO" rev-parse --quiet --verify "refs/tags/$tag" >/dev/null; then
    log "$tag is not a tag of $REPO"
    return 1
  fi
  data=$(data_of "$tag") || return 1
  previous=$(state deployed)
  # state data holds the data settings; a record from before IR785 has the migrations' hash on the line before
  if [ -n "$previous" ] && { [ "$(state data | tail -n 1)" != "$data" ] || ! migrations_kept "$previous" "$tag"; }; then
    fresh=1
    log "$tag: a migration of $previous was changed, or the data settings changed; the data is built again from the seed"
  fi
  if [ "$fresh" = 0 ] && rolls_after "$previous" "$tag"; then
    rolling=1
  fi
  log "deploying $tag ($(git -C "$REPO" rev-parse --short "refs/tags/$tag^{commit}")) after ${previous:-nothing}$(
    [ "$rolling" = 1 ] && printf ', one container at a time' || printf ', the whole stack')"
  if deploy "$tag" "$fresh" "$rolling"; then # each step returns its own failure
    printf '%s\n' "$tag" >"$STATE/deployed"
    printf '%s\n' "$data" >"$STATE/data"
    rm -f "$STATE/schema" # the migrations alone, the record before DEC-96
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
    # one container at a time again when the failed tag ran with web-edge and the tag before it has it too
    [ -f "$REPO/$ROLLING_FILE" ] && git -C "$REPO" rev-parse --quiet --verify "refs/tags/$previous:$ROLLING_FILE" \
      >/dev/null && back=1
    log "back to $previous"
    { checkout "$previous" && prepare && start "$previous" "$back" && broker_logins "$previous"; } ||
      log "$previous did not start again"
    end_step
  fi
  return 1
}

# after a reboot: the realm and the broker logins from the environment file (the tree may have been checked out again
# since the deployment), the stack again, then the API services once Keycloak answers
boot() {
  local deployed
  deployed=$(state deployed)
  if [ -z "$deployed" ]; then
    return 0
  fi
  log "after the boot: starting $deployed"
  checkout "$deployed" || return 1
  prepare || return 1
  edge_files "$deployed" || return 1
  compose "$deployed" up -d --remove-orphans || return 1
  broker_logins "$deployed" || return 1
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
