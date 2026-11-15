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


if __name__ == "__main__":
    unittest.main()
