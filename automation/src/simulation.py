"""Pure synthetic weather, operation and energy helpers."""

from datetime import date, datetime, timedelta
from decimal import Decimal
from zoneinfo import ZoneInfo
from .monthly_data import local_midnight

ZONE = ZoneInfo("Asia/Tokyo")


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
    target = 22 if profile.get("hot") and (not profile.get("cool_change")) else 24
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
