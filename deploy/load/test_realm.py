"""Test of deploy/load/realm.py: the load test's realm copy has the load client with the apps' claims, HQ's client with the
password grant, one user per manifest account under its own ID, and the run's secrets; it refuses a missing secret and a
manifest made with another password, and never changes the shared realm file.

usage: python3 -m unittest deploy/load/test_realm.py
"""

import hashlib
import json
import os
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
REPO = os.path.dirname(os.path.dirname(HERE))
REALM = os.path.join(REPO, "docker", "keycloak", "realm-ac.json")
WEB_URLS = """CUSTOMER_WEB_URL=http://customer-web-api:3000/
PARTNER_WEB_URL=http://partner-web-api:3000
TECHNICIAN_WEB_URL=http://technician-web-api:3000
"""
SECRETS = """LOAD_CLIENT_SECRET=load
AC_ADMIN_WEB_CLIENT_SECRET='admin'
AC_WEB_CLIENT_SECRET="web"
IDP_ADMIN_CLIENT_SECRET=idp
# a comment=not a setting
LOAD_PASSWORD=run-password
"""
ACCOUNTS = [
    {
        "username": "load-customer-001",
        "password": "run-password",
        "group": "customer",
        "role": "client",
        "userId": "00000000-0000-4000-8000-000000000001",
        "membershipId": "00000000-0000-4000-8000-000000000011",
        "tenantId": "00000000-0000-4000-8000-000000000021",
    },
    {
        "username": "load-hq-001",
        "password": "run-password",
        "group": "hq",
        "role": "admin",
        "userId": "00000000-0000-4000-8000-000000000002",
        "membershipId": "00000000-0000-4000-8000-000000000012",
        "tenantId": "00000000-0000-4000-8000-000000000022",
    },
]


def run(settings: str, accounts: list[dict]) -> tuple[subprocess.CompletedProcess, dict | None]:
    """realm.py's exit and stderr for the settings and the manifest's accounts, and the realm it wrote (None if none)."""
    with tempfile.TemporaryDirectory() as work:
        manifest_path = os.path.join(work, "manifest.json")
        target = os.path.join(work, "realm-load.json")
        with open(manifest_path, "w", encoding="utf-8") as f:
            json.dump({"customers": 1, "units": 1, "jobs": 1, "users": accounts}, f)
        # the settings (secrets, even test ones) go through a pipe, never into a file
        done = subprocess.run(
            [sys.executable, os.path.join(HERE, "realm.py"), REALM, target, "/dev/stdin", manifest_path],
            input=settings,
            capture_output=True,
            text=True,
            check=False,
        )
        if not os.path.exists(target):
            return done, None
        with open(target, encoding="utf-8") as f:
            return done, json.load(f)


def clients_of(realm: dict) -> dict[str, dict]:
    return {c["clientId"]: c for c in realm["clients"]}


def shared_realm_digest() -> str:
    with open(REALM, "rb") as f:
        return hashlib.sha256(f.read()).hexdigest()


class LoadRealmTest(unittest.TestCase):
    def written(self, settings: str) -> dict:
        done, realm = run(settings, ACCOUNTS)
        self.assertEqual(done.returncode, 0, done.stderr)
        self.assertIsNotNone(realm)
        assert realm is not None  # for the type checker; the line above fails the test first
        return realm

    def test_load_client_has_the_apps_claims_and_both_flows(self) -> None:
        clients = clients_of(self.written(WEB_URLS + SECRETS))
        load, app = clients["ac-load"], clients["ac-customer-web"]
        self.assertTrue(load["directAccessGrantsEnabled"])  # the API scenarios
        self.assertTrue(load["standardFlowEnabled"])  # the web scenarios
        self.assertEqual(load["attributes"], {"pkce.code.challenge.method": "S256"})
        self.assertEqual(load["protocolMappers"], app["protocolMappers"])  # tenant, membership and role claims
        self.assertEqual(
            load["redirectUris"],
            ["http://customer-web-api:3000/*", "http://partner-web-api:3000/*", "http://technician-web-api:3000/*"],
        )
        self.assertEqual(load["webOrigins"], ["http://customer-web-api:3000", "http://partner-web-api:3000", "http://technician-web-api:3000"])
        self.assertFalse(app["directAccessGrantsEnabled"])  # the copy's source client keeps its own flows

    def test_api_only_run_has_no_redirect_addresses(self) -> None:
        load = clients_of(self.written(SECRETS))["ac-load"]
        self.assertEqual(load["redirectUris"], [])
        self.assertEqual(load["webOrigins"], [])

    def test_secrets_come_from_the_settings_and_hq_gets_the_password_grant(self) -> None:
        clients = clients_of(self.written(WEB_URLS + SECRETS))
        want = {
            "ac-load": "load",
            "ac-admin-web": "admin",
            "ac-customer-web": "web",
            "ac-partner-web": "web",
            "ac-technician-web": "web",
            "ac-identity-admin": "idp",
        }
        self.assertEqual({name: clients[name]["secret"] for name in want}, want)
        self.assertTrue(clients["ac-admin-web"]["directAccessGrantsEnabled"])  # HQ signs in only on its own client (IR510)

    def test_one_user_per_account_under_its_own_id(self) -> None:
        with open(REALM, encoding="utf-8") as f:
            shared = len(json.load(f)["users"])
        users = self.written(SECRETS)["users"]
        self.assertEqual(len(users), shared + len(ACCOUNTS))
        by_name = {u["username"]: u for u in users}
        for account in ACCOUNTS:
            user = by_name[account["username"]]
            self.assertEqual(user["id"], account["userId"])  # the token's subject is identity.users.id
            self.assertEqual(
                user["attributes"],
                {"tenant_id": [account["tenantId"]], "membership_id": [account["membershipId"]], "role": [account["role"]]},
            )
            self.assertEqual(user["credentials"], [{"type": "password", "value": "run-password", "temporary": False}])
            self.assertTrue(user["enabled"])

    def test_refuses_a_missing_secret(self) -> None:
        done, realm = run(SECRETS.replace("IDP_ADMIN_CLIENT_SECRET=idp\n", "IDP_ADMIN_CLIENT_SECRET=\n"), ACCOUNTS)
        self.assertNotEqual(done.returncode, 0)
        self.assertIn("IDP_ADMIN_CLIENT_SECRET is not set", done.stderr)
        self.assertIsNone(realm)

    def test_refuses_a_manifest_made_with_another_password(self) -> None:
        other = [dict(ACCOUNTS[0]), dict(ACCOUNTS[1], password="older-run")]
        done, realm = run(SECRETS, other)
        self.assertNotEqual(done.returncode, 0)
        self.assertIn("load-hq-001 is not LOAD_PASSWORD", done.stderr)
        self.assertIsNone(realm)

    def test_shared_realm_file_is_never_changed(self) -> None:
        before = shared_realm_digest()
        self.written(WEB_URLS + SECRETS)
        self.assertEqual(shared_realm_digest(), before)


if __name__ == "__main__":
    unittest.main()
