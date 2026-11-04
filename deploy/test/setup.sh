#!/usr/bin/env bash
# One-time set-up of the shared test environment on the lab server (DEC-93). Run it there as the user who will own the
# environment; that user needs Docker without sudo, and sudo is asked once, for the systemd units:
#
#   git clone https://github.com/Aquarius0715/AC-Project.git ~/ac-test/repo
#   bash ~/ac-test/repo/deploy/test/setup.sh
#
# It asks for the address others open and the real broker's login, writes ~/ac-test/env (this user only) with its own
# random secrets, and installs the timer that runs deploy.sh every minute. Nothing is deployed until a tag
# test<major>.<minor>.<patch>r<n> is pushed. Running it again keeps the environment file, asks for what it lacks (the
# GitHub token of the deployment status, IR630) and reinstalls the timer.
set -euo pipefail

REPO_URL=https://github.com/Aquarius0715/AC-Project.git
ROOT=${AC_TEST_ROOT:-$HOME/ac-test}
SETTINGS=$ROOT/env
UNIT=ac-test-deploy

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
if [ ! -d "$ROOT/repo/.git" ]; then
  git clone --quiet "$REPO_URL" "$ROOT/repo"
fi

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
KEYCLOAK_PORT=8281
MAILPIT_PORT=8225
CUSTOMER_WEB_URL=http://$host:3200
PARTNER_WEB_URL=http://$host:3201
TECHNICIAN_WEB_URL=http://$host:3202
ADMIN_WEB_URL=http://$host:3203
KEYCLOAK_URL=http://$host:8281
KEYCLOAK_REALM_FILE=$ROOT/realm-ac.json
# this environment's own secrets, in place of the local values in compose.yaml and the realm
KEYCLOAK_ADMIN_PASSWORD=$(secret)
AC_WEB_CLIENT_SECRET=$(secret)
AC_ADMIN_WEB_CLIENT_SECRET=$(secret)
IDP_ADMIN_CLIENT_SECRET=$(secret)
SESSION_SECRET=$(secret)
INTERNAL_API_TOKEN=$(secret)
VALKEY_PASSWORD=$(secret)
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

if ! grep -q '^GITHUB_DEPLOY_TOKEN=' "$SETTINGS"; then
  echo "Optional: a fine-grained GitHub token of this repository with the Deployments permission (read and write) shows"
  echo "each deployment's progress on GitHub (the repository's Deployments, environment test). Enter skips it."
  read -rsp "GitHub token (GITHUB_DEPLOY_TOKEN): " github_token
  echo
  if [ -n "$github_token" ]; then
    case "$github_token" in
    *"'"*) fail "a token with a single quote cannot be written here; write $SETTINGS by hand" ;;
    esac
    umask 077
    printf "# the progress on GitHub's Deployments (deploy/test/github.py)\nGITHUB_DEPLOY_TOKEN='%s'\n" "$github_token" >>"$SETTINGS"
    echo "added GITHUB_DEPLOY_TOKEN to $SETTINGS"
  fi
fi

sudo tee "/etc/systemd/system/$UNIT.service" >/dev/null <<EOF
[Unit]
Description=AC Project shared test environment: deploys new test tags (deploy/test/deploy.sh)
After=docker.service network-online.target
Wants=network-online.target

[Service]
Type=oneshot
User=$(id -un)
Environment=AC_TEST_ROOT=$ROOT
ExecStart=/usr/bin/env bash $ROOT/repo/deploy/test/deploy.sh
TimeoutStartSec=90min
EOF
sudo tee "/etc/systemd/system/$UNIT.timer" >/dev/null <<EOF
[Unit]
Description=AC Project shared test environment: a check for new test tags every minute

[Timer]
OnBootSec=2min
OnUnitInactiveSec=1min

[Install]
WantedBy=timers.target
EOF
sudo systemctl daemon-reload
sudo systemctl enable --now "$UNIT.timer"

cat <<EOF

The timer is on. Push a tag such as test1.0.0r0 on a commit of main whose CI passed; within a minute or two this
server builds and starts it (the first build takes a while). Then:
  apps      $(grep '^CUSTOMER_WEB_URL=' "$SETTINGS" | cut -d= -f2-) (customer), :3201 partner, :3202 technician, :3203 HQ
  progress  journalctl -u $UNIT -f
  state     bash $ROOT/repo/deploy/test/deploy.sh status
EOF
