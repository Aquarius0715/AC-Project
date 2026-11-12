"""Causal phone-derived household presence; no GPS hardware, paths or network I/O."""

from bisect import bisect_left, bisect_right
from datetime import datetime, timedelta

from .presence_data import Household

OBSERVATION_TTL = timedelta(hours=2)
GEOFENCE_DWELL = timedelta(minutes=10)


class PresenceIndex:
    def __init__(self, household: Household, as_of: datetime):
        self.household = household
        self.as_of = as_of
        self.records = {
            resident: tuple(
                o for o in household.observations if o.resident_id == resident
            )
            for resident in household.residents
        }
        self.times = {
            resident: tuple(o.at for o in records)
            for resident, records in self.records.items()
        }
        self.cache = {}

    def resident_at(self, resident: str, query: datetime) -> dict:
        times = self.times[resident]
        left = bisect_left(times, query - OBSERVATION_TTL - GEOFENCE_DWELL)
        right = bisect_right(times, query)
        records = [
            o
            for o in self.records[resident][left:right]
            if o.received_at <= query and o.received_at <= self.as_of
        ]
        state, reason, latest, pending, proof = (
            "unknown",
            "missing_or_stale",
            None,
            None,
            [],
        )
        for obs in records:
            if not obs.sharing_enabled or obs.source == "permission":
                state, reason, pending, latest, proof = (
                    "unknown",
                    "sharing_disabled",
                    None,
                    obs.at,
                    [obs.id],
                )
            elif obs.source == "phone_proximity":
                continue  # Approaching the home is not an arrival or a fresh state.
            elif obs.source == "phone_geofence" and not obs.location_enabled:
                if state == "home" and reason == "home_wifi":
                    continue  # GPS unavailable does not erase a still-fresh Wi-Fi confirmation.
                state, reason, pending, latest, proof = (
                    "unknown",
                    "location_disabled",
                    None,
                    obs.at,
                    [obs.id],
                )
            elif obs.state == "unknown" or (
                obs.source == "home_wifi"
                and (obs.event == "disconnected" or obs.state != "home")
            ):
                state, reason, pending, latest, proof = (
                    "unknown",
                    "unconfirmed_signal",
                    None,
                    obs.at,
                    [obs.id],
                )
            elif obs.source == "home_wifi" and obs.state == "home":
                state, reason, pending, latest, proof = (
                    "home",
                    "home_wifi",
                    None,
                    obs.at,
                    [obs.id],
                )
            elif obs.source == "phone_geofence" and obs.state == "away":
                state, reason, pending, latest, proof = (
                    "away",
                    "geofence_away",
                    None,
                    obs.at,
                    [obs.id],
                )
            elif obs.source == "phone_geofence" and obs.state == "home":
                if (
                    state == "home"
                    and latest is not None
                    and obs.at - latest <= OBSERVATION_TTL
                ):
                    latest, proof = obs.at, [*proof[-1:], obs.id]
                elif (
                    pending is not None
                    and GEOFENCE_DWELL <= obs.at - pending.at <= OBSERVATION_TTL
                ):
                    state, reason, latest, proof = (
                        "home",
                        "geofence_dwell",
                        obs.at,
                        [pending.id, obs.id],
                    )
                else:
                    if pending is None or obs.at - pending.at > OBSERVATION_TTL:
                        pending = obs
                    state, reason, latest, proof = (
                        "unknown",
                        "awaiting_dwell",
                        obs.at,
                        [pending.id, obs.id],
                    )
        if latest is None or query - latest > OBSERVATION_TTL:
            state, reason = "unknown", "missing_or_stale"
        # Simultaneous contradictory authoritative reports are uncertain.
        if latest is not None:
            simultaneous = {
                o.state
                for o in records
                if o.at == latest
                and o.sharing_enabled
                and (
                    o.source == "home_wifi"
                    and o.state == "home"
                    or o.source == "phone_geofence"
                    and o.location_enabled
                )
            }
            if {"home", "away"} <= simultaneous:
                state, reason = "unknown", "conflicting_signals"
        return dict(
            resident_id=resident,
            state=state,
            reason=reason,
            observed_at=latest.isoformat() if latest else None,
            evidence_ids=proof,
        )

    def at(self, query: datetime) -> dict:
        if query in self.cache:
            return self.cache[query]
        if query > self.as_of:
            return dict(state="unknown", reason="future", residents=[])
        residents = [
            self.resident_at(resident, query) for resident in self.household.residents
        ]
        states = [r["state"] for r in residents]
        state = (
            "home"
            if "home" in states
            else "away"
            if all(s == "away" for s in states)
            else "unknown"
        )
        report = dict(
            state=state,
            reason="someone_home"
            if state == "home"
            else "everyone_away"
            if state == "away"
            else "resident_unknown",
            residents=residents,
        )
        self.cache[query] = report
        return report

    def first_home_after(self, start: datetime, end: datetime) -> dict | None:
        points = sorted(
            {
                max(obs.at, obs.received_at)
                for obs in self.household.observations
                if start < max(obs.at, obs.received_at) < end
                and max(obs.at, obs.received_at) <= self.as_of
                and obs.source != "phone_proximity"
            }
        )
        for point in points:
            presence = self.at(point)
            if presence["state"] == "home":
                return dict(
                    confirmed_at=point.isoformat(),
                    residents=[
                        r for r in presence["residents"] if r["state"] == "home"
                    ],
                )
        return None
