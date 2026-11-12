"""Monthly acceptance and counterfactual tests. Expected proposals live here only."""

import copy
import io
import json
import unittest
from contextlib import redirect_stderr, redirect_stdout
from datetime import date, datetime, timedelta
from decimal import Decimal
from pathlib import Path
from tempfile import TemporaryDirectory

from automation.src.cli import main
from automation.src.generate_monthly import build_data
from automation.src.loader import InputError
from automation.src.monthly_data import normalize_bundle
from automation.src.monthly_engine import evaluate_bundle, evaluate_customer

DATA = Path(__file__).resolve().parents[1] / "data" / "monthly" / "dataset.json"
# Tuples: kind, weekday, inferred start / temperature / plan as applicable.
EXPECTED = {
    "C001": [],
    "C002": [("start", 2, "20:00")],
    "C003": [("start", 0, "17:30"), ("start", 2, "20:00")],
    "C004": [("start", 2, "20:00"), ("temperature", None, "22")],
    "C005": [("add_schedule", 0, "19:00"), ("add_schedule", 2, "20:00")],
    "C006": [("temperature", None, "22")],
    "C007": [("precool", 6, "17時から予冷")],
    "C008": [],
    "C009": [],
    "C010": [],
    "C011": [],
    "C012": [],
    "C013": [],
    "C014": [],
    "C015": [("start", 2, "19:30")],
    "C016": [],
    "C017": [("start", 2, "20:00")],
    "C018": [("start", 2, "20:00")],
    "C019": [("start", 1, "21:00")],
    "C020": [("start", 2, "20:00")],
    "C021": [
        ("start", 0, "17:30"),
        ("start", 2, "20:00"),
        ("start", 4, "21:00"),
        ("temperature", None, "22"),
        ("precool", 6, "17時から予冷"),
    ],
    "C022": [],
    "C023": [("start", 2, "20:00")],
    "C024": [("start", 3, "17:00")],
    "C025": [("temperature", None, "22")],
    "C026": [("start", 2, "17:30")],
    "C027": [],
}


def summarize(report: dict) -> list[tuple]:
    return [
        (
            p["kind"],
            p["weekday"],
            p["values"].get("plan")
            if p["kind"] == "precool"
            else p["values"].get("temperature_c")
            if p["kind"] == "temperature"
            else p["values"].get("start"),
        )
        for p in report["proposals"]
    ]


def raw_customer(raw: dict, customer_id: str) -> dict:
    return next(
        customer
        for customer in raw["customers"]
        if customer["customer_id"] == customer_id
    )


def report_for(raw: dict, customer_id: str) -> dict:
    bundle = normalize_bundle(raw)
    customer = next(
        customer for customer in bundle.customers if customer.id == customer_id
    )
    return evaluate_customer(bundle, customer)


def find_decision(report: dict, kind: str, weekday: int | None = None) -> dict:
    return next(
        item
        for item in report["decisions"]
        if item["kind"] == kind and item["weekday"] == weekday
    )


def history_record(proposal: dict, response: str, selected: dict | None = None) -> dict:
    return dict(
        id="test-choice",
        proposed_at="2026-10-31T08:00:00+09:00",
        responded_at=None if response == "no_response" else "2026-10-31T09:00:00+09:00",
        kind=proposal["kind"],
        weekday=proposal["weekday"],
        values=copy.deepcopy(proposal["values"]),
        reason=proposal["reason"],
        evidence_ids=proposal["evidence_ids"],
        evidence_signature=proposal["evidence_signature"],
        response=response,
        selected=selected or {},
    )


class MonthlyTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.raw = json.loads(DATA.read_text(encoding="utf-8"))
        cls.reports = {
            r["customer_id"]: r for r in evaluate_bundle(normalize_bundle(cls.raw))
        }

    def test_all_customers_have_exact_counts_and_contents(self):
        self.assertEqual(set(self.reports), set(EXPECTED))
        for customer_id, expected in EXPECTED.items():
            with self.subTest(customer_id=customer_id):
                report = self.reports[customer_id]
                self.assertEqual(report["proposal_count"], len(expected))
                self.assertEqual(summarize(report), expected)
                self.assertEqual(
                    len(report["current_schedules"]),
                    len(raw_customer(self.raw, customer_id)["schedules"]),
                )

    def test_zero_one_and_multiple_have_distinct_reasons_and_types(self):
        self.assertEqual(self.reports["C001"]["status"], "変更の必要を示す根拠なし")
        self.assertEqual(self.reports["C014"]["status"], "提案を準備中")
        self.assertIn("履歴不足", self.reports["C011"]["status"])
        self.assertEqual(self.reports["C016"]["status"], "同じ根拠の再提案を抑制")
        self.assertEqual(
            [p["kind"] for p in self.reports["C003"]["proposals"]], ["start", "start"]
        )
        self.assertEqual(
            {p["kind"] for p in self.reports["C004"]["proposals"]},
            {"start", "temperature"},
        )

    def test_generator_is_reproducible_and_matches_checked_in_data(self):
        rebuilt = build_data(42)
        self.assertEqual(rebuilt, self.raw)
        self.assertEqual(build_data(42), rebuilt)
        self.assertNotEqual(build_data(43), rebuilt)

    def test_30_continuous_days_and_shared_weather_tariff_references(self):
        self.assertEqual(
            {
                j["response"]
                for customer in self.raw["customers"]
                for j in customer["judgments"]
            },
            {"approved", "edited", "declined", "no_response"},
        )
        for customer in self.raw["customers"]:
            with self.subTest(customer=customer["customer_id"]):
                self.assertEqual(
                    [day["date"] for day in customer["days"]],
                    [str(date(2026, 10, 1) + timedelta(days=i)) for i in range(30)],
                )
                self.assertTrue(all(len(day["kwh"]) == 48 for day in customer["days"]))
                self.assertIn(customer["site_id"], self.raw["sites"])
                self.assertIn(customer["tariff_id"], self.raw["tariffs"])
                self.assertNotIn("expected", customer)
                self.assertNotIn("weather", customer)
        for site in self.raw["sites"].values():
            observed = {day["date"] for day in site["observations"]}
            future = {day["date"] for day in site["forecast"]}
            self.assertFalse(observed & future)

    def test_observed_energy_has_zero_only_when_no_overlap_with_operation(self):
        for customer in self.raw["customers"]:
            for day in customer["days"]:
                if day["operation_status"] != "complete":
                    continue
                ons = [
                    datetime.fromisoformat(op["at"])
                    for op in day["operations"]
                    if op["action"] == "on"
                ]
                offs = [
                    datetime.fromisoformat(op["at"])
                    for op in day["operations"]
                    if op["action"] == "off"
                ]
                for slot, kwh in enumerate(day["kwh"]):
                    if kwh is None:
                        continue
                    begin = datetime.fromisoformat(
                        day["date"] + "T00:00:00+09:00"
                    ) + timedelta(minutes=30 * slot)
                    end = begin + timedelta(minutes=30)
                    running = any(
                        on < end and off > begin for on, off in zip(ons, offs)
                    )
                    self.assertEqual(
                        Decimal(kwh) > 0,
                        running,
                        (customer["customer_id"], day["date"], slot),
                    )

    def test_hot_weather_and_lower_setpoint_increase_observed_energy(self):
        customer = raw_customer(self.raw, "C006")
        hot = customer["days"][0]
        cool = customer["days"][2]
        self.assertGreater(Decimal(hot["kwh"][40]), Decimal(cool["kwh"][40]))
        # Same outdoor temperature at 18:30 and 19:00; 19:00 changes 24→22.
        self.assertGreater(Decimal(hot["kwh"][38]), Decimal(hot["kwh"][37]))

    def test_unused_zero_and_missing_are_not_interchangeable(self):
        weekend = next(
            day
            for day in self.reports["C022"]["billing"]["daily"]
            if day["usage"] == "unused"
        )
        self.assertEqual(Decimal(weekend["total_jpy"]), 0)
        self.assertTrue(all(Decimal(slot["kwh"]) == 0 for slot in weekend["intervals"]))
        missing = next(
            day
            for day in self.reports["C011"]["billing"]["daily"]
            if day["operation_status"] == "missing"
        )
        self.assertIsNone(missing["total_jpy"])
        self.assertTrue(all(slot["kwh"] is None for slot in missing["intervals"]))
        self.assertIsNone(self.reports["C023"]["billing"]["total_jpy"])
        self.assertGreater(
            Decimal(self.reports["C023"]["billing"]["known_subtotal_jpy"]), 0
        )

    def test_each_interval_and_daily_bill_is_energy_times_price(self):
        for report in self.reports.values():
            for day in report["billing"]["daily"]:
                calculated = Decimal(0)
                for slot in day["intervals"]:
                    if slot["kwh"] is None or slot["jpy_per_kwh"] is None:
                        self.assertIsNone(slot["jpy"])
                    else:
                        cost = Decimal(slot["kwh"]) * Decimal(slot["jpy_per_kwh"])
                        self.assertEqual(Decimal(slot["jpy"]), cost)
                        calculated += cost
                self.assertEqual(Decimal(day["known_subtotal_jpy"]), calculated)
                if day["total_jpy"] is not None:
                    self.assertEqual(Decimal(day["total_jpy"]), calculated)

    def test_precool_uses_predictions_and_same_comfort_baseline(self):
        item = find_decision(self.reports["C007"], "precool", 6)
        self.assertEqual(item["estimate"]["basis"], "synthetic_prediction")
        self.assertEqual(Decimal(item["estimate"]["costs_jpy"]["普段の24℃運転"]), 120)
        self.assertEqual(Decimal(item["estimate"]["costs_jpy"]["17時から予冷"]), 80)
        self.assertIn("40.00円", item["reason"])
        hot = find_decision(self.reports["C021"], "precool", 6)
        self.assertIn("70.00円", hot["reason"])
        self.assertTrue(
            any("30.00円" in text and "節約案と別" in text for text in hot["evidence"])
        )
        self.assertEqual(hot["values"]["start"], "17:00")
        self.assertEqual(hot["values"]["temperature_c"], "22")

    def test_no_judgment_history_is_not_the_same_as_no_operation_history(self):
        self.assertFalse(self.reports["C019"]["judgments"])
        self.assertEqual(self.reports["C019"]["proposal_count"], 1)
        self.assertEqual(self.reports["C014"]["proposal_count"], 0)
        self.assertEqual(
            self.reports["C014"]["coverage"]["operation_status"]["not_enrolled"], 27
        )

    def test_customer_ids_and_labels_are_only_identifiers(self):
        raw = copy.deepcopy(self.raw)
        customer = raw_customer(raw, "C002")
        customer.update(customer_id="unseen-customer", description="unseen-label")
        self.assertEqual(
            summarize(report_for(raw, "unseen-customer")), EXPECTED["C002"]
        )

    def test_operation_only_changes_move_weekday_start_and_temperature(self):
        raw = copy.deepcopy(self.raw)
        customer = raw_customer(raw, "C001")
        original_schedule = copy.deepcopy(customer["schedules"])
        for day in customer["days"]:
            if date.fromisoformat(day["date"]).weekday() == 2:
                day["operations"][0]["at"] = day["date"] + "T20:00:00+09:00"
        self.assertEqual(summarize(report_for(raw, "C001")), [("start", 2, "20:00")])
        for day in customer["days"]:
            weekday = date.fromisoformat(day["date"]).weekday()
            day["operations"][0]["at"] = day["date"] + (
                "T17:00:00+09:00" if weekday == 1 else "T18:30:00+09:00"
            )
            if int(day["date"][-2:]) % 3:
                day["operations"].append(
                    dict(
                        id=f"extra:{day['date']}",
                        device_id=customer["device_id"],
                        at=day["date"] + "T19:00:00+09:00",
                        source="web_manual",
                        action="temperature",
                        once=False,
                        from_c=24,
                        to_c=23,
                    )
                )
        self.assertEqual(
            summarize(report_for(raw, "C001")),
            [("start", 1, "17:00"), ("temperature", None, "23")],
        )
        self.assertEqual(customer["schedules"], original_schedule)

    def test_weather_only_changes_remove_hot_day_proposal(self):
        raw = copy.deepcopy(self.raw)
        customer = raw_customer(raw, "C004")
        for day in raw["sites"][customer["site_id"]]["observations"]:
            day["outdoor_c"] = ["27"] * 48
        self.assertEqual(summarize(report_for(raw, "C004")), [("start", 2, "20:00")])
        self.assertIn(
            "涼しい日", find_decision(report_for(raw, "C004"), "temperature")["reason"]
        )

    def test_tariff_only_changes_remove_savings(self):
        raw = copy.deepcopy(self.raw)
        raw["tariffs"]["time_of_use"]["jpy_per_kwh"][34:36] = ["100", "100"]
        report = report_for(raw, "C007")
        self.assertEqual(report["proposal_count"], 0)
        item = find_decision(report, "precool", 6)
        self.assertTrue(any("総額が下がらない" in text for text in item["evidence"]))

    def test_edited_choice_overrides_inference_and_future_choices_do_not(self):
        raw = copy.deepcopy(self.raw)
        raw_customer(raw, "C015")["judgments"][0]["selected"]["start"] = "21:00"
        self.assertEqual(summarize(report_for(raw, "C015")), [("start", 2, "21:00")])
        self.assertEqual(summarize(self.reports["C020"]), [("start", 2, "20:00")])
        self.assertEqual(
            self.reports["C020"]["judgments"][0]["response"], "no_response"
        )
        self.assertEqual(self.reports["C020"]["judgments"][0]["selected"], {})

    def test_judgment_only_changes_approval_rejection_and_no_response(self):
        for response, expected in (
            ("declined", []),
            ("no_response", EXPECTED["C002"]),
            ("approved", EXPECTED["C002"]),
            ("edited", [("start", 2, "19:30")]),
        ):
            with self.subTest(response=response):
                raw = copy.deepcopy(self.raw)
                raw_customer(raw, "C002")["judgments"] = [
                    history_record(
                        self.reports["C002"]["proposals"][0],
                        response,
                        {"start": "19:30"} if response == "edited" else {},
                    )
                ]
                self.assertEqual(summarize(report_for(raw, "C002")), expected)

    def test_same_declined_evidence_is_suppressed_but_new_operations_are_reconsidered(
        self,
    ):
        raw = copy.deepcopy(self.raw)
        customer = raw_customer(raw, "C016")
        for day in customer["days"]:
            if date.fromisoformat(day["date"]).weekday() == 2:
                day["operations"][0]["at"] = day["date"] + "T21:00:00+09:00"
        result = report_for(raw, "C016")
        self.assertEqual(summarize(result), [("start", 2, "21:00")])
        self.assertTrue(
            any(
                "根拠が変わった" in text
                for text in find_decision(result, "start", 2)["judgment_influence"]
            )
        )
        self.assertEqual(self.reports["C017"]["proposal_count"], 1)

    def test_unrelated_weather_and_tariff_do_not_unsuppress_a_declined_start(self):
        raw = copy.deepcopy(self.raw)
        raw["tariffs"]["time_of_use"]["jpy_per_kwh"] = ["50"] * 48
        for site in raw["sites"].values():
            for day in site["observations"]:
                day["outdoor_c"] = ["25"] * 48
        self.assertEqual(report_for(raw, "C016")["proposal_count"], 0)

    def test_later_approval_supersedes_an_older_decline(self):
        raw = copy.deepcopy(self.raw)
        customer = raw_customer(raw, "C016")
        record = copy.deepcopy(customer["judgments"][0])
        record.update(
            id="later-approval",
            response="approved",
            responded_at="2026-10-31T10:00:00+09:00",
        )
        customer["judgments"].append(record)
        self.assertEqual(summarize(report_for(raw, "C016")), EXPECTED["C002"])

    def test_operations_and_judgments_after_cutoff_do_not_leak(self):
        raw = copy.deepcopy(self.raw)
        raw["as_of"] = "2026-10-25T12:00:00+09:00"
        before = report_for(raw, "C002")
        customer = raw_customer(raw, "C002")
        for day in customer["days"]:
            if day["date"] >= "2026-10-25":
                for op in day["operations"]:
                    if op["action"] == "on":
                        op["at"] = day["date"] + "T16:00:00+09:00"
        future = history_record(before["proposals"][0], "edited", {"start": "17:00"})
        future.update(
            proposed_at="2026-10-26T08:00:00+09:00",
            responded_at="2026-10-26T09:00:00+09:00",
        )
        customer["judgments"].append(future)
        self.assertEqual(report_for(raw, "C002"), before)

    def test_changes_to_one_customers_operations_and_judgments_do_not_affect_others(
        self,
    ):
        raw = copy.deepcopy(self.raw)
        customer = raw_customer(raw, "C002")
        for day in customer["days"]:
            if date.fromisoformat(day["date"]).weekday() == 2:
                day["operations"][0]["at"] = day["date"] + "T21:00:00+09:00"
        raw_customer(raw, "C015")["judgments"][0]["selected"]["start"] = "17:00"
        reports = {r["customer_id"]: r for r in evaluate_bundle(normalize_bundle(raw))}
        for customer_id, report in self.reports.items():
            if customer_id not in ("C002", "C015"):
                self.assertEqual(reports[customer_id], report, customer_id)

    def test_model_outputs_are_not_replaced_with_observed_energy(self):
        raw = copy.deepcopy(self.raw)
        customer = raw_customer(raw, "C007")
        for day in customer["days"]:
            day["kwh"] = ["0"] * 48
        after = report_for(raw, "C007")
        self.assertEqual(after["proposals"], self.reports["C007"]["proposals"])
        self.assertEqual(Decimal(after["billing"]["total_jpy"]), 0)

    def test_forecast_change_requires_candidate_prediction_refresh(self):
        raw = copy.deepcopy(self.raw)
        customer = raw_customer(raw, "C007")
        raw["sites"][customer["site_id"]]["forecast"][1]["outdoor_c"][40] = "40"
        result = find_decision(report_for(raw, "C007"), "precool", 6)
        self.assertEqual(result["status"], "unavailable")
        self.assertIn("前提が一致", result["reason"])

    def test_hot_temperature_edit_requires_matching_comfort_prediction(self):
        raw = copy.deepcopy(self.raw)
        heat = find_decision(self.reports["C021"], "temperature")
        raw_customer(raw, "C021")["judgments"] = [
            history_record(heat, "edited", {"temperature_c": "23"})
        ]
        report = report_for(raw, "C021")
        self.assertEqual(
            find_decision(report, "temperature")["values"]["temperature_c"], "23"
        )
        self.assertEqual(find_decision(report, "precool", 6)["status"], "unavailable")

    def test_candidate_missing_comfort_and_missing_tariff_prevent_savings_claim(self):
        for field in ("comfort", "tariff"):
            with self.subTest(field=field):
                raw = copy.deepcopy(self.raw)
                if field == "comfort":
                    raw_customer(raw, "C007")["predictions"][0]["plans"][-1][
                        "indoor_c_at_required"
                    ] = None
                else:
                    raw["tariffs"]["time_of_use"]["jpy_per_kwh"][35] = None
                self.assertEqual(report_for(raw, "C007")["proposal_count"], 0)

    def test_initial_schedule_needs_off_and_temperature_on_common_dates(self):
        raw = copy.deepcopy(self.raw)
        customer = raw_customer(raw, "C005")
        for day in customer["days"]:
            if date.fromisoformat(day["date"]).weekday() == 2:
                day["operations"] = [
                    op for op in day["operations"] if op["action"] != "off"
                ]
        self.assertEqual(
            summarize(report_for(raw, "C005")), [("add_schedule", 0, "19:00")]
        )

    def test_normalization_and_evaluation_do_not_mutate_the_input(self):
        raw = copy.deepcopy(self.raw)
        original = copy.deepcopy(raw)
        report_for(raw, "C021")
        self.assertEqual(raw, original)

    def test_invalid_monthly_inputs_produce_input_errors(self):
        mutations = (
            lambda raw: raw["customers"][0]["days"].pop(),
            lambda raw: raw["customers"][0]["days"][0]["kwh"].pop(),
            lambda raw: raw["customers"][0].update(site_id="absent"),
            lambda raw: raw["customers"][1].update(customer_id="C001"),
            lambda raw: raw["customers"][0]["days"][0]["operations"][0].update(
                device_id="another"
            ),
            lambda raw: raw["customers"][0]["days"][0].update(usage="unused"),
            lambda raw: raw["customers"][6]["predictions"][0].update(start="17:15"),
        )
        for index, mutate in enumerate(mutations):
            with self.subTest(mutation=index):
                raw = copy.deepcopy(self.raw)
                mutate(raw)
                with self.assertRaises(InputError):
                    normalize_bundle(raw)

    def test_cli_summary_and_individual_day_json(self):
        out = io.StringIO()
        with redirect_stdout(out):
            self.assertEqual(main(["--customers", "--json"]), 0)
        rows = json.loads(out.getvalue())
        self.assertEqual(
            {r["customer_id"]: r["proposal_count"] for r in rows},
            {key: len(value) for key, value in EXPECTED.items()},
        )
        out = io.StringIO()
        with redirect_stdout(out):
            self.assertEqual(
                main(["--customer", "C023", "--day", "2026-10-14", "--json"]), 0
            )
        report = json.loads(out.getvalue())
        self.assertEqual(len(report["day_detail"]["intervals"]), 48)
        self.assertEqual(report["day_detail"]["weather_kind"], "observation")
        self.assertTrue(report["day_detail"]["operations"])
        self.assertIsNone(report["day_detail"]["total_jpy"])

    def test_fixed_cli_command_follows_only_operation_data_changes(self):
        raw = copy.deepcopy(self.raw)
        with TemporaryDirectory(dir="/tmp") as directory:
            path = Path(directory) / "month.json"
            command = ["--customer", "C001", "--monthly-data", str(path), "--json"]
            path.write_text(json.dumps(raw), encoding="utf-8")
            before = io.StringIO()
            with redirect_stdout(before):
                self.assertEqual(main(command), 0)
            for day in raw_customer(raw, "C001")["days"]:
                if date.fromisoformat(day["date"]).weekday() == 4:
                    day["operations"][0]["at"] = day["date"] + "T21:00:00+09:00"
            path.write_text(json.dumps(raw), encoding="utf-8")
            after = io.StringIO()
            with redirect_stdout(after):
                self.assertEqual(main(command), 0)
            self.assertEqual(json.loads(before.getvalue())["proposal_count"], 0)
            self.assertEqual(
                summarize(json.loads(after.getvalue())), [("start", 4, "21:00")]
            )

    def test_cli_missing_customer_invalid_day_and_malformed_file(self):
        err = io.StringIO()
        with redirect_stderr(err):
            self.assertEqual(main(["--customer", "unknown"]), 2)
            self.assertEqual(main(["--customer", "C001", "--day", "bad-date"]), 2)
        self.assertIn("入力エラー", err.getvalue())

    def test_selected_precool_still_requires_cost_and_comfort_checks(self):
        raw = copy.deepcopy(self.raw)
        proposal = find_decision(self.reports["C007"], "precool", 6)
        raw_customer(raw, "C007")["judgments"] = [history_record(proposal, "approved")]
        raw["tariffs"]["time_of_use"]["jpy_per_kwh"][34:36] = ["100", "100"]
        self.assertEqual(report_for(raw, "C007")["proposal_count"], 0)

    def test_forecast_baseline_must_match_current_schedule(self):
        raw = copy.deepcopy(self.raw)
        customer = raw_customer(raw, "C007")
        customer["schedules"][6]["temperature_c"] = 23
        report = report_for(raw, "C007")
        item = find_decision(report, "precool", 6)
        self.assertEqual(item["status"], "unavailable")
        self.assertIn("現行予定", item["reason"])

    def test_edited_temperature_only_constrains_its_applicable_weekdays(self):
        raw = copy.deepcopy(self.raw)
        customer = raw_customer(raw, "C021")
        customer["schedules"][6]["temperature_c"] = 26
        prediction = customer["predictions"][0]
        prediction["target_c"] = 26
        prediction["plans"][0]["controls"]["temperature_c"] = 26
        prediction["plans"][0]["indoor_c_at_required"] = 26
        prediction["plans"] = [
            plan for plan in prediction["plans"] if plan["kind"] != "comfort"
        ]
        prediction["plans"][-1]["controls"]["temperature_c"] = 26
        prediction["plans"][-1]["indoor_c_at_required"] = 26
        report = report_for(raw, "C021")
        self.assertEqual(find_decision(report, "precool", 6)["status"], "proposed")

    def test_unrelated_tariff_slots_do_not_unsuppress_declined_precool(self):
        raw = copy.deepcopy(self.raw)
        proposal = find_decision(self.reports["C007"], "precool", 6)
        raw_customer(raw, "C007")["judgments"] = [history_record(proposal, "declined")]
        raw["tariffs"]["time_of_use"]["jpy_per_kwh"][0] = "100"
        self.assertEqual(
            find_decision(report_for(raw, "C007"), "precool", 6)["status"], "suppressed"
        )

    def test_automatic_on_then_temperature_change_is_not_a_manual_start(self):
        customer = raw_customer(self.raw, "C025")
        for day in customer["days"]:
            if date.fromisoformat(day["date"]).weekday() != 2:
                continue
            on = next(op for op in day["operations"] if op["action"] == "on")
            change = next(
                op for op in day["operations"] if op["action"] == "temperature"
            )
            self.assertEqual((on["at"][11:16], on["source"]), ("18:30", "automation"))
            self.assertEqual(
                (change["at"][11:16], change["source"]), ("20:00", "web_manual")
            )
            self.assertTrue(all(Decimal(kwh) == 0 for kwh in day["kwh"][:37]))
            self.assertGreater(
                Decimal(day["kwh"][39]), 0
            )  # Already running before 20:00.
        item = find_decision(self.reports["C025"], "start", 2)
        self.assertEqual(item["status"], "unchanged")
        self.assertEqual(item["support_count"], 0)
        self.assertEqual(item["evidence_ids"], [])
        self.assertFalse(
            any(
                p["kind"] == "start" and p["weekday"] == 2
                for p in self.reports["C025"]["proposals"]
            )
        )
        self.assertTrue(
            any(p["kind"] == "temperature" for p in self.reports["C025"]["proposals"])
        )

    def test_three_early_manual_starts_have_exact_dated_evidence_and_keep_end(self):
        customer = raw_customer(self.raw, "C026")
        power = None
        starts = []
        manual_ids = []
        for day in customer["days"]:
            for op in day["operations"]:
                if (
                    op["action"] == "on"
                    and date.fromisoformat(day["date"]).weekday() == 2
                ):
                    self.assertIs(
                        power, False
                    )  # Previous day's recorded off, no intervening on.
                    starts.append((day["date"], op["at"][11:16], op["source"]))
                    if op["source"] == "web_manual":
                        manual_ids.append(op["id"])
                if op["action"] in ("on", "off"):
                    power = op["action"] == "on"
        self.assertEqual(
            starts,
            [
                ("2026-10-07", "17:20", "web_manual"),
                ("2026-10-14", "17:30", "web_manual"),
                ("2026-10-21", "17:40", "web_manual"),
                ("2026-10-28", "18:30", "automation"),
            ],
        )
        item = find_decision(self.reports["C026"], "start", 2)
        self.assertEqual(item["status"], "proposed")
        self.assertEqual(item["values"]["start"], "17:30")
        self.assertEqual(item["values"]["end"], "23:00")
        self.assertEqual(item["support_count"], 3)
        self.assertEqual(item["evidence_ids"], sorted(manual_ids))
        for day, time, source in starts:
            if source == "web_manual":
                self.assertTrue(
                    any(day in line and time in line for line in item["evidence"])
                )

    def test_variable_weekly_starts_show_dispersion_not_absence_of_history(self):
        customer = raw_customer(self.raw, "C027")
        times = [
            op["at"][11:16]
            for day in customer["days"]
            if date.fromisoformat(day["date"]).weekday() == 2
            for op in day["operations"]
            if op["action"] == "on"
        ]
        self.assertEqual(times, ["17:10", "18:00", "18:50", "19:40"])
        item = find_decision(self.reports["C027"], "start", 2)
        self.assertNotEqual(item["status"], "proposed")
        self.assertEqual(len(item["evidence_ids"]), 4)
        self.assertEqual(item["support_count"], 1)
        self.assertIn("履歴は4日", item["reason"])
        self.assertIn("時刻が分散", item["reason"])
        for time in times:
            self.assertTrue(any(time in line for line in item["evidence"]))

    def test_mixed_start_inference_follows_times_and_not_customer_identity(self):
        raw = copy.deepcopy(self.raw)
        customer = raw_customer(raw, "C026")
        customer.update(customer_id="renamed-mixed", description="unseen")
        for day in customer["days"]:
            if date.fromisoformat(day["date"]).weekday() == 2:
                for op in day["operations"]:
                    if op["action"] == "on" and op["source"] == "web_manual":
                        op["at"] = (
                            datetime.fromisoformat(op["at"]) - timedelta(hours=1)
                        ).isoformat()
        item = find_decision(report_for(raw, "renamed-mixed"), "start", 2)
        self.assertEqual(item["values"]["start"], "16:30")
        self.assertEqual(item["support_count"], 3)


if __name__ == "__main__":
    unittest.main()
