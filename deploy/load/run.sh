#!/bin/sh
# One load run (docs/02-design/backend-go-design.md, load tests): builds the Go images, starts the throwaway compose
# project ac-load (deploy/load/compose.load.yaml: no published port, its own database ac_load, Keycloak, broker and
# Valkey), fills it with the launch-size data, runs one k6 scenario inside its network, and removes it.
#   sh deploy/load/run.sh smoke|load|stress|web-smoke|web|spike|soak|iot-smoke|iot|iot-design     (the GitHub workflow load.yml runs the same script)
# smoke, load and stress call the Core API through the gateway (phase 1). The web scenarios (phase 2) also start the
# customer, partner and technician web apps (images built here, one at a time) and sign in through them and the identity
# provider; HQ has no web scenario because its sign-in needs a one-time code. TIME_SCALE and VU_SCALE shorten or shrink a
# scenario for trying the script; the report then says it is not a measurement.
# The IoT scenarios (phase 3) also start the bridge that stands in for AWS IoT rules and a generator (cmd/iotload) that plays
# the 3,000 registered devices on the broker of this project (iot: 3,000 devices a minute, 50 batches a second; iot-design: one
# batch per device every 9 seconds, about 333 a second, the design capacity of section 13), while k6 sends commands; the report
# gives the delay from sending a reading to its being stored (probe devices), the completeness, and the command round trip.
# The results are in $LOAD_OUT (default ./load-out): summary.json, k6.log, postgres.csv (connections and CPU sampled while
# k6 ran), compose-logs.txt after a failure. The exit status is k6's: non-zero when a threshold is breached.
# For trying a scenario again: KEEP_STACK=1 leaves the stack (and the settings) up at the end, and REUSE_STACK=1 then runs
# only k6 against it; remove it with `docker compose -p ac-load down --volumes` afterwards.
# It never touches the shared test environment ac-test or the real MQTT broker: the project name is fixed, the images
# are tagged `load` (a development stack's :dev images stay as they are), and REAL_MQTT_* is emptied here.
set -eu
cd "$(dirname "$0")/../.."

scenario=${1:-smoke}
case $scenario in
  smoke | load | stress) web=0 iot=0 ;;
  web-smoke | web | spike | soak) web=1 iot=0 ;;
  iot-smoke) web=0 iot=1 iot_seconds=90 iot_interval=60s ;;
  iot) web=0 iot=1 iot_seconds=600 iot_interval=60s ;;
  iot-design) web=0 iot=1 iot_seconds=600 iot_interval=9s ;;
  *) echo "usage: sh deploy/load/run.sh smoke|load|stress|web-smoke|web|spike|soak|iot-smoke|iot|iot-design" >&2; exit 2 ;;
esac
k6_script=$scenario
if [ "$iot" = 1 ]; then
  k6_script="iot-command"
  # a shortened run (TIME_SCALE) shortens the generator and k6 alike, but not below 30 seconds
  iot_seconds=$(awk -v s="$iot_seconds" -v t="${TIME_SCALE:-1}" 'BEGIN { n = int(s * t); print (n < 30 ? 30 : n) }')
  LOAD_IOT_SECONDS=$iot_seconds
  export LOAD_IOT_SECONDS
fi

PROJECT=ac-load
LOAD_OUT=$(mkdir -p "${LOAD_OUT:-load-out}" && cd "${LOAD_OUT:-load-out}" && pwd)
LOAD_UID=$(id -u)
LOAD_GID=$(id -g)
LOAD_WRITES=$([ "$scenario" = stress ] && echo 0 || echo 1)   # not from the settings file: a reused stack may have been made for another scenario
LOAD_TIME_SCALE=${TIME_SCALE:-1}
LOAD_VU_SCALE=${VU_SCALE:-1}
export LOAD_OUT LOAD_UID LOAD_GID LOAD_WRITES LOAD_TIME_SCALE LOAD_VU_SCALE
unset REAL_MQTT_URL REAL_MQTT_USER REAL_MQTT_PASSWORD REAL_MQTT_COMMANDS REAL_MQTT_TOPIC

rand() { openssl rand -hex "$1"; }
settings=$LOAD_OUT/settings.env
umask 077
[ "${REUSE_STACK:-}" = 1 ] || cat >"$settings" <<EOS
IMAGE_TAG=load
KEYCLOAK_URL=http://keycloak:8080
DEMO_CLOCK_START=now
KEYCLOAK_REALM_FILE=$LOAD_OUT/realm-load.json
MQTT_DEVICE_SIM_KEY=$(rand 12)
MQTT_IOT_BRIDGE_KEY=$(rand 12)
AC_WEB_CLIENT_SECRET=$(rand 16)
AC_ADMIN_WEB_CLIENT_SECRET=$(rand 16)
IDP_ADMIN_CLIENT_SECRET=$(rand 16)
LOAD_CLIENT_SECRET=$(rand 16)
LOAD_PASSWORD=$(rand 12)
LOAD_OUT=$LOAD_OUT
LOAD_UID=$LOAD_UID
LOAD_GID=$LOAD_GID
LOAD_WRITES=$([ "$scenario" = stress ] && echo 0 || echo 1)
CUSTOMER_WEB_URL=http://customer-web-api:3000
PARTNER_WEB_URL=http://partner-web-api:3000
TECHNICIAN_WEB_URL=http://technician-web-api:3000
REAL_MQTT_URL=
EOS
umask 022

compose() { docker compose -p "$PROJECT" --env-file "$settings" -f compose.yaml -f deploy/load/compose.load.yaml "$@"; }
all_profiles() { compose --profile backend --profile full --profile iot --profile load --profile k6 "$@"; }

sampler=
generator=
# shellcheck disable=SC2329 # run by the EXIT, INT and TERM traps below
cleanup() {
  status=$?
  [ -z "$sampler" ] || kill "$sampler" 2>/dev/null || true
  [ -z "$generator" ] || kill "$generator" 2>/dev/null || true
  if [ "$status" -ne 0 ] || [ "${KEEP_LOGS:-}" = 1 ]; then
    all_profiles logs --no-color --tail 300 >"$LOAD_OUT/compose-logs.txt" 2>&1 || true
  fi
  if [ "${KEEP_STACK:-}" != 1 ]; then
    all_profiles down --volumes --remove-orphans >/dev/null 2>&1 || true
    rm -f "$settings" "$LOAD_OUT/realm-load.json"   # the run's secrets go with its stack
  fi
  exit "$status"
}
trap cleanup EXIT INT TERM

if [ "${REUSE_STACK:-}" != 1 ]; then
echo "== remove what an earlier run left"
all_profiles down --volumes --remove-orphans >/dev/null 2>&1 || true

echo "== build the images"
compose --profile backend --profile load build gateway identity-api equipment-api maintenance-api billing-api energy-api migrate worker-outbox loadseed

if [ "$web" = 1 ]; then
  echo "== build the web apps (one at a time: each build needs several GB)"
  for app in customer-web-api partner-web-api technician-web-api; do
    compose --profile full build "$app"
  done
fi

echo "== accounts and realm (the realm is imported when Keycloak first starts)"
compose --profile load run --rm --no-deps -T loadseed /app/loadseed -manifest-only -manifest /out/manifest.json -links /out/links.json   # the password comes from LOAD_PASSWORD in the service's environment
python3 deploy/load/realm.py docker/keycloak/realm-ac.json "$LOAD_OUT/realm-load.json" "$settings" "$LOAD_OUT/manifest.json"

echo "== accounts of the broker (a password file of this run, not docker/mosquitto/passwd of the development stack)"
broker_key() { sed -n "s/^$1=//p" "$settings"; }
# the broker image of compose.yaml, the one Dependabot keeps up to date, so this copy never lags behind it (IR727)
broker_image=$(sed -n 's/^ *image: *\(eclipse-mosquitto:[^ ]*\).*/\1/p' compose.yaml)
[ -n "$broker_image" ] || { echo "no eclipse-mosquitto image in compose.yaml" >&2; exit 1; }
MQTT_IOT_BRIDGE_KEY=$(broker_key MQTT_IOT_BRIDGE_KEY) MQTT_DEVICE_SIM_KEY=$(broker_key MQTT_DEVICE_SIM_KEY) \
  docker run --rm --user "$LOAD_UID:$LOAD_GID" --entrypoint sh -e MQTT_IOT_BRIDGE_KEY -e MQTT_DEVICE_SIM_KEY -v "$LOAD_OUT:/work" \
  "$broker_image" -c '
    set -eu
    rm -f /work/mosquitto-passwd
    mosquitto_passwd -c -b /work/mosquitto-passwd iot-bridge "$MQTT_IOT_BRIDGE_KEY"
    mosquitto_passwd -b /work/mosquitto-passwd device-sim "$MQTT_DEVICE_SIM_KEY"
    chmod 644 /work/mosquitto-passwd'

echo "== start the stack"
# valkey holds the write limit (60 a minute per membership), mosquitto, stripe-mock and weather-mock are what the services
# call; none is a dependency of the gateway, so they are named here. The limit stays on.
web_apps=
[ "$web" = 0 ] || web_apps="customer-web-api partner-web-api technician-web-api"
[ "$iot" = 0 ] || web_apps="$web_apps iot-bridge"
# shellcheck disable=SC2086 # the app names are separate words, or none
compose --profile backend --profile full --profile iot up -d --wait --wait-timeout 600 valkey mosquitto stripe-mock weather-mock gateway worker-outbox worker-scheduler-equipment worker-scheduler-maintenance \
  worker-scheduler-billing worker-iot worker-telemetry worker-rollup worker-automation worker-notification worker-importexport $web_apps

echo "== isolation: nothing of this project is published on the host, and no real broker is named"
published=$(docker ps --filter "label=com.docker.compose.project=$PROJECT" --format '{{.Names}} {{.Ports}}' | grep -- '->' || true)
[ -z "$published" ] || { echo "published ports:"; echo "$published"; exit 1; }
real=$(docker ps -q --filter "label=com.docker.compose.project=$PROJECT" | xargs docker inspect --format '{{.Name}} {{range .Config.Env}}{{println .}}{{end}}' | grep -E 'REAL_MQTT_(URL|USER|PASSWORD)=.' || true)
[ -z "$real" ] || { echo "a container names the real broker:"; echo "$real"; exit 1; }

echo "== fill the database ac_load with the launch-size data"
compose --profile load run --rm --no-deps -T loadseed
fi

echo "== sample connections and CPU while k6 runs"
(
  echo "time,pg_connections,pg_active,cpu_and_memory_by_container" >"$LOAD_OUT/postgres.csv"
  while true; do
    conns=$(compose exec -T postgres psql -U postgres -d ac_load -tA -F, -c "select count(*), count(*) filter (where state = 'active') from pg_stat_activity where datname = 'ac_load'" 2>/dev/null || echo ",")
    ids=$(docker ps -q --filter "label=com.docker.compose.project=$PROJECT")
    cpu=
    if [ -n "$ids" ]; then
      # shellcheck disable=SC2086 # the container IDs are separate words
      cpu=$(docker stats --no-stream --format '{{.Name}}={{.CPUPerc}}/{{.MemUsage}}' $ids 2>/dev/null | tr '\n' ' ' || true)
    fi
    echo "$(date +%s),$conns,$cpu" >>"$LOAD_OUT/postgres.csv"
    sleep 10
  done
) &
sampler=$!

if [ "$iot" = 1 ]; then
  iot_started=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  echo "== iotload: 3,000 devices, a batch each $iot_interval, for $((iot_seconds + 20)) s (with probes that time the way into the database)"
  compose --profile backend --profile iot run --rm --no-deps -T iotload /app/iotload --links /out/links.json --out /out/iotload.json \
    --interval "$iot_interval" --duration "$((iot_seconds + 20))s" >"$LOAD_OUT/iotload.log" 2>&1 &   # outlasts k6, which starts 8 s later, so every command has a device to answer it
  generator=$!
  sleep 8   # the devices connect and the first batches are on their way
fi

echo "== k6 $scenario"
# k6's exit status goes through a file, because the pipe to tee would hide it: non-zero when a threshold is breached
{ status=0; compose --profile k6 run --rm --no-deps -T k6 run "/load/scenarios/$k6_script.ts" 2>&1 || status=$?; echo "$status" >"$LOAD_OUT/k6.status"; } | tee "$LOAD_OUT/k6.log"
rc=$(cat "$LOAD_OUT/k6.status")
if [ "$iot" = 1 ]; then
  wait "$generator" || echo "iotload exited with an error; see iotload.log" >&2
  generator=
  tail -n 3 "$LOAD_OUT/iotload.log" | cut -c1-200
  if [ ! -s "$LOAD_OUT/iotload.json" ] || ! python3 deploy/load/iot_check.py "$LOAD_OUT/iotload.json"; then
    echo "FAILED: the telemetry half of the IoT run did not meet its checks" >&2
    [ "$rc" -ne 0 ] || rc=97
  fi
  # the broker drops what a slow subscriber cannot take: say so, as the missing readings and slow commands it causes would otherwise look like a fault of the API
  broker_dropped=$(all_profiles logs --no-color --since "$iot_started" mosquitto 2>&1 | grep -c "Outgoing messages are being dropped" || true)
  if [ "$broker_dropped" -gt 0 ]; then
    echo "FAILED: the broker dropped messages for the bridge ($broker_dropped times): the bridge could not keep up with the load" >&2
    [ "$rc" -ne 0 ] || rc=97
  fi
fi
# the write limit must have been checked on every write: a service that could not reach its store lets the write through
# (fail open) and logs it, which would make the writes look fast and the limit absent
unchecked=$(all_profiles logs --no-color 2>&1 | grep -c 'write limit not checked' || true)
if [ "$unchecked" -gt 0 ]; then
  echo "FAILED: the write limit was not checked $unchecked times (is valkey up?)" >&2
  [ "$rc" -ne 0 ] || rc=98
fi
echo "k6 finished; status $rc"
exit "$rc"
