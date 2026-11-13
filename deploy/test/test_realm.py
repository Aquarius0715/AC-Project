"""Test of deploy/test/realm.py (IR720): the shared test environment's realm takes DEMO_USER_PASSWORD for every demo
user, and keeps the local passwords when the environment file names none.

usage: python3 -m unittest deploy/test/test_realm.py
"""

import json
import os
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
REPO = os.path.dirname(os.path.dirname(HERE))
REALM = os.path.join(REPO, "docker", "keycloak", "realm-ac.json")
SETTINGS = """CUSTOMER_WEB_URL=http://lab:3200
PARTNER_WEB_URL=http://lab:3201
TECHNICIAN_WEB_URL=http://lab:3202
ADMIN_WEB_URL=http://lab:3203
AC_WEB_CLIENT_SECRET=web
AC_ADMIN_WEB_CLIENT_SECRET=admin
IDP_ADMIN_CLIENT_SECRET=idp
"""


def written(extra: str) -> dict:
    """The realm realm.py writes for the settings plus `extra`."""
    with tempfile.TemporaryDirectory() as work:
        settings = os.path.join(work, "env")
        target = os.path.join(work, "realm.json")
        with open(settings, "w", encoding="utf-8") as f:
            f.write(SETTINGS + extra)
        subprocess.run([sys.executable, os.path.join(HERE, "realm.py"), REALM, target, settings], check=True)
        with open(target, encoding="utf-8") as f:
            return json.load(f)


def passwords(realm: dict) -> dict[str, str]:
    """Each user's password credential."""
    out = {}
    for user in realm["users"]:
        for credential in user.get("credentials", []):
            if credential.get("type") == "password":
                out[user["username"]] = credential.get("value", "")
    return out


class RealmTest(unittest.TestCase):
    def test_demo_users_take_the_environment_password(self) -> None:
        got = passwords(written("DEMO_USER_PASSWORD=s3cret-from-env\n"))
        self.assertGreaterEqual(len(got), 10)
        self.assertEqual(set(got.values()), {"s3cret-from-env"})

    def test_local_passwords_stay_without_one(self) -> None:
        with open(REALM, encoding="utf-8") as f:
            local = passwords(json.load(f))
        self.assertEqual(passwords(written("")), local)

    def test_each_app_has_its_own_client_with_its_own_address_and_theme(self) -> None:
        clients = {c["clientId"]: c for c in written("")["clients"]}  # IR790: the sign-in page names the service
        for app, port in (("customer", 3200), ("partner", 3201), ("technician", 3202)):
            client = clients[f"ac-{app}-web"]
            self.assertEqual(client["redirectUris"], [f"http://lab:{port}/*"])
            self.assertEqual(client["secret"], "web")
            self.assertEqual(client["attributes"]["login_theme"], f"ac-{app}")
        self.assertNotIn("ac-web", clients)


if __name__ == "__main__":
    unittest.main()
