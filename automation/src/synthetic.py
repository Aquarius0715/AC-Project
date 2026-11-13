"""In-memory synthetic inputs; persisted only through the split JSON writer."""

import random
from datetime import date, datetime, timedelta
from decimal import Decimal
from .simulation import ZONE, weather_day, measured_energy, operation_record, prediction
from .monthly_data import local_midnight
from .monthly_engine import signature
from .presence_data import normalize_presence


BEHAVIORS = (
    "regular",
    "verified_manual",
    "roommate",
    "disabled",
    "missing",
    "passby",
    "detour",
    "manual_without_location",
    "travel",
    "one_off",
    "precool",
    "irregular",
)

END = date(2026, 10, 31)


def phone_observation(
    resident: str,
    installation: str,
    day: date,
    minute: int,
    source: str,
    event: str,
    state: str,
    *,
    enabled: bool = True,
    location: bool = True,
    delay: int = 0,
) -> dict:
    at = local_midnight(day, ZONE) + timedelta(minutes=minute)
    return dict(
        id=f"{resident}:{day}:{minute}:{source}:{event}",
        resident_id=resident,
        installation_site_id=installation,
        observed_at=at.isoformat(),
        received_at=(at + timedelta(minutes=delay)).isoformat(),
        source=source,
        event=event,
        state=state,
        sharing_enabled=enabled,
        location_enabled=location,
    )


def resident_day(
    resident: str,
    installation: str,
    day: date,
    behavior: str,
    return_minute: int,
    is_roommate: bool,
) -> list[dict]:

    def observation(minute, source, event, state, **options):
        return phone_observation(
            resident, installation, day, minute, source, event, state, **options
        )

    if behavior in ("disabled", "manual_without_location"):
        return [
            observation(
                1080, "permission", "disabled", "unknown", enabled=False, location=False
            )
        ]
    records = [
        observation(420, "home_wifi", "connected", "home"),
        observation(480, "phone_geofence", "exit", "away"),
    ]
    if is_roommate and (day.weekday() == 2 or day.day % 3):
        records.append(
            observation(1100, "home_wifi", "connected", "home", location=False)
        )
        records.append(
            observation(1320, "home_wifi", "heartbeat", "home", location=False)
        )
        return records
    if behavior == "missing" and (day.weekday() == 2 or day.day % 4 == 0):
        records.append(observation(return_minute, "home_wifi", "connected", "home"))
        return records
    if return_minute > 1080:
        records.append(observation(1080, "phone_geofence", "heartbeat", "away"))
    else:
        records.append(observation(1100, "home_wifi", "heartbeat", "home"))
    if behavior == "passby" and (day.weekday() == 2 or day.day % 2 == 0):
        records.extend(
            [
                observation(1090, "phone_geofence", "enter", "home"),
                observation(1095, "phone_geofence", "exit", "away"),
                observation(1100, "phone_geofence", "heartbeat", "away"),
            ]
        )
        return_minute = max(return_minute, 1260)
    elif behavior == "detour" and (day.weekday() == 2 or day.day % 2):
        records.append(observation(1090, "phone_proximity", "nearby", "unknown"))
        return_minute = max(return_minute, 1290)
    elif day.day % 5 == 0:
        records.append(observation(1090, "phone_proximity", "nearby", "unknown"))
    if behavior == "travel" and date(2026, 10, 2) <= day < date(2026, 10, 25):
        records.append(observation(1320, "phone_geofence", "heartbeat", "away"))
    else:
        records.extend(
            [
                observation(return_minute, "home_wifi", "connected", "home"),
                observation(1380, "home_wifi", "heartbeat", "home"),
            ]
        )
    return records


def build_presence_data(seed: int = 42, households: int = 100, days: int = 90) -> dict:
    if not 1 <= households <= 1000 or not 28 <= days <= 180:
        raise ValueError("世帯数は1〜1000、日数は28〜180を指定してください")
    rng = random.Random(seed)
    start = END - timedelta(days=days)
    weather = {
        site_id: dict(
            timezone=str(ZONE),
            observations=[
                weather_day(start + timedelta(days=i), index) for i in range(days)
            ],
            forecast=[
                weather_day(END + timedelta(days=i), index, True) for i in range(2)
            ],
        )
        for index, site_id in enumerate(("weather-tokyo", "weather-kanagawa"))
    }
    monthly = dict(
        schema_version=1,
        seed=seed,
        as_of="2026-10-31T12:00:00+09:00",
        start_date=str(start),
        end_date=str(END),
        sites=weather,
        tariffs=dict(
            time_of_use=dict(
                jpy_per_kwh=[
                    "10" if i < 36 else "20" if i < 40 else "40" for i in range(48)
                ]
            ),
            flat=dict(jpy_per_kwh=["30"] * 48),
        ),
        customers=[],
    )
    data = dict(
        schema_version=1,
        seed=seed,
        monthly=monthly,
        installation_sites={},
        households=[],
        operation_power={},
    )
    for index in range(households):
        behavior = BEHAVIORS[index % len(BEHAVIORS)]
        household_id = f"H{index + 1:03d}"
        installation = f"home-{index + 1:03d}"
        weather_id = tuple(weather)[index % 2]
        residents = [f"{household_id}-resident-1"]
        if behavior == "roommate" or (
            index % 4 == 0 and behavior not in ("regular", "verified_manual")
        ):
            residents.append(f"{household_id}-resident-2")
        device_ids = [f"{household_id}-AC1"] + (
            [f"{household_id}-AC2"] if (index + 1) % 5 == 0 else []
        )
        data["installation_sites"][installation] = dict(
            household_id=household_id,
            weather_site_id=weather_id,
            label=f"仮の住居{index + 1}",
        )
        house = dict(
            household_id=household_id,
            installation_site_id=installation,
            residents=residents,
            device_ids=device_ids,
            observations=[],
        )
        returns = {}
        for offset in range(days):
            day = start + timedelta(days=offset)
            return_minute = (
                1200
                if day.weekday() == 2
                and behavior in ("verified_manual", "manual_without_location")
                else 1200 + rng.choice((-30, 0, 15, 30, 60))
            )
            if behavior == "irregular":
                return_minute = (1030, 1080, 1130, 1180)[offset // 7 % 4]
            elif behavior == "regular" and day.weekday() != 2:
                return_minute = rng.choice((1020, 1050))
            returns[day] = return_minute
            for resident_index, resident in enumerate(residents):
                house["observations"].extend(
                    resident_day(
                        resident,
                        installation,
                        day,
                        behavior,
                        return_minute + resident_index * 10,
                        resident_index > 0 and behavior == "roommate",
                    )
                )
        data["households"].append(house)
        for device_id in device_ids:
            customer_id = device_id
            customer = dict(
                customer_id=customer_id,
                description=f"スマホ由来の在宅デモ: {behavior}",
                device_id=device_id,
                site_id=weather_id,
                timezone=str(ZONE),
                tariff_id="time_of_use"
                if behavior == "precool" or index % 2 == 0
                else "flat",
                enrolled_at=local_midnight(start, ZONE).isoformat(),
                schedules=[
                    dict(
                        weekday=w,
                        start="20:30" if behavior == "irregular" else "18:30",
                        end="23:00",
                        temperature_c=24,
                    )
                    for w in range(7)
                ],
                days=[],
                trips=[],
                judgments=[],
                predictions=[],
            )
            if behavior == "travel":
                customer["trips"] = [
                    dict(
                        start="2026-10-02T00:00:00+09:00",
                        end="2026-10-25T00:00:00+09:00",
                    )
                ]
            efficiency = Decimal(rng.randrange(96, 105)) / 100
            power_samples = []
            for offset in range(days):
                day = start + timedelta(days=offset)
                travelling = behavior == "travel" and date(2026, 10, 2) <= day < date(
                    2026, 10, 25
                )
                once = behavior == "one_off" and day.weekday() == 2 and (day.day < 25)

                def event(at, action, source):
                    record = operation_record(
                        customer_id,
                        day,
                        at,
                        action,
                        source,
                        once,
                        after=24 if action == "on" else None,
                    )
                    record["device_id"] = device_id
                    return record

                operations = []
                if not travelling:
                    if behavior in ("verified_manual", "manual_without_location"):
                        operations = [
                            event(1110, "on", "automation"),
                            event(1140, "off", "web_manual"),
                            event(returns[day], "on", "web_manual"),
                            event(1380, "off", "automation"),
                        ]
                    elif behavior == "irregular":
                        operations = [
                            event(returns[day], "on", "web_manual"),
                            event(1380, "off", "automation"),
                        ]
                    else:
                        operations = [
                            event(1110, "on", "automation"),
                            event(max(1200, returns[day]), "on", "web_manual"),
                            event(1380, "off", "automation"),
                        ]
                state = False
                for record in sorted(operations, key=lambda op: op["at"]):
                    at = datetime.fromisoformat(record["at"])
                    after = record["action"] == "on"
                    power_samples.append(
                        dict(
                            event_id=record["id"],
                            before=state,
                            after=after,
                            before_at=(at - timedelta(seconds=1)).isoformat(),
                            after_at=(at + timedelta(seconds=1)).isoformat(),
                            received_at=(at + timedelta(seconds=2)).isoformat(),
                        )
                    )
                    state = after
                kwh = measured_energy(
                    day,
                    operations,
                    weather[weather_id]["observations"][offset]["outdoor_c"],
                    efficiency,
                )
                customer["days"].append(
                    dict(
                        date=str(day),
                        operation_status="complete",
                        usage="unused" if travelling else "used",
                        operations=operations,
                        kwh=kwh,
                    )
                )
            if behavior == "precool":
                customer["predictions"] = [prediction(dict(prediction="cheap"))]
            monthly["customers"].append(customer)
            data["operation_power"][device_id] = power_samples
    parsed = normalize_presence(data)
    for customer in monthly["customers"]:
        for candidate in customer["predictions"]:
            candidate["weather_signature"] = signature(
                parsed.monthly.sites[customer["site_id"]]["forecast"][
                    date.fromisoformat(candidate["date"])
                ]
            )
    return data
