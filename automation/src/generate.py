"""Generate only the editable split JSON dataset; behavior inputs, no expected answers."""

import argparse
import copy
from datetime import date, datetime, timedelta
from decimal import Decimal
from pathlib import Path

from .dataset import DEFAULT_DATA, inside_automation, load_dataset, write_json
from .monthly_data import local_midnight
from .monthly_engine import signature
from .simulation import ZONE, measured_energy, operation_record, prediction, weather_day
from .synthetic import build_presence_data


def make_dataset(
    output: Path = DEFAULT_DATA, customers: int = 100, days: int = 90, seed: int = 42
):
    output = inside_automation(output)
    if not 1 <= customers <= 100 or not 28 <= days <= 180:
        raise ValueError("顧客数は1〜100、日数は28〜180にしてください")
    source = build_presence_data(seed, customers, days)
    monthly = source["monthly"]
    start = date.fromisoformat(monthly["start_date"])
    end = date.fromisoformat(monthly["end_date"])
    pivot = start + timedelta(days=days // 2)
    weather_names = {"weather-tokyo": "tokyo-east", "weather-kanagawa": "kanagawa-west"}
    for old, name in weather_names.items():
        site = monthly["sites"][old]
        site["forecast"] = []
        for offset in range(days + 2):
            day = start + timedelta(days=offset)
            row = weather_day(day, list(weather_names).index(old), True)
            row["issued_at"] = (
                local_midnight(day, ZONE) + timedelta(hours=6)
            ).isoformat()
            site["forecast"].append(row)
        write_json(output / "weather" / f"{name}.json", site)
    for old, tariff in monthly["tariffs"].items():
        write_json(output / "tariffs" / f"{old.replace('_', '-')}.json", tariff)
    config = dict(
        schema_version=2,
        seed=seed,
        start_date=str(start),
        end_date=str(end),
        as_of=monthly["as_of"],
        evaluation_time="12:00",
        evaluation_day_offset=1,
        timezone=str(ZONE),
        customers=[h["household_id"] for h in source["households"]],
        weather=list(weather_names.values()),
        tariffs=[t.replace("_", "-") for t in monthly["tariffs"]],
    )
    for folder, field in (
        ("customers", "customers"),
        ("weather", "weather"),
        ("tariffs", "tariffs"),
    ):
        for path in (output / folder).glob("*.json"):
            if path.stem not in config[field]:
                inside_automation(path).unlink()
    write_json(output / "dataset.json", config)
    lookup = {d["device_id"]: d for d in monthly["customers"]}
    for index, house in enumerate(source["households"]):
        mode = index % 24
        observations = house["observations"]
        # Return times and presence change within the same customer's observation period.
        for obs in observations:
            day = datetime.fromisoformat(obs["observed_at"]).date()
            if (
                mode in (0, 1, 7)
                and day.weekday() == 2
                and obs["source"] == "home_wifi"
                and obs["event"] == "connected"
            ):
                minute = 1260 if day >= pivot else 1200
                stamp = local_midnight(day, ZONE) + timedelta(minutes=minute)
                obs.update(observed_at=stamp.isoformat(), received_at=stamp.isoformat())
            if (
                mode == 2
                and day >= pivot
                and obs["resident_id"] == house["residents"][-1]
                and obs["observed_at"][11:16] == "18:20"
            ):
                obs.update(
                    source="phone_geofence",
                    event="heartbeat",
                    state="away",
                    location_enabled=True,
                )
            if mode == 3 and start + timedelta(days=20) <= day < start + timedelta(
                days=40
            ):
                obs.update(
                    source="phone_geofence",
                    event="heartbeat",
                    state="away",
                    sharing_enabled=True,
                    location_enabled=True,
                )
        site_id = source["installation_sites"][house["installation_site_id"]][
            "weather_site_id"
        ]
        site = monthly["sites"][site_id]
        customer = dict(
            schema_version=2,
            household_id=house["household_id"],
            installation=dict(
                id=house["installation_site_id"],
                weather_site_id=weather_names[site_id],
                label=f"仮の住居{index + 1}",
            ),
            residents=house["residents"],
            presence=observations,
            devices=[],
            feedback=[],
        )
        for device_id in house["device_ids"]:
            device = copy.deepcopy(lookup[device_id])
            device.pop("judgments", None)
            device["site_id"] = weather_names[site_id]
            device["tariff_id"] = device["tariff_id"].replace("_", "-")
            plans = device.pop("schedules")
            for plan in plans:
                plan["end"] = ("22:00", "22:30", "23:00", "23:30")[
                    (index + plan["weekday"]) % 4
                ]
            if mode == 15:
                for plan in plans:
                    plan["start"] = "20:30"
            if mode == 16:
                plans = []
            device["schedule_versions"] = [
                dict(
                    valid_from=local_midnight(start, ZONE).isoformat(),
                    recorded_at=local_midnight(start, ZONE).isoformat(),
                    schedules=plans,
                )
            ]
            if mode in (0, 1) and days >= 60:
                change_day = start + timedelta(days=61 if mode == 0 else 30)
                revised = copy.deepcopy(plans)
                for plan in revised:
                    if plan["weekday"] == 2:
                        plan["start"] = "21:00" if mode == 0 else "20:00"
                device["schedule_versions"].append(
                    dict(
                        valid_from=local_midnight(change_day, ZONE).isoformat(),
                        recorded_at=(
                            local_midnight(change_day, ZONE) - timedelta(hours=12)
                        ).isoformat(),
                        schedules=revised,
                    )
                )
            for offset, row in enumerate(device["days"]):
                day = date.fromisoformat(row["date"])
                ops = row["operations"]
                if mode in (1, 7) and day.weekday() == 2 and day >= pivot:
                    for op in ops:
                        if op["action"] == "on" and op["source"] == "web_manual":
                            op["at"] = (
                                local_midnight(day, ZONE) + timedelta(hours=21)
                            ).isoformat()

                def op(
                    minute, action="on", source="web_manual", before=None, after=None
                ):
                    result = operation_record(
                        device_id,
                        day,
                        minute,
                        action,
                        source,
                        before=before,
                        after=after
                        if after is not None
                        else 24
                        if action == "on"
                        else None,
                    )
                    result["device_id"] = device_id
                    return result

                if (
                    mode == 0
                    and days >= 60
                    and day.weekday() == 2
                    and day >= start + timedelta(days=61)
                ):
                    ops = [
                        op(1260, source="automation"),
                        op(1265),
                        op(1380, "off", "automation"),
                    ]
                if (
                    mode == 1
                    and days >= 60
                    and day.weekday() == 2
                    and day >= start + timedelta(days=30)
                ):
                    ops = (
                        [
                            op(1200, source="automation"),
                            op(1230, "off"),
                            op(1260),
                            op(1380, "off", "automation"),
                        ]
                        if day >= pivot
                        else [
                            op(1200, source="automation"),
                            op(1205),
                            op(1380, "off", "automation"),
                        ]
                    )

                if mode == 13:
                    ops = [
                        op(1110, source="automation"),
                        op(
                            1200,
                            "temperature",
                            before=24,
                            after=22 if day.day % 3 else 23,
                        ),
                        op(1380, "off", "automation"),
                    ]
                elif mode == 14 and day.weekday() == 2:
                    time, source_type = (
                        (1040, "web_manual"),
                        (1050, "web_manual"),
                        (1060, "web_manual"),
                        (1110, "automation"),
                    )[(offset // 7) % 4]
                    ops = [op(time, source=source_type), op(1380, "off", "automation")]
                elif mode == 15:
                    time = (1030, 1080, 1130, 1180)[(offset // 7) % 4]
                    ops = [op(time), op(1380, "off", "automation")]
                elif mode == 16:
                    ops = [op(1140), op(1380, "off")]
                elif mode == 17:
                    time = {0: 1050, 2: 1200, 4: 1260}.get(day.weekday(), 1110)
                    ops = (
                        [op(1110, source="automation"), op(1140, "off")]
                        if time > 1110
                        else []
                    ) + [op(time), op(1380, "off", "automation")]
                elif mode == 18 and day.weekday() == 2:
                    ops = [
                        op(
                            1200 if day.day >= 25 else 1110,
                            source="web_manual" if day.day >= 25 else "automation",
                        ),
                        op(1380, "off", "automation"),
                    ]
                elif mode == 23:
                    ops = [op(1110), op(1380, "off")]
                if mode in (12, 17, 22) and ops and (day.day % 3 or mode == 22):
                    on = next(o for o in reversed(ops) if o["action"] == "on")
                    minute = (
                        datetime.fromisoformat(on["at"]).hour * 60
                        + datetime.fromisoformat(on["at"]).minute
                        + 30
                    )
                    ops.append(op(minute, "temperature", before=24, after=22))
                versions = [
                    v
                    for v in device["schedule_versions"]
                    if v["valid_from"][:10] <= str(day)
                ]
                current = next(
                    (
                        p
                        for p in versions[-1]["schedules"]
                        if p["weekday"] == day.weekday()
                    ),
                    None,
                )
                if current:
                    for event in ops:
                        if event["source"] == "automation" and event["action"] == "off":
                            # Keep IDs opaque; timestamps are authoritative.
                            event["at"] = f"{day}T{current['end']}:00+09:00"
                row["operations"] = sorted(ops, key=lambda o: o["at"])
                row["usage"] = "used" if ops else "unused"
                row["available_at"] = (
                    local_midnight(day + timedelta(days=1), ZONE) + timedelta(minutes=5)
                ).isoformat()
                row["kwh"] = measured_energy(
                    day,
                    row["operations"],
                    site["observations"][offset]["outdoor_c"],
                    Decimal("1.00"),
                )
                if mode == 21 and day.day % 7 == 0:
                    row["kwh"][40] = None
                state = False
                for event in row["operations"]:
                    stamp = datetime.fromisoformat(event["at"])
                    event["received_at"] = (stamp + timedelta(seconds=2)).isoformat()
                    if event["action"] in ("on", "off"):
                        after = event["action"] == "on"
                        event["power"] = dict(
                            before=state,
                            after=after,
                            before_at=(stamp - timedelta(seconds=1)).isoformat(),
                            after_at=(stamp + timedelta(seconds=1)).isoformat(),
                            received_at=(stamp + timedelta(seconds=2)).isoformat(),
                        )
                        state = after
            template = device["predictions"][0] if device["predictions"] else None
            if mode in (19, 20, 21):
                template = prediction(
                    dict(
                        prediction="expensive"
                        if mode == 19
                        else "uncomfortable"
                        if mode == 20
                        else "missing"
                    )
                )
            device["predictions"] = []
            if template:
                for offset in range(1, days + 1):
                    day = start + timedelta(days=offset)
                    candidate = copy.deepcopy(template)
                    candidate.update(
                        date=str(day),
                        generated_at=(
                            local_midnight(day, ZONE) + timedelta(hours=7)
                        ).isoformat(),
                    )
                    plan = next(
                        (p for p in plans if p["weekday"] == day.weekday()), None
                    )
                    if plan:
                        candidate["end"] = plan["end"]
                        slots = (
                            int(plan["end"][:2]) * 60 + int(plan["end"][3:]) - 1020
                        ) // 30
                        for forecast_plan in candidate["plans"]:
                            energy = forecast_plan["kwh"]
                            forecast_plan["kwh"] = (energy + [energy[-1]] * slots)[
                                :slots
                            ]
                            forecast_plan["controls"]["end"] = plan["end"]
                    forecast = next(
                        r for r in site["forecast"] if r["date"] == str(day)
                    )
                    candidate["weather_signature"] = signature(
                        dict(
                            outdoor_c=tuple(Decimal(c) for c in forecast["outdoor_c"]),
                            condition=forecast["condition"],
                            issued_at=datetime.fromisoformat(forecast["issued_at"]),
                        )
                    )
                    device["predictions"].append(candidate)
            customer["devices"].append(device)
        write_json(output / "customers" / f"{house['household_id']}.json", customer)
    # Record realistic past responses to actual proposals, not expected answers.
    dataset = load_dataset(output)
    from .presence_proposals import evaluate_household

    for position, kind, response, offset in (
        (0, "start_review", "declined", 19),
        (0, "start_review", "edited", 60),
        (1, "start", "approved", 29),
    ):
        if position >= customers or offset >= days:
            continue
        hid = config["customers"][position]
        row = dataset.customers[hid]
        prepared = dataset.prepare(hid)
        when = local_midnight(start + timedelta(days=offset), ZONE) + timedelta(
            hours=12
        )
        snapshot = prepared.snapshot(when)
        report = evaluate_household(
            snapshot, snapshot.households[0], include_billing=False
        )
        proposal = next(
            (
                p
                for d in report["devices"]
                for p in d["proposals"]
                if p["kind"] == kind and p["weekday"] == 2
            ),
            None,
        )
        if proposal:
            row["feedback"].append(
                dict(
                    id=f"{hid}-response-{offset}",
                    device_id=proposal["device_id"],
                    kind=kind,
                    weekday=2,
                    proposed_at=when.isoformat(),
                    responded_at=(when + timedelta(minutes=5)).isoformat(),
                    response=response,
                    values=proposal["values"],
                    selected={"start": "21:00"} if response == "edited" else {},
                    evidence_ids=proposal["evidence_ids"],
                    evidence_signature=proposal["evidence_signature"],
                    reason=proposal["reason"],
                )
            )
            write_json(output / "customers" / f"{hid}.json", row)
    return load_dataset(output)


def main(argv=None):
    parser = argparse.ArgumentParser(description="編集可能な顧客別JSONを生成")
    parser.add_argument("--output", type=Path, default=DEFAULT_DATA)
    parser.add_argument("--customers", type=int, default=100)
    parser.add_argument("--days", type=int, default=90)
    parser.add_argument("--seed", type=int, default=42)
    args = parser.parse_args(argv)
    make_dataset(args.output, args.customers, args.days, args.seed)
    print(
        f"生成: {args.output} / {args.customers}世帯 × {args.days}日 / seed {args.seed}"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
