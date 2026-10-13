package main

import (
	"strings"
	"testing"

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
	a, b := seedOf("71f1fdc1-0d2b-516c-8bfa-bd8dfecd338b"), seedOf("88ee2ed2-79db-53b6-be15-a92f7829c53d")
	if a == b || a != seedOf("71f1fdc1-0d2b-516c-8bfa-bd8dfecd338b") {
		t.Errorf("seeds %d %d", a, b)
	}
}

// TestDemoDevicesAreTheFixtureLinks: the simulator runs the four demo devices that report measurements, by fixture name.
func TestDemoDevicesAreTheFixtureLinks(t *testing.T) {
	names := demoDevices()
	if len(names) != 4 || !strings.HasPrefix(names[0], "device-") {
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
