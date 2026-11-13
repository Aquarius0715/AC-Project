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

# One client per app (IR790), so the one sign-in page names the service: a realm imported before has only ac-web.
# ensure_client CLIENT_ID TITLE URL THEME creates the client with the claims the apps read when it is missing.
ensure_client() {
  cid=$("$KC" get clients -r ac -q "clientId=$1" --fields id --format csv --noquotes)
  if [ -z "$cid" ]; then
    "$KC" create clients -r ac -s "clientId=$1" -s "name=$2" -s publicClient=false \
      -s "secret=${AC_WEB_CLIENT_SECRET:-local-ac-web-secret}" -s standardFlowEnabled=true -s directAccessGrantsEnabled=false \
      -s "redirectUris=[\"$3/*\"]" -s "webOrigins=[\"$3\"]" -s 'attributes."pkce.code.challenge.method"=S256'
    cid=$("$KC" get clients -r ac -q "clientId=$1" --fields id --format csv --noquotes)
    for claim in tenant_id membership_id role; do
      "$KC" create "clients/$cid/protocol-mappers/models" -r ac -s "name=$claim" -s protocol=openid-connect \
        -s protocolMapper=oidc-usermodel-attribute-mapper -s consentRequired=false \
        -s "config.\"user.attribute\"=$claim" -s "config.\"claim.name\"=$claim" -s 'config."jsonType.label"=String' \
        -s 'config."id.token.claim"=true' -s 'config."access.token.claim"=true' -s 'config."userinfo.token.claim"=true'
    done
    echo "realm ac: client $1 created"
  fi
  "$KC" update "clients/$cid" -r ac -s "attributes.login_theme=$4"
}
ensure_client ac-customer-web "Customer app" "${CUSTOMER_WEB_URL%/}" ac-customer
ensure_client ac-partner-web "Partner app" "${PARTNER_WEB_URL%/}" ac-partner
ensure_client ac-technician-web "Technician app" "${TECHNICIAN_WEB_URL%/}" ac-technician

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
  for app in customer:CUSTOMER partner:PARTNER technician:TECHNICIAN; do
    eval "url=\${${app#*:}_WEB_URL}"
    web=$(urls_on_hosts "${url%/}")
    webid=$("$KC" get clients -r ac -q "clientId=ac-${app%%:*}-web" --fields id --format csv --noquotes)
    "$KC" update "clients/$webid" -r ac -s "redirectUris=[$web]" -s "webOrigins=[$(origins "$web")]"
  done
  admin=$(urls_on_hosts "${ADMIN_WEB_URL%/}")
  "$KC" update "clients/$id" -r ac -s "redirectUris=[$admin]" -s "webOrigins=[$(origins "$admin")]"
fi
# Keycloak 26.8.2 admits a service account to the admin API only with the built-in client scope kc-admin-api, which
# puts the admin audience in its token (CVE-2026-19688, IR731); identity-api's account gets it. A realm imported before
# has the scope but not on the client; a Keycloak before 26.8.2 has no such scope and needs none
scope=$("$KC" get client-scopes -r ac --fields id,name --format csv --noquotes | sed -n 's/^\([^,]*\),kc-admin-api$/\1/p')
if [ -n "$scope" ]; then
  admin_id=$("$KC" get clients -r ac -q clientId=ac-identity-admin --fields id --format csv --noquotes)
  "$KC" update "clients/$admin_id/default-client-scopes/$scope" -r ac
  echo "realm ac: identity-api's service account has the admin API scope"
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
