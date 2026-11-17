package loadseed_test

import (
	"errors"
	"fmt"
	"slices"
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

// smallConfig has an odd number of customers, so the offers (every second customer) and the technicians' four
// customers each (wrapping past the last customer) both reach the end of the list.
func smallConfig() loadseed.Config {
	return loadseed.Config{
		Customers:        9,
		UnitsPerCustomer: 3,
		Contractors:      3,
		CustomerUsers:    2,
		PartnerUsers:     3,
		TechnicianUsers:  3,
		HQUsers:          0,
		Probes:           0,
		Password:         "pw",
		Now:              time.Date(2026, time.November, 9, 3, 0, 0, 0, time.UTC),
	}
}

// A partner account's jobs are exactly the offered jobs of its own company: the load scripts open them as that
// company, so a job of another company there would be refused and one left out would never be measured.
func TestBuildPartnerJobsAreTheirCompanysOffers(t *testing.T) {
	t.Parallel()

	result, err := loadseed.Build(smallConfig())
	if err != nil {
		t.Fatal(err)
	}

	offered := map[string]map[string]bool{} // contractor org UUID → offered job UUIDs

	for _, offer := range result.Fixture.DemoSeed.Offers {
		jobID, jobOK := offer["jobId"].(string)
		orgID, orgOK := offer["contractorOrgId"].(string)

		if !jobOK || !orgOK {
			t.Fatalf("offer %v has no job or contractor", offer)
		}

		org := seed.ID(orgID).String()
		if offered[org] == nil {
			offered[org] = map[string]bool{}
		}

		offered[org][seed.ID(jobID).String()] = true
	}

	for _, job := range result.Fixture.DemoSeed.Jobs {
		if job["status"] != "offered" {
			continue
		}

		jobID, jobOK := job["id"].(string)
		orgID, orgOK := job["contractorOrgId"].(string)

		if !jobOK || !orgOK || !offered[seed.ID(orgID).String()][seed.ID(jobID).String()] {
			t.Errorf("offered job %v has no offer to its contractor", job)
		}
	}

	total := 0
	partners := 0

	for _, user := range result.Manifest.Users {
		if user.Group != "partner" {
			continue
		}

		partners++

		want := offered[user.OrgID]
		got := map[string]bool{}

		for _, id := range user.JobIDs {
			got[id] = true

			if !want[id] {
				t.Errorf(
					"partner %s lists job %s, which is not offered to its company",
					user.Username,
					id,
				)
			}
		}

		if len(got) != len(want) || len(user.JobIDs) != len(want) {
			t.Errorf(
				"partner %s lists %d jobs, its company has %d offers",
				user.Username,
				len(user.JobIDs),
				len(want),
			)
		}

		total += len(user.JobIDs)
	}

	// customers 1, 3, 5, 7 and 9 each have their first job offered, and the 3 partners are one per company
	if partners != 3 || total != 5 || len(result.Fixture.DemoSeed.Offers) != 5 {
		t.Errorf("partners %d with %d jobs, offers %d, want 3 partners sharing all 5 offers",
			partners, total, len(result.Fixture.DemoSeed.Offers))
	}
}

// A technician's units are the units of four customers, counted on past the last customer, and each is a real unit.
func TestBuildTechnicianUnitsWrapAround(t *testing.T) {
	t.Parallel()

	cfg := smallConfig()

	result, err := loadseed.Build(cfg)
	if err != nil {
		t.Fatal(err)
	}

	units := map[string]bool{}
	for _, unit := range result.Fixture.DemoSeed.Units {
		units[seed.ID(unit.ID).String()] = true
	}

	customerUnits := func(n int) []string {
		out := make([]string, 0, cfg.UnitsPerCustomer)
		for j := range cfg.UnitsPerCustomer {
			out = append(out, seed.ID(fmt.Sprintf("load-unit-%04d-%d", n, j+1)).String())
		}

		return out
	}

	technicians := 0

	for _, user := range result.Manifest.Users {
		if user.Group != "technician" {
			continue
		}

		technicians++

		if len(user.UnitIDs) != 4*cfg.UnitsPerCustomer {
			t.Errorf(
				"technician %s has %d units, want %d",
				user.Username,
				len(user.UnitIDs),
				4*cfg.UnitsPerCustomer,
			)
		}

		for _, id := range user.UnitIDs {
			if !units[id] {
				t.Errorf(
					"technician %s names unit %s, which the seed does not have",
					user.Username,
					id,
				)
			}
		}

		// the third technician has customers 9, 1, 2 and 3 (numbered from 1): the list wraps after the last customer
		if user.Username == "load-technician-0003" {
			want := append(customerUnits(9), customerUnits(1)...)
			if !slices.Equal(user.UnitIDs[:len(want)], want) {
				t.Errorf(
					"technician %s units start %v, want customers 9 then 1: %v",
					user.Username,
					user.UnitIDs,
					want,
				)
			}
		}
	}

	if technicians != cfg.TechnicianUsers {
		t.Errorf("technicians = %d, want %d", technicians, cfg.TechnicianUsers)
	}
}

// With no probes every link is an ordinary device; with probes only the last units' links are.
func TestBuildProbesAreTheLastUnits(t *testing.T) {
	t.Parallel()

	for _, probes := range []int{0, 2} {
		cfg := smallConfig()
		cfg.Probes = probes

		result, err := loadseed.Build(cfg)
		if err != nil {
			t.Fatal(err)
		}

		links := result.Links.Links
		if len(links) != cfg.Customers*cfg.UnitsPerCustomer {
			t.Fatalf("probes %d: links = %d, want one per unit", probes, len(links))
		}

		for i, link := range links {
			wantProbe := i >= len(links)-probes
			if link.Probe != wantProbe {
				t.Errorf(
					"probes %d: link %d (%s) probe = %v, want %v",
					probes,
					i,
					link.Unit,
					link.Probe,
					wantProbe,
				)
			}
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
