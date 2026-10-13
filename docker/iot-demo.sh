#!/bin/sh
# IoT demo on the local stack: the web apps on the Core API, the mock fleet (one air conditioner and IoT module per
# state pattern) sending readings and answering commands through the broker, and 30 days of past readings so the
# energy and air-quality charts have a long series. Development only.
#   sh docker/iot-demo.sh          start (generates the broker keys in .env.local on the first run)
#   sh docker/iot-demo.sh down     stop the IoT services (the rest of the stack keeps running)
set -eu
cd "$(dirname "$0")/.."
compose() { docker compose --env-file .env.local --profile full --profile iot "$@"; }

if [ "${1:-}" = "down" ]; then
  compose stop device-sim iot-bridge
  exit 0
fi

# 1. broker API keys: generated once, kept in .env.local (git-ignored), never printed
[ -f .env.local ] || cp .env.local.example .env.local
for key in MQTT_DEVICE_SIM_KEY MQTT_IOT_BRIDGE_KEY; do
  if ! grep -q "^$key=..*" .env.local; then
    value=$(openssl rand -hex 24)
    if grep -q "^$key=" .env.local; then
      sed -i.bak "s/^$key=.*/$key=$value/" .env.local && rm -f .env.local.bak
    else
      printf '%s=%s\n' "$key" "$value" >> .env.local
    fi
  fi
done

# 2. the broker's password file from those keys, then the stack with the IoT profile; migrate adds the mock fleet
sh docker/mosquitto/passwd.sh
compose up -d --build
compose restart mosquitto

# 3. 30 days of history up to the scenario clock (one-off; keeps rows that exist)
compose run --rm mock-history

cat <<'EOF'

IoT demo is running.
  Customer app  http://localhost:3000   sign in as customer-b  → Units & locations › Mock fleet,
                                         Energy & cost (30d), Air quality (temperature, CO2, PM2.5, humidity)
  HQ app        http://localhost:3003   sign in as hq-operator → devices and alerts of every customer
  Passwords: the local demo realm (docker/keycloak/realm-ac.json).
  Live readings arrive every minute; each "Mock ·" unit shows one state (offline, tamper, wrong unit, late answers, …).
Stop the IoT services: sh docker/iot-demo.sh down
EOF
