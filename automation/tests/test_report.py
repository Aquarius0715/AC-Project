"""Generation and offline-report tests, all artifacts confined to automation/."""

import copy
import io
import json
import unittest
from contextlib import redirect_stdout, redirect_stderr
from datetime import datetime
from html.parser import HTMLParser
from pathlib import Path
from tempfile import TemporaryDirectory

from automation.src.cli import main
from automation.src.dataset import ROOT, load_dataset
from automation.src.generate import make_dataset
from automation.src.html_report import page, write_report
from automation.src.monthly_engine import signature
from automation.src.replay import replay_customer, replay_dataset


class Scripts(HTMLParser):
    def __init__(self, text):
        super().__init__()
        self.data = ""
        self.capture = False
        self.external = []
        self.feed(text)

    def handle_starttag(self, tag, attrs):
        attrs = dict(attrs)
        if tag == "script" and attrs.get("id") == "data":
            self.capture = True
        if tag in ("script", "link") and (
            attrs.get("src") or attrs.get("href", "").startswith("http")
        ):
            self.external.append(attrs)

    def handle_endtag(self, tag):
        if tag == "script":
            self.capture = False

    def handle_data(self, data):
        if self.capture:
            self.data += data


class OfflineReportTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        (ROOT / "work").mkdir(exist_ok=True)
        cls.dataset = load_dataset()
        with redirect_stdout(io.StringIO()):
            cls.result = replay_dataset(cls.dataset, ["H001"])

    def test_report_is_direct_file_html_with_embedded_data_and_relative_links(self):
        with TemporaryDirectory(dir=ROOT / "work") as directory:
            output = Path(directory) / "report"
            index = write_report(self.result, output)
            parsed = Scripts(index.read_text())
            self.assertEqual(json.loads(parsed.data)["new_proposal_count"], 2)
            self.assertEqual(parsed.external, [])
            customer = output / "customers/H001.html"
            parsed = Scripts(customer.read_text())
            self.assertEqual(len(json.loads(parsed.data)["days"]), 90)
            self.assertEqual(parsed.external, [])
            self.assertIn('href="../index.html"', customer.read_text())
            self.assertNotIn("fetch(", customer.read_text())

    def test_edited_text_cannot_close_the_embedded_json_script(self):
        payload = {"label": "</script><img src=x onerror=alert(1)>"}
        result = page("report", "<main></main>", payload, "")
        self.assertNotIn("</script><img", result)
        self.assertEqual(json.loads(Scripts(result).data), payload)

    def test_fixed_seed_is_byte_reproducible_across_output_folders(self):
        with TemporaryDirectory(dir=ROOT / "work") as directory:
            first = Path(directory) / "a"
            second = Path(directory) / "b"
            make_dataset(first, 1, 28, 42)
            make_dataset(second, 1, 28, 42)
            files = sorted(p.relative_to(first) for p in first.rglob("*.json"))
            self.assertTrue(files)
            for name in files:
                self.assertEqual(
                    (first / name).read_bytes(), (second / name).read_bytes(), str(name)
                )
            make_dataset(second, 1, 28, 43)
            self.assertNotEqual(
                (first / "customers/H001.json").read_bytes(),
                (second / "customers/H001.json").read_bytes(),
            )
            make_dataset(second, 2, 28, 42)
            self.assertTrue((second / "customers/H002.json").exists())
            make_dataset(second, 1, 28, 42)
            self.assertFalse((second / "customers/H002.json").exists())

    def test_single_customer_generation_matches_its_full_dataset_input(self):
        with TemporaryDirectory(dir=ROOT / "work") as directory:
            path = Path(directory) / "single"
            make_dataset(path, 1, 90, 42)
            self.assertEqual(
                (path / "customers/H001.json").read_bytes(),
                (self.dataset.path / "customers/H001.json").read_bytes(),
            )

    def test_cli_generates_replays_and_writes_all_results_under_automation(self):
        with TemporaryDirectory(dir=ROOT / "work") as directory:
            path = Path(directory)
            with redirect_stdout(io.StringIO()):
                self.assertEqual(
                    main(
                        [
                            "generate",
                            "--customers",
                            "1",
                            "--days",
                            "28",
                            "--output",
                            str(path / "data"),
                        ]
                    ),
                    0,
                )
                self.assertEqual(
                    main(
                        [
                            "replay",
                            "--data",
                            str(path / "data"),
                            "--customer",
                            "H001",
                            "--output",
                            str(path / "results"),
                        ]
                    ),
                    0,
                )
                self.assertEqual(
                    main(
                        [
                            "snapshot",
                            "--data",
                            str(path / "data"),
                            "--output",
                            str(path / "snapshot"),
                        ]
                    ),
                    0,
                )
            self.assertTrue((path / "results/index.html").exists())
            self.assertEqual(
                json.loads((path / "results/summary.json").read_text())["day_count"], 28
            )
            self.assertEqual(
                json.loads((path / "snapshot/summary.json").read_text())["day_count"], 1
            )

    def test_a_precool_candidate_remaining_for_two_days_is_not_counted_twice(self):
        from test_dataset import single

        dataset = single(self.dataset, "H011")
        target = "2026-08-04"
        weather_id = dataset.customers["H011"]["installation"]["weather_site_id"]
        forecast = next(
            r for r in dataset.weather[weather_id]["forecast"] if r["date"] == target
        )
        forecast["issued_at"] = "2026-08-03T06:00:00+09:00"
        for dev in dataset.customers["H011"]["devices"]:
            candidate = next(p for p in dev["predictions"] if p["date"] == target)
            candidate["generated_at"] = "2026-08-03T07:00:00+09:00"
        prepared = dataset.prepare("H011")
        known = prepared.full.monthly.sites[weather_id]["forecast"][
            datetime.fromisoformat(target).date()
        ]
        for dev in dataset.customers["H011"]["devices"]:
            next(p for p in dev["predictions"] if p["date"] == target)[
                "weather_signature"
            ] = signature(known)
        report = replay_customer(dataset, "H011")
        events = [
            e
            for r in report["days"]
            for e in r["events"]
            if e["kind"] == "precool"
            and e.get("target_date") == target
            and e["event"] != "ended"
        ]
        self.assertEqual([e["event"] for e in events], ["first", "continued"])
        self.assertEqual(sum(e["is_new"] for e in events), 1)
        self.assertEqual(events[0]["proposal_id"], events[1]["proposal_id"])

    def test_future_schedule_revision_does_not_rewrite_history(self):
        from test_dataset import single

        dataset = single(self.dataset, "H001")
        for device in dataset.customers["H001"]["devices"]:
            version = copy.deepcopy(device["schedule_versions"][-1])
            version.update(
                valid_from="2026-11-02T00:00:00+09:00",
                recorded_at="2026-11-01T12:10:00+09:00",
            )
            for plan in version["schedules"]:
                plan["start"] = "22:00"
            device["schedule_versions"].append(version)
        self.assertEqual(
            replay_customer(dataset, "H001")["days"],
            self.result["customers"][0]["days"],
        )

    def test_unknown_customer_and_external_output_fail_without_writes(self):
        with redirect_stderr(io.StringIO()):
            self.assertEqual(main(["replay", "--customer", "missing"]), 2)
            self.assertEqual(
                main(
                    [
                        "generate",
                        "--customers",
                        "1",
                        "--output",
                        "/tmp/disallowed-ac-data",
                    ]
                ),
                2,
            )
