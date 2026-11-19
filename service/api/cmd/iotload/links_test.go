package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

func linksFile(t *testing.T, body string) config {
	t.Helper()

	path := filepath.Join(t.TempDir(), "links.json")

	err := os.WriteFile(path, []byte(body), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	cfg := validConfig()
	cfg.links = path

	return cfg
}

func TestReadLinksSplitsDevicesAndProbes(t *testing.T) {
	t.Parallel()

	cfg := linksFile(t, `{"links":[
		{"device":"dev-1","unit":"u-1","sensors":{"temperature":"s-1t","power":"s-1p"}},
		{"device":"probe-1","unit":"u-2","sensors":{"temperature":"s-2t","power":"s-2p"},"probe":true}
	]}`)

	devices, probes, err := readLinks(cfg)
	if err != nil {
		t.Fatal(err)
	}

	if len(devices) != 1 || len(probes) != 1 {
		t.Fatalf("got %d devices and %d probes, want 1 and 1", len(devices), len(probes))
	}

	device := devices[0]
	if device.module.DeviceID != seed.ID("dev-1").String() || device.sensors != 2 {
		t.Fatalf(
			"device %s with %d sensors, want dev-1's ID with 2",
			device.module.DeviceID,
			device.sensors,
		)
	}

	if probes[0].sensor != seed.ID("s-2t").String() {
		t.Fatalf("probe sensor %s, want its temperature sensor s-2t's ID", probes[0].sensor)
	}

	// a probe batch has its temperature reading only, so a probe adds one expected reading
	raw, err := probes[0].module.Telemetry(time.Now())

	_, ok := firstSequence(raw, err)
	if !ok {
		t.Fatalf("probe telemetry %s, %v: no first reading", raw, err)
	}
}

func TestReadLinksRefuses(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name, body string
		want       error
	}{
		{
			"no device",
			`{"links":[{"device":"p","sensors":{"temperature":"s"},"probe":true}]}`,
			errNoDevices,
		},
		{"no probe", `{"links":[{"device":"d","sensors":{"temperature":"s"}}]}`, errNoProbes},
		{
			"probe without its temperature sensor",
			`{"links":[{"device":"d","sensors":{"temperature":"s"}},{"device":"p","sensors":{"power":"s"},"probe":true}]}`,
			errProbeSensor,
		},
	}

	for _, entry := range cases {
		t.Run(entry.name, func(t *testing.T) {
			t.Parallel()

			_, _, err := readLinks(linksFile(t, entry.body))
			if !errors.Is(err, entry.want) {
				t.Fatalf("readLinks() = %v, want %v", err, entry.want)
			}
		})
	}

	_, _, err := readLinks(linksFile(t, `{"links":`))
	if err == nil {
		t.Fatal("readLinks() of broken JSON = nil, want an error")
	}
}

func TestFirstSequence(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		raw  string
		err  error
		seq  int64
		ok   bool
	}{
		{"first reading", `{"measurements":[{"sequence":7},{"sequence":8}]}`, nil, 7, true},
		{"no reading", `{"measurements":[]}`, nil, 0, false},
		{"not JSON", `{`, nil, 0, false},
		{"telemetry failed", `{"measurements":[{"sequence":7}]}`, errNoDevices, 0, false},
	}

	for _, entry := range cases {
		t.Run(entry.name, func(t *testing.T) {
			t.Parallel()

			seq, ok := firstSequence([]byte(entry.raw), entry.err)
			if seq != entry.seq || ok != entry.ok {
				t.Fatalf("firstSequence() = %d, %v, want %d, %v", seq, ok, entry.seq, entry.ok)
			}
		})
	}

	_, ok := firstSequence(nil, nil)
	if ok {
		t.Fatal("firstSequence(nil) is ok, want not")
	}
}
