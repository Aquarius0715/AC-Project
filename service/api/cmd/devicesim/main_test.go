package main

import (
	"strings"
	"testing"
	"time"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// TestClientTagIsShortAndStable: the MQTT client ID uses at most the first eight characters of the tenant.
func TestClientTagIsShortAndStable(t *testing.T) {
	if got := clientTag("820cc6b6-9ea8-5d18-8b10-5c8c1d126814"); got != "820cc6b6" {
		t.Errorf("long tenant: %q", got)
	}

	if got := clientTag("abc"); got != "abc" {
		t.Errorf("short tenant: %q", got)
	}
}

// TestSeedIsPerDevice: each device has its own seed, the same device always the same seed.
func TestSeedIsPerDevice(t *testing.T) {
	a, b := seedOf(
		"71f1fdc1-0d2b-516c-8bfa-bd8dfecd338b",
	), seedOf(
		"88ee2ed2-79db-53b6-be15-a92f7829c53d",
	)
	if a == b || a != seedOf("71f1fdc1-0d2b-516c-8bfa-bd8dfecd338b") {
		t.Errorf("seeds %d %d", a, b)
	}
}

// TestDemoDevicesAreTheFixtureLinks: the simulator runs the five fixture devices, by fixture name (IR613).
func TestDemoDevicesAreTheFixtureLinks(t *testing.T) {
	names := demoDevices()
	if len(names) != 5 || !strings.HasPrefix(names[0], "device-") {
		t.Fatalf("demo devices %v", names)
	}

	if seed.ID(names[0]).String() != "71f1fdc1-0d2b-516c-8bfa-bd8dfecd338b" {
		t.Errorf("first device %s does not derive the API ID", names[0])
	}
}

func TestEnvOr(t *testing.T) {
	t.Setenv("DEVICESIM_TEST", "x")

	if envOr("DEVICESIM_TEST", "d") != "x" || envOr("DEVICESIM_TEST_UNSET", "d") != "d" {
		t.Error("envOr")
	}
}

// TestSensorsOfAFixtureDevice: a fixture device reports the sensors its link names, an unknown device none.
func TestSensorsOfAFixtureDevice(t *testing.T) {
	got := map[string]bool{}
	for _, s := range sensorsOf("device-tamper") {
		got[s] = true
	}

	if len(got) != 3 || !got["temperature"] || !got["power"] || !got["humidity"] {
		t.Errorf("device-tamper sensors %v", got)
	}

	if sensorsOf("device-unknown") != nil {
		t.Error("an unknown device has sensors")
	}
}

// TestFixtureModulesPlayTheirConnection: an online fixture device reports and beats; the device that is offline in the
// fixture (Meeting room AC's) sends no readings, no heartbeats and no command answers; a device without sensors only
// beats (IR613).
func TestFixtureModulesPlayTheirConnection(t *testing.T) {
	at := time.Date(2026, 9, 15, 1, 30, 0, 0, time.UTC)

	online, beats := fixtureModule("t", "device-online-rto", "v1")
	raw, err := online.Telemetry(at)
	if err != nil || raw == nil || !beats {
		t.Errorf("online: %s %v beats=%v", raw, err, beats)
	}

	offline, beats := fixtureModule("t", "device-offline-rto", "v1")
	raw, err = offline.Telemetry(at)
	if err != nil || raw != nil || beats {
		t.Errorf("offline: %s %v beats=%v", raw, err, beats)
	}

	cmd := []byte(
		`{"commandId":"c1","kind":"set_temperature","setpoint":25,"expiresAt":"2026-09-15T01:31:00Z"}`,
	)
	ack, _, err := offline.Acknowledge(cmd, at)
	if err != nil || ack != nil {
		t.Errorf("offline answers a command: %s %v", ack, err)
	}

	if _, beats := fixtureModule("t", "device-unknown", "v1"); !beats {
		t.Error("a device without a link is not taken as online")
	}
	// device-no-sensor (Bedroom AC B) has no sensors: its module only beats
	bare, beats := fixtureModule("t", "device-no-sensor", "v1")
	raw, err = bare.Telemetry(at)
	if err != nil || raw != nil || !beats {
		t.Errorf("no sensors: %s %v beats=%v", raw, err, beats)
	}
}
