package loadseed_test

import (
	"errors"
	"testing"
	"time"

	"github.com/pradita/ac-project/service/api/internal/loadseed"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

// loadseed writes to the database ac_load only: every other name is refused before anything connects.
func TestGuardRefusesEveryDatabaseButAcLoad(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		url  string
		pass bool
	}{
		{"postgres://postgres:local@postgres:5432/ac_load?sslmode=disable", true},
		{"postgres://postgres:local@localhost:5432/ac_load", true},
		{"postgres://postgres:local@postgres:5432/ac?sslmode=disable", false},
		{"postgres://postgres:local@postgres:5432/ac_test?sslmode=disable", false},
		{"postgres://postgres:local@postgres:5432/ac_loader", false},
		{"postgres://postgres:local@postgres:5432/AC_LOAD", false},
		{"postgres://postgres:local@postgres:5432/ac_load/extra", false},
		{"postgres://postgres:local@postgres:5432/", false},
		{"postgres://postgres:local@postgres:5432", false},
		{"postgres://postgres:local@postgres:5432/postgres", false},
		{"postgres://postgres:local@postgres:5432/ac?dbname=ac_load", false},
		{"", false},
		{"://not a url", false},
	} {
		err := loadseed.Guard(test.url)

		if test.pass && err != nil {
			t.Errorf("Guard(%q) = %v, want nil", test.url, err)
		}

		if !test.pass && !errors.Is(err, loadseed.ErrWrongDatabase) {
			t.Errorf("Guard(%q) = %v, want ErrWrongDatabase", test.url, err)
		}
	}
}

func TestBuildLaunchSize(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.November, 9, 3, 0, 0, 0, time.UTC)

	result, err := loadseed.Build(loadseed.LaunchConfig("pw", now))
	if err != nil {
		t.Fatal(err)
	}

	got := result.Fixture.DemoSeed
	if len(got.Customers) != 600 || len(got.Units) != 3000 || len(got.Contracts) != 600 ||
		len(got.Invoices) != 1800 ||
		len(got.Jobs) != 1200 ||
		len(got.Properties) != 600 ||
		len(got.Spaces) != 1200 {
		t.Errorf(
			"sizes: customers %d, units %d, contracts %d, invoices %d, jobs %d",
			len(got.Customers),
			len(got.Units),
			len(got.Contracts),
			len(got.Invoices),
			len(got.Jobs),
		)
	}

	if len(got.Devices) != 3000 || len(result.Links.Links) != 3000 {
		t.Errorf(
			"devices %d, links %d, want one device on each of the 3000 units",
			len(got.Devices),
			len(result.Links.Links),
		)
	}

	probes := 0

	for _, link := range result.Links.Links {
		if link.Probe {
			probes++
		}

		if len(link.Sensors) != 2 || link.Unit == "" || link.Connection != "online" {
			t.Fatalf("link %+v is incomplete", link)
		}
	}

	if probes != 60 {
		t.Errorf("probe devices = %d, want 60", probes)
	}

	if len(got.Consents) != 210 {
		t.Errorf("consents = %d, want one for each of the 210 customer accounts", len(got.Consents))
	}

	if len(got.Organizations) != 610 || len(got.Offers) != 300 {
		t.Errorf(
			"organizations %d (want 610), offers %d (want 300)",
			len(got.Organizations),
			len(got.Offers),
		)
	}

	if len(result.Fixture.Actors) != 300 || len(result.Manifest.Users) != 300 {
		t.Fatalf(
			"accounts: actors %d, manifest %d, want 300",
			len(result.Fixture.Actors),
			len(result.Manifest.Users),
		)
	}

	groups := map[string]int{}
	for _, user := range result.Manifest.Users {
		groups[user.Group]++
	}

	if groups["customer"] != 210 || groups["partner"] != 45 || groups["technician"] != 30 ||
		groups["hq"] != 15 {
		t.Errorf(
			"groups = %v, want 70%% customer, 15%% partner, 10%% technician, 5%% HQ of 300",
			groups,
		)
	}
}

// Every ID is unique, the manifest names the database's IDs, and a second run makes the same data.
func TestBuildIsDeterministicAndConsistent(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.November, 9, 3, 0, 0, 0, time.UTC)
	cfg := loadseed.Config{
		Customers:        8,
		UnitsPerCustomer: 3,
		Contractors:      2,
		CustomerUsers:    4,
		PartnerUsers:     2,
		TechnicianUsers:  2,
		HQUsers:          1,
		Probes:           0,
		Password:         "pw",
		Now:              now,
	}

	first, err := loadseed.Build(cfg)
	if err != nil {
		t.Fatal(err)
	}

	second, err := loadseed.Build(cfg)
	if err != nil {
		t.Fatal(err)
	}

	if len(first.Manifest.Users) != len(second.Manifest.Users) ||
		first.Manifest.Users[0].UserID != second.Manifest.Users[0].UserID {
		t.Error("two builds differ")
	}

	seen := map[string]bool{}

	for _, unit := range first.Fixture.DemoSeed.Units {
		if seen[unit.ID] {
			t.Errorf("unit ID %s twice", unit.ID)
		}

		seen[unit.ID] = true
	}

	customer := first.Manifest.Users[0]
	if customer.Group != "customer" ||
		customer.MembershipID != seed.ID("load-customer-0001").String() ||
		len(customer.UnitIDs) != 3 ||
		len(customer.JobIDs) != 2 ||
		customer.CustomerID != seed.ID("load-cust-0001").String() {
		t.Errorf("first customer account = %+v", customer)
	}

	for _, user := range first.Manifest.Users {
		if user.Group == "technician" && len(user.UnitIDs) == 0 {
			t.Errorf("technician %s has no units", user.Username)
		}

		if user.Password != "pw" || user.TenantID == "" || user.OrgID == "" {
			t.Errorf("account %s is incomplete: %+v", user.Username, user)
		}
	}
}

func TestBuildRefusesABadConfig(t *testing.T) {
	t.Parallel()

	good := loadseed.LaunchConfig("pw", time.Now())

	for name, mutate := range map[string]func(*loadseed.Config){
		"no customers":        func(c *loadseed.Config) { c.Customers = 0 },
		"more owners":         func(c *loadseed.Config) { c.CustomerUsers = c.Customers + 1 },
		"negative accounts":   func(c *loadseed.Config) { c.HQUsers = -1 },
		"no password":         func(c *loadseed.Config) { c.Password = "" },
		"no time":             func(c *loadseed.Config) { c.Now = time.Time{} },
		"too many probes":     func(c *loadseed.Config) { c.Probes = c.Customers*c.UnitsPerCustomer + 1 },
		"no units a customer": func(c *loadseed.Config) { c.UnitsPerCustomer = 0 },
	} {
		cfg := good
		mutate(&cfg)

		_, err := loadseed.Build(cfg)
		if err == nil {
			t.Errorf("%s: Build accepted the config", name)
		}
	}
}
