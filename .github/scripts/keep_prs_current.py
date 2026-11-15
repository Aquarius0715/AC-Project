"""Keeps the owner's auto-merge pull requests moving into main, one at a time (workflow keep-prs-current.yml).

main requires a branch that is up to date with it, so once one pull request merges, every other one is behind and GitHub's
auto-merge waits for ever. And a merge or a branch update made with the workflow's own token starts no workflow run: no CI
on the updated branch, none on main. Each run of this script therefore starts CI on main when main's head has none, and
takes one step for the pull requests:

1. an up-to-date pull request has no CI run on its head (a branch this script updated): start CI on its branch.
2. a pull request's CI still runs, or one passed and is about to merge: wait for it (the next run looks again).
3. otherwise the oldest behind pull request is brought up to date with main, and its CI started.

A pull request with a conflict (dirty) is left alone with a warning: it needs its own merge of main. Only open, non-draft
pull requests of the repository owner from a branch of this repository with auto-merge on are touched (the ones
auto-merge.yml turns it on for).

usage: GITHUB_TOKEN=... GITHUB_REPOSITORY=owner/name python3 .github/scripts/keep_prs_current.py
"""

import json
import os
import sys
import time
import urllib.error
import urllib.request
from dataclasses import dataclass
from datetime import datetime, timedelta, timezone

API = os.environ.get("GITHUB_API_URL", "https://api.github.com").rstrip("/")
RUNNING = {"queued", "in_progress", "waiting", "requested", "pending"}
# states in which a pull request whose CI passed merges by itself (unknown: GitHub has not worked the state out yet)
MERGING = {"clean", "unstable", "has_hooks", "unknown"}
GRACE = timedelta(minutes=3)  # a pull request's own CI is created a few seconds after a push: do not start a second one


@dataclass(frozen=True)
class Pull:
    number: int
    branch: str
    head: str
    state: str  # GitHub's mergeable_state: behind, dirty, clean, blocked, unstable, unknown, ...
    ci: str  # none, running, success, failure (the latest CI run on the head)
    updated: datetime


@dataclass(frozen=True)
class Step:
    action: str  # ci, wait, update, idle
    pull: Pull | None = None


def decide(pulls: list[Pull], now: datetime) -> Step:
    """The one step of this run; pulls are oldest first."""
    current = [p for p in pulls if p.state not in ("behind", "dirty")]
    for p in current:
        if p.ci == "none" and now - p.updated > GRACE:
            return Step("ci", p)
    for p in current:
        if p.ci in ("running", "none") or (p.ci == "success" and p.state in MERGING):
            return Step("wait", p)
    behind = [p for p in pulls if p.state == "behind"]
    return Step("update", behind[0]) if behind else Step("idle")


def call(method: str, path: str, body: dict | None = None) -> dict:
    request = urllib.request.Request(
        API + path,
        method=method,
        data=None if body is None else json.dumps(body).encode(),
        headers={
            "Authorization": "Bearer " + os.environ["GITHUB_TOKEN"],
            "Accept": "application/vnd.github+json",
            "X-GitHub-Api-Version": "2022-11-28",
            "User-Agent": "ac-keep-prs-current",
        },
    )
    with urllib.request.urlopen(request, timeout=30) as response:
        text = response.read()
    return json.loads(text) if text else {}


def ci_of(repo: str, sha: str) -> str:
    runs = call("GET", f"/repos/{repo}/actions/workflows/ci.yml/runs?head_sha={sha}&per_page=1").get("workflow_runs", [])
    if not runs:
        return "none"
    run = runs[0]
    if run.get("status") in RUNNING:
        return "running"
    return "success" if run.get("conclusion") == "success" else "failure"


def pulls_of(repo: str) -> list[Pull]:
    owner = repo.split("/")[0]
    listed = call("GET", f"/repos/{repo}/pulls?state=open&base=main&sort=created&direction=asc&per_page=100")
    pulls = []
    for item in listed if isinstance(listed, list) else []:
        if item.get("draft") or not item.get("auto_merge"):
            continue
        if item["user"]["login"] != owner or (item["head"].get("repo") or {}).get("full_name") != repo:
            continue
        detail = call("GET", f"/repos/{repo}/pulls/{item['number']}")  # mergeable_state is only on the single pull request
        pulls.append(
            Pull(
                number=detail["number"],
                branch=detail["head"]["ref"],
                head=detail["head"]["sha"],
                state=detail.get("mergeable_state") or "unknown",
                ci=ci_of(repo, detail["head"]["sha"]),
                updated=datetime.fromisoformat(detail["updated_at"].replace("Z", "+00:00")),
            )
        )
    return pulls


def start_ci(repo: str, ref: str) -> None:
    call("POST", f"/repos/{repo}/actions/workflows/ci.yml/dispatches", {"ref": ref})


def update(repo: str, pull: Pull) -> None:
    call("PUT", f"/repos/{repo}/pulls/{pull.number}/update-branch", {"expected_head_sha": pull.head})
    for _ in range(24):  # the update is asynchronous: CI must start on the new head, not the old one
        time.sleep(5)
        if call("GET", f"/repos/{repo}/pulls/{pull.number}")["head"]["sha"] != pull.head:
            start_ci(repo, pull.branch)
            print(f"#{pull.number}: brought up to date with main, CI started")
            return
    print(f"#{pull.number}: brought up to date with main; its CI starts on the next run")


def main() -> int:
    repo = os.environ["GITHUB_REPOSITORY"]
    main_sha = call("GET", f"/repos/{repo}/branches/main")["commit"]["sha"]
    pulls = pulls_of(repo)
    for p in pulls:
        if p.state == "dirty":
            print(f"::warning::#{p.number} ({p.branch}) conflicts with main: merge main into it by hand")
    if ci_of(repo, main_sha) == "none":
        start_ci(repo, "main")
        print(f"main {main_sha[:8]} had no CI run: started it")
    step = decide(pulls, datetime.now(timezone.utc))
    if step.action == "ci" and step.pull:
        start_ci(repo, step.pull.branch)
        print(f"#{step.pull.number}: no CI on {step.pull.head[:8]}: started it")
    elif step.action == "wait" and step.pull:
        print(f"#{step.pull.number}: CI {step.pull.ci}, state {step.pull.state}: waiting for it")
    elif step.action == "update" and step.pull:
        update(repo, step.pull)
    else:
        print("nothing behind")
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except urllib.error.HTTPError as err:
        print(f"::error::GitHub API {err.code} on {err.url}: {err.read().decode(errors='replace')[:300]}")
        sys.exit(1)
