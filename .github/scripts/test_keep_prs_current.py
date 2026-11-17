"""The step keep_prs_current.py takes for each state of main and the pull requests (python3 -m unittest)."""

import importlib.util
import pathlib
import unittest
from datetime import datetime, timedelta, timezone

spec = importlib.util.spec_from_file_location("keep", pathlib.Path(__file__).with_name("keep_prs_current.py"))
assert spec and spec.loader
keep = importlib.util.module_from_spec(spec)
spec.loader.exec_module(keep)

NOW = datetime(2026, 11, 15, 12, 0, tzinfo=timezone.utc)
OLD = NOW - timedelta(minutes=10)


def pr(number: int, state: str, ci: str, updated: datetime = OLD):
    return keep.Pull(number=number, branch=f"b{number}", head=f"sha{number}", state=state, ci=ci, updated=updated)


class Decide(unittest.TestCase):
    def step(self, pulls):
        s = keep.decide(pulls, NOW)
        return s.action, s.pull.number if s.pull else None

    def test_updates_the_oldest_behind_one(self):
        self.assertEqual(self.step([pr(1, "behind", "success"), pr(2, "behind", "success")]), ("update", 1))

    def test_waits_while_a_current_one_runs(self):
        self.assertEqual(self.step([pr(1, "behind", "success"), pr(2, "blocked", "running")]), ("wait", 2))

    def test_waits_for_one_that_passed_and_merges(self):
        self.assertEqual(self.step([pr(1, "behind", "success"), pr(2, "clean", "success")]), ("wait", 2))
        self.assertEqual(self.step([pr(1, "behind", "success"), pr(2, "unknown", "success")]), ("wait", 2))

    def test_starts_ci_on_an_updated_branch_without_one(self):
        self.assertEqual(self.step([pr(1, "behind", "success"), pr(2, "blocked", "none")]), ("ci", 2))

    def test_gives_a_new_push_time_for_its_own_ci(self):
        fresh = pr(2, "blocked", "none", updated=NOW - timedelta(seconds=30))
        self.assertEqual(self.step([pr(1, "behind", "success"), fresh]), ("wait", 2))

    def test_a_failed_or_conflicting_one_does_not_hold_the_others(self):
        self.assertEqual(self.step([pr(1, "blocked", "failure"), pr(2, "dirty", "success"), pr(3, "behind", "failure")]), ("update", 3))

    def test_idle_when_nothing_is_behind(self):
        self.assertEqual(self.step([pr(1, "dirty", "success"), pr(2, "blocked", "failure")]), ("idle", None))


class Answers(list):
    """Answers to one call, in the order the calls come."""


class Api(unittest.TestCase):
    """The GitHub calls, with call() and time.sleep replaced: what the script reads and what it sends."""

    def setUp(self):
        self.sent = []
        self.answers = {}
        real_call, real_sleep = keep.call, keep.time.sleep
        keep.call = self.fake
        keep.time.sleep = lambda _seconds: None
        self.addCleanup(setattr, keep, "call", real_call)
        self.addCleanup(setattr, keep.time, "sleep", real_sleep)

    def fake(self, method, path, body=None):
        self.sent.append((method, path, body))
        answer = self.answers[(method, path)]
        return answer.pop(0) if isinstance(answer, Answers) else answer

    def runs(self, sha, *runs):
        self.answers[("GET", f"/repos/o/r/actions/workflows/ci.yml/runs?head_sha={sha}&per_page=1")] = {"workflow_runs": list(runs)}

    def test_ci_of_maps_the_latest_run(self):
        cases = [
            ((), "none"),
            (({"status": "queued"},), "running"),
            (({"status": "in_progress"},), "running"),
            (({"status": "completed", "conclusion": "success"},), "success"),
            (({"status": "completed", "conclusion": "failure"},), "failure"),
            (({"status": "completed", "conclusion": "cancelled"},), "failure"),
        ]
        for runs, want in cases:
            with self.subTest(runs=runs):
                self.runs("s", *runs)
                self.assertEqual(keep.ci_of("o/r", "s"), want)

    def item(self, number, login="o", repo="o/r", draft=False, auto=True):
        return {"number": number, "draft": draft, "auto_merge": {"merge_method": "merge"} if auto else None, "user": {"login": login}, "head": {"repo": {"full_name": repo} if repo else None}}

    def test_pulls_of_takes_only_the_owners_auto_merge_pulls_from_this_repository(self):
        self.answers[("GET", "/repos/o/r/pulls?state=open&base=main&sort=created&direction=asc&per_page=100")] = [
            self.item(1),
            self.item(2, draft=True),
            self.item(3, auto=False),
            self.item(4, login="someone"),
            self.item(5, repo="someone/r"),  # a fork's branch
            self.item(6, repo=None),  # its fork was deleted
        ]
        self.answers[("GET", "/repos/o/r/pulls/1")] = {"number": 1, "head": {"ref": "b1", "sha": "s1"}, "mergeable_state": None, "updated_at": "2026-11-15T12:00:00Z"}
        self.runs("s1", {"status": "completed", "conclusion": "success"})
        self.assertEqual(
            keep.pulls_of("o/r"),
            [keep.Pull(number=1, branch="b1", head="s1", state="unknown", ci="success", updated=NOW)],
        )
        self.assertNotIn(("GET", "/repos/o/r/pulls/2", None), self.sent)  # nothing else is even read

    def test_update_starts_ci_only_on_the_new_head(self):
        pull = pr(7, "behind", "success")
        self.answers[("PUT", "/repos/o/r/pulls/7/update-branch")] = {}
        self.answers[("GET", "/repos/o/r/pulls/7")] = Answers([{"head": {"sha": "sha7"}}, {"head": {"sha": "sha7"}}, {"head": {"sha": "new"}}])
        self.answers[("POST", "/repos/o/r/actions/workflows/ci.yml/dispatches")] = {}
        keep.update("o/r", pull)
        self.assertEqual(self.sent[0], ("PUT", "/repos/o/r/pulls/7/update-branch", {"expected_head_sha": "sha7"}))
        self.assertEqual([s[0] for s in self.sent], ["PUT", "GET", "GET", "GET", "POST"])
        self.assertEqual(self.sent[-1], ("POST", "/repos/o/r/actions/workflows/ci.yml/dispatches", {"ref": "b7"}))

    def test_update_starts_no_ci_while_the_head_is_still_old(self):
        self.answers[("PUT", "/repos/o/r/pulls/7/update-branch")] = {}
        self.answers[("GET", "/repos/o/r/pulls/7")] = {"head": {"sha": "sha7"}}
        keep.update("o/r", pr(7, "behind", "success"))
        self.assertNotIn("POST", [s[0] for s in self.sent])
        self.assertEqual(len(self.sent), 25)  # the update and 24 looks


if __name__ == "__main__":
    unittest.main()
