"""Applied approvals remain historical facts after a later independent schedule edit."""

import copy
import json
import unittest
from datetime import datetime
from decimal import Decimal
from pathlib import Path
from tempfile import TemporaryDirectory

from automation.src.approval_state import (
    apply_card_approval,
    card_approval_states,
    first_application,
)
from automation.src.dataset import ROOT
from automation.src.generate_approval_history import make_approval_history
from automation.src.html_report import customer_html
from automation.src.models import Schedule
from automation.src.replay import known_feedback, replay_customer
from test_dataset import single
from test_report import Scripts


def at(stamp):
    return datetime.fromisoformat(stamp + "T12:00:00+09:00")


def day(report, stamp):
    return next(row for row in report["days"] if row["decision_at"].startswith(stamp))


def monday(row):
    return next(s for s in row["devices"][0]["approval_states"] if s["weekday"] == 0)


def version(stamp, start):
    return dict(
        recorded_at=datetime.fromisoformat(stamp),
        valid_from=datetime.fromisoformat(stamp),
        schedules=(Schedule(0, start, 1380, Decimal(24)),),
    )


class EffectiveApprovalHistoryTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        (ROOT / "work").mkdir(exist_ok=True)
        cls.directory = TemporaryDirectory(dir=ROOT / "work")
        cls.dataset = make_approval_history(Path(cls.directory.name))
        cls.report = replay_customer(cls.dataset, "G003")

    @classmethod
    def tearDownClass(cls):
        cls.directory.cleanup()

    def test_pending_applied_then_edited_keeps_old_approval_applied(self):
        accepted = next(
            m
            for m in self.dataset.customers["G003"]["card_responses"][0]["members"]
            if m["weekday"] == 0
        )
        for stamp, current, status, applied in (
            ("2026-08-25", "18:30", "approved_pending", None),
            ("2026-08-27", "20:00", "applied", "2026-08-27T00:00:00+09:00"),
            ("2026-09-01", "21:00", "applied", "2026-08-27T00:00:00+09:00"),
            ("2026-10-31", "21:00", "applied", "2026-08-27T00:00:00+09:00"),
        ):
            with self.subTest(stamp=stamp):
                row = day(self.report, stamp)
                state = monday(row)
                self.assertEqual(state["current_schedule"]["start"], current)
                self.assertEqual(state["values"], accepted["values"])
                self.assertEqual(state["proposal_id"], accepted["proposal_id"])
                self.assertEqual(state["status"], status)
                self.assertEqual(state["applied_at"], applied)
                self.assertEqual(state["superseded"], current == "21:00")
                current_plan = next(
                    p
                    for p in row["devices"][0]["current_schedules"]
                    if p["weekday"] == 0
                )
                self.assertEqual(current_plan, state["current_schedule"])

    def test_snapshot_retains_history_and_current_plan_from_daily_replay(self):
        snapshot = replay_customer(self.dataset, "G003", mode="snapshot")
        self.assertEqual(len(snapshot["days"]), 1)
        self.assertEqual(snapshot["days"], self.report["days"][-1:])
        state = monday(snapshot["days"][0])
        self.assertEqual(
            (
                state["current_schedule"]["start"],
                state["values"]["start"],
                state["status"],
            ),
            ("21:00", "20:00", "applied"),
        )

    def test_new_candidate_with_old_target_is_not_overwritten_or_silenced(self):
        row = day(self.report, "2026-09-01")
        state = monday(row)
        item = next(
            i
            for i in row["devices"][0]["decisions"]
            if i["kind"] == "start" and i["weekday"] == 0
        )
        self.assertEqual(item["status"], "proposed")
        self.assertEqual(item["values"]["start"], "20:00")
        self.assertNotIn("approval", item)
        self.assertNotEqual(item["proposal_id"], state["proposal_id"])
        member = next(
            m
            for c in row["cards"]
            for m in c["members"]
            if m["proposal_id"] == item["proposal_id"]
        )
        self.assertEqual(member["current"]["start"], "21:00")
        self.assertEqual(member["values"]["start"], "20:00")
        self.assertTrue(row["notification"]["scheduled"])
        self.assertIn(
            item["proposal_id"],
            [m["proposal_id"] for m in row["notification"]["targets"]],
        )
        self.assertEqual(state["status"], "applied")

    def test_old_approval_does_not_overwrite_an_unchanged_or_insufficient_decision(
        self,
    ):
        row = day(self.report, "2026-10-31")
        state = monday(row)
        item = next(
            i
            for i in row["devices"][0]["decisions"]
            if i["kind"] == "start" and i["weekday"] == 0
        )
        self.assertIn(item["status"], ("unchanged", "insufficient"))
        self.assertNotIn("start", item["values"])
        original = copy.deepcopy(item)
        self.assertFalse(apply_card_approval(item, [state]))
        self.assertEqual(item, original)

    def test_future_matching_version_is_not_used_for_application_or_current_values(
        self,
    ):
        dataset = single(self.dataset, "G003")
        versions = dataset.customers["G003"]["devices"][0]["schedule_versions"]
        next(p for p in versions[1]["schedules"] if p["weekday"] == 0)["start"] = (
            "21:00"
        )
        prepared = dataset.prepare("G003")
        snapshot = prepared.snapshot(at("2026-10-31"))
        plans = {
            s["weekday"]: s
            for s in self.report["days"][-1]["devices"][0]["current_schedules"]
        }
        states = card_approval_states(
            known_feedback(dataset.customers["G003"], at("2026-10-31")),
            "G003-AC1",
            plans,
            versions=snapshot.schedule_versions["G003-AC1"],
            at=at("2026-10-31"),
        )
        state = next(s for s in states if s["weekday"] == 0)
        self.assertEqual(state["status"], "approved_pending")
        self.assertIsNone(state["applied_at"])
        self.assertEqual(state["current_schedule"]["start"], "21:00")

    def test_editing_future_versions_does_not_rewrite_any_daily_or_snapshot_result(
        self,
    ):
        dataset = single(self.dataset, "G003")
        future = dataset.customers["G003"]["devices"][0]["schedule_versions"][-1]
        future["recorded_at"] = "2026-08-24T12:10:00+09:00"
        for plan in future["schedules"]:
            plan.update(start="22:00", end="23:45", temperature_c=25)
        report = replay_customer(dataset, "G003")
        self.assertEqual(report["days"], self.report["days"])
        snapshot = replay_customer(dataset, "G003", mode="snapshot")
        self.assertEqual(snapshot["days"], self.report["days"][-1:])

    def test_both_html_modes_embed_current_21_and_applied_20_separately(self):
        snapshot = replay_customer(self.dataset, "G003", mode="snapshot")
        with TemporaryDirectory(dir=ROOT / "work") as folder:
            for index, report in enumerate((self.report, snapshot)):
                path = Path(folder) / f"{index}.html"
                customer_html(report, path)
                decoded = json.loads(Scripts(path.read_text()).data)
                state = monday(decoded["days"][-1])
                self.assertEqual(state["status"], "applied")
                self.assertEqual(state["current_schedule"]["start"], "21:00")
                self.assertEqual(state["values"]["start"], "20:00")
                self.assertIn("承認履歴の状態", path.read_text())
                self.assertIn("判定時点の現行予定", path.read_text())


class ApplicationIntervalTests(unittest.TestCase):
    def setUp(self):
        self.values = dict(start="20:00", end="23:00", temperature_c="24")
        self.approved = datetime.fromisoformat("2026-08-24T12:05:00+09:00")

    def test_matching_version_ended_before_approval_does_not_count(self):
        versions = [
            version("2026-08-02T00:00:00+09:00", 1200),
            version("2026-08-20T00:00:00+09:00", 1110),
        ]
        self.assertIsNone(
            first_application(self.values, 0, self.approved, versions, at("2026-10-31"))
        )

    def test_existing_matching_version_counts_from_acceptance_not_before_it(self):
        versions = [
            version("2026-08-02T00:00:00+09:00", 1200),
            version("2026-09-01T00:00:00+09:00", 1260),
        ]
        self.assertEqual(
            first_application(
                self.values, 0, self.approved, versions, at("2026-10-31")
            ),
            self.approved.isoformat(),
        )

    def test_matching_interval_between_two_daily_decisions_is_preserved(self):
        versions = [
            version("2026-08-02T00:00:00+09:00", 1110),
            version("2026-08-26T14:00:00+09:00", 1200),
            version("2026-08-26T16:00:00+09:00", 1260),
        ]
        self.assertIsNone(
            first_application(self.values, 0, self.approved, versions, at("2026-08-26"))
        )
        self.assertEqual(
            first_application(
                self.values, 0, self.approved, versions, at("2026-08-27")
            ),
            "2026-08-26T14:00:00+09:00",
        )

    def test_matching_start_requires_matching_end_and_temperature(self):
        for field, value in (("end", "22:30"), ("temperature_c", "25")):
            values = dict(self.values, **{field: value})
            self.assertIsNone(
                first_application(
                    values,
                    0,
                    self.approved,
                    [version("2026-08-27T00:00:00+09:00", 1200)],
                    at("2026-08-28"),
                )
            )

    def test_later_approval_does_not_remove_previously_applied_history(self):
        original = dict(
            id="old",
            origin="card_response",
            response="approved",
            device_id="AC1",
            kind="start",
            weekday=0,
            proposed_at="2026-08-24T12:00:00+09:00",
            responded_at=self.approved.isoformat(),
            proposal_id="P1",
            values=self.values,
        )
        later = dict(
            original,
            id="new",
            proposal_id="P2",
            proposed_at="2026-09-01T12:00:00+09:00",
            responded_at="2026-09-01T12:05:00+09:00",
            values=dict(self.values, start="22:00"),
        )
        plans = {0: dict(weekday=0, start="21:00", end="23:00", temperature_c="24")}
        states = card_approval_states(
            [original, later],
            "AC1",
            plans,
            versions=[
                version("2026-08-27T00:00:00+09:00", 1200),
                version("2026-09-01T00:00:00+09:00", 1260),
            ],
            at=at("2026-09-02"),
        )
        self.assertEqual(
            [(s["proposal_id"], s["status"]) for s in states],
            [("P1", "applied"), ("P2", "approved_pending")],
        )


if __name__ == "__main__":
    unittest.main()
