"""Deterministic synthetic month. Profiles specify behavior, never expected results."""

import argparse
import json
import random
from dataclasses import replace
from datetime import date, datetime, timedelta
from decimal import Decimal
from pathlib import Path
from zoneinfo import ZoneInfo

from .monthly_data import local_midnight, normalize_bundle
from .monthly_engine import evaluate_customer, signature

DEFAULT_OUTPUT = (
    Path(__file__).resolve().parents[1] / "data" / "monthly" / "dataset.json"
)
START = date(2026, 10, 1)
ZONE = ZoneInfo("Asia/Tokyo")

# Behavior inputs belong here; acceptance expectations live only in tests.
PROFILES = (
    dict(
        description="現在の予定で十分（承認済み）",
        response="approved",
        approved_current=True,
        history_weekday=6,
    ),
    dict(description="水曜の開始が遅い", starts={2: 1200}),
    dict(description="月曜は早く、水曜は遅い", starts={0: 1050, 2: 1200}),
    dict(description="水曜の開始変更と暑い日の温度", starts={2: 1200}, hot=True),
    dict(
        description="予定未設定、月曜と水曜に利用",
        no_schedule=True,
        active=(0, 2),
        starts={0: 1140, 2: 1200},
    ),
    dict(description="暑い日だけ22℃", hot=True),
    dict(description="予冷で総額が下がる", prediction="cheap"),
    dict(description="予冷で総額が上がる", prediction="expensive"),
    dict(description="予冷では快適性を満たせない", prediction="uncomfortable"),
    dict(description="長期旅行と水曜の変更", starts={2: 1200}, trip=True),
    dict(description="水曜の操作記録が欠損", starts={2: 1200}, missing=True),
    dict(description="水曜の変更の大半が今回だけ", starts={2: 1200}, once=True),
    dict(description="自動運転だけで手動の変更根拠なし", automatic=True),
    dict(description="利用開始直後", starts={2: 1200}, enrolled_day=28),
    dict(description="水曜を修正して承認", starts={2: 1200}, response="edited"),
    dict(
        description="同じ根拠で水曜を見送り済み", starts={2: 1200}, response="declined"
    ),
    dict(
        description="水曜の見送り後に新しい操作",
        starts={2: 1200},
        response="declined",
        old_basis=True,
    ),
    dict(description="水曜の提案に無反応", starts={2: 1200}, response="no_response"),
    dict(description="十分な履歴、判断履歴なし", starts={1: 1260}),
    dict(description="将来の判断が混在", starts={2: 1200}, future_response=True),
    dict(
        description="複数曜日・暑さ・予冷が重なる",
        starts={0: 1050, 2: 1200, 4: 1260},
        hot=True,
        prediction="cheap",
    ),
    dict(
        description="週末は未利用を確認",
        active=(0, 1, 2, 3, 4),
        weekdays=(0, 1, 2, 3, 4),
    ),
    dict(
        description="水曜の変更と電力量欠損",
        starts={2: 1200},
        energy_missing=True,
        prediction="missing",
    ),
    dict(
        description="木曜の開始変更、涼しい日も温度変更、予冷は割高",
        starts={3: 1020},
        hot=True,
        cool_change=True,
        prediction="expensive",
    ),
)


def weather_day(day: date, site_index: int, forecast: bool = False) -> dict:
    hot = day.day % 3 != 0 or forecast
    base = (34 if hot else 27) + site_index
    values = [
        str(base + (1 if 12 <= index // 2 < 18 else 0) - (5 if index // 2 < 6 else 0))
        for index in range(48)
    ]
    issued = (
        datetime(2026, 10, 31, 6, tzinfo=ZONE)
        if forecast
        else local_midnight(day + timedelta(days=1), ZONE)
    )
    return dict(
        date=str(day),
        issued_at=issued.isoformat(),
        condition="晴れ" if hot else "曇り",
        outdoor_c=values,
    )


def operation_record(
    customer_id: str,
    day: date,
    at: int,
    action: str,
    source: str,
    once: bool = False,
    before: int | None = None,
    after: int | None = None,
) -> dict:
    stamp = local_midnight(day, ZONE) + timedelta(minutes=at)
    record = dict(
        id=f"{customer_id}:{day}:{action}:{at}",
        device_id=f"AC-{customer_id}",
        at=stamp.isoformat(),
        source=source,
        action=action,
        once=once,
    )
    if before is not None:
        record["from_c"] = before
    if after is not None:
        record["to_c"] = after
    return record


def measured_energy(
    day: date, operations: list[dict], outdoor: list[str], efficiency: Decimal
) -> list[str]:
    """Piecewise integration of on/off/setpoint over each half hour (kWh)."""
    events = sorted(operations, key=lambda item: item["at"])
    state_on = False
    setpoint = Decimal(24)
    index = 0
    values = []
    for slot in range(48):
        begin = local_midnight(day, ZONE) + timedelta(minutes=slot * 30)
        end = begin + timedelta(minutes=30)
        cursor = begin
        energy = Decimal(0)
        while cursor < end:
            while (
                index < len(events)
                and datetime.fromisoformat(events[index]["at"]) <= cursor
            ):
                event = events[index]
                if event["action"] in ("on", "off"):
                    state_on = event["action"] == "on"
                if event.get("to_c") is not None:
                    setpoint = Decimal(event["to_c"])
                index += 1
            boundary = (
                min(end, datetime.fromisoformat(events[index]["at"]))
                if index < len(events)
                else end
            )
            if state_on:
                kw = (
                    Decimal("0.35")
                    + max(Decimal(outdoor[slot]) - setpoint, Decimal(0))
                    * Decimal("0.06")
                ) * efficiency
                energy += (
                    kw
                    * Decimal(int((boundary - cursor).total_seconds()))
                    / Decimal(3600)
                )
            cursor = boundary
        values.append(str(energy.quantize(Decimal("0.0001"))))
    return values


def prediction(profile: dict) -> dict:
    target = 22 if profile.get("hot") and not profile.get("cool_change") else 24
    plans = [
        dict(
            name="普段の24℃運転",
            kind="usual",
            kwh=["0"] * 3 + ["0.4"] * 9,
            indoor_c_at_required="24",
            controls=dict(start="18:30", end="23:00", temperature_c=24),
        )
    ]
    if target == 22:
        plans.append(
            dict(
                name="快適性のため22℃で運転",
                kind="comfort",
                kwh=["0"] * 3 + ["0.5"] * 9,
                indoor_c_at_required="22",
                controls=dict(start="18:30", end="23:00", temperature_c=22),
            )
        )
    mode = profile["prediction"]
    energy = (
        ["2"] * 2 + ["0.5"] * 10
        if mode == "expensive"
        else ["0.6"] * 2 + ["0.1"] * 4 + ["0.25"] * 6
    )
    if mode == "missing":
        energy[3] = None
    plans.append(
        dict(
            name="17時から予冷",
            kind="precool",
            kwh=energy,
            indoor_c_at_required="26" if mode == "uncomfortable" else str(target),
            controls=dict(start="17:00", end="23:00", temperature_c=target),
        )
    )
    return dict(
        date="2026-11-01",
        generated_at="2026-10-31T07:00:00+09:00",
        start="17:00",
        end="23:00",
        required_at="20:00",
        target_c=target,
        weather_signature="pending",
        plans=plans,
    )


def build_data(seed: int = 42) -> dict:
    rng = random.Random(seed)
    sites = {
        site_id: dict(
            timezone=str(ZONE),
            observations=[
                weather_day(START + timedelta(days=i), site_index) for i in range(30)
            ],
            forecast=[
                weather_day(date(2026, 10, 31) + timedelta(days=i), site_index, True)
                for i in range(2)
            ],
        )
        for site_index, site_id in enumerate(("tokyo-east", "kanagawa-west"))
    }
    tariffs = dict(
        time_of_use=dict(
            jpy_per_kwh=[
                "10" if i < 36 else "20" if i < 40 else "40" for i in range(48)
            ]
        ),
        flat=dict(jpy_per_kwh=["30"] * 48),
    )
    data = dict(
        schema_version=1,
        seed=seed,
        as_of="2026-10-31T12:00:00+09:00",
        start_date=str(START),
        end_date="2026-10-31",
        sites=sites,
        tariffs=tariffs,
        customers=[],
    )
    for index, profile in enumerate(PROFILES, 1):
        customer_id = f"C{index:03d}"
        site_id = tuple(sites)[index % 2]
        tariff_id = (
            "time_of_use" if profile.get("prediction") or index % 2 == 0 else "flat"
        )
        enrollment = date(2026, 10, profile.get("enrolled_day", 1))
        schedules = (
            []
            if profile.get("no_schedule")
            else [
                dict(weekday=weekday, start="18:30", end="23:00", temperature_c=24)
                for weekday in profile.get("weekdays", range(7))
            ]
        )
        customer = dict(
            customer_id=customer_id,
            description=profile["description"],
            device_id=f"AC-{customer_id}",
            site_id=site_id,
            timezone=str(ZONE),
            tariff_id=tariff_id,
            enrolled_at=local_midnight(enrollment, ZONE).isoformat(),
            schedules=schedules,
            days=[],
            trips=[],
            judgments=[],
            predictions=[],
        )
        if profile.get("trip"):
            customer["trips"] = [
                dict(start="2026-10-01T00:00:00+09:00", end="2026-10-25T00:00:00+09:00")
            ]
        efficiency = Decimal(rng.randrange(96, 105)) / 100
        for offset in range(30):
            day = START + timedelta(days=offset)
            status, usage, operations = "complete", "unused", []
            enrolled = day >= enrollment
            missing = profile.get("missing") and day.weekday() == 2 and day.day < 28
            travelling = profile.get("trip") and day.day < 25
            active = day.weekday() in profile.get("active", range(7)) and not travelling
            if not enrolled or missing:
                status, usage = ("missing" if enrolled else "not_enrolled"), "unknown"
            elif active:
                usage = "used"
                start = profile.get("starts", {}).get(day.weekday(), 1110)
                if day.weekday() in profile.get("starts", {}):
                    start += (-10, 0, 10, 0)[(offset // 7) % 4]
                once = bool(profile.get("once") and day.weekday() == 2 and day.day < 28)
                source = "automation" if profile.get("automatic") else "web_manual"
                operations.append(
                    operation_record(
                        customer_id, day, start, "on", source, once, after=24
                    )
                )
                if profile.get("hot") and (
                    day.day % 3 != 0 or profile.get("cool_change")
                ):
                    operations.append(
                        operation_record(
                            customer_id,
                            day,
                            start + 30,
                            "temperature",
                            "web_manual",
                            before=24,
                            after=22,
                        )
                    )
                operations.append(
                    operation_record(customer_id, day, 1380, "off", source, once)
                )
            outdoor = sites[site_id]["observations"][offset]["outdoor_c"]
            kwh = (
                measured_energy(day, operations, outdoor, efficiency)
                if status == "complete"
                else [None] * 48
            )
            if profile.get("energy_missing") and day.day in (14, 28):
                kwh[40] = None
            customer["days"].append(
                dict(
                    date=str(day),
                    operation_status=status,
                    usage=usage,
                    operations=operations,
                    kwh=kwh,
                )
            )
        if profile.get("prediction"):
            customer["predictions"].append(prediction(profile))
        data["customers"].append(customer)
    bundle = normalize_bundle(data)
    for raw, customer in zip(data["customers"], bundle.customers):
        for forecast in raw["predictions"]:
            forecast["weather_signature"] = signature(
                bundle.sites[customer.site_id]["forecast"][
                    date.fromisoformat(forecast["date"])
                ]
            )
    bundle = normalize_bundle(data)
    for raw, customer, profile in zip(data["customers"], bundle.customers, PROFILES):
        if not profile.get("response") and not profile.get("future_response"):
            continue
        historical_time = datetime(
            2026, 10, 25 if profile.get("old_basis") else 31, 8, tzinfo=ZONE
        )
        history_weekday = profile.get("history_weekday", 2)
        historical_customer = (
            replace(
                customer,
                schedules=tuple(
                    replace(plan, start=1050)
                    if plan.weekday == history_weekday
                    else plan
                    for plan in customer.schedules
                ),
            )
            if profile.get("approved_current")
            else customer
        )
        report = evaluate_customer(
            replace(bundle, as_of=historical_time), historical_customer
        )
        proposal = next(
            item
            for item in report["proposals"]
            if item["kind"] == "start" and item["weekday"] == history_weekday
        )
        response = profile.get("response", "edited")
        raw["judgments"].append(
            dict(
                id=f"{customer.id}:J1",
                proposed_at=historical_time.isoformat(),
                kind=proposal["kind"],
                weekday=proposal["weekday"],
                values=proposal["values"],
                reason=proposal["reason"],
                evidence_ids=proposal["evidence_ids"],
                evidence_signature=proposal["evidence_signature"],
                response=response,
                responded_at=None
                if response == "no_response"
                else (
                    datetime(2026, 11, 2, 9, tzinfo=ZONE)
                    if profile.get("future_response")
                    else historical_time + timedelta(hours=1)
                ).isoformat(),
                selected=dict(start="19:30") if response == "edited" else {},
            )
        )
    return data


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        description="24顧客・30日分の再現可能な仮データを生成"
    )
    parser.add_argument("--seed", type=int, default=42)
    parser.add_argument("--output", type=Path, default=DEFAULT_OUTPUT)
    args = parser.parse_args(argv)
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(
        json.dumps(build_data(args.seed), ensure_ascii=False, indent=2) + "\n",
        encoding="utf-8",
    )
    print(f"生成: {args.output} (24顧客 × 30日、seed={args.seed})")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
