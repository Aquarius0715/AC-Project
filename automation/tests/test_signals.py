"""Presence signal regression retained independently of obsolete fixtures."""

import unittest
from datetime import datetime, timedelta
from automation.src.presence import PresenceIndex
from automation.src.presence_data import Household, Observation


def at(time: str) -> datetime:
    return datetime.fromisoformat(f"2026-10-07T{time}:00+09:00")


def obs(
    resident, time, source, event, state, *, enabled=True, location=True, received=None
):
    return Observation(
        f"{resident}:{time}:{source}:{event}",
        resident,
        at(time),
        at(received or time),
        source,
        event,
        state,
        enabled,
        location,
    )


def index(records, residents=("a",), cutoff="23:59"):
    return PresenceIndex(
        Household(
            "house",
            "home",
            residents,
            tuple(sorted(records, key=lambda o: (o.at, o.id))),
            ("device",),
        ),
        at(cutoff),
    )


class PresenceSignalTests(unittest.TestCase):
    def test_one_confirmed_person_overrides_other_unknown_people(self):
        p = index([obs("a", "18:20", "home_wifi", "connected", "home")], ("a", "b"))
        self.assertEqual(p.at(at("18:30"))["state"], "home")

    def test_everyone_must_have_fresh_away_confirmation(self):
        records = [obs("a", "18:00", "phone_geofence", "exit", "away")]
        self.assertEqual(index(records, ("a", "b")).at(at("18:30"))["state"], "unknown")
        records.append(obs("b", "18:00", "phone_geofence", "heartbeat", "away"))
        self.assertEqual(index(records, ("a", "b")).at(at("18:30"))["state"], "away")

    def test_stale_disabled_and_location_unavailable_are_unknown(self):
        variants = [
            obs("a", "15:00", "phone_geofence", "exit", "away"),
            obs("a", "18:00", "phone_geofence", "exit", "away", location=False),
            obs("a", "18:00", "permission", "disabled", "unknown", enabled=False),
        ]
        for record in variants:
            with self.subTest(record=record):
                self.assertEqual(index([record]).at(at("18:30"))["state"], "unknown")

    def test_wifi_can_confirm_home_without_location_permission(self):
        record = obs("a", "18:20", "home_wifi", "connected", "home", location=False)
        self.assertEqual(index([record]).at(at("18:30"))["state"], "home")
        no_gps = obs(
            "a", "18:25", "phone_geofence", "heartbeat", "unknown", location=False
        )
        self.assertEqual(index([record, no_gps]).at(at("18:30"))["state"], "home")

    def test_approach_does_not_confirm_arrival_or_refresh_old_away(self):
        records = [
            obs("a", "18:00", "phone_geofence", "exit", "away"),
            obs("a", "18:20", "phone_proximity", "nearby", "unknown"),
        ]
        self.assertEqual(index(records).at(at("18:30"))["state"], "away")
        self.assertEqual(index(records[1:]).at(at("18:30"))["state"], "unknown")

    def test_passby_and_detour_do_not_become_an_early_return(self):
        records = [
            obs("a", "18:00", "phone_geofence", "exit", "away"),
            obs("a", "18:10", "phone_geofence", "enter", "home"),
            obs("a", "18:15", "phone_geofence", "exit", "away"),
            obs("a", "18:20", "phone_proximity", "nearby", "unknown"),
            obs("a", "21:00", "home_wifi", "connected", "home"),
        ]
        p = index(records)
        self.assertEqual(p.at(at("18:30"))["state"], "away")
        self.assertEqual(
            p.first_home_after(at("18:00"), at("23:00"))["confirmed_at"],
            at("21:00").isoformat(),
        )

    def test_geofence_requires_two_inside_observations_ten_minutes_apart(self):
        records = [
            obs("a", "18:00", "phone_geofence", "enter", "home"),
            obs("a", "18:05", "phone_geofence", "heartbeat", "home"),
            obs("a", "18:10", "phone_geofence", "heartbeat", "home"),
        ]
        p = index(records)
        self.assertEqual(p.at(at("18:05"))["state"], "unknown")
        self.assertEqual(p.at(at("18:10"))["state"], "home")
        self.assertEqual(index(records[:1]).at(at("18:15"))["state"], "unknown")

    def test_future_observed_or_late_received_information_does_not_leak(self):
        records = [
            obs("a", "18:00", "phone_geofence", "exit", "away"),
            obs("a", "18:20", "home_wifi", "connected", "home", received="19:00"),
        ]
        p = index(records)
        self.assertEqual(p.at(at("18:30"))["state"], "away")
        self.assertEqual(
            p.first_home_after(at("18:30"), at("23:00"))["confirmed_at"],
            at("19:00").isoformat(),
        )
        self.assertEqual(
            index(records, cutoff="18:30").at(at("19:00"))["state"], "unknown"
        )

    def test_a_later_opt_out_does_not_change_earlier_presence(self):
        records = [
            obs("a", "18:00", "home_wifi", "connected", "home"),
            obs("a", "18:20", "permission", "disabled", "unknown", enabled=False),
        ]
        p = index(records)
        self.assertEqual(p.at(at("18:10"))["state"], "home")
        self.assertEqual(p.at(at("18:30"))["state"], "unknown")

    def test_ttl_boundary_and_conflicting_observations(self):
        p = index([obs("a", "16:30", "home_wifi", "connected", "home")])
        self.assertEqual(p.at(at("18:30"))["state"], "home")
        self.assertEqual(p.at(at("18:30") + timedelta(seconds=1))["state"], "unknown")
        p = index(
            [
                obs("a", "18:20", "home_wifi", "connected", "home"),
                obs("a", "18:20", "phone_geofence", "exit", "away"),
            ]
        )
        self.assertEqual(p.at(at("18:30"))["state"], "unknown")
