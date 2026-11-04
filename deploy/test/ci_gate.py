"""Tells whether the CI workflow passed on a commit (DEC-94): the test deployment workflow asks it before deploy.sh, so
a tag deploys only a commit whose lint and tests passed. It reads the job's token from GITHUB_TOKEN (actions: read),
never from a command line that other users of the server could see, and prints each change of the CI run's progress.

usage: GITHUB_TOKEN=... GITHUB_REPOSITORY=owner/name python3 deploy/test/ci_gate.py SHA [MINUTES]
  exit 0 when CI passed; 1 when it failed, or did not pass within MINUTES (60). MINUTES 0 reads once and exits 3 while
  CI still runs: the CI runs on the same runner, so the deployment workflow must not hold it waiting. GITHUB_API_URL
  and CI_GATE_POLL_SECONDS change the API and the pause between two reads (tests).
"""

import json
import os
import sys
import time
import urllib.request

API = os.environ.get("GITHUB_API_URL", "https://api.github.com").rstrip("/")
WORKFLOW = "CI"
POLL_SECONDS = int(os.environ.get("CI_GATE_POLL_SECONDS", "20"))  # shorter in tests


def get(path: str) -> dict:
    request = urllib.request.Request(
        API + path,
        headers={
            "Authorization": "Bearer " + os.environ["GITHUB_TOKEN"],
            "Accept": "application/vnd.github+json",
            "X-GitHub-Api-Version": "2022-11-28",
            "User-Agent": "ac-test-deploy",
        },
    )
    with urllib.request.urlopen(request, timeout=20) as response:
        return json.load(response)


def progress(repo: str, run: dict) -> str:
    jobs = get(f"/repos/{repo}/actions/runs/{run['id']}/jobs?per_page=100").get("jobs", [])
    done = sum(1 for job in jobs if job.get("status") == "completed")
    return f"{run.get('status')}, {done} of {len(jobs)} jobs done"


def main() -> int:
    sha = sys.argv[1]
    minutes = int(sys.argv[2]) if len(sys.argv) > 2 else 60
    repo = os.environ["GITHUB_REPOSITORY"]
    deadline = time.monotonic() + minutes * 60
    last = None
    while True:
        runs = [
            r
            for r in get(f"/repos/{repo}/actions/runs?head_sha={sha}&per_page=50").get("workflow_runs", [])
            if r.get("name") == WORKFLOW
        ]
        if any(r.get("conclusion") == "success" for r in runs):
            print(f"{WORKFLOW} passed on {sha[:8]}", flush=True)
            return 0
        if runs and all(r.get("status") == "completed" for r in runs):
            print(f"::error::{WORKFLOW} on {sha[:8]} ended {runs[0].get('conclusion')}: not deployed", flush=True)
            return 1
        now = progress(repo, runs[0]) if runs else "not started yet"
        if minutes == 0:
            print(f"{WORKFLOW} on {sha[:8]}: {now}", flush=True)
            return 3
        if now != last:
            print(f"waiting for {WORKFLOW} on {sha[:8]}: {now}", flush=True)
            last = now
        if time.monotonic() >= deadline:
            print(f"::error::{WORKFLOW} on {sha[:8]} did not pass within {minutes} minutes", flush=True)
            return 1
        time.sleep(POLL_SECONDS)


if __name__ == "__main__":
    sys.exit(main())
