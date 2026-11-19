"""JSON adapter. Missing measurements stay unknown; malformed inputs fail clearly."""

import re
from datetime import datetime
from decimal import Decimal, InvalidOperation
from zoneinfo import ZoneInfo, ZoneInfoNotFoundError

from .models import (
    EnergyComparison,
    Operation,
    Period,
    Plan,
    Scenario,
    Schedule,
    Slot,
    Weather,
)


class InputError(ValueError):
    """An input does not meet the normalized data contract."""


def obj(value: object, field: str) -> dict:
    if not isinstance(value, dict):
        raise InputError(f"{field}: オブジェクトが必要です")
    return value


def array(value: object, field: str) -> list:
    if not isinstance(value, list):
        raise InputError(f"{field}: 配列が必要です")
    return value


def string(value: object, field: str) -> str:
    if not isinstance(value, str) or not value.strip():
        raise InputError(f"{field}: 空でない文字列が必要です")
    return value


def number(value: object, field: str, low: str, high: str) -> Decimal:
    if isinstance(value, bool) or not isinstance(value, (str, int, float, Decimal)):
        raise InputError(f"{field}: 数値が必要です")
    try:
        result = Decimal(str(value))
    except InvalidOperation as exc:
        raise InputError(f"{field}: 数値が必要です") from exc
    if not result.is_finite() or not Decimal(low) <= result <= Decimal(high):
        raise InputError(f"{field}: {low}〜{high}の有限値が必要です")
    return result


def timestamp(value: object, field: str) -> datetime:
    try:
        result = datetime.fromisoformat(string(value, field))
    except ValueError as exc:
        raise InputError(f"{field}: ISO 8601日時が必要です") from exc
    if result.utcoffset() is None:
        raise InputError(f"{field}: UTCオフセットが必要です")
    return result


def minute(value: object, field: str) -> int:
    text = string(value, field)
    if re.fullmatch(r"[0-9]{2}:[0-9]{2}", text) is None:
        raise InputError(f"{field}: HH:MMが必要です")
    hour, minutes = map(int, text.split(":"))
    if not 0 <= hour < 24 or not 0 <= minutes < 60:
        raise InputError(f"{field}: HH:MMが必要です")
    return hour * 60 + minutes


def operation(value: object) -> Operation:
    raw = obj(value, "operation")
    source = raw.get("source")
    action = raw.get("action")
    if source not in ("web_manual", "automation", "remote"):
        raise InputError("operation.source: web_manual / automation / remoteが必要です")
    if action not in ("on", "off", "temperature"):
        raise InputError("operation.action: on / off / temperatureが必要です")
    once = raw.get("once")
    if not isinstance(once, bool):
        raise InputError("operation.once: true / falseが必要です")
    before = raw.get("from_c")
    after = raw.get("to_c")
    return Operation(
        string(raw.get("device_id"), "operation.device_id"),
        timestamp(raw.get("at"), "operation.at"),
        source,
        action,
        once,
        None if before is None else number(before, "operation.from_c", "16", "30"),
        None if after is None else number(after, "operation.to_c", "16", "30"),
    )


def weather_samples(value: object) -> tuple[Weather, ...]:
    samples = []
    keys = set()
    for item in array(value, "weather"):
        raw = obj(item, "weather sample")
        device_id = string(raw.get("device_id"), "weather.device_id")
        at = timestamp(raw.get("at"), "weather.at")
        key = (device_id, at)
        if key in keys:
            raise InputError("weather: 同じ機器・日時の気温が重複しています")
        keys.add(key)
        if raw.get("outdoor_c") is not None:
            samples.append(
                Weather(
                    device_id,
                    at,
                    number(raw["outdoor_c"], "weather.outdoor_c", "-60", "60"),
                )
            )
    return tuple(samples)


def energy_comparison(value: object) -> EnergyComparison | None:
    if value is None:
        return None
    raw = obj(value, "energy")
    start = timestamp(raw.get("start"), "energy.start")
    end = timestamp(raw.get("end"), "energy.end")
    required_at = timestamp(raw.get("required_at"), "energy.required_at")
    if not start <= required_at < end:
        raise InputError("energy: start <= required_at < endが必要です")
    slots = []
    cursor = start
    for item in array(raw.get("slots"), "energy.slots"):
        slot = obj(item, "energy.slot")
        slot_start = timestamp(slot.get("start"), "slot.start")
        slot_end = timestamp(slot.get("end"), "slot.end")
        if slot_start != cursor or slot_end <= slot_start or slot_end > end:
            raise InputError(
                "energy.slots: 比較期間を隙間・重複なく時刻順で覆ってください"
            )
        price = slot.get("jpy_per_kwh")
        slots.append(
            Slot(
                slot_start,
                slot_end,
                None
                if price is None
                else number(price, "slot.jpy_per_kwh", "0", "10000"),
            )
        )
        cursor = slot_end
    if not slots or cursor != end:
        raise InputError("energy.slots: 比較期間全体の単価が必要です")
    plans = []
    names = set()
    for item in array(raw.get("plans"), "energy.plans"):
        plan = obj(item, "energy.plan")
        name = string(plan.get("name"), "plan.name")
        kind = plan.get("kind")
        if name in names or kind not in ("usual", "comfort", "precool"):
            raise InputError(
                "energy.plans: 固有のnameとkind (usual / comfort / precool) が必要です"
            )
        names.add(name)
        energy = array(plan.get("kwh"), "plan.kwh")
        if len(energy) != len(slots):
            raise InputError(
                "plan.kwh: 単価の時間帯と同じ要素数が必要です。不明はnullにしてください"
            )
        indoor = plan.get("indoor_c_at_required")
        plans.append(
            Plan(
                name,
                kind,
                tuple(
                    None if kwh is None else number(kwh, "plan.kwh", "0", "10000")
                    for kwh in energy
                ),
                None
                if indoor is None
                else number(indoor, "plan.indoor_c_at_required", "-20", "60"),
            )
        )
    if sum(plan.kind == "usual" for plan in plans) != 1:
        raise InputError("energy.plans: 普段の運転 (usual) を1件指定してください")
    return EnergyComparison(
        tuple(slots),
        required_at,
        number(raw.get("comfort_target_c"), "energy.comfort_target_c", "16", "30"),
        tuple(plans),
    )


def normalize(value: object) -> Scenario:
    raw = obj(value, "scenario")
    try:
        timezone = ZoneInfo(string(raw.get("timezone"), "timezone"))
    except (ZoneInfoNotFoundError, ValueError) as exc:
        raise InputError("timezone: 有効なIANAタイムゾーンが必要です") from exc
    schedule = obj(raw.get("schedule"), "schedule")
    weekday = schedule.get("weekday")
    if type(weekday) is not int or not 0 <= weekday <= 6:
        raise InputError("schedule.weekday: 月曜=0〜日曜=6の整数が必要です")
    start = minute(schedule.get("start"), "schedule.start")
    end = minute(schedule.get("end"), "schedule.end")
    if start >= end:
        raise InputError("schedule: このデモは同日内で開始 < 終了の予定に対応します")
    checks = array(raw.get("checks"), "checks")
    if not checks or any(c not in ("start", "temperature", "precool") for c in checks):
        raise InputError("checks: start / temperature / precoolを指定してください")
    if len(set(checks)) != len(checks):
        raise InputError("checks: 判定の重複があります")
    as_of = timestamp(raw.get("as_of"), "as_of")
    history_start = timestamp(raw.get("history_start"), "history_start")
    if history_start >= as_of:
        raise InputError("history_start: as_ofより前にしてください")
    trips = []
    for item in array(raw.get("trips", []), "trips"):
        trip = obj(item, "trip")
        period = Period(
            timestamp(trip.get("start"), "trip.start"),
            timestamp(trip.get("end"), "trip.end"),
        )
        if period.start >= period.end:
            raise InputError("trip: start < endが必要です")
        trips.append(period)
    operations = tuple(
        operation(item) for item in array(raw.get("operations"), "operations")
    )
    return Scenario(
        string(raw.get("name"), "name"),
        string(raw.get("description"), "description"),
        string(raw.get("device_id"), "device_id"),
        timezone,
        as_of,
        history_start,
        timestamp(raw.get("learning_enabled_at"), "learning_enabled_at"),
        tuple(checks),
        Schedule(
            weekday,
            start,
            end,
            number(schedule.get("temperature_c"), "schedule.temperature_c", "16", "30"),
        ),
        operations,
        tuple(trips),
        weather_samples(raw.get("weather", [])),
        energy_comparison(raw.get("energy")),
    )
