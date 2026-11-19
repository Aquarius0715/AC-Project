"""Test of deploy/load/iot_check.py: a fair run within the section 13 target passes, and each check fails the run on its own
(slow probes, a lost probe, no probe at all, a generator behind its pace, a drop or a refusal by the broker, readings not
stored after the drain), with the exit status 1 that deploy/load/run.sh reads.

usage: python3 -m unittest deploy/load/test_iot_check.py
"""

import copy
import json
import os
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
SCRIPT = os.path.join(HERE, "iot_check.py")

# A report as service/api/internal/iotload's Report writes it: 100 devices every 10 s for 60 s, 10 a second asked.
GOOD = {
    "durationSeconds": 60.0,
    "devices": 100,
    "intervalSeconds": 10.0,
    "targetPerSecond": 10.0,
    "published": 600,
    "publishFailed": 0,
    "dropped": 0,
    "heartbeats": 12,
    "commandsAnswered": 3,
    "expectedReadings": 1000,
    "storedReadings": 1000,
    "drainSeconds": 4.0,
    "probes": {"sent": 20, "arrived": 20, "lost": 0, "p50Ms": 120.0, "p95Ms": 900.0, "p99Ms": 1500.0, "maxMs": 2100.0},
}


class IoTCheckTest(unittest.TestCase):
    def run_check(self, report: dict) -> subprocess.CompletedProcess:
        with tempfile.TemporaryDirectory() as tmp:
            path = os.path.join(tmp, "iotload.json")
            with open(path, "w", encoding="utf-8") as f:
                json.dump(report, f)
            return subprocess.run([sys.executable, "-I", SCRIPT, path], capture_output=True, text=True, check=False)

    def failed(self, report: dict) -> list[str]:
        """The names of the checks the run failed; also asserts the exit status matches."""
        result = self.run_check(report)
        names = [line.split("FAILED", 1)[1].strip().split(":", 1)[0] for line in result.stdout.splitlines() if "FAILED" in line]
        self.assertEqual(result.returncode, 1 if names else 0, result.stdout + result.stderr)
        return names

    def changed(self, **fields) -> dict:
        report = copy.deepcopy(GOOD)
        for key, value in fields.items():
            if key.startswith("probe_"):
                report["probes"][key.removeprefix("probe_")] = value
            else:
                report[key] = value
        return report

    def test_fair_run_within_target_passes(self):
        result = self.run_check(GOOD)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("100 devices every 10 s for 60 s; 12 heartbeats, 3 commands answered", result.stdout)
        self.assertEqual(result.stdout.count("ok    "), 5)

    def test_boundaries_hold(self):
        # p95 exactly 10 s, exactly 95 % of the pace and exactly 99 % stored still pass
        self.assertEqual(self.failed(self.changed(probe_p95Ms=10_000.0, published=570, storedReadings=990)), [])

    def test_slow_probes_fail(self):
        self.assertEqual(self.failed(self.changed(probe_p95Ms=10_001.0)), ["probe readings stored within 10 s (p95)"])

    def test_no_probe_arrived_fails(self):
        # a run whose probes never arrived has no delay to judge, even though p95 reads 0
        names = self.failed(self.changed(probe_arrived=0, probe_lost=20, probe_p95Ms=0.0))
        self.assertEqual(names, ["probe readings stored within 10 s (p95)", "no probe reading lost"])

    def test_lost_probe_fails(self):
        self.assertEqual(self.failed(self.changed(probe_arrived=19, probe_lost=1)), ["no probe reading lost"])

    def test_generator_behind_its_pace_fails(self):
        self.assertEqual(self.failed(self.changed(published=569)), ["the generator kept its pace"])
        # nothing asked (no duration) is not a pace kept
        self.assertIn("the generator kept its pace", self.failed(self.changed(durationSeconds=0.0)))

    def test_dropped_or_refused_fails(self):
        self.assertEqual(self.failed(self.changed(dropped=1)), ["nothing dropped or refused by the broker"])
        self.assertEqual(self.failed(self.changed(publishFailed=1)), ["nothing dropped or refused by the broker"])

    def test_readings_not_stored_fail(self):
        self.assertEqual(self.failed(self.changed(storedReadings=989)), ["readings sent are stored after the drain"])
        # nothing expected is not everything stored
        self.assertEqual(
            self.failed(self.changed(expectedReadings=0, storedReadings=0)), ["readings sent are stored after the drain"]
        )


if __name__ == "__main__":
    unittest.main()
