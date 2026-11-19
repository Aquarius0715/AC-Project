"""Replay two genuinely displayed proposals for the same device and weekday."""

import copy
import unittest
from datetime import date, datetime
from decimal import Decimal

from automation.src.approval_state import card_approval_states
from automation.src.dataset import ROOT, load_dataset
from automation.src.replay import replay_customer
from automation.src.simulation import measured_energy
from test_approval_history import at, day, version
from test_dataset import single


def monday_plan(schedule_version):
    return next(s for s in schedule_version["schedules"] if s["weekday"] == 0)


def replacement_case():
    source = load_dataset(ROOT / "data" / "review-cases")
    dataset = single(source, "G003")
    house = dataset.customers["G003"]
    device = house["devices"][0]
    partial = device["schedule_versions"][1]
    monday_plan(partial)["start"] = "18:30"
    for valid_from, start in (("2026-09-18", "21:00"), ("2026-09-25", "20:00")):
        edited = copy.deepcopy(device["schedule_versions"][-1])
        edited.update(
            valid_from=valid_from + "T00:00:00+09:00",
            recorded_at=valid_from + "T00:00:00+09:00",
        )
        monday_plan(edited)["start"] = start
        device["schedule_versions"].append(edited)
    weather = {
        r["date"]: r["outdoor_c"]
        for r in dataset.weather[device["site_id"]]["observations"]
    }
    manual_template = next(
        row["operations"] for row in device["days"] if row["date"] == "2026-08-24"
    )
    for row in device["days"]:
        if row["date"] not in (
            "2026-08-31",
            "2026-09-07",
            "2026-09-14",
            "2026-09-21",
        ):
            continue
        pending = row["date"] < "2026-09-18"
        operations = copy.deepcopy(manual_template if pending else row["operations"])
        for op in operations:
            old = datetime.fromisoformat(op["at"])
            clock = old.strftime("%H:%M")
            replacement = {
                "20:00": "21:00",
                "20:05": "21:05",
            }.get(clock, clock)
            new = datetime.fromisoformat(f"{row['date']}T{replacement}:00+09:00")
            shift = new - old
            op["id"] = (
                f"{device['device_id']}:{row['date']}:{op['action']}:{replacement}"
            )
            for obj, key in (
                (op, "at"),
                (op, "received_at"),
                (op["power"], "before_at"),
                (op["power"], "after_at"),
                (op["power"], "received_at"),
            ):
                obj[key] = (datetime.fromisoformat(obj[key]) + shift).isoformat()
        row.update(
            operations=operations,
            kwh=measured_energy(
                date.fromisoformat(row["date"]),
                operations,
                weather[row["date"]],
                Decimal(1),
            ),
        )

    preview = replay_customer(dataset, "G003")
    shown = day(preview, "2026-09-15")
    card = next(
        c
        for c in shown["cards"]
        if c["kind"] == "start"
        and any(
            m["weekday"] == 0 and m["values"]["start"] == "21:00" for m in c["members"]
        )
    )
    member = next(m for m in card["members"] if m["weekday"] == 0)
    house["card_responses"].append(
        dict(
            id="G003-second-monday-approval",
            card_id=card["card_id"],
            proposed_at=shown["decision_at"],
            responded_at="2026-09-15T12:05:00+09:00",
            members=[
                dict(
                    {
                        k: member[k]
                        for k in (
                            "device_id",
                            "weekday",
                            "proposal_id",
                            "values",
                            "evidence_signature",
                            "evidence_ids",
                        )
                    },
                    decision="approved",
                )
            ],
        )
    )
    return dataset


def monday_states(row):
    return [s for s in row["devices"][0]["approval_states"] if s["weekday"] == 0]


class ApprovalReplacementReplayTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.dataset = replacement_case()
        cls.report = replay_customer(cls.dataset, "G003")

    def test_pending_replaced_applied_and_later_matching_edit(self):
        for stamp, current, expected in (
            ("2026-09-15", "18:30", [("20:00", "approved_pending", None)]),
            (
                "2026-09-16",
                "18:30",
                [
                    ("20:00", "replaced_by_approval", None),
                    ("21:00", "approved_pending", None),
                ],
            ),
            (
                "2026-09-18",
                "21:00",
                [
                    ("20:00", "replaced_by_approval", None),
                    ("21:00", "applied", "2026-09-18T00:00:00+09:00"),
                ],
            ),
            (
                "2026-09-25",
                "20:00",
                [
                    ("20:00", "replaced_by_approval", None),
                    ("21:00", "applied", "2026-09-18T00:00:00+09:00"),
                ],
            ),
        ):
            with self.subTest(day=stamp):
                states = monday_states(day(self.report, stamp))
                self.assertEqual(
                    [
                        (s["values"]["start"], s["status"], s["applied_at"])
                        for s in states
                    ],
                    expected,
                )
                self.assertTrue(
                    all(s["current_schedule"]["start"] == current for s in states)
                )
                if len(states) == 2:
                    self.assertEqual(
                        states[0]["replaced_at"], "2026-09-15T12:05:00+09:00"
                    )
                    self.assertEqual(
                        states[0]["replacement_proposal_id"], states[1]["proposal_id"]
                    )
        decision = next(
            i
            for i in day(self.report, "2026-09-16")["devices"][0]["decisions"]
            if i["kind"] == "start" and i["weekday"] == 0
        )
        self.assertEqual(decision["values"]["start"], "21:00")
        self.assertEqual(decision["status"], "approved_pending")

    def test_snapshot_agrees_at_each_transition(self):
        for stamp in ("2026-09-15", "2026-09-16", "2026-09-18", "2026-09-25"):
            with self.subTest(day=stamp):
                dataset = single(self.dataset, "G003")
                dataset.config.update(end_date=stamp, as_of=at(stamp).isoformat())
                for device in dataset.customers["G003"]["devices"]:
                    device["days"] = [r for r in device["days"] if r["date"] < stamp]
                result = replay_customer(dataset, "G003", mode="snapshot")
                self.assertEqual(result["days"][0], day(self.report, stamp))


class ApprovalReplacementScopeTests(unittest.TestCase):
    def test_only_later_approval_for_same_device_and_weekday_replaces(self):
        old = dict(
            id="old",
            origin="card_response",
            response="approved",
            device_id="AC1",
            kind="start",
            weekday=0,
            proposal_id="P1",
            proposed_at="2026-08-24T12:00:00+09:00",
            responded_at="2026-08-24T12:05:00+09:00",
            values=dict(start="20:00", end="23:00", temperature_c="24"),
        )
        new = dict(
            old,
            id="new",
            proposal_id="P2",
            responded_at="2026-08-26T12:05:00+09:00",
            values=dict(old["values"], start="21:00"),
        )
        versions = [version("2026-08-02T00:00:00+09:00", 1110)]
        for override in (
            dict(device_id="AC2"),
            dict(weekday=1),
            dict(response="excluded_once"),
        ):
            with self.subTest(override=override):
                states = card_approval_states(
                    [dict(new, **override), old],
                    "AC1",
                    {},
                    versions=versions,
                    at=at("2026-08-27"),
                )
                state = next(s for s in states if s["proposal_id"] == "P1")
                self.assertEqual(state["status"], "approved_pending")
        states = card_approval_states(
            [new, old], "AC1", {}, versions=versions, at=at("2026-08-26")
        )
        self.assertEqual(
            [(s["proposal_id"], s["status"]) for s in states],
            [("P1", "approved_pending")],
        )
