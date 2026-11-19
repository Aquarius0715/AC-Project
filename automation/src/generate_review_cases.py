"""Reproduce review f258d3f with unchanged canonical inputs and two 90-day copies."""

import argparse
import copy
from datetime import date, datetime
from decimal import Decimal
from pathlib import Path

from .dataset import DEFAULT_DATA, ROOT, inside_automation, load_dataset, write_json
from .monthly_engine import signature
from .replay import replay_customer
from .simulation import measured_energy


def make_review_cases(output=ROOT / "data" / "review-cases"):
    output = inside_automation(Path(output))
    canonical = load_dataset()
    weekdays = load_dataset(DEFAULT_DATA / "weekday-demo")
    houses = {
        "G003": copy.deepcopy(weekdays.customers["G003"]),
        "H011": copy.deepcopy(canonical.customers["H011"]),
    }
    device = houses["G003"]["devices"][0]
    device["schedule_versions"][1]["valid_from"] = "2026-08-27T00:00:00+09:00"
    # Keep the registered future version visible on 8/25 without applying it early.
    for row in device["days"]:
        if row["date"] not in ("2026-08-25", "2026-08-26"):
            continue
        original = next(r for r in device["days"] if r["date"] == "2026-08-19")
        old_day = original["date"]
        row["operations"] = copy.deepcopy(original["operations"])
        for operation in row["operations"]:
            for key in ("id", "at", "received_at"):
                operation[key] = operation[key].replace(old_day, row["date"])
            for key in ("before_at", "after_at", "received_at"):
                operation["power"][key] = operation["power"][key].replace(
                    old_day, row["date"]
                )
        plan = next(
            p
            for p in device["schedule_versions"][0]["schedules"]
            if p["weekday"] == date.fromisoformat(row["date"]).weekday()
        )
        off = row["operations"][-1]
        previous = datetime.fromisoformat(off["at"])
        stamp = datetime.fromisoformat(row["date"] + "T" + plan["end"] + ":00+09:00")
        for record, key in (
            (off, "at"),
            (off, "received_at"),
            (off["power"], "before_at"),
            (off["power"], "after_at"),
            (off["power"], "received_at"),
        ):
            record[key] = (
                datetime.fromisoformat(record[key]) + (stamp - previous)
            ).isoformat()
        weather = next(
            w
            for w in canonical.weather[device["site_id"]]["observations"]
            if w["date"] == row["date"]
        )
        row["kwh"] = measured_energy(
            date.fromisoformat(row["date"]),
            row["operations"],
            weather["outdoor_c"],
            Decimal(1),
        )
    house = houses["H011"]
    for observation in house["presence"]:
        observation.update(
            source="permission",
            event="disabled",
            state="unknown",
            sharing_enabled=False,
            location_enabled=False,
        )
    house.update(feedback=[], card_responses=[])
    device = house["devices"][0]
    issuance = {
        "2026-08-10": "2026-08-09",
        "2026-08-12": "2026-08-11",
        "2026-08-15": "2026-08-14",
        "2026-08-17": "2026-08-14",
    }
    device["predictions"] = [p for p in device["predictions"] if p["date"] in issuance]
    weather = copy.deepcopy(canonical.weather)
    for prediction in device["predictions"]:
        issued = issuance[prediction["date"]]
        prediction["generated_at"] = issued + "T07:00:00+09:00"
        forecast = next(
            f
            for f in weather[device["site_id"]]["forecast"]
            if f["date"] == prediction["date"]
        )
        forecast["issued_at"] = issued + "T06:00:00+09:00"
    config = copy.deepcopy(canonical.config)
    config.update(
        customers=list(houses),
        weather=sorted({h["installation"]["weather_site_id"] for h in houses.values()}),
        tariffs=sorted({d["tariff_id"] for h in houses.values() for d in h["devices"]}),
    )
    write_json(output / "dataset.json", config)
    for hid, house in houses.items():
        write_json(output / "customers" / f"{hid}.json", house)
    for name in config["weather"]:
        write_json(output / "weather" / f"{name}.json", weather[name])
    for name in config["tariffs"]:
        write_json(output / "tariffs" / f"{name}.json", canonical.tariffs[name])
    dataset = load_dataset(output)
    forecasts = dataset.prepare("H011").full.monthly.sites[device["site_id"]][
        "forecast"
    ]
    for prediction in dataset.customers["H011"]["devices"][0]["predictions"]:
        prediction["weather_signature"] = signature(
            forecasts[date.fromisoformat(prediction["date"])]
        )
    write_json(output / "customers/H011.json", dataset.customers["H011"])
    report = replay_customer(dataset, "H011")
    row = next(d for d in report["days"] if d["decision_at"].startswith("2026-08-09"))
    item = next(
        i
        for dev in row["devices"]
        for i in dev["decisions"]
        if i["kind"] == "precool" and i["status"] == "proposed"
    )
    # Legacy response deliberately omits target_date; recover it from dated references.
    dataset.customers["H011"]["feedback"] = [
        dict(
            id="precool-decline",
            device_id=item["device_id"],
            kind="precool",
            weekday=item["weekday"],
            proposal_id=item["proposal_id"],
            proposed_at=row["decision_at"],
            responded_at="2026-08-09T12:05:00+09:00",
            response="declined",
            values=item["values"],
            selected={},
            evidence_ids=item["evidence_ids"],
            evidence_signature=item["evidence_signature"],
            reason=item["reason"],
        )
    ]
    write_json(output / "customers/H011.json", dataset.customers["H011"])
    return load_dataset(output)


def main():
    parser = argparse.ArgumentParser(
        description="反映待ち・日付付き予冷の90日再現データ"
    )
    parser.add_argument("--output", type=Path, default=ROOT / "data" / "review-cases")
    args = parser.parse_args()
    make_review_cases(args.output)
    print(f"生成: {args.output} / 2世帯 × 90日")


if __name__ == "__main__":
    main()
