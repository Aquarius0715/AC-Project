"""Four editable 90-day households for grouping and per-weekday responses."""

import argparse
import copy
from datetime import date, datetime, timedelta
from decimal import Decimal
from pathlib import Path

from .dataset import ROOT, inside_automation, load_dataset, write_json
from .generate import make_dataset
from .replay import replay_customer
from .simulation import measured_energy, operation_record
from .synthetic import phone_observation


def make_weekday_demo(output=ROOT / "data" / "weekday-demo", seed=42):
    output = inside_automation(Path(output))
    base = make_dataset(output, 4, 90, seed)
    start = date.fromisoformat(base.config["start_date"])
    households = {}
    for index, old in enumerate(base.config["customers"]):
        hid = f"G{index + 1:03d}"
        house = copy.deepcopy(base.customers[old])

        # Rename opaque identifiers in this independent input collection.
        def rename(value):
            if isinstance(value, str):
                return value.replace(old, hid)
            if isinstance(value, list):
                return [rename(v) for v in value]
            if isinstance(value, dict):
                return {k: rename(v) for k, v in value.items()}
            return value

        house = rename(house)
        house.update(feedback=[], card_responses=[], start_controls=[], presence=[])
        house["devices"] = house["devices"][:1]
        device = house["devices"][0]
        device.update(trips=[], predictions=[])
        device["schedule_versions"] = device["schedule_versions"][:1]
        device["description"] = "曜日別の根拠・カード・部分承認のデモ"
        if index == 1:
            for action, stamp in (
                ("lock", "2026-08-25T12:05:00+09:00"),
                ("unlock", "2026-09-15T12:05:00+09:00"),
            ):
                house["start_controls"].append(
                    dict(
                        id=f"{hid}-{action}",
                        device_id=device["device_id"],
                        weekday=2,
                        action=action,
                        at=stamp,
                        received_at=stamp,
                    )
                )
        if index == 2:
            revised = copy.deepcopy(device["schedule_versions"][0])
            revised.update(
                valid_from="2026-08-25T00:00:00+09:00",
                recorded_at="2026-08-24T12:10:00+09:00",
            )
            for plan in revised["schedules"]:
                if plan["weekday"] in (0, 1, 3, 4):
                    plan["start"] = "20:00"
            device["schedule_versions"].append(revised)
        weather = base.weather[house["installation"]["weather_site_id"]]
        for offset, row in enumerate(device["days"]):
            day = start + timedelta(days=offset)
            versions = [
                v
                for v in device["schedule_versions"]
                if v["valid_from"][:10] <= str(day)
            ]
            plan = next(
                p for p in versions[-1]["schedules"] if p["weekday"] == day.weekday()
            )
            target = "20:00"
            if index == 3:
                if day.weekday() == 1:
                    target = "20:15"
                if (
                    day.weekday() == 3
                    and day >= date(2026, 9, 1)
                    or day.weekday() == 2
                    and day >= date(2026, 9, 16)
                ):
                    target = "21:00"
            controls = (
                [(plan["start"], "on", "automation"), ("20:05", "on", "web_manual")]
                if plan["start"] == "20:00"
                else [
                    (plan["start"], "on", "automation"),
                    ("19:00", "off", "web_manual"),
                    (target, "on", "web_manual"),
                ]
            )
            controls.append((plan["end"], "off", "automation"))
            operations = []
            state = False
            for clock, action, source in controls:
                minute = int(clock[:2]) * 60 + int(clock[3:])
                op = operation_record(
                    device["device_id"],
                    day,
                    minute,
                    action,
                    source,
                    after=24 if action == "on" else None,
                )
                at = datetime.fromisoformat(op["at"])
                op.update(
                    device_id=device["device_id"],
                    received_at=(at + timedelta(seconds=2)).isoformat(),
                    power=dict(
                        before=state,
                        after=action == "on",
                        before_at=(at - timedelta(seconds=1)).isoformat(),
                        after_at=(at + timedelta(seconds=1)).isoformat(),
                        received_at=(at + timedelta(seconds=2)).isoformat(),
                    ),
                )
                operations.append(op)
                state = action == "on"
            row.update(
                operations=operations,
                operation_status="complete",
                usage="used",
                kwh=measured_energy(
                    day,
                    operations,
                    weather["observations"][offset]["outdoor_c"],
                    Decimal(1),
                ),
            )
            for resident in house["residents"]:
                house["presence"].append(
                    phone_observation(
                        resident,
                        house["installation"]["id"],
                        day,
                        1080,
                        "permission",
                        "disabled",
                        "unknown",
                        enabled=False,
                        location=False,
                    )
                )
        households[hid] = house
    for old in base.config["customers"]:
        (output / "customers" / f"{old}.json").unlink()
    base.config["customers"] = list(households)
    write_json(output / "dataset.json", base.config)
    for hid, house in households.items():
        write_json(output / "customers" / f"{hid}.json", house)
    dataset = load_dataset(output)
    for hid in ("G003", "G004"):
        report = replay_customer(dataset, hid)
        day = next(
            r for r in report["days"] if r["decision_at"].startswith("2026-08-24")
        )
        card = next(
            c
            for c in day["cards"]
            if c["kind"] == "start"
            and c["segment"] == "weekday"
            and c["members"][0]["values"]["start"] == "20:00"
        )
        members = []
        for member in card["members"]:
            decision = (
                "approved"
                if hid == "G003" and member["weekday"] != 2
                else "declined"
                if hid == "G004" and member["weekday"] == 2
                else "excluded_once"
            )
            members.append(
                dict(
                    {
                        k: member[k]
                        for k in (
                            "device_id",
                            "weekday",
                            "proposal_id",
                            "values",
                            "evidence_ids",
                            "evidence_signature",
                        )
                    },
                    decision=decision,
                )
            )
        dataset.customers[hid]["card_responses"] = [
            dict(
                id=f"{hid}-partial-choice",
                card_id=card["card_id"],
                proposed_at=day["decision_at"],
                responded_at="2026-08-24T12:05:00+09:00",
                members=members,
            )
        ]
        write_json(output / "customers" / f"{hid}.json", dataset.customers[hid])
    return load_dataset(output)


def main(argv=None):
    parser = argparse.ArgumentParser(
        description="曜日まとめ・部分承認の4世帯×90日入力を生成"
    )
    parser.add_argument("--output", type=Path, default=ROOT / "data" / "weekday-demo")
    parser.add_argument("--seed", type=int, default=42)
    args = parser.parse_args(argv)
    make_weekday_demo(args.output, args.seed)
    print(f"生成: {args.output} / 4世帯 × 90日")


if __name__ == "__main__":
    main()
