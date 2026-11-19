"""G003: accepted 20:00 becomes effective, then an independent edit selects 21:00."""

import argparse
import copy
from datetime import date, datetime, timedelta
from decimal import Decimal
from pathlib import Path

from .dataset import ROOT, inside_automation, load_dataset, write_json
from .simulation import measured_energy, operation_record


def make_approval_history(output=ROOT / "work" / "approval-history" / "data"):
    output = inside_automation(Path(output))
    source = load_dataset(ROOT / "data" / "review-cases")
    house = copy.deepcopy(source.customers["G003"])
    device = house["devices"][0]
    edited = copy.deepcopy(device["schedule_versions"][-1])
    edited.update(
        recorded_at="2026-08-31T12:10:00+09:00", valid_from="2026-09-01T00:00:00+09:00"
    )
    next(p for p in edited["schedules"] if p["weekday"] == 0)["start"] = "21:00"
    device["schedule_versions"].append(edited)
    future = copy.deepcopy(edited)
    future.update(
        recorded_at="2026-10-30T12:10:00+09:00", valid_from="2026-11-02T00:00:00+09:00"
    )
    next(p for p in future["schedules"] if p["weekday"] == 0)["start"] = "20:00"
    device["schedule_versions"].append(future)
    observations = {
        r["date"]: r for r in source.weather[device["site_id"]]["observations"]
    }
    for row in device["days"]:
        day = date.fromisoformat(row["date"])
        if day.weekday() != 0 or row["date"] < "2026-09-01":
            continue
        operations = []
        state = False
        end = next(p["end"] for p in edited["schedules"] if p["weekday"] == 0)
        for clock, action, sender in (
            ("21:00", "on", "automation"),
            ("21:05", "on", "web_manual"),
            (end, "off", "automation"),
        ):
            minute = int(clock[:2]) * 60 + int(clock[3:])
            operation = operation_record(
                device["device_id"],
                day,
                minute,
                action,
                sender,
                after=24 if action == "on" else None,
            )
            stamp = datetime.fromisoformat(operation["at"])
            received = (stamp + timedelta(seconds=2)).isoformat()
            operation.update(
                device_id=device["device_id"],
                received_at=received,
                power=dict(
                    before=state,
                    after=action == "on",
                    before_at=(stamp - timedelta(seconds=1)).isoformat(),
                    after_at=(stamp + timedelta(seconds=1)).isoformat(),
                    received_at=received,
                ),
            )
            operations.append(operation)
            state = action == "on"
        row.update(
            operations=operations,
            kwh=measured_energy(
                day, operations, observations[row["date"]]["outdoor_c"], Decimal(1)
            ),
        )
    config = copy.deepcopy(source.config)
    config.update(
        customers=["G003"], weather=[device["site_id"]], tariffs=[device["tariff_id"]]
    )
    write_json(output / "dataset.json", config)
    write_json(output / "customers/G003.json", house)
    write_json(
        output / "weather" / f"{device['site_id']}.json",
        source.weather[device["site_id"]],
    )
    write_json(
        output / "tariffs" / f"{device['tariff_id']}.json",
        source.tariffs[device["tariff_id"]],
    )
    return load_dataset(output)


def main():
    parser = argparse.ArgumentParser(
        description="承認反映後の別編集をG003の90日入力で再現"
    )
    parser.add_argument(
        "--output", type=Path, default=ROOT / "work" / "approval-history" / "data"
    )
    args = parser.parse_args()
    make_approval_history(args.output)
    print(f"生成: {args.output} / G003 × 90日")


if __name__ == "__main__":
    main()
