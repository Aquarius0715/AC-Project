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
echo "realm ac: AC Project sign-in themes and languages are set"
