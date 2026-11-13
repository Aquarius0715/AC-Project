"""Episode anchors, displayed feedback values, and varied scheduled end times."""

import copy
import unittest
from datetime import date
from decimal import Decimal
from unittest.mock import patch

from automation.src.dataset import load_dataset
from automation.src.monthly_engine import result
from automation.src.replay import replay_customer
from automation.src.simulation import measured_energy
from test_dataset import evaluate, single


def candidate(start="20:15", end="23:00", temperature="24", basis="same-history"):
    item = result(
        "start",
        2,
        "proposed",
        f"計算候補{start}",
        dict(start=start, end=end, temperature_c=temperature),
        ["日付付きの操作根拠"],
        ["manual-evidence"],
        basis,
        3,
    )
    item.update(device_id="H008-AC1", reason_code="start:proposed")
    return item


class StartEpisodeTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.dataset = load_dataset()

    def replay(self, candidates, feedback=None, tolerance=15):
        dataset = single(self.dataset, "H008")
        dataset.customers["H008"]["feedback"] = feedback or []
        plans = dataset.customers["H008"]["devices"][0]["schedule_versions"][0][
            "schedules"
        ]
        position = 0

        def daily(*args, **kwargs):
            nonlocal position
            item = copy.deepcopy(candidates[min(position, len(candidates) - 1)])
            position += 1
            if item is None:
                item = candidate()
                item.update(status="insufficient", reason="履歴不足")
            return dict(
                devices=[
                    dict(
                        device_id="H008-AC1",
                        current_schedules=plans,
                        decisions=[item],
                        proposals=[item] if item["status"] == "proposed" else [],
                    )
                ]
            )

        with patch("automation.src.replay.evaluate_household", side_effect=daily):
            return replay_customer(dataset, "H008", start_tolerance_minutes=tolerance)

    def test_zero_tolerance_reproduces_previous_exact_matching(self):
        candidates = [candidate(), candidate("20:00"), candidate()]
        exact = self.replay(candidates, tolerance=0)
        stable = self.replay(candidates)
        self.assertEqual(exact["new_proposal_count"], 3)
        self.assertEqual(stable["new_proposal_count"], 1)
        self.assertEqual(exact["start_tolerance_minutes"], 0)
        for old, new in zip(exact["days"], stable["days"]):
            self.assertEqual(
                old["events"][0]["candidate_values"],
                new["events"][0]["candidate_values"],
            )

    def test_initial_schedule_proposals_are_not_smoothed(self):
        candidates = [candidate(), candidate("20:00")]
        for item in candidates:
            item["kind"] = "add_schedule"
        report = self.replay(candidates)
        self.assertEqual(report["new_proposal_count"], 2)
        self.assertEqual(report["days"][1]["events"][0]["values"]["start"], "20:00")

    def test_fifteen_minute_boundary_in_both_directions(self):
        for shifted, event in (
            ("20:00", "continued"),
            ("20:30", "continued"),
            ("19:59", "changed"),
            ("20:31", "changed"),
        ):
            with self.subTest(shifted=shifted):
                report = self.replay([candidate(), candidate(shifted)])
                first, second = [r["events"][0] for r in report["days"][:2]]
                self.assertEqual(second["event"], event)
                self.assertEqual(
                    first["proposal_id"] == second["proposal_id"], event == "continued"
                )
                self.assertEqual(
                    second["values"]["start"],
                    "20:15" if event == "continued" else shifted,
                )
                self.assertEqual(second["candidate_values"]["start"], shifted)

    def test_oscillation_keeps_id_displayed_values_and_daily_calculation(self):
        report = self.replay([candidate(), candidate("20:00"), candidate()])
        self.assertEqual(report["new_proposal_count"], 1)
        rows = report["days"][:3]
        events = [r["events"][0] for r in rows]
        self.assertEqual(
            [e["event"] for e in events], ["first", "continued", "continued"]
        )
        self.assertEqual(len({e["proposal_id"] for e in events}), 1)
        self.assertEqual([e["values"]["start"] for e in events], ["20:15"] * 3)
        self.assertEqual(
            [e["candidate_values"]["start"] for e in events],
            ["20:15", "20:00", "20:15"],
        )
        for row, event in zip(rows, events):
            item = row["devices"][0]["decisions"][0]
            self.assertEqual(item["values"], event["values"])
            self.assertEqual(item["proposal_id"], event["proposal_id"])
            self.assertEqual(item["first_displayed_at"], rows[0]["decision_at"])

    def test_gradual_drift_is_measured_from_episode_first_display(self):
        report = self.replay(
            [candidate(s) for s in ("20:00", "20:10", "20:20", "20:30", "20:40")]
        )
        events = [r["events"][0] for r in report["days"][:5]]
        self.assertEqual(
            [e["event"] for e in events],
            ["first", "continued", "changed", "continued", "changed"],
        )
        self.assertEqual(
            [e["values"]["start"] for e in events],
            ["20:00", "20:00", "20:20", "20:20", "20:40"],
        )
        self.assertEqual(
            [e["stabilization"]["delta_minutes"] for e in events], [0, 10, 20, 10, 20]
        )
        self.assertEqual(report["new_proposal_count"], 3)

    def test_large_change_and_end_or_temperature_change_are_new(self):
        for next_candidate in (
            candidate("21:00"),
            candidate("20:00", end="22:30"),
            candidate("20:00", temperature="23"),
        ):
            with self.subTest(values=next_candidate["values"]):
                report = self.replay([candidate("20:00"), next_candidate])
                first, second = [r["events"][0] for r in report["days"][:2]]
                self.assertEqual(second["event"], "changed")
                self.assertNotEqual(first["proposal_id"], second["proposal_id"])
                self.assertEqual(second["values"], next_candidate["values"])

    def test_an_ended_episode_does_not_supply_a_tolerance_anchor(self):
        report = self.replay([candidate(), None, candidate("20:00")])
        events = [r["events"][0] for r in report["days"][:3]]
        self.assertEqual([e["event"] for e in events], ["first", "ended", "changed"])
        self.assertEqual(events[2]["values"]["start"], "20:00")
        self.assertNotEqual(events[0]["proposal_id"], events[2]["proposal_id"])

    def test_feedback_uses_displayed_values_not_the_drifting_candidate(self):
        before = self.replay([candidate(), candidate("20:00")])
        row = before["days"][1]
        item = row["devices"][0]["decisions"][0]
        for response in ("approved", "declined", "edited"):
            with self.subTest(response=response):
                feedback = dict(
                    id="choice",
                    device_id="H008-AC1",
                    kind="start",
                    weekday=2,
                    proposed_at=row["decision_at"],
                    responded_at="2026-08-04T12:05:00+09:00",
                    response=response,
                    values=item["values"],
                    selected={"start": "20:15"} if response == "edited" else {},
                    evidence_signature=item["evidence_signature"],
                    evidence_ids=item["evidence_ids"],
                    proposal_id=item["proposal_id"],
                )
                report = self.replay([candidate(), candidate("20:00")], [feedback])
                day = report["days"][2]
                decision = day["devices"][0]["decisions"][0]
                self.assertEqual(day["responses"][0]["values"], item["values"])
                self.assertEqual(
                    day["responses"][0]["proposal_id"], item["proposal_id"]
                )
                self.assertEqual(decision["values"]["start"], "20:15")
                self.assertEqual(decision["proposal_id"], item["proposal_id"])
                if response == "declined":
                    self.assertEqual(decision["status"], "suppressed")
                    self.assertEqual(day["events"][0]["event"], "ended")
                    self.assertEqual(report["new_proposal_count"], 1)
                else:
                    self.assertEqual(decision["status"], "proposed")
                    self.assertEqual(day["events"][0]["event"], "continued")

    def test_supported_explicit_edit_overrides_smoothing_but_unsupported_edit_does_not(
        self,
    ):
        before = self.replay([candidate(), candidate("20:00")])
        item = before["days"][1]["devices"][0]["decisions"][0]
        for selected, status in (("20:00", "proposed"), ("19:00", "insufficient")):
            feedback = dict(
                id="edited-choice",
                device_id="H008-AC1",
                kind="start",
                weekday=2,
                proposed_at="2026-08-04T12:00:00+09:00",
                responded_at="2026-08-04T12:05:00+09:00",
                response="edited",
                values=item["values"],
                selected={"start": selected},
                evidence_signature=item["evidence_signature"],
                evidence_ids=item["evidence_ids"],
            )
            report = self.replay([candidate(), candidate("20:00")], [feedback])
            decision = report["days"][2]["devices"][0]["decisions"][0]
            self.assertEqual(decision["status"], status)
            if status == "proposed":
                self.assertEqual(decision["values"]["start"], selected)
                self.assertEqual(
                    decision["stabilization"]["override"], "explicit_choice"
                )
                self.assertNotEqual(decision["proposal_id"], item["proposal_id"])
            else:
                self.assertEqual(decision["requested_values"]["start"], selected)


class VariedEndTimeTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.dataset = load_dataset()

    def test_scheduled_automatic_off_and_energy_match_each_days_end(self):
        ends = set()
        for household in self.dataset.customers.values():
            for device in household["devices"]:
                for row in device["days"]:
                    versions = [
                        v
                        for v in device["schedule_versions"]
                        if v["valid_from"][:10] <= row["date"]
                    ]
                    plan = next(
                        (
                            p
                            for p in versions[-1]["schedules"]
                            if p["weekday"] == date.fromisoformat(row["date"]).weekday()
                        ),
                        None,
                    )
                    if not plan:
                        continue
                    ends.add(plan["end"])
                    automatic = [
                        o
                        for o in row["operations"]
                        if o["action"] == "off" and o["source"] == "automation"
                    ]
                    self.assertTrue(
                        all(o["at"][11:16] == plan["end"] for o in automatic)
                    )
                    if automatic:
                        index = int(plan["end"][:2]) * 2 + int(plan["end"][3:]) // 30
                        self.assertTrue(
                            all(
                                k is None or Decimal(k) == 0 for k in row["kwh"][index:]
                            )
                        )
                        self.assertTrue(
                            all(
                                o["power"]["before"] and not o["power"]["after"]
                                for o in automatic
                            )
                        )
        self.assertEqual(ends, {"22:00", "22:30", "23:00", "23:30"})

    def test_energy_is_reintegrated_from_operations_and_weather(self):
        for hid in ("H001", "H008", "H015", "H020"):
            house = self.dataset.customers[hid]
            weather = self.dataset.weather[house["installation"]["weather_site_id"]]
            for dev in house["devices"]:
                for row, observed in zip(dev["days"], weather["observations"]):
                    expected = measured_energy(
                        date.fromisoformat(row["date"]),
                        row["operations"],
                        observed["outdoor_c"],
                        Decimal(1),
                    )
                    self.assertEqual(row["kwh"], expected, (hid, row["date"]))

    def test_start_only_proposals_preserve_original_end_and_temperature(self):
        preserved = set()
        for hid in ("H002", "H008", "H015"):
            report = evaluate(self.dataset, hid)
            for dev in report["devices"]:
                plans = {p["weekday"]: p for p in dev["current_schedules"]}
                for item in dev["decisions"]:
                    if item["kind"] != "start" or item["status"] != "proposed":
                        continue
                    plan = plans[item["weekday"]]
                    self.assertEqual(item["values"]["end"], plan["end"])
                    self.assertEqual(
                        item["values"]["temperature_c"], plan["temperature_c"]
                    )
                    self.assertNotEqual(item["values"]["start"], plan["start"])
                    preserved.add(plan["end"])
        self.assertEqual(preserved, {"22:00", "22:30", "23:00", "23:30"})

    def test_replay_preserves_varied_ends_on_every_active_start_proposal(self):
        report = replay_customer(self.dataset, "H008")
        for day in report["days"]:
            for dev in day["devices"]:
                plans = {p["weekday"]: p for p in dev["current_schedules"]}
                for item in dev["decisions"]:
                    if item["kind"] == "start" and item["status"] == "proposed":
                        self.assertEqual(
                            item["values"]["end"], plans[item["weekday"]]["end"]
                        )
        wednesday = [
            e
            for day in report["days"]
            for e in day["events"]
            if e["kind"] == "start" and e["weekday"] == 2 and e["is_new"]
        ]
        self.assertEqual([e["values"]["start"] for e in wednesday], ["20:00", "21:00"])
        self.assertTrue(all(e["values"]["end"] == "22:30" for e in wednesday))
