"""Writes the shared test environment's Keycloak realm (DEC-93): docker/keycloak/realm-ac.json, the local realm, with
the test URLs the apps return to after sign-in and this environment's own client secrets in place of the local ones.
The demo users and their passwords stay as they are. Keycloak imports the file only while the realm does not exist,
on the environment's first start.

usage: python3 deploy/test/realm.py REALM_IN REALM_OUT SETTINGS  (the URLs and secrets from the environment file)
"""

import json
import os
import sys
from urllib.parse import urlsplit, urlunsplit

APPS = {  # client -> the apps that sign in with it
    "ac-web": ["CUSTOMER_WEB_URL", "PARTNER_WEB_URL", "TECHNICIAN_WEB_URL"],
    "ac-admin-web": ["ADMIN_WEB_URL"],
}
SECRETS = {
    "ac-web": "AC_WEB_CLIENT_SECRET",
    "ac-admin-web": "AC_ADMIN_WEB_CLIENT_SECRET",
    "ac-identity-admin": "IDP_ADMIN_CLIENT_SECRET",
}


def on_host(url: str, host: str) -> str:
    """The app's address with its host name replaced: the same port and scheme, another address it answers on (IR651)."""
    parts = urlsplit(url)
    port = f":{parts.port}" if parts.port else ""
    return urlunsplit((parts.scheme, host + port, parts.path, "", "")).rstrip("/")


def read_settings(path: str) -> dict[str, str]:
    """KEY=value lines, a value in single or double quotes without them; comments and blank lines skipped."""
    values = {}
    with open(path, encoding="utf-8") as f:
        for line in f:
            key, sep, value = line.rstrip("\n").partition("=")
            if not sep or key.startswith("#"):
                continue
            if len(value) >= 2 and value[0] == value[-1] and value[0] in "'\"":
                value = value[1:-1]
            values[key.strip()] = value
    return values


def main() -> None:
    source, target, settings_path = sys.argv[1], sys.argv[2], sys.argv[3]
    settings = read_settings(settings_path)

    def setting(name: str) -> str:
        value = settings.get(name, "")
        if not value:
            sys.exit(f"realm.py: {name} is not set in {settings_path}")
        return value

    with open(source, encoding="utf-8") as f:
        realm = json.load(f)
    clients = {c["clientId"]: c for c in realm["clients"]}
    hosts = [h.strip() for h in settings.get("PUBLIC_HOSTS", "").split(",") if h.strip()]
    for client_id, apps in APPS.items():
        client = clients[client_id]
        urls = [setting(app).rstrip("/") for app in apps]
        urls += [on_host(url, host) for url in list(urls) for host in hosts if on_host(url, host) not in urls]  # IR651
        client["redirectUris"] = [f"{url}/*" for url in urls]
        client["webOrigins"] = urls
    for client_id, name in SECRETS.items():
        clients[client_id]["secret"] = setting(name)
    fd = os.open(target, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o644)  # Keycloak reads it as another user
    with os.fdopen(fd, "w", encoding="utf-8") as f:
        json.dump(realm, f, ensure_ascii=False, indent=2)
        f.write("\n")


if __name__ == "__main__":
    main()
