package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pradita/ac-project/service/api/internal/loadseed"
)

// smallConfig is a valid size small enough for a unit test.
func smallConfig() loadseed.Config {
	cfg := loadseed.LaunchConfig("load-pass", time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC))
	cfg.Customers = 6
	cfg.UnitsPerCustomer = 2
	cfg.Contractors = 2
	cfg.CustomerUsers = 3
	cfg.PartnerUsers = 2
	cfg.TechnicianUsers = 2
	cfg.HQUsers = 1
	cfg.Probes = 2

	return cfg
}

func outFiles(t *testing.T, withLinks bool) files {
	t.Helper()

	dir := t.TempDir()
	out := files{manifest: filepath.Join(dir, "manifest.json"), links: ""}

	if withLinks {
		out.links = filepath.Join(dir, "links.json")
	}

	return out
}

// TestRunManifestOnly writes the manifest and the links, owner-only, and touches no database (the URL is empty, which
// Guard would refuse).
func TestRunManifestOnly(t *testing.T) {
	t.Parallel()

	out := outFiles(t, true)

	err := run(context.Background(), "", out, true, smallConfig())
	if err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{out.manifest, out.links} {
		info, statErr := os.Stat(path)
		if statErr != nil {
			t.Fatal(statErr)
		}

		if info.Mode().Perm() != manifestMode {
			t.Fatalf(
				"%s has mode %o, want %o (it holds the passwords)",
				path,
				info.Mode().Perm(),
				manifestMode,
			)
		}
	}

	data, err := os.ReadFile(out.manifest)
	if err != nil {
		t.Fatal(err)
	}

	manifest := new(loadseed.Manifest)

	err = json.Unmarshal(data, manifest)
	if err != nil {
		t.Fatal(err)
	}

	if manifest.Customers != 6 || manifest.Units != 12 || len(manifest.Users) != 8 {
		t.Fatalf(
			"manifest: %d customers, %d units, %d users; want 6, 12, 8",
			manifest.Customers,
			manifest.Units,
			len(manifest.Users),
		)
	}

	data, err = os.ReadFile(out.links)
	if err != nil {
		t.Fatal(err)
	}

	links := new(loadseed.LinksFile)

	err = json.Unmarshal(data, links)
	if err != nil {
		t.Fatal(err)
	}

	probes := 0

	for _, link := range links.Links {
		if link.Probe {
			probes++
		}
	}

	if len(links.Links) != 12 || probes != 2 {
		t.Fatalf(
			"links: %d devices, %d probes; want 12 (one per unit), 2",
			len(links.Links),
			probes,
		)
	}
}

// TestRunWithoutLinksPath writes the manifest only.
func TestRunWithoutLinksPath(t *testing.T) {
	t.Parallel()

	out := outFiles(t, false)

	err := run(context.Background(), "", out, true, smallConfig())
	if err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(filepath.Dir(out.manifest))
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != 1 || entries[0].Name() != "manifest.json" {
		t.Fatalf("wrote %d files, want the manifest only", len(entries))
	}
}

// TestRunRefuses checks the refusals that come before any write: a bad size, and a database other than ac_load, which
// is refused before connecting (the port is closed, so a connect attempt would fail with another error).
func TestRunRefuses(t *testing.T) {
	t.Parallel()

	noPassword := smallConfig()
	noPassword.Password = ""

	cases := []struct {
		name string
		url  string
		cfg  loadseed.Config
		want error
	}{
		{"no password", "", noPassword, loadseed.ErrBadConfig},
		{
			"other database",
			"postgres://owner@127.0.0.1:1/ac?sslmode=disable",
			smallConfig(),
			loadseed.ErrWrongDatabase,
		},
	}

	for _, entry := range cases {
		t.Run(entry.name, func(t *testing.T) {
			t.Parallel()

			out := outFiles(t, true)

			err := run(context.Background(), entry.url, out, false, entry.cfg)
			if !errors.Is(err, entry.want) {
				t.Fatalf("err = %v, want %v", err, entry.want)
			}

			_, statErr := os.Stat(out.manifest)
			if !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("manifest written on a refused run (stat: %v)", statErr)
			}
		})
	}
}
