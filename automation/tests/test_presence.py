"""Causal presence, independent power evidence, counterfactuals and regression."""

import copy
import io
import json
import unittest
from contextlib import redirect_stdout, redirect_stderr
from datetime import datetime, timedelta, timezone
from pathlib import Path
from tempfile import TemporaryDirectory

from automation.src.cli import main
from automation.src.generate_presence import build_presence_data, write_data
from automation.src.loader import InputError
from automation.src.presence import PresenceIndex
from automation.src.presence_data import (
    Household,
    Observation,
    normalize_presence,
    read_data,
)
from automation.src.presence_proposals import evaluate_cohort, evaluate_household

DATA = (
    Path(__file__).resolve().parents[1]
    / "data"
    / "presence"
    / "representatives.json.gz"
)


def at(time: str) -> datetime:
    return datetime.fromisoformat(f"2026-10-07T{time}:00+09:00")


def obs(
    resident, time, source, event, state, *, enabled=True, location=True, received=None
):
    return Observation(
        f"{resident}:{time}:{source}:{event}",
        resident,
        at(time),
        at(received or time),
        source,
        event,
        state,
        enabled,
        location,
    )


def index(records, residents=("a",), cutoff="23:59"):
    return PresenceIndex(
        Household(
            "house",
            "home",
            residents,
            tuple(sorted(records, key=lambda o: (o.at, o.id))),
            ("device",),
        ),
        at(cutoff),
    )


def raw_house(raw, household_id):
    return next(h for h in raw["households"] if h["household_id"] == household_id)


def evaluate(raw, household_id):
    data = normalize_presence(raw)
    return evaluate_household(
        data, next(h for h in data.households if h.id == household_id)
    )


def wednesday(report, kind="start_review", device=0):
    return next(
        d
        for d in report["devices"][device]["decisions"]
        if d["kind"] == kind and d["weekday"] == 2
    )


class PresenceSignalTests(unittest.TestCase):
    def test_one_confirmed_person_overrides_other_unknown_people(self):
        p = index([obs("a", "18:20", "home_wifi", "connected", "home")], ("a", "b"))
        self.assertEqual(p.at(at("18:30"))["state"], "home")

    def test_everyone_must_have_fresh_away_confirmation(self):
        records = [obs("a", "18:00", "phone_geofence", "exit", "away")]
        self.assertEqual(index(records, ("a", "b")).at(at("18:30"))["state"], "unknown")
        records.append(obs("b", "18:00", "phone_geofence", "heartbeat", "away"))
        self.assertEqual(index(records, ("a", "b")).at(at("18:30"))["state"], "away")

    def test_stale_disabled_and_location_unavailable_are_unknown(self):
        variants = [
            obs("a", "15:00", "phone_geofence", "exit", "away"),
            obs("a", "18:00", "phone_geofence", "exit", "away", location=False),
            obs("a", "18:00", "permission", "disabled", "unknown", enabled=False),
        ]
        for record in variants:
            with self.subTest(record=record):
                self.assertEqual(index([record]).at(at("18:30"))["state"], "unknown")

    def test_wifi_can_confirm_home_without_location_permission(self):
        record = obs("a", "18:20", "home_wifi", "connected", "home", location=False)
        self.assertEqual(index([record]).at(at("18:30"))["state"], "home")
        no_gps = obs(
            "a", "18:25", "phone_geofence", "heartbeat", "unknown", location=False
        )
        self.assertEqual(index([record, no_gps]).at(at("18:30"))["state"], "home")

    def test_approach_does_not_confirm_arrival_or_refresh_old_away(self):
        records = [
            obs("a", "18:00", "phone_geofence", "exit", "away"),
            obs("a", "18:20", "phone_proximity", "nearby", "unknown"),
        ]
        self.assertEqual(index(records).at(at("18:30"))["state"], "away")
        self.assertEqual(index(records[1:]).at(at("18:30"))["state"], "unknown")

    def test_passby_and_detour_do_not_become_an_early_return(self):
        records = [
            obs("a", "18:00", "phone_geofence", "exit", "away"),
            obs("a", "18:10", "phone_geofence", "enter", "home"),
            obs("a", "18:15", "phone_geofence", "exit", "away"),
            obs("a", "18:20", "phone_proximity", "nearby", "unknown"),
            obs("a", "21:00", "home_wifi", "connected", "home"),
        ]
        p = index(records)
        self.assertEqual(p.at(at("18:30"))["state"], "away")
        self.assertEqual(
            p.first_home_after(at("18:00"), at("23:00"))["confirmed_at"],
            at("21:00").isoformat(),
        )

    def test_geofence_requires_two_inside_observations_ten_minutes_apart(self):
        records = [
            obs("a", "18:00", "phone_geofence", "enter", "home"),
            obs("a", "18:05", "phone_geofence", "heartbeat", "home"),
            obs("a", "18:10", "phone_geofence", "heartbeat", "home"),
        ]
        p = index(records)
        self.assertEqual(p.at(at("18:05"))["state"], "unknown")
        self.assertEqual(p.at(at("18:10"))["state"], "home")
        self.assertEqual(index(records[:1]).at(at("18:15"))["state"], "unknown")

    def test_future_observed_or_late_received_information_does_not_leak(self):
        records = [
            obs("a", "18:00", "phone_geofence", "exit", "away"),
            obs("a", "18:20", "home_wifi", "connected", "home", received="19:00"),
        ]
        p = index(records)
        self.assertEqual(p.at(at("18:30"))["state"], "away")
        self.assertEqual(
            p.first_home_after(at("18:30"), at("23:00"))["confirmed_at"],
            at("19:00").isoformat(),
        )
        self.assertEqual(
            index(records, cutoff="18:30").at(at("19:00"))["state"], "unknown"
        )

    def test_a_later_opt_out_does_not_change_earlier_presence(self):
        records = [
            obs("a", "18:00", "home_wifi", "connected", "home"),
            obs("a", "18:20", "permission", "disabled", "unknown", enabled=False),
        ]
        p = index(records)
        self.assertEqual(p.at(at("18:10"))["state"], "home")
        self.assertEqual(p.at(at("18:30"))["state"], "unknown")

    def test_ttl_boundary_and_conflicting_observations(self):
        p = index([obs("a", "16:30", "home_wifi", "connected", "home")])
        self.assertEqual(p.at(at("18:30"))["state"], "home")
        self.assertEqual(p.at(at("18:30") + timedelta(seconds=1))["state"], "unknown")
        p = index(
            [
                obs("a", "18:20", "home_wifi", "connected", "home"),
                obs("a", "18:20", "phone_geofence", "exit", "away"),
            ]
        )
        self.assertEqual(p.at(at("18:30"))["state"], "unknown")


class PresenceProposalTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.raw = read_data(DATA)
        cls.data = normalize_presence(cls.raw)
        cls.reports = {
            h.id: evaluate_household(cls.data, h) for h in cls.data.households
        }

    def test_expected_wednesday_proposals_are_kept_only_in_tests(self):
        expected = {
            "H001": ["start_review"],
            "H002": ["start"],
            "H003": [],
            "H004": [],
            "H005": [],
            "H006": ["start_review"],
            "H007": ["start_review"],
            "H008": ["start"],
            "H009": [],
            "H010": [],
            "H011": ["start_review"],
            "H012": [],
        }
        for household_id, kinds in expected.items():
            with self.subTest(household=household_id):
                actual = [
                    p["kind"]
                    for p in self.reports[household_id]["devices"][0]["proposals"]
                    if p["weekday"] == 2
                ]
                self.assertEqual(actual, kinds)

    def test_regular_absence_only_prompts_review_not_a_time_or_savings(self):
        report = self.reports["H001"]
        self.assertEqual(report["proposal_count"], 1)
        item = wednesday(report)
        self.assertEqual(item["current_start"], "18:30")
        self.assertEqual(item["comparison_days"], 4)
        self.assertEqual(item["support_count"], 4)
        self.assertEqual(
            item["absent_dates"],
            ["2026-10-07", "2026-10-14", "2026-10-21", "2026-10-28"],
        )
        self.assertEqual(item["values"], {})
        self.assertIsNone(item["estimate"])
        self.assertTrue(item["requires_approval"])
        self.assertFalse(item["applied"])
        self.assertTrue(all(row["confirmed_return"] for row in item["comparisons"]))

    def test_three_absent_days_are_required_without_replacing_excluded_dates(self):
        raw = copy.deepcopy(self.raw)
        house = raw_house(raw, "H001")
        for index, day in enumerate(("2026-10-07", "2026-10-14")):
            house["observations"].append(
                dict(
                    id=f"known-home:{day}",
                    resident_id=house["residents"][0],
                    installation_site_id=house["installation_site_id"],
                    observed_at=day + "T18:20:00+09:00",
                    received_at=day + "T18:20:00+09:00",
                    source="home_wifi",
                    event="connected",
                    state="home",
                    sharing_enabled=True,
                    location_enabled=False,
                )
            )
            item = wednesday(evaluate(raw, "H001"))
            self.assertEqual(item["support_count"], 3 - index)
            self.assertEqual(item["status"], "proposed" if index == 0 else "unchanged")

    def test_presence_only_counterfactual_with_roommate_and_missing_data(self):
        raw = copy.deepcopy(self.raw)
        monthly_before = copy.deepcopy(raw["monthly"])
        house = raw_house(raw, "H001")
        roommate = "new-roommate"
        house["residents"].append(roommate)
        for day in raw["monthly"]["customers"][0]["days"]:
            house["observations"].append(
                dict(
                    id=f"roommate:{day['date']}",
                    resident_id=roommate,
                    installation_site_id=house["installation_site_id"],
                    observed_at=day["date"] + "T18:20:00+09:00",
                    received_at=day["date"] + "T18:20:00+09:00",
                    source="home_wifi",
                    event="connected",
                    state="home",
                    sharing_enabled=True,
                    location_enabled=False,
                )
            )
        changed = evaluate(raw, "H001")
        self.assertEqual(changed["proposal_count"], 0)
        self.assertEqual(wednesday(changed)["reason_code"], "someone_home")
        for record in house["observations"]:
            record["sharing_enabled"] = False
        changed = evaluate(raw, "H001")
        self.assertEqual(changed["proposal_count"], 0)
        self.assertEqual(wednesday(changed)["reason_code"], "presence_unknown")
        self.assertEqual(len(wednesday(changed)["excluded_dates"]), 4)
        self.assertEqual(raw["monthly"], monthly_before)

    def test_real_power_transition_is_independent_evidence_and_merged_once(self):
        device = self.reports["H002"]["devices"][0]
        proposals = [p for p in device["proposals"] if p["weekday"] == 2]
        self.assertEqual(len(proposals), 1)
        self.assertEqual(proposals[0]["kind"], "start")
        self.assertEqual(proposals[0]["values"]["start"], "20:00")
        self.assertEqual(proposals[0]["support_count"], 4)
        self.assertIn("presence_review", proposals[0])
        self.assertEqual(wednesday(self.reports["H002"])["status"], "merged")

    def test_an_on_command_while_running_is_not_start_evidence(self):
        device = self.reports["H001"]["devices"][0]
        self.assertEqual(device["confirmed_manual_on_ids"], [])
        self.assertTrue(
            all(
                e["reason"] == "already_running"
                for e in device["power_evidence_exclusions"]
            )
        )
        raw = copy.deepcopy(self.raw)
        for sample in raw["operation_power"]["H002-AC1"]:
            sample["before"] = True
        self.assertEqual(wednesday(evaluate(raw, "H002"))["status"], "proposed")
        self.assertFalse(
            any(
                p["kind"] == "start"
                for p in evaluate(raw, "H002")["devices"][0]["proposals"]
            )
        )

    def test_missing_or_future_power_observation_does_not_support_a_time(self):
        for mode in ("unknown", "future"):
            with self.subTest(mode=mode):
                raw = copy.deepcopy(self.raw)
                for sample in raw["operation_power"]["H002-AC1"]:
                    if mode == "unknown":
                        sample["before"] = None
                    else:
                        sample["received_at"] = "2026-11-02T00:00:00+09:00"
                report = evaluate(raw, "H002")
                self.assertEqual(wednesday(report)["status"], "proposed")
                self.assertFalse(
                    any(p["kind"] == "start" for p in report["devices"][0]["proposals"])
                )

    def test_intervening_command_makes_power_transition_ambiguous(self):
        raw = copy.deepcopy(self.raw)
        customer = next(
            c for c in raw["monthly"]["customers"] if c["device_id"] == "H002-AC1"
        )
        for day in customer["days"]:
            manual = next(
                op
                for op in day["operations"]
                if op["source"] == "web_manual" and op["action"] == "on"
            )
            other = dict(manual, id=manual["id"] + ":other", source="automation")
            day["operations"].append(other)
        report = evaluate(raw, "H002")
        self.assertFalse(
            any(p["kind"] == "start" for p in report["devices"][0]["proposals"])
        )
        self.assertTrue(
            any(
                e["reason"] == "intervening_power_operation"
                for e in report["devices"][0]["power_evidence_exclusions"]
            )
        )

    def test_manual_proposal_survives_without_location(self):
        report = self.reports["H008"]
        item = wednesday(report, "start")
        self.assertEqual(item["status"], "proposed")
        self.assertEqual(item["values"]["start"], "20:00")
        self.assertEqual(wednesday(report)["reason_code"], "presence_unknown")

    def test_passby_and_detour_returns_are_after_approach(self):
        for household_id, earliest in (("H006", "21:00"), ("H007", "21:30")):
            item = wednesday(self.reports[household_id])
            self.assertEqual(item["status"], "proposed")
            for row in item["comparisons"]:
                self.assertGreaterEqual(
                    row["confirmed_return"]["confirmed_at"][11:16], earliest
                )

    def test_travel_and_one_off_days_are_excluded(self):
        for household_id, reason in (("H009", "travel"), ("H010", "one_off")):
            item = wednesday(self.reports[household_id])
            self.assertNotEqual(item["status"], "proposed")
            self.assertEqual(item["support_count"], 1)
            self.assertEqual(
                [row["reason"] for row in item["excluded_dates"]], [reason] * 3
            )

    def test_two_devices_share_the_same_household_presence(self):
        report = self.reports["H005"]
        self.assertEqual(len(report["devices"]), 2)
        self.assertEqual(
            wednesday(report, device=0)["comparisons"],
            wednesday(report, device=1)["comparisons"],
        )
        self.assertNotEqual(report["installation_site_id"], report["weather_site_id"])

    def test_presence_changes_do_not_affect_other_households(self):
        raw = copy.deepcopy(self.raw)
        for record in raw_house(raw, "H001")["observations"]:
            record["sharing_enabled"] = False
        self.assertEqual(evaluate(raw, "H002"), self.reports["H002"])

    def test_precooling_is_kept_and_presence_does_not_claim_its_savings(self):
        report = self.reports["H011"]
        proposals = report["devices"][0]["proposals"]
        self.assertTrue(
            any(p["kind"] == "precool" and p["estimate"] for p in proposals)
        )
        self.assertIsNone(wednesday(report)["estimate"])

    def test_reproducible_generator_and_gzip_bytes(self):
        self.assertEqual(build_presence_data(42, 12, 30), self.raw)
        small = build_presence_data(42, 2, 30)
        self.assertEqual(small, build_presence_data(42, 2, 30))
        self.assertNotEqual(small, build_presence_data(43, 2, 30))
        with TemporaryDirectory(dir="/tmp") as directory:
            first, second = Path(directory) / "one.gz", Path(directory) / "two.gz"
            write_data(first, small)
            write_data(second, small)
            self.assertEqual(first.read_bytes(), second.read_bytes())
            self.assertEqual(read_data(first), small)

    def test_90_day_input_and_0_1_multiple_summary(self):
        long = build_presence_data(42, 2, 90)
        self.assertTrue(all(len(c["days"]) == 90 for c in long["monthly"]["customers"]))
        report = evaluate_cohort(self.data)
        self.assertEqual(report["household_count"], 12)
        self.assertEqual(report["device_count"], 14)
        self.assertEqual(set(report["household_counts"]), {"zero", "one", "multiple"})
        self.assertEqual(
            report["proposal_count"],
            sum(h["proposal_count"] for h in report["households"]),
        )
        self.assertGreater(report["evaluation_seconds"], 0)

    def test_generation_energy_and_power_observations_are_consistent(self):
        raw_customers = {c["device_id"]: c for c in self.raw["monthly"]["customers"]}
        for device_id, customer in raw_customers.items():
            samples = {s["event_id"]: s for s in self.raw["operation_power"][device_id]}
            for day in customer["days"]:
                state = False
                for op in sorted(day["operations"], key=lambda op: op["at"]):
                    self.assertEqual(samples[op["id"]]["before"], state)
                    state = op["action"] == "on"
                    self.assertEqual(samples[op["id"]]["after"], state)
                for slot, energy in enumerate(day["kwh"]):
                    begin = datetime.fromisoformat(
                        day["date"] + "T00:00:00+09:00"
                    ) + timedelta(minutes=slot * 30)
                    end = begin + timedelta(minutes=30)
                    state = False
                    running = False
                    for op in sorted(day["operations"], key=lambda op: op["at"]):
                        op_at = datetime.fromisoformat(op["at"])
                        if op_at <= begin:
                            state = op["action"] == "on"
                        elif op_at < end and op["action"] == "on":
                            running = True
                    self.assertEqual(float(energy) > 0, state or running)

    def test_household_id_does_not_choose_a_rule(self):
        raw = copy.deepcopy(self.raw)
        house = raw_house(raw, "H001")
        house["household_id"] = "unseen"
        raw["installation_sites"][house["installation_site_id"]]["household_id"] = (
            "unseen"
        )
        self.assertEqual(wednesday(evaluate(raw, "unseen"))["support_count"], 4)

    def test_invalid_references_and_naive_observations_fail(self):
        mutations = (
            lambda raw: raw["households"][0]["observations"][0].update(
                installation_site_id="weather-tokyo"
            ),
            lambda raw: raw["households"][0]["observations"][0].update(
                resident_id="stranger"
            ),
            lambda raw: raw["households"][0]["observations"][0].update(
                observed_at="2026-10-01T00:00:00"
            ),
            lambda raw: raw["households"][0]["device_ids"].append("absent-device"),
            lambda raw: raw["operation_power"]["H001-AC1"][0].update(before="false"),
        )
        for mutate in mutations:
            raw = copy.deepcopy(self.raw)
            mutate(raw)
            with self.assertRaises(InputError):
                normalize_presence(raw)

    def test_cli_individual_day_batch_and_errors(self):
        out = io.StringIO()
        with redirect_stdout(out):
            self.assertEqual(
                main(["--household", "H001", "--day", "2026-10-07", "--json"]), 0
            )
        report = json.loads(out.getvalue())
        self.assertEqual(report["proposal_count"], 1)
        self.assertEqual(
            len(report["day_detail"]["devices"][0]["charges"]["intervals"]), 48
        )
        self.assertTrue(report["day_detail"]["phone_observations"])
        out = io.StringIO()
        with redirect_stdout(out):
            self.assertEqual(main(["--households", "--json"]), 0)
        self.assertEqual(json.loads(out.getvalue())["household_count"], 12)
        with redirect_stderr(io.StringIO()):
            self.assertEqual(main(["--household", "unknown"]), 2)

    def test_day_detail_uses_installation_timezone_for_phone_observations(self):
        raw = copy.deepcopy(self.raw)
        house = raw_house(raw, "H001")
        expected = {
            o["id"]
            for o in house["observations"]
            if o["observed_at"].startswith("2026-10-07")
        }
        for record in house["observations"]:
            for field in ("observed_at", "received_at"):
                record[field] = (
                    datetime.fromisoformat(record[field])
                    .astimezone(timezone.utc)
                    .isoformat()
                )
        with TemporaryDirectory(dir="/tmp") as directory:
            path = Path(directory) / "utc.json"
            write_data(path, raw)
            out = io.StringIO()
            with redirect_stdout(out):
                self.assertEqual(
                    main(
                        [
                            "--household",
                            "H001",
                            "--presence-data",
                            str(path),
                            "--day",
                            "2026-10-07",
                            "--json",
                        ]
                    ),
                    0,
                )
        report = json.loads(out.getvalue())
        self.assertEqual(
            {o["id"] for o in report["day_detail"]["phone_observations"]}, expected
        )
