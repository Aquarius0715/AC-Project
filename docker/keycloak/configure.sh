#!/bin/sh
# Brings an existing realm "ac" to the sign-in settings of realm-ac.json (IR646). Keycloak imports the realm file only
# while the realm does not exist, so a stack that already has its realm (the dev database, the shared test
# environment) gets the AC Project themes, the three languages and the password reset here. Safe to run again.
set -eu

KC=/opt/keycloak/bin/kcadm.sh
SERVER=${KEYCLOAK_INTERNAL_URL:-http://keycloak:8080}

"$KC" config credentials --server "$SERVER" --realm master --user "${KEYCLOAK_ADMIN_USER:-admin}" \
  --password "${KEYCLOAK_ADMIN_PASSWORD:-admin}"

"$KC" update realms/ac \
  -s displayName="AC Project" \
  -s loginTheme=ac \
  -s emailTheme=ac \
  -s internationalizationEnabled=true \
  -s 'supportedLocales=["en","ms","ja"]' \
  -s defaultLocale=en \
  -s resetPasswordAllowed=true \
  -s loginWithEmailAllowed=true \
  -s 'smtpServer.fromDisplayName=AC Project'

# HQ Admin has its own title and accent
id=$("$KC" get clients -r ac -q clientId=ac-admin-web --fields id --format csv --noquotes)
"$KC" update "clients/$id" -r ac -s 'attributes.login_theme=ac-hq'
# the apps answer on more than one address (PUBLIC_HOSTS, IR651): each address of each app may return a sign-in
if [ -n "${PUBLIC_HOSTS:-}" ]; then
  # urls_on_hosts URL...: every URL on its own host and on each listed host, as the quoted, comma-separated URIs
  urls_on_hosts() {
    out=""
    for url in "$@"; do
      scheme=${url%%://*}
      rest=${url#*://}
      hostport=${rest%%/*}
      port=${hostport#*:}
      [ "$port" = "$hostport" ] && port="" || port=":$port"
      out="$out\"$url/*\","
      for host in $(printf '%s' "$PUBLIC_HOSTS" | tr ',' ' '); do
        out="$out\"$scheme://$host$port/*\","
      done
    done
    printf '%s' "${out%,}"
  }
  origins() { printf '%s' "$1" | sed 's#/\*"#"#g'; }
  web=$(urls_on_hosts "${CUSTOMER_WEB_URL%/}" "${PARTNER_WEB_URL%/}" "${TECHNICIAN_WEB_URL%/}")
  admin=$(urls_on_hosts "${ADMIN_WEB_URL%/}")
  webid=$("$KC" get clients -r ac -q clientId=ac-web --fields id --format csv --noquotes)
  "$KC" update "clients/$webid" -r ac -s "redirectUris=[$web]" -s "webOrigins=[$(origins "$web")]"
  "$KC" update "clients/$id" -r ac -s "redirectUris=[$admin]" -s "webOrigins=[$(origins "$admin")]"
fi
# the demo users' passwords are in the repository: an environment that names its own (the shared test environment's
# DEMO_USER_PASSWORD, IR720) sets it on every start; empty (dev, CI, E2E) leaves them as imported
if [ -n "${DEMO_USER_PASSWORD:-}" ]; then
  # the users of realm-ac.json (deploy/test/realm.py sets the same password when it writes a new realm)
  for user in customer-a customer-b contractor-a contractor-b tech-internal-a tech-external-a tech-external-b \
    hq-operator hq-restriction-manager hq-override-only; do
    KC_CLI_PASSWORD=$DEMO_USER_PASSWORD "$KC" set-password -r ac --username "$user"   # from the environment, not the command line
  done
  echo "realm ac: the demo users have this environment's password"
fi
echo "realm ac: AC Project sign-in themes and languages are set"
