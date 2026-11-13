"""Editable per-household JSON files and causal snapshots of the shared rule inputs."""

import copy
import json
import re
from dataclasses import dataclass, replace
from datetime import datetime, timedelta
from pathlib import Path

from .loader import InputError, array, obj, timestamp, minute, number
from .monthly_data import local_midnight
from .presence_data import normalize_presence

ROOT = Path(__file__).resolve().parents[1]
DEFAULT_DATA = ROOT / "data"


def inside_automation(path: Path) -> Path:
    resolved = path.resolve()
    if not resolved.is_relative_to(ROOT):
        raise InputError("入力・生成先・結果のパスは automation/ 内を指定してください")
    return resolved


def read_json(path: Path):
    path = inside_automation(path)
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except (OSError, ValueError) as exc:
        raise InputError(f"{path}: JSONを読み込めません: {exc}") from exc


def write_json(path: Path, value) -> None:
    path = inside_automation(path)
    path.parent.mkdir(parents=True, exist_ok=True)
    text = json.dumps(value, ensure_ascii=False, indent=2, default=str)
    # Measurement grids stay on one line; named fields/objects remain indented.
    text = re.sub(
        r'\[\n\s*((?:"-?\d+(?:\.\d+)?"|null|\d+)(?:,\n\s*(?:"-?\d+(?:\.\d+)?"|null|\d+))*)\n\s*\]',
        lambda m: "[" + re.sub(r",\n\s*", ", ", m[1]) + "]",
        text,
    )
    path.write_text(text + "\n", encoding="utf-8")


def identifier(value):
    if not isinstance(value, str) or re.fullmatch(r"[A-Za-z0-9_-]+", value) is None:
        raise InputError(
            "ファイルIDには英数字・ハイフン・アンダースコアを使用してください"
        )
    return value


@dataclass
class Dataset:
    path: Path
    config: dict
    customers: dict
    weather: dict
    tariffs: dict

    def raw_bundle(self, household_id: str) -> dict:
        row = self.customers[household_id]
        site = row["installation"]
        devices = []
        power = {}
        for raw in row["devices"]:
            device = copy.deepcopy(raw)
            versions = device.pop("schedule_versions")
            device["schedules"] = versions[-1]["schedules"]
            power[device["device_id"]] = []
            for day in device["days"]:
                for op in day["operations"]:
                    sample = op.pop("power", None)
                    if sample:
                        power[device["device_id"]].append(
                            dict(sample, event_id=op["id"])
                        )
            device[
                "judgments"
            ] = []  # Feedback is applied after the two evidence sources merge.
            devices.append(device)
        return dict(
            schema_version=1,
            monthly=dict(
                schema_version=1,
                as_of=self.config["as_of"],
                start_date=self.config["start_date"],
                end_date=self.config["end_date"],
                sites=self.weather,
                tariffs=self.tariffs,
                customers=devices,
            ),
            installation_sites={
                site["id"]: dict(
                    household_id=household_id,
                    weather_site_id=site["weather_site_id"],
                    label=site["label"],
                )
            },
            households=[
                dict(
                    household_id=household_id,
                    installation_site_id=site["id"],
                    residents=row["residents"],
                    device_ids=[d["device_id"] for d in devices],
                    observations=row["presence"],
                )
            ],
            operation_power=power,
        )

    def prepare(self, household_id: str):
        raw = self.customers[household_id]
        device_ids = {d["device_id"] for d in raw["devices"]}
        feedback_ids = set()
        for response in array(raw.get("feedback"), "feedback"):
            response = obj(response, "feedback row")
            if (
                response.get("id") in feedback_ids
                or response.get("device_id") not in device_ids
                or response.get("kind")
                not in (
                    "start",
                    "start_review",
                    "add_schedule",
                    "temperature",
                    "precool",
                )
            ):
                raise InputError("判断ID重複、対象機器、または提案種類が不正です")
            feedback_ids.add(response.get("id"))
            proposed = timestamp(response.get("proposed_at"), "feedback.proposed_at")
            reacted = (
                timestamp(response["responded_at"], "feedback.responded_at")
                if response.get("responded_at")
                else None
            )
            if (
                response.get("response")
                not in ("approved", "edited", "declined", "no_response")
                or reacted is not None
                and reacted < proposed
            ):
                raise InputError("判断と提案・反応日時が不正です")
            values = obj(response.get("values"), "feedback.values")
            selected = obj(response.get("selected"), "feedback.selected")
            if response["response"] == "edited" and not selected:
                raise InputError("修正して承認した値が必要です")
            for fields in (values, selected):
                for key, value in fields.items():
                    if key in ("start", "end"):
                        minute(value, f"feedback.{key}")
                    elif key in ("temperature_c", "base_temperature_c"):
                        number(value, f"feedback.{key}", "16", "30")
        full = normalize_presence(self.raw_bundle(household_id))
        versions = {}
        receipts = {}
        availability = {}
        for device, parsed in zip(raw["devices"], full.monthly.customers):
            parsed_versions = []
            for version in device["schedule_versions"]:
                known = timestamp(version["recorded_at"], "schedule.recorded_at")
                valid = timestamp(version["valid_from"], "schedule.valid_from")
                if known > valid:
                    raise InputError(
                        "予定は有効化時刻までに登録してください（遡及的な適用はしません）"
                    )
                candidate = self.raw_bundle(household_id)
                chosen = next(
                    d
                    for d in candidate["monthly"]["customers"]
                    if d["device_id"] == device["device_id"]
                )
                chosen["schedules"] = version["schedules"]
                normalized = normalize_presence(candidate)
                schedules = next(
                    d.schedules
                    for d in normalized.monthly.customers
                    if d.device_id == device["device_id"]
                )
                parsed_versions.append(
                    dict(recorded_at=known, valid_from=valid, schedules=schedules)
                )
            if not parsed_versions or parsed_versions != sorted(
                parsed_versions, key=lambda v: v["valid_from"]
            ):
                raise InputError("予定の版は有効日時順に1件以上必要です")
            versions[device["device_id"]] = parsed_versions
            for day in device["days"]:
                availability[(device["device_id"], day["date"])] = timestamp(
                    day["available_at"], "day.available_at"
                )
                for op in day["operations"]:
                    received = timestamp(op["received_at"], "operation.received_at")
                    if received < timestamp(op["at"], "operation.at"):
                        raise InputError("操作の受信時刻は発生時刻以降が必要です")
                    receipts[op["id"]] = received
        return Prepared(self, household_id, full, versions, receipts, availability)


@dataclass
class Prepared:
    dataset: Dataset
    household_id: str
    full: object
    versions: dict
    receipts: dict
    availability: dict

    def snapshot(self, at: datetime):
        customers = []
        for customer in self.full.monthly.customers:
            versions = [
                v
                for v in self.versions[customer.device_id]
                if v["recorded_at"] <= at and v["valid_from"] <= at
            ]
            schedules = versions[-1]["schedules"] if versions else ()
            days = []
            for day in customer.days:
                available = self.availability[(customer.device_id, str(day.date))] <= at
                events = (
                    tuple(
                        (key, op)
                        for key, op in day.events
                        if op.at <= at and self.receipts[key] <= at
                    )
                    if available
                    else ()
                )
                days.append(
                    replace(
                        day,
                        status=day.status if available else "missing",
                        events=events,
                    )
                )
            predictions = tuple(
                p
                for p in customer.predictions
                if timestamp(p["generated_at"], "prediction.generated_at") <= at
                and local_midnight(
                    datetime.fromisoformat(p["date"]).date(), customer.timezone
                )
                + timedelta(hours=int(p["start"][:2]), minutes=int(p["start"][3:]))
                >= at
            )
            customers.append(
                replace(
                    customer,
                    schedules=schedules,
                    days=tuple(days),
                    predictions=predictions,
                )
            )
        sites = {
            key: dict(
                site,
                observations={
                    day: r
                    for day, r in site["observations"].items()
                    if r["issued_at"] <= at
                },
                forecast={
                    day: r
                    for day, r in site["forecast"].items()
                    if r["issued_at"] <= at
                },
            )
            for key, site in self.full.monthly.sites.items()
        }
        return replace(
            self.full,
            monthly=replace(
                self.full.monthly, as_of=at, customers=tuple(customers), sites=sites
            ),
            schedule_versions={
                k: tuple(v for v in vs if v["recorded_at"] <= at)
                for k, vs in self.versions.items()
            },
        )


def load_dataset(path: Path = DEFAULT_DATA) -> Dataset:
    path = inside_automation(path)
    config = obj(read_json(path / "dataset.json"), "dataset.json")
    if (
        config.get("schema_version") != 2
        or config.get("evaluation_time") != "12:00"
        or config.get("evaluation_day_offset") != 1
    ):
        raise InputError(
            "dataset.json: schema_version=2, evaluation_time=12:00が必要です"
        )
    customers = {
        identifier(key): read_json(path / "customers" / f"{identifier(key)}.json")
        for key in config["customers"]
    }
    weather = {
        identifier(key): read_json(path / "weather" / f"{identifier(key)}.json")
        for key in config["weather"]
    }
    tariffs = {
        identifier(key): read_json(path / "tariffs" / f"{identifier(key)}.json")
        for key in config["tariffs"]
    }
    for key, customer in customers.items():
        customer = obj(customer, "customer")
        if customer.get("household_id") != key or customer.get("schema_version") != 2:
            raise InputError("顧客ファイル名とhousehold_idが一致しません")
    return Dataset(path, config, customers, weather, tariffs)
