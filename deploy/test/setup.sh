#!/usr/bin/env bash
# One-time set-up of the shared test environment on the lab server (DEC-93, DEC-94). Run it there, from a checkout of
# the repository, as the user who will own the environment and run the GitHub Actions runner; that user needs Docker
# without sudo, and sudo is asked once, for the systemd unit:
#
#   bash deploy/test/setup.sh
#
# It asks for the address others open and the real broker's login, writes ~/ac-test/env (this user only) with its own
# random secrets, and installs the unit ac-test-boot, which starts the deployed tag again after a reboot. The
# deployments themselves run on the runner (.github/workflows/deploy-test.yml): the steps to register it come at the
# end. Running it again keeps the environment file, puts the boot unit back and removes the timer of the first version
# (IR629), which pulled the tags itself.
set -euo pipefail

HERE=$(cd "$(dirname "$0")" && pwd)
ROOT=${AC_TEST_ROOT:-$HOME/ac-test}
SETTINGS=$ROOT/env
UNIT=ac-test-boot

fail() {
  echo "setup: $*" >&2
  exit 1
}

for tool in git curl python3 flock openssl sudo; do
  command -v "$tool" >/dev/null || fail "$tool is needed"
done
docker compose version >/dev/null 2>&1 || fail "Docker with the Compose plugin is needed"
docker ps >/dev/null 2>&1 || fail "$(id -un) cannot use Docker without sudo (add the user to the docker group)"

install -d -m 700 "$ROOT" "$ROOT/state"
chmod 700 "$ROOT"

if [ ! -f "$SETTINGS" ]; then
  default_host=$(hostname -I | awk '{print $1}')
  read -rp "Address others open the apps at [$default_host]: " host
  host=${host:-$default_host}
  read -rp "Real broker user (REAL_MQTT_USER): " mqtt_user
  read -rsp "Real broker password (REAL_MQTT_PASSWORD): " mqtt_password
  echo
  case "$mqtt_user$mqtt_password" in
  *"'"*) fail "a login with a single quote cannot be written here; write $SETTINGS by hand" ;;
  esac
  secret() { openssl rand -hex 32; }
  umask 077
  cat >"$SETTINGS" <<EOF
# The shared test environment (deploy/test/setup.sh, DEC-93): the settings compose reads for compose.yaml and
# deploy/test/compose.test.yaml. Only $(id -un) can read this file; it is never committed.
PUBLISH_ADDR=0.0.0.0
CUSTOMER_WEB_PORT=3200
PARTNER_WEB_PORT=3201
TECHNICIAN_WEB_PORT=3202
ADMIN_WEB_PORT=3203
PORTAL_WEB_PORT=3204
KEYCLOAK_PORT=8281
MAILPIT_PORT=8225
CUSTOMER_WEB_URL=http://$host:3200
PARTNER_WEB_URL=http://$host:3201
TECHNICIAN_WEB_URL=http://$host:3202
ADMIN_WEB_URL=http://$host:3203
PORTAL_WEB_URL=http://$host:3204
KEYCLOAK_URL=http://$host:8281
# more than one address (IR651): the lab network and the Tailnet, the canonical one first, e.g.
# PUBLIC_HOSTS=192.0.2.10,100.64.0.10
KEYCLOAK_REALM_FILE=$ROOT/realm-ac.json
# this environment's own secrets, in place of the local values in compose.yaml and the realm
KEYCLOAK_ADMIN_PASSWORD=$(secret)
AC_WEB_CLIENT_SECRET=$(secret)
AC_ADMIN_WEB_CLIENT_SECRET=$(secret)
IDP_ADMIN_CLIENT_SECRET=$(secret)
SESSION_SECRET=$(secret)
INTERNAL_API_TOKEN=$(secret)
VALKEY_PASSWORD=$(secret)
# the demo users' password in this environment (IR720): the repository's are public to anyone who reads it
DEMO_USER_PASSWORD=$(secret)
MQTT_DEVICE_SIM_KEY=$(openssl rand -hex 24)
MQTT_IOT_BRIDGE_KEY=$(openssl rand -hex 24)
# the real sensor nodes' broker on this server (DEC-86); 1 lets the app switch the real air conditioner (DEC-89)
REAL_MQTT_URL=tcp://$host:1883
REAL_MQTT_USER='$mqtt_user'
REAL_MQTT_PASSWORD='$mqtt_password'
REAL_MQTT_COMMANDS=1
EOF
  echo "wrote $SETTINGS"
else
  echo "keeping $SETTINGS"
fi

# the first version's timer pulled the tags itself (IR629); the runner deploys them now
if [ -f /etc/systemd/system/ac-test-deploy.timer ] || [ -f /etc/systemd/system/ac-test-deploy.service ]; then
  sudo systemctl disable --now ac-test-deploy.timer >/dev/null 2>&1 || true
  sudo rm -f /etc/systemd/system/ac-test-deploy.timer /etc/systemd/system/ac-test-deploy.service
  echo "removed the timer ac-test-deploy"
fi

# the boot unit runs the deploy script of the last deployment (each deployment leaves it there)
if [ ! -f "$ROOT/deploy.sh" ]; then
  install -m 700 "$HERE/deploy.sh" "$ROOT/deploy.sh"
fi
sudo tee "/etc/systemd/system/$UNIT.service" >/dev/null <<EOF
[Unit]
Description=AC Project shared test environment: starts the deployed tag again after a reboot (deploy.sh boot)
After=docker.service network-online.target
Wants=network-online.target
Requires=docker.service

[Service]
Type=oneshot
User=$(id -un)
Environment=AC_TEST_ROOT=$ROOT
ExecStart=/usr/bin/env bash $ROOT/deploy.sh boot
TimeoutStartSec=30min

[Install]
WantedBy=multi-user.target
EOF
sudo systemctl daemon-reload
sudo systemctl enable "$UNIT.service" >/dev/null
echo "installed the unit $UNIT (after a reboot)"

cat <<EOF

Next, register this server as the repository's self-hosted runner, as $(id -un):
  1. GitHub: the repository's Settings > Actions > Runners > New self-hosted runner > Linux, x64.
  2. Here, in ~/actions-runner, run the commands that page shows. To ./config.sh add: --labels ac-test
  3. Then: sudo ./svc.sh install $(id -un) && sudo ./svc.sh start
Then a tag such as test1.0.0r2 on a commit of main runs the workflow "Test deployment" on this server: the Actions
tab shows its build and deploy logs, and the apps open at $(grep '^CUSTOMER_WEB_URL=' "$SETTINGS" | cut -d= -f2-)
(customer), :3201 partner, :3202 technician, :3203 HQ, :3204 the portal with every service address. On this server:
  state     bash $ROOT/deploy.sh status
  reset     bash $ROOT/deploy.sh reset
EOF
