"""Normalized inputs and results; the decision engine has no file or network I/O."""

from dataclasses import dataclass
from datetime import datetime
from decimal import Decimal
from zoneinfo import ZoneInfo


@dataclass(frozen=True)
class Schedule:
    weekday: int
    start: int  # Local minutes since midnight.
    end: int
    temperature: Decimal


@dataclass(frozen=True)
class Operation:
    device_id: str
    at: datetime
    source: str
    action: str
    once: bool
    before: Decimal | None = None
    after: Decimal | None = None


@dataclass(frozen=True)
class Period:
    start: datetime
    end: datetime


@dataclass(frozen=True)
class Weather:
    device_id: str
    at: datetime
    outdoor_c: Decimal


@dataclass(frozen=True)
class Slot:
    start: datetime
    end: datetime
    price: Decimal | None


@dataclass(frozen=True)
class Plan:
    name: str
    kind: str
    energy: tuple[Decimal | None, ...]
    indoor_c: Decimal | None


@dataclass(frozen=True)
class EnergyComparison:
    slots: tuple[Slot, ...]
    required_at: datetime
    comfort_target_c: Decimal
    plans: tuple[Plan, ...]


@dataclass(frozen=True)
class Scenario:
    name: str
    description: str
    device_id: str
    timezone: ZoneInfo
    as_of: datetime
    history_start: datetime
    learning_enabled_at: datetime
    checks: tuple[str, ...]
    schedule: Schedule
    operations: tuple[Operation, ...]
    trips: tuple[Period, ...]
    weather: tuple[Weather, ...] = ()
    energy: EnergyComparison | None = None


@dataclass(frozen=True)
class Decision:
    check: str
    proposed: bool
    reason: str
    evidence: tuple[str, ...] = ()
    changes: tuple[str, ...] = ()
    target_temperature_c: str | None = None
    target_plan: str | None = None


def clock(minutes: int) -> str:
    return f"{minutes // 60:02d}:{minutes % 60:02d}"
