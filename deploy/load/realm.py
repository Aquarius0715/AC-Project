"""Writes the Keycloak realm of the load test (deploy/load): a copy of docker/keycloak/realm-ac.json with a load client
that allows the password grant and the synthetic accounts of the manifest loadseed made. The shared realm file itself is
never changed: the copy is imported by the throwaway Keycloak of the compose project ac-load only.

- client ac-load: confidential, password grant on (the API scenarios) and the authorization code flow with PKCE for the
  three web apps (the web scenarios), which sign in with it; it carries the same claims as the apps' clients. The Core API takes it as
  its app client (OIDC_APP_CLIENT_ID in deploy/load/compose.load.yaml).
- client ac-admin-web: password grant on in the copy, because HQ's accounts may only use their own app's client (the Core
  API refuses an HQ membership on another client, IR510). The copy's accounts have no one-time code device.
- one user per account of the manifest, with the manifest's user ID (the token's subject) and the one password of the run.
- the clients' secrets come from the run's settings, as in deploy/test/realm.py.

usage: python3 deploy/load/realm.py REALM_IN REALM_OUT SETTINGS MANIFEST
"""

import copy
import json
import os
import sys

LOAD_CLIENT = "ac-load"
SECRETS = {
    LOAD_CLIENT: "LOAD_CLIENT_SECRET",
    "ac-admin-web": "AC_ADMIN_WEB_CLIENT_SECRET",
    "ac-customer-web": "AC_WEB_CLIENT_SECRET",
    "ac-partner-web": "AC_WEB_CLIENT_SECRET",
    "ac-technician-web": "AC_WEB_CLIENT_SECRET",
    "ac-identity-admin": "IDP_ADMIN_CLIENT_SECRET",
}


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


def load_client(app: dict, urls: list[str]) -> dict:
    """The load client: the app client's claims (tenant, membership, role), the password grant, and the code flow for the
    web apps' addresses in the load stack."""
    client = copy.deepcopy(app)
    client["clientId"] = LOAD_CLIENT
    client["name"] = "Load test (deploy/load only)"
    client["standardFlowEnabled"] = True
    client["directAccessGrantsEnabled"] = True
    client["redirectUris"] = [f"{url}/*" for url in urls]
    client["webOrigins"] = urls
    client["attributes"] = {"pkce.code.challenge.method": "S256"}
    return client


def user_of(account: dict, password: str) -> dict:
    """The realm's user for one account of the manifest; its ID is the token's subject (identity.users.id)."""
    return {
        "id": account["userId"],
        "username": account["username"],
        "email": account["username"] + "@load.local",
        "emailVerified": True,
        "enabled": True,
        "firstName": "Load",
        "lastName": account["group"],
        "credentials": [{"type": "password", "value": password, "temporary": False}],
        "attributes": {
            "tenant_id": [account["tenantId"]],
            "membership_id": [account["membershipId"]],
            "role": [account["role"]],
        },
    }


def main() -> None:
    source, target, settings_path, manifest_path = sys.argv[1:5]
    settings = read_settings(settings_path)

    def setting(name: str) -> str:
        value = settings.get(name, "")
        if not value:
            sys.exit(f"realm.py: {name} is not set in {settings_path}")
        return value

    with open(source, encoding="utf-8") as f:
        realm = json.load(f)
    with open(manifest_path, encoding="utf-8") as f:
        manifest = json.load(f)

    clients = {c["clientId"]: c for c in realm["clients"]}
    urls = [settings[name].rstrip("/") for name in ("CUSTOMER_WEB_URL", "PARTNER_WEB_URL", "TECHNICIAN_WEB_URL") if settings.get(name)]
    realm["clients"].append(load_client(clients["ac-customer-web"], urls))
    clients[LOAD_CLIENT] = realm["clients"][-1]
    clients["ac-admin-web"]["directAccessGrantsEnabled"] = True
    for client_id, name in SECRETS.items():
        clients[client_id]["secret"] = setting(name)

    password = setting("LOAD_PASSWORD")
    for account in manifest["users"]:
        if account["password"] != password:
            sys.exit(f"realm.py: the manifest's password for {account['username']} is not LOAD_PASSWORD")
        realm["users"].append(user_of(account, password))

    fd = os.open(target, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o644)  # Keycloak reads it as another user
    with os.fdopen(fd, "w", encoding="utf-8") as f:
        json.dump(realm, f, ensure_ascii=False, indent=2)
        f.write("\n")


if __name__ == "__main__":
    main()
