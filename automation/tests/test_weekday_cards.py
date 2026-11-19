"""Per-weekday evidence/consent survives grouping and household delivery limits."""

import copy
import unittest
from datetime import date, datetime, timedelta
from decimal import Decimal
from pathlib import Path
from tempfile import TemporaryDirectory

from automation.src.dataset import ROOT, load_dataset
from automation.src.generate_weekday_demo import make_weekday_demo
from automation.src.loader import InputError
from automation.src.notifications import NotificationPlanner
from automation.src.replay import replay_customer
from automation.src.simulation import measured_energy
from automation.src.weekday_cards import build_cards
from test_dataset import single


DEMO = ROOT / "data" / "weekday-demo"


def day(report, stamp):
    return next(d for d in report["days"] if d["decision_at"].startswith(stamp))


def starts(row, weekday=None):
    return [
        i
        for d in row["devices"]
        for i in d["decisions"]
        if i["kind"] == "start" and (weekday is None or i["weekday"] == weekday)
    ]


def at(stamp):
    return datetime.fromisoformat(stamp + "T12:00:00+09:00")


def sample(weekday=0, start="20:00", device="AC1", proposal_id=None):
    return dict(
        device_id=device,
        kind="start",
        weekday=weekday,
        proposal_id=proposal_id or f"P{weekday}-{start}",
        values=dict(start=start, end="23:00", temperature_c="24"),
        evidence_ids=["new-operation"],
        evidence_signature="basis",
        support_count=3,
        first_displayed_at="2026-08-03T12:00:00+09:00",
    )


def delivery(planner, stamp, members, known=(), new=True):
    cards = [dict(card_id="container", members=members)] if members else []
    events = [
        dict(m, event="first" if new else "continued", is_new=new) for m in members
    ]
    return planner.evaluate(at(stamp), cards, events, known)


class CardGroupingTests(unittest.TestCase):
    def report(self, members):
        devices = {}
        for member in members:
            device = devices.setdefault(
                member["device_id"], dict(current_schedules=[], decisions=[])
            )
            plan = dict(
                weekday=member["weekday"],
                start="18:30",
                end=member["values"]["end"],
                temperature_c="24",
            )
            device["current_schedules"].append(plan)
            item = dict(
                member,
                status=member.get("status", "proposed"),
                evidence=["2026-08-03: 20:00 / complete:used"],
            )
            device["decisions"].append(item)
        return dict(devices=list(devices.values()))

    def test_all_workdays_and_both_weekend_days_are_two_cards(self):
        members = [sample(w) for w in range(7)]
        members[1]["values"]["end"] = "22:00"
        cards = build_cards(self.report(members), "test")
        self.assertEqual(len(cards), 2)
        self.assertEqual({c["label"] for c in cards}, {"平日すべて", "土日まとめて"})
        weekday = next(c for c in cards if c["segment"] == "weekday")
        self.assertEqual(weekday["members"][1]["values"]["end"], "22:00")
        self.assertEqual(
            {m["proposal_id"] for c in cards for m in c["members"]},
            {m["proposal_id"] for m in members},
        )

    def test_different_times_devices_and_noneligible_days_do_not_merge(self):
        members = [sample(w) for w in range(5)]
        members[2]["status"] = "fixed"
        members[1]["values"]["start"] = "20:15"
        members[4]["status"] = "insufficient"
        members.append(sample(0, device="AC2"))
        cards = build_cards(self.report(members), "test")
        self.assertEqual(len(cards), 3)
        card = next(
            c
            for c in cards
            if c["device_id"] == "AC1" and c["members"][0]["values"]["start"] == "20:00"
        )
        self.assertEqual(card["label"], "月・木")
        self.assertEqual(card["weekdays"], [0, 3])

    def test_group_membership_does_not_replace_weekday_identity(self):
        original = build_cards(self.report([sample(0), sample(1), sample(2)]), "test")[
            0
        ]
        changed = build_cards(self.report([sample(0), sample(2), sample(4)]), "test")[0]
        self.assertEqual(original["card_id"], changed["card_id"])
        self.assertEqual(
            original["members"][0]["proposal_id"], changed["members"][0]["proposal_id"]
        )
        self.assertEqual(
            original["members"][0]["values"], changed["members"][0]["values"]
        )


class NotificationTests(unittest.TestCase):
    def test_new_valid_targets_wait_until_next_week_and_are_not_lost(self):
        planner = NotificationPlanner()
        monday = sample()
        tuesday = sample(1, "21:00")
        self.assertTrue(delivery(planner, "2026-08-03", [monday])["scheduled"])
        pending = delivery(planner, "2026-08-04", [monday, tuesday])
        self.assertFalse(pending["scheduled"])
        self.assertEqual([m["weekday"] for m in pending["pending"]], [1])
        following = delivery(planner, "2026-08-10", [monday, tuesday], new=False)
        self.assertTrue(following["scheduled"])
        self.assertEqual(
            [m["proposal_id"] for m in following["targets"]], [tuesday["proposal_id"]]
        )
        self.assertEqual(
            following["targets"][0]["first_waiting_at"], "2026-08-04T12:00:00+09:00"
        )
        self.assertFalse(
            delivery(planner, "2026-08-17", [monday, tuesday], new=False)["scheduled"]
        )

    def test_continuation_no_response_and_one_added_support_day_do_not_remind(self):
        planner = NotificationPlanner()
        member = sample()
        delivery(planner, "2026-08-03", [member])
        changed = dict(
            member,
            proposal_id="reappeared-ID",
            evidence_ids=["one-more-day"],
            support_count=4,
        )
        response = dict(member, response="no_response")
        self.assertFalse(
            delivery(planner, "2026-08-10", [changed], [response])["scheduled"]
        )

    def test_declined_choice_needs_material_change_and_drift_uses_sent_anchor(self):
        planner = NotificationPlanner()
        declined = dict(sample(), response="declined", selected={})
        self.assertFalse(
            delivery(planner, "2026-08-03", [sample()], [declined])["scheduled"]
        )
        self.assertTrue(
            delivery(planner, "2026-08-04", [sample(start="21:00")], [declined])[
                "scheduled"
            ]
        )
        self.assertFalse(
            delivery(planner, "2026-08-10", [sample(start="21:10")], [declined])[
                "scheduled"
            ]
        )
        material = delivery(planner, "2026-08-11", [sample(start="21:20")], [declined])
        self.assertTrue(material["scheduled"])
        self.assertEqual(material["targets"][0]["values"]["start"], "21:20")

    def test_invalidated_pending_targets_are_removed(self):
        planner = NotificationPlanner()
        delivery(planner, "2026-08-03", [sample()])
        delivery(planner, "2026-08-04", [sample(), sample(1)])
        gone = delivery(planner, "2026-08-05", [sample()], new=False)
        self.assertEqual(gone["pending"], [])
        self.assertEqual([m["weekday"] for m in gone["dropped"]], [1])
        self.assertFalse(
            delivery(planner, "2026-08-10", [sample()], new=False)["scheduled"]
        )

    def test_week_is_monday_local_time_and_digest_covers_devices(self):
        planner = NotificationPlanner()
        sunday = delivery(
            planner, "2026-08-09", [sample(0, device="AC1"), sample(1, device="AC2")]
        )
        self.assertEqual(sunday["week_start"], "2026-08-03")
        self.assertEqual(len(sunday["targets"]), 2)
        monday = delivery(planner, "2026-08-10", [sample(2, device="AC1")])
        self.assertTrue(monday["scheduled"])
        self.assertEqual(monday["week_start"], "2026-08-10")
        self.assertTrue(
            delivery(NotificationPlanner(), "2026-08-09", [sample()])["scheduled"]
        )


class WeekdayReplayTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.dataset = load_dataset(DEMO)
        cls.reports = {
            hid: replay_customer(cls.dataset, hid) for hid in cls.dataset.customers
        }

    def test_first_full_workweek_and_weekend_have_exact_evidence(self):
        workday = next(
            c
            for c in day(self.reports["G001"], "2026-08-22")["cards"]
            if c["segment"] == "weekday"
        )
        self.assertEqual(workday["label"], "平日すべて")
        self.assertEqual(workday["weekdays"], [0, 1, 2, 3, 4])
        friday = workday["members"][-1]
        self.assertEqual(
            friday["support_dates"], ["2026-08-07", "2026-08-14", "2026-08-21"]
        )
        self.assertEqual(friday["support_count"], 3)
        self.assertFalse(
            any(
                c["label"] == "平日すべて"
                for r in self.reports["G001"]["days"]
                if r["decision_at"] < "2026-08-22"
                for c in r["cards"]
            )
        )
        weekend = next(
            c
            for c in day(self.reports["G001"], "2026-08-23")["cards"]
            if c["segment"] == "weekend"
        )
        self.assertEqual(weekend["label"], "土日まとめて")
        self.assertEqual(weekend["weekdays"], [5, 6])

    def test_wednesday_fix_is_causal_and_last_until_explicit_unlock(self):
        report = self.reports["G002"]
        self.assertEqual(starts(day(report, "2026-08-25"), 2)[0]["status"], "proposed")
        for stamp in ("2026-08-26", "2026-09-01", "2026-09-15"):
            row = day(report, stamp)
            self.assertEqual(starts(row, 2)[0]["status"], "fixed")
            weekdays = next(c for c in row["cards"] if c["segment"] == "weekday")
            self.assertEqual(weekdays["label"], "月・火・木・金")
        self.assertEqual(starts(day(report, "2026-09-16"), 2)[0]["status"], "proposed")

    def test_partial_approval_changes_only_selected_weekdays_and_once_is_not_fixed(
        self,
    ):
        report = self.reports["G003"]
        previous = day(report, "2026-08-24")
        self.assertFalse(previous["responses"])
        row = day(report, "2026-08-25")
        self.assertEqual([m["weekday"] for m in row["approval_targets"]], [0, 1, 3, 4])
        plans = {p["weekday"]: p for p in row["devices"][0]["current_schedules"]}
        self.assertEqual(
            [plans[w]["start"] for w in range(5)],
            ["20:00", "20:00", "18:30", "20:00", "20:00"],
        )
        self.assertEqual(starts(row, 2)[0]["status"], "proposed")
        self.assertTrue(
            any(
                r["response"] == "excluded_once" and r["weekday"] == 2
                for r in row["responses"]
            )
        )
        self.assertEqual(row["start_controls"], [])
        for response in row["responses"]:
            shown = next(
                m
                for c in previous["cards"]
                for m in c["members"]
                if m["proposal_id"] == response["proposal_id"]
            )
            self.assertEqual(shown["values"], response["values"])

    def test_declined_day_is_not_reintroduced_by_evidence_or_group_changes(self):
        report = self.reports["G004"]
        for stamp in ("2026-08-25", "2026-09-03", "2026-09-18"):
            row = day(report, stamp)
            self.assertFalse(
                any(
                    m["weekday"] == 2
                    for c in row["cards"]
                    if c["kind"] == "start"
                    for m in c["members"]
                )
            )
        changed = day(report, "2026-10-01")
        item = starts(changed, 2)[0]
        self.assertEqual(item["values"]["start"], "21:00")
        self.assertEqual(item["status"], "proposed")
        self.assertEqual(item["support_count"], 3)
        self.assertEqual(
            next(c for c in changed["cards"] if 2 in c["weekdays"])["label"], "水・木"
        )
        self.assertTrue(changed["notification"]["scheduled"])
        self.assertTrue(
            any(
                m["weekday"] == 2 and m["values"]["start"] == "21:00"
                for m in changed["notification"]["targets"]
            )
        )

    def test_different_displayed_weekday_times_remain_separate_even_at_15_minutes(self):
        cards = day(self.reports["G004"], "2026-08-24")["cards"]
        work = [c for c in cards if c["segment"] == "weekday"]
        self.assertEqual(len(work), 2)
        self.assertEqual(
            {c["members"][0]["values"]["start"] for c in work}, {"20:00", "20:15"}
        )

    def test_weekly_max_no_reminders_and_carryover_in_real_replay(self):
        for report in self.reports.values():
            seen = set()
            for row in report["days"]:
                notice = row["notification"]
                if notice["scheduled"]:
                    self.assertNotIn(notice["week_start"], seen)
                    seen.add(notice["week_start"])
        report = self.reports["G001"]
        notices = [r for r in report["days"] if r["notification"]["scheduled"]]
        self.assertEqual(
            [r["decision_at"][:10] for r in notices], ["2026-08-17", "2026-08-24"]
        )
        carried = day(report, "2026-08-24")["notification"]["targets"]
        self.assertEqual({m["weekday"] for m in carried}, {0, 1, 2, 3, 4, 5})
        self.assertTrue(all(m["first_waiting_at"] < "2026-08-24" for m in carried))

    def test_future_reactions_and_lock_receipts_cannot_change_past_rows(self):
        dataset = single(self.dataset, "G002")
        dataset.customers["G002"]["start_controls"][0]["received_at"] = (
            "2026-09-01T12:05:00+09:00"
        )
        changed = replay_customer(dataset, "G002")
        original = self.reports["G002"]
        self.assertEqual(
            [r for r in original["days"] if r["decision_at"] < "2026-08-26"],
            [r for r in changed["days"] if r["decision_at"] < "2026-08-26"],
        )
        self.assertEqual(starts(day(changed, "2026-08-26"), 2)[0]["status"], "proposed")
        dataset = single(self.dataset, "G003")
        dataset.customers["G003"]["card_responses"][0]["members"][0]["decision"] = (
            "excluded_once"
        )
        changed = replay_customer(dataset, "G003")
        self.assertEqual(
            [r for r in changed["days"] if r["decision_at"] < "2026-08-25"],
            [
                r
                for r in self.reports["G003"]["days"]
                if r["decision_at"] < "2026-08-25"
            ],
        )

    def test_reference_tampering_and_duplicate_weekdays_fail_validation(self):
        for field in ("proposal_id", "values", "duplicate", "evidence_ids"):
            dataset = single(self.dataset, "G003")
            members = dataset.customers["G003"]["card_responses"][0]["members"]
            if field == "proposal_id":
                members[0]["proposal_id"] = "someone-elses-ID"
            elif field == "values":
                members[0]["values"]["start"] = "21:00"
            elif field == "duplicate":
                members.append(copy.deepcopy(members[0]))
            else:
                members[0]["evidence_ids"] = ["unrelated-operation"]
            with self.assertRaises(InputError):
                replay_customer(dataset, "G003")

    def test_reference_accepts_equivalent_timestamp_offsets(self):
        dataset = single(self.dataset, "G003")
        dataset.customers["G003"]["card_responses"][0]["proposed_at"] = (
            "2026-08-24T03:00:00+00:00"
        )
        report = replay_customer(dataset, "G003")
        self.assertEqual(
            day(report, "2026-08-25")["approval_targets"],
            day(self.reports["G003"], "2026-08-25")["approval_targets"],
        )

    def test_snapshot_with_card_responses_keeps_validated_historical_ids(self):
        snapshot = replay_customer(self.dataset, "G003", mode="snapshot")
        self.assertEqual(snapshot["mode"], "snapshot")
        self.assertEqual(snapshot["days"], self.reports["G003"]["days"][-1:])
        self.assertEqual(snapshot["card_day_count"], snapshot["days"][0]["card_count"])
        self.assertEqual(
            snapshot["new_proposal_count"], snapshot["days"][0]["new_count"]
        )

    def test_partial_approval_does_not_permanently_fix_a_new_manual_pattern(self):
        dataset = single(self.dataset, "G003")
        device = dataset.customers["G003"]["devices"][0]
        weather = dataset.weather[device["site_id"]]["observations"]
        for row, observed in zip(device["days"], weather):
            if (
                row["date"] < "2026-09-01"
                or date.fromisoformat(row["date"]).weekday() != 0
            ):
                continue
            manual = next(o for o in row["operations"] if o["source"] == "web_manual")
            before = copy.deepcopy(row["operations"][-1])
            before.update(id=before["id"] + "-manual", source="web_manual")
            for operation, clock, state_before, state_after in (
                (before, "20:30", True, False),
                (manual, "21:00", False, True),
            ):
                stamp = datetime.fromisoformat(row["date"] + "T" + clock + ":00+09:00")
                operation.update(
                    at=stamp.isoformat(),
                    received_at=(stamp + timedelta(seconds=2)).isoformat(),
                    power=dict(
                        before=state_before,
                        after=state_after,
                        before_at=(stamp - timedelta(seconds=1)).isoformat(),
                        after_at=(stamp + timedelta(seconds=1)).isoformat(),
                        received_at=(stamp + timedelta(seconds=2)).isoformat(),
                    ),
                )
            row["operations"].append(before)
            row["operations"].sort(key=lambda o: o["at"])
            row["kwh"] = measured_energy(
                date.fromisoformat(row["date"]),
                row["operations"],
                observed["outdoor_c"],
                Decimal(1),
            )
        report = replay_customer(dataset, "G003")
        first = next(
            r
            for r in report["days"]
            if any(
                i["status"] == "proposed" and i["values"].get("start") == "21:00"
                for i in starts(r, 0)
            )
        )
        self.assertEqual(first["decision_at"][:10], "2026-09-22")
        member = next(
            m for c in first["cards"] for m in c["members"] if m["weekday"] == 0
        )
        self.assertEqual(
            member["support_dates"], ["2026-09-07", "2026-09-14", "2026-09-21"]
        )
        self.assertEqual(first["start_controls"], [])

    def test_individual_daily_comparison_preserves_weekday_results_and_consent(self):
        report = replay_customer(self.dataset, "G003", presentation="individual-daily")
        grouped = self.reports["G003"]
        self.assertEqual(report["new_proposal_count"], grouped["new_proposal_count"])
        self.assertEqual(
            report["start_weekday_day_count"], grouped["start_weekday_day_count"]
        )
        self.assertGreater(
            report["start_card_day_count"], grouped["start_card_day_count"]
        )
        self.assertEqual(
            day(report, "2026-08-25")["approval_targets"],
            day(grouped, "2026-08-25")["approval_targets"],
        )

    def test_demo_generation_is_reproducible_with_editable_input(self):
        with TemporaryDirectory(dir=ROOT / "work") as folder:
            target = Path(folder)
            make_weekday_demo(target)
            for path in DEMO.rglob("*.json"):
                self.assertEqual(
                    path.read_bytes(), (target / path.relative_to(DEMO)).read_bytes()
                )


if __name__ == "__main__":
    unittest.main()
