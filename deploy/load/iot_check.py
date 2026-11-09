"""Judges the telemetry half of an IoT load run (deploy/load, phase 3): reads the report cmd/iotload wrote and prints what it
found against the targets of docs/02-design/backend-architecture.md section 13 (PROPOSED values): a reading is stored within
10 seconds of being sent (p95). The command round trip (p95 within 5 seconds) is judged by k6 itself.

It also checks that the run was a fair one: the generator kept its pace (so a slow broker or bridge does not look like a
quiet system), nothing was dropped or refused, and the readings sent were all stored once the backlog had drained.

usage: python3 deploy/load/iot_check.py IOTLOAD_JSON
exit status: 0 when every check holds, 1 when one does not.
"""

import json
import sys

FRESHNESS_P95_MS = 10_000  # section 13: telemetry freshness p95 within 10 seconds
PACE_MIN = 0.95  # the generator has to have sent at least this part of what its schedule asked for
STORED_MIN = 0.99  # at least this part of the readings sent has to be stored after the drain


def checks(report: dict) -> list[tuple[str, bool, str]]:
    """Each check as (name, held, what was seen)."""
    probes = report["probes"]
    asked = report["targetPerSecond"] * report["durationSeconds"]
    pace = report["published"] / asked if asked else 0.0
    stored = report["storedReadings"] / report["expectedReadings"] if report["expectedReadings"] else 0.0
    return [
        (
            "probe readings stored within 10 s (p95)",
            probes["arrived"] > 0 and probes["p95Ms"] <= FRESHNESS_P95_MS,
            f"p50 {probes['p50Ms']:.0f} ms, p95 {probes['p95Ms']:.0f} ms, p99 {probes['p99Ms']:.0f} ms, max {probes['maxMs']:.0f} ms",
        ),
        ("no probe reading lost", probes["lost"] == 0, f"{probes['lost']} of {probes['sent']} never stored"),
        (
            "the generator kept its pace",
            pace >= PACE_MIN,
            f"{report['published']} batches of {asked:.0f} asked ({pace:.1%}; target {report['targetPerSecond']:.0f} a second)",
        ),
        (
            "nothing dropped or refused by the broker",
            report["dropped"] == 0 and report["publishFailed"] == 0,
            f"{report['dropped']} dropped, {report['publishFailed']} failed",
        ),
        (
            "readings sent are stored after the drain",
            stored >= STORED_MIN,
            f"{report['storedReadings']} of {report['expectedReadings']} ({stored:.1%}), backlog drained in {report['drainSeconds']:.0f} s",
        ),
    ]


def main() -> int:
    with open(sys.argv[1], encoding="utf-8") as f:
        report = json.load(f)
    print(
        f"\nIoT telemetry: {report['devices']} devices every {report['intervalSeconds']:.0f} s for "
        f"{report['durationSeconds']:.0f} s; {report['heartbeats']} heartbeats, {report['commandsAnswered']} commands answered"
    )
    failed = False
    for name, held, seen in checks(report):
        print(f"  {'ok    ' if held else 'FAILED'}  {name}: {seen}")
        failed = failed or not held
    print("  (the targets are the PROPOSED values of backend-architecture section 13; the delay includes the broker, the bridge and the Core API)")
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
