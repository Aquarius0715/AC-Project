#!/bin/sh
# Writes the broker's password file (docker/mosquitto/passwd) from the API keys in .env.local, which is git-ignored.
# Each demo client logs in with its own key: device-sim (the simulated modules) and iot-bridge (the bridge to the demo
# trigger). The keys are passed in the environment, not on the command line. Run after changing a key:
#   sh docker/mosquitto/passwd.sh && docker compose restart mosquitto
set -eu
here=$(cd "$(dirname "$0")" && pwd)
env_file="$here/../../.env.local"
[ -f "$env_file" ] || { echo "missing $env_file (copy .env.local.example and generate the keys)" >&2; exit 1; }
set -a
# shellcheck disable=SC1090
. "$env_file"
set +a
: "${MQTT_DEVICE_SIM_KEY:?set in .env.local}" "${MQTT_IOT_BRIDGE_KEY:?set in .env.local}"
# the file only changes when a key changes: a stamp (git-ignored) keeps a hash of the keys it was written from
stamp="$here/.passwd.keys"
sum=$(printf '%s\n%s\n' "$MQTT_IOT_BRIDGE_KEY" "$MQTT_DEVICE_SIM_KEY" | shasum -a 256 | cut -d' ' -f1)
if [ -f "$stamp" ] && [ "$(cat "$stamp")" = "$sum" ] && grep -q '^device-sim:' "$here/passwd" 2>/dev/null; then
  echo "broker password file is up to date"
  exit 0
fi
docker run --rm --entrypoint sh -e MQTT_DEVICE_SIM_KEY -e MQTT_IOT_BRIDGE_KEY -v "$here:/work" eclipse-mosquitto:2@sha256:38c0da4f2ef84284d47b3b3eeea1cb3bdeabe81ee10caf0cd5c5ff61ee3ea408 -c '
  set -eu
  rm -f /work/passwd.new
  mosquitto_passwd -c -b /work/passwd.new iot-bridge "$MQTT_IOT_BRIDGE_KEY"
  mosquitto_passwd -b /work/passwd.new device-sim "$MQTT_DEVICE_SIM_KEY"
  chmod 644 /work/passwd.new
'
mv "$here/passwd.new" "$here/passwd"
printf '%s\n' "$sum" > "$stamp"
echo "wrote $here/passwd for: $(cut -d: -f1 "$here/passwd" | tr '\n' ' ')"
