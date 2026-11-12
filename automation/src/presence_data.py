"""Phone/installation/power-evidence contract, separate from weather and legacy data."""

import gzip
import json
from dataclasses import dataclass
from datetime import datetime
from pathlib import Path

from .loader import InputError, array, obj, string, timestamp
from .monthly_data import Bundle, normalize_bundle


@dataclass(frozen=True)
class Observation:
    id: str
    resident_id: str
    at: datetime
    received_at: datetime
    source: str
    event: str
    state: str
    sharing_enabled: bool
    location_enabled: bool


@dataclass(frozen=True)
class Household:
    id: str
    installation_site_id: str
    residents: tuple[str, ...]
    observations: tuple[Observation, ...]
    device_ids: tuple[str, ...]


@dataclass(frozen=True)
class PowerEvidence:
    event_id: str
    before: bool | None
    after: bool | None
    before_at: datetime
    after_at: datetime
    received_at: datetime


@dataclass(frozen=True)
class PresenceBundle:
    monthly: Bundle
    installations: dict
    households: tuple[Household, ...]
    power: dict[str, tuple[PowerEvidence, ...]]


def boolean(value: object, field: str) -> bool:
    if not isinstance(value, bool):
        raise InputError(f"{field}: true / falseが必要です")
    return value


def normalize_presence(value: object) -> PresenceBundle:
    raw = obj(value, "presence bundle")
    if raw.get("schema_version") != 1:
        raise InputError("presence.schema_version: 1が必要です")
    monthly = normalize_bundle(raw.get("monthly"))
    installations = {}
    for key, item in obj(raw.get("installation_sites"), "installation_sites").items():
        site = obj(item, "installation")
        household_id = string(site.get("household_id"), "installation.household_id")
        weather_id = string(site.get("weather_site_id"), "installation.weather_site_id")
        if weather_id not in monthly.sites:
            raise InputError("installation: 天気地点IDの参照先がありません")
        installations[key] = dict(
            household_id=household_id,
            weather_site_id=weather_id,
            label=string(site.get("label"), "installation.label"),
        )
    devices = {customer.device_id: customer for customer in monthly.customers}
    households = []
    household_ids = set()
    linked_devices = set()
    for item in array(raw.get("households"), "households"):
        house = obj(item, "household")
        household_id = string(house.get("household_id"), "household_id")
        installation_id = string(
            house.get("installation_site_id"), "installation_site_id"
        )
        if (
            household_id in household_ids
            or installation_id not in installations
            or installations[installation_id]["household_id"] != household_id
        ):
            raise InputError("household: 世帯ID重複または設置地点の参照が不正です")
        household_ids.add(household_id)
        residents = tuple(
            string(r, "resident_id") for r in array(house.get("residents"), "residents")
        )
        if not residents or len(set(residents)) != len(residents):
            raise InputError("residents: 1人以上の重複しない住人IDが必要です")
        device_ids = tuple(
            string(d, "device_id") for d in array(house.get("device_ids"), "device_ids")
        )
        if not device_ids or len(set(device_ids)) != len(device_ids):
            raise InputError("household.device_ids: 重複しない機器が必要です")
        for device_id in device_ids:
            if (
                device_id not in devices
                or device_id in linked_devices
                or devices[device_id].site_id
                != installations[installation_id]["weather_site_id"]
            ):
                raise InputError(
                    "household: 機器の参照、所属世帯、天気地点が不整合です"
                )
            linked_devices.add(device_id)
        observations = []
        obs_ids = set()
        for row in array(house.get("observations"), "observations"):
            row = obj(row, "observation")
            obs_id = string(row.get("id"), "observation.id")
            resident_id = string(row.get("resident_id"), "observation.resident_id")
            if (
                obs_id in obs_ids
                or resident_id not in residents
                or row.get("installation_site_id") != installation_id
            ):
                raise InputError("observation: ID重複、住人、設置地点が不正です")
            obs_ids.add(obs_id)
            at = timestamp(row.get("observed_at"), "observation.observed_at")
            received = timestamp(row.get("received_at"), "observation.received_at")
            source, event, state = row.get("source"), row.get("event"), row.get("state")
            allowed = {
                "phone_geofence": ("enter", "exit", "heartbeat"),
                "home_wifi": ("connected", "disconnected", "heartbeat"),
                "phone_proximity": ("nearby",),
                "permission": ("disabled",),
            }
            if (
                not isinstance(source, str)
                or source not in allowed
                or event not in allowed[source]
                or state not in ("home", "away", "unknown")
                or received < at
            ):
                raise InputError(
                    "observation: 情報源・イベント・状態・受信日時が不正です"
                )
            if (
                source == "phone_geofence"
                and (
                    event == "enter"
                    and state == "away"
                    or event == "exit"
                    and state == "home"
                )
                or source == "home_wifi"
                and event == "connected"
                and state == "away"
            ):
                raise InputError("observation: イベントと在宅状態が矛盾しています")
            if source in ("phone_proximity", "permission") and state != "unknown":
                raise InputError("接近・利用停止だけでhome / awayを宣言できません")
            observations.append(
                Observation(
                    obs_id,
                    resident_id,
                    at,
                    received,
                    source,
                    event,
                    state,
                    boolean(row.get("sharing_enabled"), "sharing_enabled"),
                    boolean(row.get("location_enabled"), "location_enabled"),
                )
            )
        households.append(
            Household(
                household_id,
                installation_id,
                residents,
                tuple(sorted(observations, key=lambda o: (o.at, o.received_at, o.id))),
                device_ids,
            )
        )
    if (
        not households
        or linked_devices != set(devices)
        or {s["household_id"] for s in installations.values()} != household_ids
    ):
        raise InputError("全機器・全設置地点を実在する世帯へひも付けてください")
    power = {}
    for device_id, rows in obj(raw.get("operation_power"), "operation_power").items():
        if device_id not in devices:
            raise InputError("operation_power: 不明な機器IDです")
        events = {
            event_id: op
            for day in devices[device_id].days
            for event_id, op in day.events
        }
        samples = []
        ids = set()
        for row in array(rows, "operation_power rows"):
            row = obj(row, "power evidence")
            event_id = string(row.get("event_id"), "power.event_id")
            if (
                event_id not in events
                or event_id in ids
                or events[event_id].action not in ("on", "off")
            ):
                raise InputError("power: 操作IDの参照または重複が不正です")
            ids.add(event_id)
            before_at = timestamp(row.get("before_at"), "power.before_at")
            after_at = timestamp(row.get("after_at"), "power.after_at")
            received = timestamp(row.get("received_at"), "power.received_at")
            if not before_at <= events[event_id].at <= after_at <= received:
                raise InputError("power: 前後の観測と操作・受信の時系列が不正です")
            before = (
                None
                if row.get("before") is None
                else boolean(row["before"], "power.before")
            )
            after = (
                None
                if row.get("after") is None
                else boolean(row["after"], "power.after")
            )
            samples.append(
                PowerEvidence(event_id, before, after, before_at, after_at, received)
            )
        power[device_id] = tuple(samples)
    return PresenceBundle(monthly, installations, tuple(households), power)


def read_data(path: Path) -> dict:
    try:
        opener = gzip.open if path.suffix == ".gz" else open
        with opener(path, "rt", encoding="utf-8") as stream:
            return obj(json.load(stream), "presence bundle")
    except (OSError, ValueError, EOFError) as exc:
        raise InputError(f"在宅データを読み込めません: {exc}") from exc


def load_presence(path: Path) -> PresenceBundle:
    return normalize_presence(read_data(path))
