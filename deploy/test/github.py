"""Reports the shared test environment's deployments to GitHub (DEC-93, IR630): one deployment of the environment
"test" per tag, and its status at each step, so the progress shows under the repository's Deployments. It signs in
with GITHUB_DEPLOY_TOKEN of the environment file, a fine-grained token of this repository with the Deployments
permission (read and write); without it nothing is sent. The token is read here, never passed on a command line that
other users of the server could see. Nothing names the server, as the repository is public. A failure only returns
non-zero: deploy.sh logs it and goes on.

usage: python3 deploy/test/github.py SETTINGS SLUG STATE_DIR TAG STATE DESCRIPTION
  STATE is queued, in_progress, success, failure or error; GITHUB_API_URL changes the API's address (tests).
"""

import json
import os
import sys
import urllib.request

from realm import read_settings

API = os.environ.get("GITHUB_API_URL", "https://api.github.com").rstrip("/")


def call(token: str, path: str, body: dict) -> dict:
    request = urllib.request.Request(
        API + path,
        data=json.dumps(body).encode(),
        method="POST",
        headers={
            "Authorization": "Bearer " + token,
            "Accept": "application/vnd.github+json",
            "X-GitHub-Api-Version": "2022-11-28",
            "User-Agent": "ac-test-deploy",
        },
    )
    with urllib.request.urlopen(request, timeout=20) as response:
        return json.load(response)


def main() -> None:
    settings_path, slug, state_dir, tag, state, description = sys.argv[1:7]
    token = read_settings(settings_path).get("GITHUB_DEPLOY_TOKEN", "")
    if not token:
        return
    ids_path = os.path.join(state_dir, "github-deployments.json")
    ids = {}
    if os.path.exists(ids_path):
        with open(ids_path, encoding="utf-8") as f:
            ids = json.load(f)
    if tag not in ids:
        created = call(
            token,
            f"/repos/{slug}/deployments",
            {
                "ref": tag,
                "environment": "test",
                "description": f"{tag} to the shared test environment",
                "auto_merge": False,
                "required_contexts": [],  # deploy.sh checks the CI result itself
                "production_environment": False,
                "transient_environment": False,
            },
        )
        ids[tag] = created["id"]
        with open(ids_path, "w", encoding="utf-8") as f:
            json.dump(ids, f)
    call(
        token,
        f"/repos/{slug}/deployments/{ids[tag]}/statuses",
        {"state": state, "description": description[:140], "environment": "test", "auto_inactive": True},
    )


if __name__ == "__main__":
    main()
