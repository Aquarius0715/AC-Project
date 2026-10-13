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
docker run --rm --entrypoint sh -e MQTT_DEVICE_SIM_KEY -e MQTT_IOT_BRIDGE_KEY -v "$here:/work" eclipse-mosquitto:2 -c '
  set -eu
  rm -f /work/passwd.new
  mosquitto_passwd -c -b /work/passwd.new iot-bridge "$MQTT_IOT_BRIDGE_KEY"
  mosquitto_passwd -b /work/passwd.new device-sim "$MQTT_DEVICE_SIM_KEY"
  chmod 644 /work/passwd.new
'
mv "$here/passwd.new" "$here/passwd"
echo "wrote $here/passwd for: $(cut -d: -f1 "$here/passwd" | tr '\n' ' ')"
