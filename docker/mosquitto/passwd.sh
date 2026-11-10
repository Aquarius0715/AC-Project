#!/bin/sh
# Writes the broker's password file (docker/mosquitto/passwd) from the API keys in .env.local, which is git-ignored.
# Each demo client logs in with its own key: device-sim (the simulated modules) and iot-bridge (the bridge to the demo
# trigger). The keys are passed in the environment, not on the command line. Run after changing a key:
#   sh docker/mosquitto/passwd.sh && docker compose restart mosquitto
# ENV_FILE names another settings file than .env.local (the shared test environment's, deploy/test/deploy.sh).
set -eu
here=$(cd "$(dirname "$0")" && pwd)
env_file="${ENV_FILE:-$here/../../.env.local}"
[ -f "$env_file" ] || { echo "missing $env_file (copy .env.local.example and generate the keys)" >&2; exit 1; }
set -a
# shellcheck disable=SC1090
. "$env_file"
set +a
: "${MQTT_DEVICE_SIM_KEY:?set in .env.local}" "${MQTT_IOT_BRIDGE_KEY:?set in .env.local}"
# the file only changes when a key changes: a stamp (git-ignored) keeps a hash of the keys it was written from and a
# hash of the file it wrote. The file is tracked in git, and a forced checkout (deploy/test/deploy.sh) puts the
# committed one back while the stamp stays: a file that is not the one written is written again (IR684).
stamp="$here/.passwd.keys"
sha256() { if command -v sha256sum >/dev/null 2>&1; then sha256sum; else shasum -a 256; fi; }
sum=$(printf '%s\n%s\n' "$MQTT_IOT_BRIDGE_KEY" "$MQTT_DEVICE_SIM_KEY" | sha256 | cut -d' ' -f1)
file_sum() { sha256 <"$here/passwd" | cut -d' ' -f1; }
if [ -f "$stamp" ] && [ -f "$here/passwd" ] && [ "$(cat "$stamp")" = "$sum $(file_sum)" ] && grep -q '^device-sim:' "$here/passwd"; then
  echo "broker password file is up to date"
  exit 0
fi
# the broker image of compose.yaml, the one Dependabot keeps up to date, so this copy never lags behind it (IR727)
image=$(sed -n 's/^ *image: *\(eclipse-mosquitto:[^ ]*\).*/\1/p' "$here/../../compose.yaml")
[ -n "$image" ] || { echo "no eclipse-mosquitto image in compose.yaml" >&2; exit 1; }
docker run --rm --entrypoint sh -e MQTT_DEVICE_SIM_KEY -e MQTT_IOT_BRIDGE_KEY -v "$here:/work" "$image" -c '
  set -eu
  rm -f /work/passwd.new
  mosquitto_passwd -c -b /work/passwd.new iot-bridge "$MQTT_IOT_BRIDGE_KEY"
  mosquitto_passwd -b /work/passwd.new device-sim "$MQTT_DEVICE_SIM_KEY"
  chmod 644 /work/passwd.new
'
mv "$here/passwd.new" "$here/passwd"
printf '%s %s\n' "$sum" "$(file_sum)" >"$stamp"
echo "wrote $here/passwd for: $(cut -d: -f1 "$here/passwd" | tr '\n' ' ')"
