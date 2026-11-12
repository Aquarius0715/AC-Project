"""Executable acceptance cases plus boundaries that affect recommendation safety."""

import copy
import io
import json
import unittest
from contextlib import redirect_stderr, redirect_stdout
from decimal import Decimal
from pathlib import Path

from automation.src.cli import main
from automation.src.engine import evaluate, plan_cost
from automation.src.loader import InputError, load, normalize

DATA = Path(__file__).resolve().parents[1] / "data"
EXPECTED = {
    "wednesday-start": {"start": True},
    "insufficient-start": {"start": False},
    "hot-temperature": {"temperature": True},
    "insufficient-temperature": {"temperature": False},
    "missing-weather": {"temperature": False},
    "missing-temperature": {"temperature": False},
    "temperature-not-hot-only": {"temperature": False},
    "cheap-precool": {"precool": True},
    "expensive-precool": {"precool": False},
    "hot-expensive": {"temperature": True, "precool": False},
    "excluded-history": {"start": False},
    "missing-energy": {"precool": False},
    "missing-comfort": {"precool": False},
    "missing-tariff": {"precool": False},
}


def raw_case(name: str) -> dict:
    return json.loads((DATA / f"{name}.json").read_text(encoding="utf-8"))


def decision(raw: dict, check: str):
    return next(item for item in evaluate(normalize(raw)) if item.check == check)


class AcceptanceTests(unittest.TestCase):
    def test_every_fixture_matches_its_expected_decisions(self):
        self.assertEqual({path.stem for path in DATA.glob("*.json")}, set(EXPECTED))
        for name, expected in EXPECTED.items():
            with self.subTest(case=name):
                decisions = evaluate(load(DATA / f"{name}.json"))
                self.assertEqual({item.check: item.proposed for item in decisions if item.check != "history"}, expected)

    def test_wednesday_start_and_end_are_exact(self):
        result = decision(raw_case("wednesday-start"), "start")
        self.assertIn("開始 18:30 → 20:00", result.changes)
        self.assertIn("終了 23:30 を維持（水曜のみ）", result.changes)
        self.assertIn("直近4回中3回", result.reason)

    def test_temperature_is_conditional_and_not_claimed_as_savings(self):
        result = decision(raw_case("hot-temperature"), "temperature")
        self.assertEqual(result.target_temperature_c, "22")
        self.assertIn("暑い日（外気温32℃以上）に限り 24℃ → 22℃", result.changes)
        self.assertIn("節約を意味しません", result.reason)

    def test_money_is_computed_from_each_slot_not_the_cheapest_rate(self):
        for name, expected in (("cheap-precool", "78.00"), ("expensive-precool", "130.00")):
            with self.subTest(case=name):
                scenario = load(DATA / f"{name}.json")
                self.assertEqual(plan_cost(scenario, scenario.energy.plans[0]), Decimal("100"))
                self.assertEqual(plan_cost(scenario, scenario.energy.plans[1]), Decimal(expected))

    def test_heat_and_price_outputs_remain_distinguishable(self):
        result = decision(raw_case("hot-expensive"), "precool")
        self.assertTrue(any("32.00円" in item and "節約提案とは別" in item for item in result.evidence))
        self.assertTrue(any("快適性を保てないため節約候補から除外" in item for item in result.evidence))


class HistoryTests(unittest.TestCase):
    def test_learning_enable_date_does_not_discard_older_web_operations(self):
        raw = raw_case("wednesday-start")
        raw["learning_enabled_at"] = "2026-11-01T00:00:00+09:00"
        self.assertTrue(decision(raw, "start").proposed)

    def test_four_wednesdays_are_not_four_arbitrary_events(self):
        raw = raw_case("insufficient-start")
        for day in (9, 16, 23):
            op = dict(raw["operations"][0], at=f"2026-09-{day:02d}T20:00:00+09:00")
            raw["operations"].append(op)
        self.assertFalse(decision(raw, "start").proposed)

    def test_repeated_clicks_on_one_day_do_not_supply_multiple_votes(self):
        for fixture, check in (("insufficient-start", "start"), ("insufficient-temperature", "temperature")):
            with self.subTest(case=fixture):
                raw = raw_case(fixture)
                raw["operations"] *= 5
                self.assertFalse(decision(raw, check).proposed)

    def test_exclusion_reasons_each_prevent_a_false_proposal(self):
        for field, value in (("once", True), ("source", "automation"),
                             ("source", "remote"), ("device_id", "another-device")):
            with self.subTest(field=field, value=value):
                raw = raw_case("wednesday-start")
                raw["operations"][0][field] = value
                self.assertFalse(decision(raw, "start").proposed)
        raw = raw_case("wednesday-start")
        raw["trips"] = [{"start": "2026-10-07T00:00:00+09:00", "end": "2026-10-08T00:00:00+09:00"}]
        self.assertFalse(decision(raw, "start").proposed)

    def test_timezone_controls_the_weekday_and_time(self):
        raw = raw_case("wednesday-start")
        # Tuesday UTC is Wednesday in the device's timezone.
        for op, at in zip(raw["operations"][:3], ("2026-10-06T15:50:00+00:00",
                                                  "2026-10-13T16:00:00+00:00",
                                                  "2026-10-20T16:10:00+00:00")):
            op["at"] = at
        self.assertIn("開始 18:30 → 01:00", decision(raw, "start").changes)

    def test_tolerance_includes_15_minutes_but_rejects_31_minute_span(self):
        raw = raw_case("wednesday-start")
        for op, time in zip(raw["operations"][:3], ("19:45", "20:00", "20:15")):
            op["at"] = op["at"][:11] + time + ":00+09:00"
        self.assertIn("開始 18:30 → 20:00", decision(raw, "start").changes)
        raw["operations"][2]["at"] = "2026-10-21T20:16:00+09:00"
        self.assertFalse(decision(raw, "start").proposed)

    def test_single_feasible_minute_can_support_three_dates(self):
        raw = raw_case("wednesday-start")
        for op, time in zip(raw["operations"][:3], ("20:00", "20:16", "20:30")):
            op["at"] = op["at"][:11] + time + ":00+09:00"
        self.assertIn("開始 18:30 → 20:15", decision(raw, "start").changes)

    def test_same_start_and_start_after_end_are_not_proposed(self):
        for time in ("18:30", "23:40"):
            with self.subTest(time=time):
                raw = raw_case("wednesday-start")
                for op in raw["operations"]:
                    op["at"] = op["at"][:11] + time + ":00+09:00"
                self.assertFalse(decision(raw, "start").proposed)

    def test_reversing_a_temperature_change_cancels_that_days_vote(self):
        raw = raw_case("hot-temperature")
        last = dict(raw["operations"][-1], at="2026-10-21T19:10:00+09:00", from_c=22, to_c=24)
        raw["operations"].append(last)
        self.assertFalse(decision(raw, "temperature").proposed)

    def test_weather_more_than_30_minutes_away_is_unknown(self):
        raw = raw_case("hot-temperature")
        for sample in raw["weather"]:
            sample["at"] = sample["at"].replace("19:00", "19:31")
        self.assertFalse(decision(raw, "temperature").proposed)

    def test_normalization_and_evaluation_do_not_mutate_the_source(self):
        raw = raw_case("hot-expensive")
        original = copy.deepcopy(raw)
        scenario = normalize(raw)
        self.assertEqual(evaluate(scenario), evaluate(scenario))
        self.assertEqual(raw, original)


class EnergyTests(unittest.TestCase):
    def test_equal_price_is_not_savings(self):
        raw = raw_case("cheap-precool")
        raw["energy"]["plans"][1]["kwh"] = ["0", "1", "2"]
        self.assertFalse(decision(raw, "precool").proposed)

    def test_overcooling_is_not_equivalent_comfort(self):
        raw = raw_case("cheap-precool")
        raw["energy"]["plans"][1]["indoor_c_at_required"] = 20
        self.assertFalse(decision(raw, "precool").proposed)

    def test_missing_baseline_energy_or_comfort_prevents_savings_claim(self):
        for field, value in (("kwh", ["0", None, "2"]), ("indoor_c_at_required", None)):
            with self.subTest(field=field):
                raw = raw_case("cheap-precool")
                raw["energy"]["plans"][0][field] = value
                self.assertFalse(decision(raw, "precool").proposed)

    def test_savings_must_use_the_comfortable_baseline_when_hot(self):
        raw = raw_case("hot-expensive")
        raw["energy"]["plans"][-1]["indoor_c_at_required"] = 22
        result = decision(raw, "precool")
        self.assertTrue(result.proposed)
        self.assertIn("54.00円", result.reason)
        self.assertIn("比較期間の総額 132.00円 → 78.00円", result.changes)

    def test_conflicting_comfort_target_is_not_accepted(self):
        raw = raw_case("hot-expensive")
        raw["energy"]["comfort_target_c"] = 24
        self.assertFalse(decision(raw, "precool").proposed)


class InputAndCliTests(unittest.TestCase):
    def test_invalid_measurements_fail_validation(self):
        for value in (-1, True, "NaN", "Infinity", "bad"):
            with self.subTest(value=value):
                raw = raw_case("cheap-precool")
                raw["energy"]["plans"][0]["kwh"][0] = value
                with self.assertRaises(InputError):
                    normalize(raw)

    def test_different_periods_or_slot_counts_fail_validation(self):
        mutations = (
            lambda raw: raw["energy"]["slots"][1].update(start="2026-11-04T18:30:00+09:00"),
            lambda raw: raw["energy"]["slots"][1].update(start="2026-11-04T17:30:00+09:00"),
            lambda raw: raw["energy"].update(end="2026-11-05T00:00:00+09:00"),
            lambda raw: raw["energy"]["plans"][1].update(kwh=["1"]),
        )
        for index, mutate in enumerate(mutations):
            with self.subTest(mutation=index):
                raw = raw_case("cheap-precool")
                mutate(raw)
                with self.assertRaises(InputError):
                    normalize(raw)

    def test_naive_datetime_and_unknown_timezone_fail_validation(self):
        for key, value in (("as_of", "2026-11-01T00:00:00"), ("timezone", "NoSuch/Zone")):
            with self.subTest(field=key):
                raw = raw_case("wednesday-start")
                raw[key] = value
                with self.assertRaises(InputError):
                    normalize(raw)

    def test_invalid_clock_formats_fail_validation(self):
        for value in ("1:00", "-0:05", " 1:00", "24:00", "18:60"):
            with self.subTest(clock=value):
                raw = raw_case("wednesday-start")
                raw["schedule"]["start"] = value
                with self.assertRaises(InputError):
                    normalize(raw)

    def test_cli_all_json_matches_acceptance_expectations(self):
        out = io.StringIO()
        with redirect_stdout(out):
            self.assertEqual(main(["--all", "--json"]), 0)
        reports = json.loads(out.getvalue())
        self.assertEqual(len(reports), len(EXPECTED))
        for report in reports:
            self.assertTrue(report["simulation"])
            self.assertEqual({item["check"]: item["proposed"] for item in report["decisions"]
                              if item["check"] != "history"}, EXPECTED[report["case"]])

    def test_cli_list_and_custom_local_file(self):
        out = io.StringIO()
        with redirect_stdout(out):
            self.assertEqual(main(["--list"]), 0)
        self.assertEqual(len(out.getvalue().splitlines()), len(EXPECTED))
        with redirect_stdout(io.StringIO()):
            self.assertEqual(main(["--file", str(DATA / "wednesday-start.json")]), 0)

    def test_unknown_case_and_missing_file_exit_with_input_error(self):
        err = io.StringIO()
        with redirect_stderr(err), self.assertRaises(SystemExit) as exc:
            main(["--case", "../unknown"])
        self.assertEqual(exc.exception.code, 2)
        with redirect_stderr(err):
            self.assertEqual(main(["--file", str(DATA / "does-not-exist.json")]), 2)
        self.assertIn("入力エラー", err.getvalue())


if __name__ == "__main__":
    unittest.main()
