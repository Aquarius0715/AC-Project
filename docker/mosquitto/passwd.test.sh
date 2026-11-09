#!/bin/sh
# Test of passwd.sh (IR684) without Docker: a shim `docker` writes the file mosquitto_passwd would. The password file
# is tracked in git and `git checkout --force` (what deploy/test/deploy.sh does) puts the committed one back, while the
# stamp beside it is ignored and stays: the file must then be written again from the keys, not kept as "up to date".
set -eu

src=$(cd "$(dirname "$0")" && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
mkdir -p "$work/docker/mosquitto" "$work/bin"
cp "$src/passwd.sh" "$work/docker/mosquitto/passwd.sh"
printf 'iot-bridge:committed\ndevice-sim:committed\n' >"$work/docker/mosquitto/passwd"
printf 'MQTT_DEVICE_SIM_KEY=simkey\nMQTT_IOT_BRIDGE_KEY=bridgekey\n' >"$work/keys.env"

# the shim: finds the mounted directory in `-v DIR:/work` and writes passwd.new there from the keys in its environment
cat >"$work/bin/docker" <<'SHIM'
#!/bin/sh
dir=""
while [ $# -gt 0 ]; do
  case "$1" in
  -v) dir=${2%%:*}; shift ;;
  esac
  shift
done
echo run >>"$dir/docker.calls"
printf 'iot-bridge:fake-%s\ndevice-sim:fake-%s\n' "$MQTT_IOT_BRIDGE_KEY" "$MQTT_DEVICE_SIM_KEY" >"$dir/passwd.new"
SHIM
chmod +x "$work/bin/docker"

run() { PATH="$work/bin:$PATH" ENV_FILE="$work/keys.env" sh "$work/docker/mosquitto/passwd.sh" >/dev/null; }
calls() { wc -l <"$work/docker/mosquitto/docker.calls" | tr -d ' '; }
fail() { echo "passwd.test: $*" >&2; exit 1; }

run
grep -q 'fake-bridgekey' "$work/docker/mosquitto/passwd" || fail "the first run did not write the file from the keys"
[ "$(calls)" = 1 ] || fail "the first run should call the generator once"

run
[ "$(calls)" = 1 ] || fail "an unchanged file and keys are up to date: the generator should not run again"

# git checkout --force: the committed file comes back, the stamp stays
printf 'iot-bridge:committed\ndevice-sim:committed\n' >"$work/docker/mosquitto/passwd"
run
grep -q 'fake-bridgekey' "$work/docker/mosquitto/passwd" || fail "the committed file came back and was kept: the file was not written again"
[ "$(calls)" = 2 ] || fail "the file that came back should have been generated again"

# a changed key
printf 'MQTT_DEVICE_SIM_KEY=other\nMQTT_IOT_BRIDGE_KEY=bridgekey\n' >"$work/keys.env"
run
grep -q 'fake-other' "$work/docker/mosquitto/passwd" || fail "a changed key was not written"
echo "passwd.test: ok"
