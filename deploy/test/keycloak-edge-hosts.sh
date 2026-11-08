#!/bin/sh
# Writes the list of hosts Keycloak answers on (PUBLIC_HOSTS, comma separated) as an nginx map, $host_ok: 1 for a listed
# host, 0 for any other. A host is a name or an IP of letters, digits, dots and hyphens; anything else stops the start.
set -eu

pattern=""
for host in $(printf '%s' "$PUBLIC_HOSTS" | tr ',' ' '); do
  case "$host" in
  *[!A-Za-z0-9.-]* | "")
    echo "keycloak-edge: '$host' in PUBLIC_HOSTS is not a host name or an IP" >&2
    exit 1
    ;;
  esac
  escaped=$(printf '%s' "$host" | sed 's/\./\\./g')
  pattern="${pattern:+$pattern|}$escaped"
done
[ -n "$pattern" ] || {
  echo "keycloak-edge: PUBLIC_HOSTS names no host" >&2
  exit 1
}

cat >/etc/nginx/conf.d/00-hosts.conf <<CONF
map \$host \$host_ok {
  default 0;
  ~*^($pattern)\$ 1;
}
CONF
