"""Validate a monthly bundle and normalize shared references before evaluation."""

import json
from dataclasses import dataclass
from datetime import date, datetime, timedelta
from decimal import Decimal
from pathlib import Path
from zoneinfo import ZoneInfo, ZoneInfoNotFoundError

from .loader import (
    InputError,
    array,
    energy_comparison,
    minute,
    number,
    obj,
    operation,
    string,
    timestamp,
)
from .models import Operation, Period, Schedule

INTERVAL_MINUTES = 30
SLOTS_PER_DAY = 48


@dataclass(frozen=True)
class Day:
    date: date
    status: str
    usage: str
    events: tuple[tuple[str, Operation], ...]
    kwh: tuple[Decimal | None, ...]


@dataclass(frozen=True)
class Customer:
    id: str
    description: str
    device_id: str
    site_id: str
    tariff_id: str
    timezone: ZoneInfo
    enrolled_at: datetime
    schedules: tuple[Schedule, ...]
    days: tuple[Day, ...]
    trips: tuple[Period, ...]
    judgments: tuple[dict, ...]
    predictions: tuple[dict, ...]


@dataclass(frozen=True)
class Bundle:
    as_of: datetime
    start_date: date
    end_date: date  # Exclusive.
    sites: dict
    tariffs: dict[str, tuple[Decimal | None, ...]]
    customers: tuple[Customer, ...]


def local_midnight(day: date, timezone: ZoneInfo) -> datetime:
    return datetime.combine(day, datetime.min.time(), timezone)


def measurements(
    value: object, field: str, count: int, low: str, high: str
) -> tuple[Decimal | None, ...]:
    values = array(value, field)
    if len(values) != count:
        raise InputError(f"{field}: {count}区間が必要です。欠損はnullで表してください")
    return tuple(
        None if item is None else number(item, field, low, high) for item in values
    )


def calendar_date(value: object, field: str) -> date:
    try:
        return date.fromisoformat(string(value, field))
    except ValueError as exc:
        raise InputError(f"{field}: YYYY-MM-DDが必要です") from exc


def normalize_judgment(value: object) -> dict:
    raw = obj(value, "judgment")
    kind = raw.get("kind")
    weekday = raw.get("weekday")
    if kind not in ("start", "add_schedule", "temperature", "precool"):
        raise InputError("judgment.kind: 不明な提案種別です")
    if weekday is not None and (type(weekday) is not int or not 0 <= weekday < 7):
        raise InputError("judgment.weekday: 0〜6またはnullが必要です")
    if kind in ("start", "add_schedule") and weekday is None:
        raise InputError("judgment.weekday: 開始・予定追加には曜日が必要です")
    proposed_at = timestamp(raw.get("proposed_at"), "judgment.proposed_at")
    response = raw.get("response")
    if response not in ("approved", "edited", "declined", "no_response"):
        raise InputError(
            "judgment.response: approved / edited / declined / no_responseが必要です"
        )
    responded_at = (
        None
        if raw.get("responded_at") is None
        else timestamp(raw["responded_at"], "judgment.responded_at")
    )
    if response != "no_response" and (
        responded_at is None or responded_at < proposed_at
    ):
        raise InputError("judgment.responded_at: 提案以降の判断日時が必要です")
    values = obj(raw.get("values"), "judgment.values")
    selected = obj(raw.get("selected", {}), "judgment.selected")
    if response == "edited" and not selected:
        raise InputError("judgment.selected: 修正して承認した値が必要です")
    if response != "edited" and selected:
        raise InputError("judgment.selected: 修正して承認した場合だけ指定してください")
    if response == "no_response" and responded_at is not None:
        raise InputError("judgment: 無反応には反応日時を指定しないでください")
    allowed = (
        {"start", "end", "temperature_c"}
        if kind in ("start", "add_schedule")
        else {"temperature_c", "base_temperature_c", "condition"}
        if kind == "temperature"
        else {"plan", "start", "end", "temperature_c"}
    )
    for fields in (values, selected):
        for key, item in fields.items():
            if key not in allowed:
                raise InputError(f"judgment: {kind}で使えない変更項目 {key}")
            if key in ("start", "end"):
                minute(item, f"judgment.{key}")
            elif key in ("temperature_c", "base_temperature_c"):
                number(item, f"judgment.{key}", "16", "30")
            elif key in ("condition", "plan"):
                string(item, f"judgment.{key}")
            else:
                raise InputError(f"judgment: 不明な変更項目 {key}")
    if kind in ("start", "add_schedule") and "start" not in values:
        raise InputError("judgment.values.startが必要です")
    if kind == "temperature" and "temperature_c" not in values:
        raise InputError("judgment.values.temperature_cが必要です")
    if (
        kind == "add_schedule"
        and not {"start", "end", "temperature_c"} <= values.keys()
    ):
        raise InputError("judgment: 初回予定には開始・終了・温度が必要です")
    if kind == "precool" and "plan" not in values:
        raise InputError("judgment.values.planが必要です")
    return dict(
        id=string(raw.get("id"), "judgment.id"),
        kind=kind,
        weekday=weekday,
        proposed_at=proposed_at,
        response=response,
        responded_at=responded_at,
        values=dict(values),
        selected=dict(selected),
        evidence_ids=tuple(
            string(item, "judgment.evidence_ids")
            for item in array(raw.get("evidence_ids"), "judgment.evidence_ids")
        ),
        evidence_signature=string(
            raw.get("evidence_signature"), "judgment.evidence_signature"
        ),
        reason=string(raw.get("reason"), "judgment.reason"),
    )


def normalize_prediction(value: object, timezone: ZoneInfo) -> dict:
    raw = obj(value, "prediction")
    day = calendar_date(raw.get("date"), "prediction.date")
    start, end = (
        minute(raw.get("start"), "prediction.start"),
        minute(raw.get("end"), "prediction.end"),
    )
    required = minute(raw.get("required_at"), "prediction.required_at")
    if start >= end or start % 30 or end % 30 or not start <= required < end:
        raise InputError(
            "prediction: 同日内の30分単位の比較期間と期間内の必要時刻が必要です"
        )
    begin = local_midnight(day, timezone) + timedelta(minutes=start)
    finish = local_midnight(day, timezone) + timedelta(minutes=end)
    plans = array(raw.get("plans"), "prediction.plans")
    slots = [
        dict(
            start=(begin + timedelta(minutes=offset)).isoformat(),
            end=(begin + timedelta(minutes=offset + 30)).isoformat(),
            jpy_per_kwh=0,
        )
        for offset in range(0, end - start, 30)
    ]
    # Reuse the existing validated candidate comparison contract.
    energy_comparison(
        dict(
            start=begin.isoformat(),
            end=finish.isoformat(),
            required_at=(
                local_midnight(day, timezone) + timedelta(minutes=required)
            ).isoformat(),
            slots=slots,
            plans=plans,
            comfort_target_c=raw.get("target_c"),
        )
    )
    for plan in plans:
        controls = obj(plan.get("controls"), "prediction.plan.controls")
        on, off = (
            minute(controls.get("start"), "controls.start"),
            minute(controls.get("end"), "controls.end"),
        )
        if not start <= on < off <= end:
            raise InputError("prediction.controls: 運転時間を比較期間内にしてください")
        number(controls.get("temperature_c"), "controls.temperature_c", "16", "30")
        for index, kwh in enumerate(plan["kwh"]):
            slot_start = start + index * 30
            if (
                (slot_start + 30 <= on or slot_start >= off)
                and kwh is not None
                and Decimal(str(kwh)) != 0
            ):
                raise InputError(
                    "prediction: 停止区間の消費電力量は既知のゼロか欠損にしてください"
                )
    timestamp(raw.get("generated_at"), "prediction.generated_at")
    string(raw.get("weather_signature"), "prediction.weather_signature")
    return dict(raw)


def normalize_bundle(value: object) -> Bundle:
    raw = obj(value, "bundle")
    if raw.get("schema_version") != 1:
        raise InputError("schema_version: 1が必要です")
    as_of = timestamp(raw.get("as_of"), "as_of")
    start_date = calendar_date(raw.get("start_date"), "start_date")
    end_date = calendar_date(raw.get("end_date"), "end_date")
    if not 1 <= (end_date - start_date).days <= 366:
        raise InputError("観測期間は1〜366日にしてください")
    sites = {}
    for site_id, item in obj(raw.get("sites"), "sites").items():
        site = obj(item, "site")
        try:
            timezone = ZoneInfo(string(site.get("timezone"), "site.timezone"))
        except (ZoneInfoNotFoundError, ValueError) as exc:
            raise InputError("site.timezone: 有効なIANAタイムゾーンが必要です") from exc
        rows = {}
        for kind in ("observations", "forecast"):
            rows[kind] = {}
            for weather in array(site.get(kind), f"site.{kind}"):
                sample = obj(weather, "weather")
                day = calendar_date(sample.get("date"), "weather.date")
                if day in rows[kind]:
                    raise InputError("weather: 日付が重複しています")
                issued = timestamp(sample.get("issued_at"), "weather.issued_at")
                rows[kind][day] = dict(
                    outdoor_c=measurements(
                        sample.get("outdoor_c"), "weather.outdoor_c", 48, "-60", "60"
                    ),
                    condition=string(sample.get("condition"), "weather.condition"),
                    issued_at=issued,
                )
        sites[site_id] = dict(timezone=timezone, **rows)
    tariffs = {}
    for tariff_id, item in obj(raw.get("tariffs"), "tariffs").items():
        tariff = obj(item, "tariff")
        tariffs[tariff_id] = measurements(
            tariff.get("jpy_per_kwh"), "tariff.jpy_per_kwh", 48, "0", "10000"
        )
    customers = []
    ids = set()
    device_ids = set()
    for item in array(raw.get("customers"), "customers"):
        customer = obj(item, "customer")
        customer_id = string(customer.get("customer_id"), "customer_id")
        device_id = string(customer.get("device_id"), "device_id")
        if customer_id in ids or device_id in device_ids:
            raise InputError("顧客ID・機器IDが重複しています")
        ids.add(customer_id)
        device_ids.add(device_id)
        site_id = string(customer.get("site_id"), "site_id")
        tariff_id = string(customer.get("tariff_id"), "tariff_id")
        if site_id not in sites or tariff_id not in tariffs:
            raise InputError("site_id / tariff_idの参照先がありません")
        timezone = sites[site_id]["timezone"]
        if customer.get("timezone") != str(timezone):
            raise InputError("顧客と地点のタイムゾーンが一致しません")
        schedules = []
        weekdays = set()
        for plan in array(customer.get("schedules"), "schedules"):
            plan = obj(plan, "schedule")
            weekday = plan.get("weekday")
            if type(weekday) is not int or not 0 <= weekday < 7 or weekday in weekdays:
                raise InputError("schedule.weekday: 0〜6の重複しない曜日が必要です")
            weekdays.add(weekday)
            begin, end = (
                minute(plan.get("start"), "schedule.start"),
                minute(plan.get("end"), "schedule.end"),
            )
            if begin >= end:
                raise InputError("schedule: 同日内の開始 < 終了が必要です")
            schedules.append(
                Schedule(
                    weekday,
                    begin,
                    end,
                    number(plan.get("temperature_c"), "temperature_c", "16", "30"),
                )
            )
        days = []
        event_ids = set()
        for index, row in enumerate(array(customer.get("days"), "days")):
            row = obj(row, "day")
            day = calendar_date(row.get("date"), "day.date")
            if day != start_date + timedelta(days=index):
                raise InputError("days: 観測期間の全日を連続・日付順で指定してください")
            status, usage = row.get("operation_status"), row.get("usage")
            if status not in ("complete", "missing", "not_enrolled") or usage not in (
                "used",
                "unused",
                "unknown",
            ):
                raise InputError("day: operation_status / usageが不正です")
            events = []
            for event in array(row.get("operations"), "day.operations"):
                event = obj(event, "event")
                event_id = string(event.get("id"), "event.id")
                op = operation(event)
                if (
                    event_id in event_ids
                    or op.device_id != device_id
                    or op.at.astimezone(timezone).date() != day
                ):
                    raise InputError(
                        "event: ID重複、別機器、または日付の不整合があります"
                    )
                event_ids.add(event_id)
                events.append((event_id, op))
            if status != "complete" and (events or usage != "unknown"):
                raise InputError(
                    "記録欠損・利用開始前の日は操作なし・usage=unknownにしてください"
                )
            kwh = measurements(row.get("kwh"), "day.kwh", 48, "0", "10000")
            if usage == "unused" and (events or any(value != 0 for value in kwh)):
                raise InputError(
                    "未利用確認日は操作なし・電力は全区間で既知のゼロが必要です"
                )
            days.append(
                Day(
                    day,
                    status,
                    usage,
                    tuple(sorted(events, key=lambda event: event[1].at)),
                    kwh,
                )
            )
        if len(days) != (end_date - start_date).days:
            raise InputError("days: 観測期間の日数が一致しません")
        trips = []
        for trip in array(customer.get("trips", []), "trips"):
            trip = obj(trip, "trip")
            begin, end = (
                timestamp(trip.get("start"), "trip.start"),
                timestamp(trip.get("end"), "trip.end"),
            )
            if begin >= end:
                raise InputError("trip: start < endが必要です")
            trips.append(Period(begin, end))
        judgments = tuple(
            normalize_judgment(j)
            for j in array(customer.get("judgments", []), "judgments")
        )
        predictions = tuple(
            normalize_prediction(p, timezone)
            for p in array(customer.get("predictions", []), "predictions")
        )
        if len({j["id"] for j in judgments}) != len(judgments):
            raise InputError("judgments: 顧客内の判断IDが重複しています")
        if len({p["date"] for p in predictions}) != len(predictions):
            raise InputError("predictions: 同じ日の候補は1つの比較にまとめてください")
        customers.append(
            Customer(
                customer_id,
                string(customer.get("description"), "description"),
                device_id,
                site_id,
                tariff_id,
                timezone,
                timestamp(customer.get("enrolled_at"), "enrolled_at"),
                tuple(schedules),
                tuple(days),
                tuple(trips),
                judgments,
                predictions,
            )
        )
    if not customers:
        raise InputError("customers: 1顧客以上が必要です")
    return Bundle(as_of, start_date, end_date, sites, tariffs, tuple(customers))


def load_bundle(path: Path) -> Bundle:
    try:
        with path.open(encoding="utf-8") as stream:
            raw = json.load(stream)
    except (OSError, ValueError) as exc:
        raise InputError(f"月次データを読み込めません: {exc}") from exc
    return normalize_bundle(raw)
