"""Review regressions: separate acceptance/application and scope precool to execution date."""

import copy
import unittest
from datetime import datetime
from pathlib import Path
from tempfile import TemporaryDirectory

from automation.src.approval_state import apply_card_approval, card_approval_states
from automation.src.dataset import ROOT, load_dataset
from automation.src.feedback_scope import feedback_matches, resolve_precool_date
from automation.src.generate_review_cases import make_review_cases
from automation.src.loader import InputError
from automation.src.notifications import NotificationPlanner
from automation.src.replay import apply_feedback, known_feedback, replay_customer
from test_dataset import single


DATA = ROOT / "data" / "review-cases"


def at(stamp):
    return datetime.fromisoformat(stamp + "T12:00:00+09:00")


def day(report, stamp):
    return next(r for r in report["days"] if r["decision_at"].startswith(stamp))


def precool(row, target):
    return next(
        i
        for d in row["devices"]
        for i in d["decisions"]
        if i["kind"] == "precool" and i.get("target_date") == target
    )


class ApprovalApplicationTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.dataset = load_dataset(DATA)
        cls.report = replay_customer(cls.dataset, "G003")
        cls.members = {
            m["weekday"]: m
            for m in cls.dataset.customers["G003"]["card_responses"][0]["members"]
        }

    def test_acceptance_pending_and_applied_are_distinct_decision_days(self):
        for stamp, status, current in (
            ("2026-08-24", "proposed", "18:30"),
            ("2026-08-25", "approved_pending", "18:30"),
            ("2026-08-26", "approved_pending", "18:30"),
            ("2026-08-27", "applied", "20:00"),
        ):
            with self.subTest(stamp=stamp):
                row = day(self.report, stamp)
                device = row["devices"][0]
                plans = {s["weekday"]: s for s in device["current_schedules"]}
                decisions = {
                    i["weekday"]: i for i in device["decisions"] if i["kind"] == "start"
                }
                for weekday in (0, 1, 3, 4):
                    self.assertEqual(plans[weekday]["start"], current)
                    self.assertEqual(decisions[weekday]["status"], status)
                    self.assertEqual(
                        decisions[weekday]["proposal_id"],
                        self.members[weekday]["proposal_id"],
                    )
                    self.assertEqual(
                        decisions[weekday]["values"], self.members[weekday]["values"]
                    )
                active = sorted(
                    m["weekday"]
                    for c in row["cards"]
                    if c["kind"] == "start"
                    for m in c["members"]
                )
                self.assertEqual(
                    active, list(range(7)) if status == "proposed" else [2, 5, 6]
                )
                self.assertEqual(plans[2]["start"], "18:30")
                self.assertEqual(decisions[2]["status"], "proposed")
                if status != "proposed":
                    self.assertEqual(
                        {s["weekday"] for s in device["approval_states"]}, {0, 1, 3, 4}
                    )
                    self.assertTrue(
                        all(s["status"] == status for s in device["approval_states"])
                    )
                    candidates = (
                        row["notification"]["targets"] + row["notification"]["pending"]
                    )
                    self.assertFalse(
                        any(m["weekday"] in (0, 1, 3, 4) for m in candidates)
                    )
        self.assertEqual(
            day(self.report, "2026-08-24")["devices"][0]["approval_states"], []
        )

    def test_registered_future_schedule_is_not_applied_until_valid_from(self):
        raw = self.dataset.customers["G003"]["devices"][0]
        self.assertEqual(
            raw["schedule_versions"][1]["recorded_at"], "2026-08-24T12:10:00+09:00"
        )
        self.assertEqual(
            raw["schedule_versions"][1]["valid_from"], "2026-08-27T00:00:00+09:00"
        )
        for stamp in ("2026-08-25", "2026-08-26"):
            self.assertTrue(
                all(
                    s["status"] == "approved_pending"
                    for s in day(self.report, stamp)["devices"][0]["approval_states"]
                )
            )

    def test_matching_start_alone_does_not_mark_different_end_or_temperature_applied(
        self,
    ):
        known = known_feedback(self.dataset.customers["G003"], at("2026-08-27"))
        plans = {
            p["weekday"]: p
            for p in day(self.report, "2026-08-27")["devices"][0]["current_schedules"]
        }
        for field, value in (("end", "23:30"), ("temperature_c", "25")):
            changed = copy.deepcopy(plans)
            changed[0][field] = value
            state = next(
                s
                for s in card_approval_states(known, "G003-AC1", changed)
                if s["weekday"] == 0
            )
            self.assertEqual(state["status"], "approved_pending")

    def test_a_materially_different_candidate_is_not_fixed_by_pending_approval(self):
        states = day(self.report, "2026-08-25")["devices"][0]["approval_states"]
        item = dict(
            kind="start",
            weekday=0,
            status="proposed",
            values=dict(states[0]["values"], start="21:00"),
        )
        self.assertFalse(apply_card_approval(item, states))
        self.assertEqual(item["status"], "proposed")
        self.assertEqual(item["values"]["start"], "21:00")

    def test_approval_state_persists_after_manual_evidence_ages_out(self):
        row = day(self.report, "2026-10-30")
        states = row["devices"][0]["approval_states"]
        self.assertTrue(all(s["status"] == "applied" for s in states))
        self.assertEqual(
            {s["proposal_id"] for s in states},
            {m["proposal_id"] for w, m in self.members.items() if w != 2},
        )

    def test_unreceived_future_application_cannot_change_earlier_decisions(self):
        dataset = single(self.dataset, "G003")
        future = dataset.customers["G003"]["devices"][0]["schedule_versions"][1]
        future.update(
            recorded_at="2026-08-28T12:10:00+09:00",
            valid_from="2026-08-29T00:00:00+09:00",
        )
        report = replay_customer(dataset, "G003")
        self.assertEqual(
            [r for r in report["days"] if r["decision_at"] < "2026-08-27"],
            [r for r in self.report["days"] if r["decision_at"] < "2026-08-27"],
        )
        self.assertTrue(
            all(
                s["status"] == "approved_pending"
                for s in day(report, "2026-08-27")["devices"][0]["approval_states"]
            )
        )


class PrecoolDateScopeTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.dataset = load_dataset(DATA)
        cls.report = replay_customer(cls.dataset, "H011")
        cls.feedback = cls.dataset.customers["H011"]["feedback"][0]

    def test_same_execution_is_suppressed_but_next_mondays_equal_controls_wait_and_notify(
        self,
    ):
        self.assertNotIn("target_date", self.feedback)
        first = precool(day(self.report, "2026-08-10"), "2026-08-10")
        later = precool(day(self.report, "2026-08-14"), "2026-08-17")
        self.assertEqual(first["status"], "suppressed")
        self.assertEqual(later["status"], "proposed")
        self.assertEqual(first["weekday"], later["weekday"])
        self.assertEqual(first["values"], later["values"])
        for stamp in ("2026-08-10", "2026-08-11"):
            notice = day(self.report, stamp)["notification"]
            self.assertFalse(
                any(
                    m["target_date"] == "2026-08-10"
                    for m in notice["targets"] + notice["pending"]
                )
            )
        for stamp in ("2026-08-14", "2026-08-15", "2026-08-16"):
            notice = day(self.report, stamp)["notification"]
            self.assertFalse(notice["scheduled"])
            target = next(
                m for m in notice["pending"] if m["target_date"] == "2026-08-17"
            )
            self.assertEqual(target["first_waiting_at"], "2026-08-14T12:00:00+09:00")
        scheduled = day(self.report, "2026-08-17")["notification"]
        self.assertTrue(scheduled["scheduled"])
        self.assertEqual(
            [m["target_date"] for m in scheduled["targets"]], ["2026-08-17"]
        )
        self.assertEqual(scheduled["scheduled_at"], "2026-08-17T12:00:00+09:00")
        self.assertEqual(scheduled["week_start"], "2026-08-17")
        for row in self.report["days"]:
            if row["notification"]["scheduled"]:
                self.assertEqual(
                    sum(
                        d["notification"]["scheduled"]
                        and d["notification"]["week_start"]
                        == row["notification"]["week_start"]
                        for d in self.report["days"]
                    ),
                    1,
                )

    def test_expired_pending_execution_is_removed_and_never_notified(self):
        queued = day(self.report, "2026-08-14")["notification"]
        self.assertEqual(
            {m["target_date"] for m in queued["pending"]}, {"2026-08-15", "2026-08-17"}
        )
        dropped = day(self.report, "2026-08-16")["notification"]["dropped"]
        old = next(m for m in dropped if m["target_date"] == "2026-08-15")
        self.assertIn("実行対象日が過ぎた", old["drop_reason"])
        self.assertFalse(
            any(
                m["target_date"] == "2026-08-15"
                for row in self.report["days"]
                for m in row["notification"]["targets"]
            )
        )

    def test_legacy_forecast_proposal_id_and_signature_references_are_resolved(self):
        for reference in ("forecast", "proposal_id", "signature"):
            with self.subTest(reference=reference):
                dataset = single(self.dataset, "H011")
                feedback = dataset.customers["H011"]["feedback"][0]
                if reference != "forecast":
                    feedback["evidence_ids"] = []
                if reference != "proposal_id":
                    feedback.pop("proposal_id", None)
                if reference != "signature":
                    feedback.pop("evidence_signature", None)
                report = replay_customer(dataset, "H011")
                row = day(report, "2026-08-10")
                self.assertEqual(row["responses"][0]["target_date"], "2026-08-10")
                self.assertEqual(precool(row, "2026-08-10")["status"], "suppressed")
                self.assertTrue(day(report, "2026-08-17")["notification"]["scheduled"])

    def test_unresolvable_old_response_is_not_reused_for_any_other_execution(self):
        dataset = single(self.dataset, "H011")
        feedback = dataset.customers["H011"]["feedback"][0]
        feedback.update(evidence_ids=[], evidence_signature="unknown")
        feedback.pop("proposal_id")
        report = replay_customer(dataset, "H011")
        row = day(report, "2026-08-10")
        self.assertIsNone(row["responses"][0]["target_date"])
        self.assertEqual(precool(row, "2026-08-10")["status"], "proposed")
        self.assertTrue(
            any(
                "対象日不明" in s
                for s in precool(row, "2026-08-10")["judgment_influence"]
            )
        )
        self.assertTrue(day(report, "2026-08-17")["notification"]["scheduled"])

    def test_execution_date_suppresses_revised_evidence_and_controls_for_that_day_only(
        self,
    ):
        feedback = dict(self.feedback, target_date="2026-08-10")
        raw = dict(feedback=[feedback])
        for target, expected in (
            ("2026-08-10", "suppressed"),
            ("2026-08-17", "proposed"),
        ):
            report = copy.deepcopy(day(self.report, "2026-08-14"))
            item = precool(report, "2026-08-17")
            item.update(
                target_date=target, status="proposed", evidence_signature="new-evidence"
            )
            item["values"]["temperature_c"] = "23"
            apply_feedback(report, raw, at("2026-08-14"))
            self.assertEqual(item["status"], expected)

    def test_notification_matching_uses_device_and_execution_date_not_weekday(self):
        row = day(self.report, "2026-08-17")
        card = copy.deepcopy(next(c for c in row["cards"] if c["kind"] == "precool"))
        events = row["events"]
        for target, device, expected in (
            ("2026-08-10", "H011-AC1", True),
            ("2026-08-17", "H011-AC1", False),
            ("2026-08-17", "another-device", True),
        ):
            feedback = dict(
                self.feedback,
                target_date=target,
                device_id=device,
                weekday=6,
                evidence_ids=[],
            )
            feedback.pop("proposal_id", None)
            feedback.pop("evidence_signature", None)
            result = NotificationPlanner().evaluate(
                at("2026-08-17"), [card], events, [feedback]
            )
            self.assertEqual(result["scheduled"], expected)

    def test_snapshot_can_resolve_historical_proposal_references(self):
        dataset = single(self.dataset, "H011")
        feedback = dataset.customers["H011"]["feedback"][0]
        feedback.update(evidence_ids=[])
        feedback.pop("evidence_signature")
        snapshot = replay_customer(dataset, "H011", mode="snapshot")
        self.assertEqual(snapshot["days"], self.report["days"][-1:])


class FeedbackDateResolutionTests(unittest.TestCase):
    def test_unknown_or_conflicting_refs_do_not_guess_from_proposal_or_response_day(
        self,
    ):
        feedback = dict(
            device_id="AC1",
            kind="precool",
            weekday=0,
            proposed_at="2026-08-09T12:00:00+09:00",
            responded_at="2026-08-09T12:05:00+09:00",
            values={},
        )
        self.assertIsNone(resolve_precool_date(feedback)[0])
        feedback["evidence_ids"] = [
            "forecast:tokyo:2026-08-10",
            "forecast:tokyo:2026-08-17",
        ]
        self.assertIsNone(resolve_precool_date(feedback)[0])
        feedback["target_date"] = "2026-08-10"
        with self.assertRaises(InputError):
            resolve_precool_date(feedback)

    def test_future_proposal_reference_cannot_determine_a_past_response_date(self):
        feedback = dict(
            device_id="AC1",
            kind="precool",
            proposal_id="P1",
            proposed_at="2026-08-09T12:00:00+09:00",
        )
        record = dict(
            device_id="AC1",
            proposal_id="P1",
            target_date="2026-08-17",
            evidence_signature="x",
            at=at("2026-08-16"),
        )
        self.assertIsNone(resolve_precool_date(feedback, [record])[0])

    def test_invalid_explicit_dates_are_rejected(self):
        for value in ("2026-02-30", "20260810", "2026-08-10T17:00:00+09:00", True):
            with self.assertRaises(InputError):
                resolve_precool_date(dict(target_date=value))

    def test_undated_response_does_not_match_a_dated_candidate(self):
        member = dict(
            device_id="AC1", kind="precool", weekday=0, target_date="2026-08-17"
        )
        response = dict(device_id="AC1", kind="precool", weekday=0)
        self.assertFalse(feedback_matches(member, response))


class ReviewDataTests(unittest.TestCase):
    def test_review_inputs_can_be_recreated_without_touching_canonical_inputs(self):
        with TemporaryDirectory(dir=ROOT / "work") as directory:
            output = Path(directory)
            make_review_cases(output)
            for path in DATA.rglob("*.json"):
                self.assertEqual(
                    path.read_bytes(), (output / path.relative_to(DATA)).read_bytes()
                )


if __name__ == "__main__":
    unittest.main()
