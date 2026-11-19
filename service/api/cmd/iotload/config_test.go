package main

import (
	"errors"
	"testing"
	"time"
)

func validConfig() config {
	cfg := new(config)
	cfg.broker = "tcp://localhost:1883"
	cfg.tenant = "tenant"
	cfg.links = "links.json"
	cfg.db = "postgres://localhost/ac_load"
	cfg.out = "iotload.json"
	cfg.interval = time.Minute
	cfg.heartbeat = time.Minute
	cfg.probePeriod = time.Minute
	cfg.drainBy = time.Minute

	return *cfg
}

func TestConfigValidate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		edit func(cfg *config)
		ok   bool
	}{
		{"defaults", func(*config) {}, true},
		{
			"until a signal and no drain",
			func(cfg *config) { cfg.duration, cfg.drainBy = 0, 0 },
			true,
		},
		{
			"probe period of exactly a minute",
			func(cfg *config) { cfg.probePeriod = time.Minute },
			true,
		},
		{"no links file", func(cfg *config) { cfg.links = "" }, false},
		{"no database", func(cfg *config) { cfg.db = " " }, false},
		{"no broker", func(cfg *config) { cfg.broker = "" }, false},
		{"no tenant", func(cfg *config) { cfg.tenant = "" }, false},
		{"no report path", func(cfg *config) { cfg.out = "" }, false},
		{"zero interval", func(cfg *config) { cfg.interval = 0 }, false},
		{"negative heartbeat", func(cfg *config) { cfg.heartbeat = -time.Second }, false},
		// NewTicker panics on a non-positive period
		{"zero probe period", func(cfg *config) { cfg.probePeriod = 0 }, false},
		// a sensor keeps one reading a minute, so a second probe of the minute is never stored
		{
			"probe period under a minute",
			func(cfg *config) { cfg.probePeriod = 59 * time.Second },
			false,
		},
		{"negative duration", func(cfg *config) { cfg.duration = -time.Second }, false},
		{"negative drain", func(cfg *config) { cfg.drainBy = -time.Second }, false},
	}

	for _, entry := range cases {
		t.Run(entry.name, func(t *testing.T) {
			t.Parallel()

			cfg := validConfig()
			entry.edit(&cfg)

			err := cfg.Validate()
			if entry.ok && err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}

			if !entry.ok && !errors.Is(err, errConfig) {
				t.Fatalf("Validate() = %v, want errConfig", err)
			}
		})
	}
}
