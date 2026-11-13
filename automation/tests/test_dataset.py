"""Regression against the new editable customer JSON, not retired fixtures."""

import copy
import unittest
from datetime import datetime

from automation.src.dataset import (
    DEFAULT_DATA,
    Dataset,
    inside_automation,
    load_dataset,
)
from automation.src.loader import InputError
from automation.src.presence_proposals import evaluate_household
from automation.src.replay import apply_feedback, replay_customer


def single(dataset, hid):
    return Dataset(
        dataset.path,
        copy.deepcopy(dataset.config),
        {hid: copy.deepcopy(dataset.customers[hid])},
        copy.deepcopy(dataset.weather),
        copy.deepcopy(dataset.tariffs),
    )


def evaluate(dataset, hid):
    prepared = dataset.prepare(hid)
    at = datetime.fromisoformat(dataset.config["as_of"])
    snapshot = prepared.snapshot(at)
    report = evaluate_household(snapshot, snapshot.households[0], include_billing=False)
    apply_feedback(report, dataset.customers[hid], at)
    return report


def decisions(report, kind):
    return [
        d for dev in report["devices"] for d in dev["decisions"] if d["kind"] == kind
    ]


class DatasetRegressionTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.dataset = load_dataset()

    def test_only_pretty_uncompressed_customer_shared_json(self):
        self.assertEqual(len(self.dataset.customers), 100)
        self.assertFalse(list(DEFAULT_DATA.rglob("*.gz")))
        self.assertEqual(len(list((DEFAULT_DATA / "customers").glob("*.json"))), 100)
        self.assertFalse((DEFAULT_DATA / "monthly").exists())
        self.assertFalse((DEFAULT_DATA / "presence").exists())
        for hid, row in self.dataset.customers.items():
            self.assertIn(
                '\n  "', (DEFAULT_DATA / "customers" / f"{hid}.json").read_text()
            )
            self.assertNotIn("expected", row)
            self.assertNotIn("weather", row)
            self.assertIn(row["installation"]["weather_site_id"], self.dataset.weather)
            self.assertTrue(all(len(dev["days"]) == 90 for dev in row["devices"]))

    def test_early_manual_starts_have_three_real_power_transitions(self):
        report = evaluate(self.dataset, "H015")
        item = next(d for d in decisions(report, "start") if d["weekday"] == 2)
        self.assertEqual(item["status"], "proposed")
        self.assertEqual(item["values"]["start"], "17:30")
        self.assertEqual(item["values"]["end"], "23:00")
        self.assertEqual(item["support_count"], 3)
        self.assertEqual(len(item["evidence_ids"]), 3)

    def test_auto_on_and_temperature_changes_are_not_manual_start_votes(self):
        report = evaluate(self.dataset, "H014")
        self.assertTrue(
            all(d["status"] != "proposed" for d in decisions(report, "start"))
        )
        self.assertTrue(
            all(d["support_count"] == 0 for d in decisions(report, "start"))
        )
        self.assertTrue(
            any(d["status"] == "proposed" for d in decisions(report, "temperature"))
        )

    def test_variable_manual_times_show_dispersion(self):
        report = evaluate(self.dataset, "H016")
        self.assertTrue(
            all(d["status"] != "proposed" for d in decisions(report, "start"))
        )
        self.assertTrue(any("分散" in d["reason"] for d in decisions(report, "start")))

    def test_initial_schedules_need_common_on_off_and_temperature(self):
        report = evaluate(self.dataset, "H017")
        items = decisions(report, "add_schedule")
        self.assertEqual(len(items), 7)
        self.assertTrue(all(d["status"] == "proposed" for d in items))
        self.assertTrue(
            all(
                d["values"] == {"start": "19:00", "end": "23:00", "temperature_c": "24"}
                for d in items
            )
        )

    def test_one_manual_start_is_not_enough(self):
        item = next(
            d
            for d in decisions(evaluate(self.dataset, "H019"), "start")
            if d["weekday"] == 2
        )
        self.assertEqual(item["status"], "insufficient")
        self.assertEqual(item["support_count"], 1)

    def test_hot_temperature_and_cool_day_counterevidence(self):
        self.assertTrue(
            any(
                d["status"] == "proposed" and d["values"]["temperature_c"] == "22"
                for d in decisions(evaluate(self.dataset, "H013"), "temperature")
            )
        )
        self.assertFalse(
            any(
                d["status"] == "proposed"
                for d in decisions(evaluate(self.dataset, "H023"), "temperature")
            )
        )

    def test_weather_only_edit_changes_temperature_proposal(self):
        dataset = single(self.dataset, "H013")
        for site in dataset.weather.values():
            for row in site["observations"]:
                row["outdoor_c"] = ["27"] * 48
        self.assertFalse(
            any(
                d["status"] == "proposed"
                for d in decisions(evaluate(dataset, "H013"), "temperature")
            )
        )

    def test_missing_weather_or_temperature_and_single_adjustment_are_insufficient(
        self,
    ):
        for mode in ("weather", "temperature", "single"):
            dataset = single(self.dataset, "H013")
            if mode == "weather":
                for site in dataset.weather.values():
                    for row in site["observations"]:
                        row["outdoor_c"] = [None] * 48
            else:
                kept = False
                for device in dataset.customers["H013"]["devices"]:
                    for day in device["days"]:
                        if mode == "temperature":
                            for op in day["operations"]:
                                if op["action"] == "temperature":
                                    op["from_c"] = None
                        else:
                            operations = []
                            for op in day["operations"]:
                                if op["action"] != "temperature" or not kept:
                                    operations.append(op)
                                    if op["action"] == "temperature":
                                        kept = True
                            day["operations"] = operations
            self.assertFalse(
                any(
                    d["status"] == "proposed"
                    for d in decisions(evaluate(dataset, "H013"), "temperature")
                ),
                mode,
            )

    def test_missing_tariff_or_comfort_never_becomes_free_or_comfortable(self):
        for mode in ("tariff", "comfort"):
            dataset = single(self.dataset, "H011")
            if mode == "tariff":
                dataset.tariffs["time-of-use"]["jpy_per_kwh"][34] = None
            else:
                for device in dataset.customers["H011"]["devices"]:
                    for candidate in device["predictions"]:
                        candidate["plans"][-1]["indoor_c_at_required"] = None
            self.assertFalse(
                any(
                    d["status"] == "proposed"
                    for d in decisions(evaluate(dataset, "H011"), "precool")
                ),
                mode,
            )

    def test_invalid_numeric_and_naive_datetime_inputs_fail_validation(self):
        for value in (-1, True, "NaN", "Infinity"):
            dataset = single(self.dataset, "H011")
            dataset.tariffs["time-of-use"]["jpy_per_kwh"][34] = value
            with self.assertRaises(InputError):
                dataset.prepare("H011")
        dataset = single(self.dataset, "H001")
        dataset.customers["H001"]["presence"][0]["observed_at"] = "2026-08-02T07:00:00"
        with self.assertRaises(InputError):
            dataset.prepare("H001")

    def test_cheap_expensive_uncomfortable_and_missing_precool(self):
        cheap = decisions(evaluate(self.dataset, "H011"), "precool")[0]
        self.assertEqual(cheap["status"], "proposed")
        self.assertIn("40.00円", cheap["reason"])
        for hid in ("H020", "H021", "H022"):
            with self.subTest(hid=hid):
                item = decisions(evaluate(self.dataset, hid), "precool")[0]
                self.assertNotEqual(item["status"], "proposed")

    def test_tariff_only_edit_removes_savings_and_does_not_change_operations(self):
        dataset = single(self.dataset, "H011")
        before = copy.deepcopy(dataset.customers["H011"]["devices"])
        dataset.tariffs["time-of-use"]["jpy_per_kwh"][34:36] = ["100", "100"]
        self.assertNotEqual(
            decisions(evaluate(dataset, "H011"), "precool")[0]["status"], "proposed"
        )
        self.assertEqual(dataset.customers["H011"]["devices"], before)

    def test_real_start_survives_location_opt_out(self):
        report = evaluate(self.dataset, "H008")
        self.assertTrue(
            any(d["status"] == "proposed" for d in decisions(report, "start"))
        )
        self.assertTrue(
            all(d["status"] != "proposed" for d in decisions(report, "start_review"))
        )

    def test_existing_running_on_commands_never_support_a_time(self):
        prepared = self.dataset.prepare("H001")
        snapshot = prepared.snapshot(
            datetime.fromisoformat(self.dataset.config["as_of"])
        )
        report = evaluate_household(
            snapshot, snapshot.households[0], include_billing=False
        )
        self.assertEqual(report["devices"][0]["confirmed_manual_on_ids"], [])
        self.assertTrue(
            all(
                e["reason"] == "already_running"
                for e in report["devices"][0]["power_evidence_exclusions"]
            )
        )

    def test_missing_power_telemetry_removes_concrete_start(self):
        dataset = single(self.dataset, "H008")
        for device in dataset.customers["H008"]["devices"]:
            for day in device["days"]:
                for op in day["operations"]:
                    if op.get("power"):
                        op["power"]["before"] = None
        self.assertFalse(
            any(
                d["status"] == "proposed"
                for d in decisions(evaluate(dataset, "H008"), "start")
            )
        )

    def test_multiple_devices_share_the_same_household_observations(self):
        report = evaluate(self.dataset, "H005")
        self.assertEqual(len(report["devices"]), 2)
        left = [
            d["comparisons"]
            for d in report["devices"][0]["decisions"]
            if d["kind"] == "start_review"
        ]
        right = [
            d["comparisons"]
            for d in report["devices"][1]["decisions"]
            if d["kind"] == "start_review"
        ]
        self.assertEqual(left, right)

    def test_travel_and_once_do_not_vote_for_absence(self):
        for hid, reason in (("H009", "travel"), ("H010", "one_off")):
            item = next(
                d
                for d in decisions(evaluate(self.dataset, hid), "start_review")
                if d["weekday"] == 2
            )
            self.assertNotEqual(item["status"], "proposed")
            self.assertEqual(item["support_count"], 1)
            self.assertEqual(
                [r["reason"] for r in item["excluded_dates"]], [reason] * 3
            )

    def test_unknown_observations_are_not_away(self):
        item = next(
            d
            for d in decisions(evaluate(self.dataset, "H005"), "start_review")
            if d["weekday"] == 2
        )
        self.assertEqual(item["support_count"], 0)
        self.assertEqual(len(item["excluded_dates"]), 4)

    def test_all_known_unused_zeros_and_missing_energy_stay_distinct(self):
        travel = self.dataset.customers["H009"]["devices"][0]["days"]
        unused = next(d for d in travel if d["usage"] == "unused")
        self.assertTrue(all(float(k) == 0 for k in unused["kwh"]))
        missing = self.dataset.customers["H022"]["devices"][0]["days"]
        self.assertTrue(any(None in d["kwh"] for d in missing))

    def test_customer_identity_does_not_choose_rule(self):
        dataset = single(self.dataset, "H015")
        row = dataset.customers.pop("H015")
        row["household_id"] = "renamed"
        dataset.customers["renamed"] = row
        report = evaluate(dataset, "renamed")
        item = next(d for d in decisions(report, "start") if d["weekday"] == 2)
        self.assertEqual(item["values"]["start"], "17:30")

    def test_invalid_grid_and_reference_are_rejected(self):
        for mode in ("grid", "site"):
            dataset = single(self.dataset, "H001")
            if mode == "grid":
                dataset.customers["H001"]["devices"][0]["days"][0]["kwh"].pop()
            else:
                dataset.customers["H001"]["installation"]["weather_site_id"] = "missing"
            with self.assertRaises(InputError):
                dataset.prepare("H001")

    def test_outputs_cannot_leave_automation(self):
        from pathlib import Path

        with self.assertRaises(InputError):
            inside_automation(Path("/tmp/outside.json"))

    def test_automatic_execution_matches_the_schedule_version_active_that_day(self):
        for hid, time in (("H001", "21:00"), ("H002", "20:00")):
            device = self.dataset.customers[hid]["devices"][0]
            day = next(d for d in device["days"] if d["date"] == "2026-10-07")
            automatic = [
                o
                for o in day["operations"]
                if o["source"] == "automation" and o["action"] == "on"
            ]
            self.assertEqual([o["at"][11:16] for o in automatic], [time])
            self.assertTrue(
                all(
                    o["power"]["before"] is False and o["power"]["after"] is True
                    for o in automatic
                )
            )


class TemporalReplayTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.dataset = load_dataset()
        cls.regular = replay_customer(cls.dataset, "H001")
        cls.manual = replay_customer(cls.dataset, "H008")

    def test_first_proposal_date_and_exact_evidence(self):
        report = self.regular
        self.assertEqual(report["first_proposal_at"], "2026-08-20T12:00:00+09:00")
        earlier = [r for r in report["days"] if r["decision_at"] < "2026-08-20"]
        self.assertTrue(all(r["active_count"] == 0 for r in earlier))
        first = next(
            r for r in report["days"] if r["decision_at"].startswith("2026-08-20")
        )
        event = next(e for e in first["events"] if e["event"] == "first")
        self.assertEqual(event["support_count"], 3)
        self.assertTrue(any("2026-08-19" in k for k in event["evidence_ids"]))
        item = next(
            d
            for dev in first["devices"]
            for d in dev["decisions"]
            if d["kind"] == "start_review" and d["weekday"] == 2
        )
        self.assertEqual(
            item["absent_dates"], ["2026-08-05", "2026-08-12", "2026-08-19"]
        )

    def test_continuations_are_not_new_proposals(self):
        self.assertEqual(self.regular["new_proposal_count"], 2)
        self.assertGreater(sum(r["active_count"] for r in self.regular["days"]), 2)
        self.assertTrue(
            all(
                not e["is_new"]
                for r in self.regular["days"]
                for e in r["events"]
                if e["event"] == "continued"
            )
        )

    def test_decline_and_new_evidence_are_separate_from_proposal_events(self):
        suppressed = next(
            r for r in self.regular["days"] if r["decision_at"].startswith("2026-08-22")
        )
        self.assertEqual(suppressed["new_count"], 0)
        self.assertEqual(suppressed["active_count"], 0)
        self.assertEqual(suppressed["responses"][0]["response"], "declined")
        self.assertEqual(
            suppressed["responses"][0]["responded_at"], "2026-08-21T12:05:00+09:00"
        )
        reappeared = next(
            r for r in self.regular["days"] if r["decision_at"].startswith("2026-08-27")
        )
        event = next(e for e in reappeared["events"] if e["event"] == "reappeared")
        self.assertTrue(event["is_new"])
        self.assertTrue(any("2026-08-26" in k for k in event["evidence_ids"]))

    def test_midperiod_schedule_change_ends_old_review(self):
        changed = next(
            r for r in self.regular["days"] if r["decision_at"].startswith("2026-10-02")
        )
        plan = next(
            s for s in changed["devices"][0]["current_schedules"] if s["weekday"] == 2
        )
        self.assertEqual(plan["start"], "21:00")
        observed = next(
            s
            for s in changed["context"]["devices"][0]["observed_schedules"]
            if s["weekday"] == 2
        )
        self.assertEqual(observed["start"], "18:30")
        self.assertEqual(changed["active_count"], 0)
        self.assertTrue(any(e["event"] == "ended" for e in changed["events"]))

    def test_manual_pattern_change_has_a_date_and_supporting_operations(self):
        events = [
            (r, e)
            for r in self.manual["days"]
            for e in r["events"]
            if e["kind"] == "start" and e["weekday"] == 2 and e["is_new"]
        ]
        self.assertEqual(events[0][0]["decision_at"], "2026-08-20T12:00:00+09:00")
        self.assertEqual(events[0][1]["values"]["start"], "20:00")
        changed = next((r, e) for r, e in events if e["event"] == "changed")
        self.assertEqual(changed[0]["decision_at"], "2026-10-01T12:00:00+09:00")
        self.assertEqual(changed[1]["values"]["start"], "21:00")
        self.assertEqual(changed[1]["support_count"], 3)
        for day in ("2026-09-16", "2026-09-23", "2026-09-30"):
            self.assertTrue(any(day in k for k in changed[1]["evidence_ids"]))

    def test_future_records_of_every_source_do_not_change_past_rows(self):
        dataset = single(self.dataset, "H001")
        raw = dataset.customers["H001"]
        for obs in raw["presence"]:
            if obs["observed_at"] >= "2026-09-01":
                obs["sharing_enabled"] = False
        raw["feedback"].append(
            dict(
                raw["feedback"][0],
                id="future",
                proposed_at="2026-09-30T12:00:00+09:00",
                responded_at="2026-10-01T12:05:00+09:00",
            )
        )
        for dev in raw["devices"]:
            for day in dev["days"]:
                if day["date"] >= "2026-09-01":
                    extra = copy.deepcopy(day["operations"][0])
                    extra.pop("power", None)
                    extra.update(
                        id=extra["id"] + "-future",
                        at=day["date"] + "T22:45:00+09:00",
                        received_at=day["date"] + "T22:45:02+09:00",
                    )
                    day["operations"].append(extra)
        for site in dataset.weather.values():
            for row in site["forecast"]:
                if row["issued_at"] >= "2026-09-01":
                    row["outdoor_c"] = ["40"] * 48
        changed = replay_customer(dataset, "H001")
        before = [r for r in self.regular["days"] if r["decision_at"] < "2026-09-01"]
        after = [r for r in changed["days"] if r["decision_at"] < "2026-09-01"]
        self.assertEqual(after, before)

    def test_presence_only_change_removes_review_with_fixed_operations(self):
        dataset = single(self.dataset, "H001")
        raw = dataset.customers["H001"]
        devices = copy.deepcopy(raw["devices"])
        for obs in raw["presence"]:
            if (
                obs["source"] == "phone_geofence"
                and obs["observed_at"][11:16] == "18:00"
            ):
                obs.update(source="home_wifi", event="connected", state="home")
        changed = replay_customer(dataset, "H001")
        first = next(
            r for r in changed["days"] if r["decision_at"].startswith("2026-08-20")
        )
        self.assertEqual(first["active_count"], 0)
        self.assertEqual(raw["devices"], devices)

    def test_location_unknown_does_not_become_absent_support(self):
        dataset = single(self.dataset, "H001")
        for obs in dataset.customers["H001"]["presence"]:
            obs["sharing_enabled"] = False
        changed = replay_customer(dataset, "H001")
        self.assertEqual(changed["new_proposal_count"], 0)

    def test_one_customers_changes_do_not_affect_another(self):
        dataset = single(self.dataset, "H008")
        first = replay_customer(dataset, "H008")
        irrelevant = copy.deepcopy(self.dataset.customers["H001"])
        irrelevant["presence"] = []
        dataset.customers["H001"] = irrelevant
        self.assertEqual(replay_customer(dataset, "H008"), first)

    def test_snapshot_mode_remains_available(self):
        report = replay_customer(self.dataset, "H001", "snapshot")
        self.assertEqual(len(report["days"]), 1)
        self.assertEqual(report["days"][0]["decision_at"], "2026-10-31T12:00:00+09:00")

    def test_day_detail_does_not_expose_power_confirmation_received_in_the_future(self):
        dataset = single(self.dataset, "H008")
        day = next(
            d
            for d in dataset.customers["H008"]["devices"][0]["days"]
            if d["date"] == "2026-08-19"
        )
        op = next(
            o
            for o in day["operations"]
            if o["action"] == "on" and o["source"] == "web_manual"
        )
        op["power"]["received_at"] = "2026-09-01T00:00:00+09:00"
        report = replay_customer(dataset, "H008")
        row = next(
            r for r in report["days"] if r["decision_at"].startswith("2026-08-20")
        )
        visible = next(
            o for o in row["context"]["devices"][0]["operations"] if o["id"] == op["id"]
        )
        self.assertIsNone(visible["power"])
        self.assertEqual(visible["power_availability"], "判定時点では未受信")

    def test_approved_manual_value_does_not_hide_a_new_independent_absence_review(self):
        report = replay_customer(self.dataset, "H002")
        row = next(
            r for r in report["days"] if r["decision_at"].startswith("2026-10-01")
        )
        start = next(
            d
            for dev in row["devices"]
            for d in dev["decisions"]
            if d["kind"] == "start" and d["weekday"] == 2
        )
        review = next(
            d
            for dev in row["devices"]
            for d in dev["decisions"]
            if d["kind"] == "start_review" and d["weekday"] == 2
        )
        self.assertEqual(start["status"], "approved")
        self.assertEqual(review["status"], "proposed")
        self.assertEqual(review["support_count"], 3)
