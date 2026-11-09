// Package loadseed builds the data of the load test (docs/02-design/load-test.md): the launch size of the platform (600
// customers, 3,000 units, their contracts, invoices and jobs) and the synthetic accounts the k6 scenarios sign in
// with. It is a fixture in the shape of fixture-contract.json that internal/seed applies on top of the demo seed, so
// the IDs, the tables and the constraints are the seed's own. The account manifest it also returns names
// each account's IDs for the realm copy (deploy/load/realm.py) and for the scenarios.
//
// It only ever writes to the database "ac_load" of the compose project ac-load, which is made and removed for each
// run: Guard refuses every other database name before anything connects.
package loadseed

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

// Database is the only database loadseed writes to.
const Database = "ac_load"

var (
	// ErrWrongDatabase is returned when the database is not Database.
	ErrWrongDatabase = errors.New("loadseed writes to the database " + Database + " only")
	// ErrBadConfig is returned for a Config that cannot make data.
	ErrBadConfig = errors.New("bad load seed configuration")
)

// The launch size of backend-architecture §13 and the split of its 300 accounts (an assumption: customer 70%, partner
// 15%, technician 10%, HQ 5%).
const (
	launchCustomers   = 600
	launchUnits       = 5
	launchContractors = 10
	launchCustomerAcc = 210
	launchPartnerAcc  = 45
	launchTechAcc     = 30
	launchHQAcc       = 15
)

// The size of one customer's records, and the dates of its contract and invoices (days before or after now).
const (
	invoicesPerCustomer = 3
	paidInvoices        = 2
	invoiceMinor        = 12000
	monthDays           = 30
	invoiceGraceDays    = 10
	unitInstalledDays   = 400
	contractStartDays   = 300
	contractEndDays     = 60
	seedAgeDays         = 90
	techCustomers       = 4 // customers whose units a technician's access names
	slotStartDays       = 3 // a request names a time 3 days ahead, lasting a day
	slotEndDays         = 4
	offerEvery          = 2 // every second customer has its first request offered to a contractor
	offerExpiryDays     = 6
	offerAccessDays     = 14
)

// The keys of the fixture's JSON objects that several records share.
const (
	keyID            = "id"
	keyKind          = "kind"
	keyVersion       = "version"
	keyCustomerOrgID = "customerOrgId"
	keyPropertyID    = "propertyId"
	keyUnitIDs       = "unitIds"
	roleClient       = "client"
	keyTenantID      = "tenantId"
	keyName          = "name"
	keyStatus        = "status"
)

// Guard returns ErrWrongDatabase unless the database URL names Database.
func Guard(databaseURL string) error {
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		return fmt.Errorf("database url: %w", ErrWrongDatabase)
	}

	if strings.TrimPrefix(parsed.Path, "/") != Database {
		return fmt.Errorf("database %q: %w", strings.TrimPrefix(parsed.Path, "/"), ErrWrongDatabase)
	}

	return nil
}

// Config is the size of the data and the accounts.
type Config struct {
	Customers        int       // customer organizations, each with one customer, property and contract
	UnitsPerCustomer int       // air conditioners per customer
	Contractors      int       // contractor companies
	CustomerUsers    int       // synthetic accounts: the owners of the first customers
	PartnerUsers     int       // synthetic accounts of the contractor companies
	TechnicianUsers  int       // synthetic technicians of the contractor companies
	HQUsers          int       // synthetic HQ operators
	Password         string    // the password of every synthetic account
	Now              time.Time // the time the data is dated from
}

// LaunchConfig is the launch size of backend-architecture §13: 600 customers, 3,000 units and 300 concurrent users. The
// split of the accounts (customer 70%, partner 15%, technician 10%, HQ 5%) is an assumption without usage data.
func LaunchConfig(password string, now time.Time) Config {
	return Config{
		Customers:        launchCustomers,
		UnitsPerCustomer: launchUnits,
		Contractors:      launchContractors,
		CustomerUsers:    launchCustomerAcc,
		PartnerUsers:     launchPartnerAcc,
		TechnicianUsers:  launchTechAcc,
		HQUsers:          launchHQAcc,
		Password:         password,
		Now:              now,
	}
}

// Validate checks the size.
func (c Config) Validate() error {
	switch {
	case c.Customers < 1 || c.UnitsPerCustomer < 1 || c.Contractors < 1:
		return fmt.Errorf(
			"%w: customers, units per customer and contractors must be at least 1",
			ErrBadConfig,
		)
	case c.CustomerUsers > c.Customers:
		return fmt.Errorf("%w: there cannot be more customer accounts than customers", ErrBadConfig)
	case c.PartnerUsers < 0 || c.TechnicianUsers < 0 || c.HQUsers < 0 || c.CustomerUsers < 0:
		return fmt.Errorf("%w: account counts cannot be negative", ErrBadConfig)
	case c.Password == "":
		return fmt.Errorf("%w: a password is required", ErrBadConfig)
	case c.Now.IsZero():
		return fmt.Errorf("%w: now is required", ErrBadConfig)
	}

	return nil
}

// User is one synthetic account in the manifest.
type User struct {
	Username     string   `json:"username"`
	Password     string   `json:"password"`
	Group        string   `json:"group"` // customer, partner, technician or hq
	Role         string   `json:"role"`  // client, contractor, technician or admin: the Core API's role
	UserID       string   `json:"userId"`
	MembershipID string   `json:"membershipId"`
	TenantID     string   `json:"tenantId"`
	OrgID        string   `json:"orgId"`
	CustomerID   string   `json:"customerId"` // the customer record of a customer account
	UnitIDs      []string `json:"unitIds"`    // units the account's screens open
	JobIDs       []string `json:"jobIds"`     // jobs the account's screens open
}

// Manifest names the synthetic accounts and the size of the data.
type Manifest struct {
	Customers int    `json:"customers"`
	Units     int    `json:"units"`
	Jobs      int    `json:"jobs"`
	Users     []User `json:"users"`
}

// Result is what Build returns: the fixture to apply and the manifest.
type Result struct {
	Fixture  *seed.Fixture
	Manifest Manifest
}

const (
	tenant          = "tenant-a" // the demo seed's tenant: the models, the HQ operator and the default policies are there
	operator        = "org-operator-a"
	model           = "ventilation-demo" // a model of the demo seed
	modelVersion    = 3
	farFuture       = "2099-12-31T00:00:00.000Z"
	jobsPerCustomer = 2
)

// hqPermissions are the permissions of the demo seed's HQ operator, less identity.write (no account grants access).
func hqPermissions() []string {
	return []string{
		"dashboard.read",
		"asset.read",
		"asset.write",
		"control.execute",
		"device.read",
		"device.write",
		"alert.read",
		"alert.resolve",
		"job.read",
		"job.write",
		"contract.read",
		"contract.write",
		"billing.read",
		"billing.write",
		"billing.payment",
		"identity.read",
		"alert.policy.read",
		"alert.policy.write",
		"automation.policy.read",
		"automation.policy.write",
		"energy.read",
		"energy.write",
		"audit.read",
	}
}

// builder collects the fixture's lists and the manifest.
type builder struct {
	cfg                                                                   Config
	orgs, customers, properties, spaces, units, contracts, invoices, jobs []map[string]any
	offers, actors, consents                                              []map[string]any
	manifest                                                              Manifest
	customerUnits, customerJobs                                           map[int][]string
	contractorOf                                                          func(i int) string
}

func newBuilder(cfg Config) *builder {
	return &builder{
		cfg:           cfg,
		orgs:          nil,
		customers:     nil,
		properties:    nil,
		spaces:        nil,
		units:         nil,
		contracts:     nil,
		invoices:      nil,
		jobs:          nil,
		offers:        nil,
		consents:      nil,
		actors:        nil,
		manifest:      Manifest{Customers: 0, Units: 0, Jobs: 0, Users: nil},
		customerUnits: map[int][]string{},
		customerJobs:  map[int][]string{},
		contractorOf:  func(i int) string { return fmt.Sprintf("load-org-k-%02d", i%cfg.Contractors) },
	}
}

// Build makes the fixture and the manifest of cfg.
func Build(cfg Config) (*Result, error) {
	err := cfg.Validate()
	if err != nil {
		return nil, err
	}

	data := newBuilder(cfg)

	for k := range cfg.Contractors {
		data.orgs = append(
			data.orgs,
			org(
				fmt.Sprintf("load-org-k-%02d", k),
				"contractor",
				fmt.Sprintf("Load Contractor %02d", k),
			),
		)
	}

	for i := range cfg.Customers {
		data.customer(i)
	}

	data.accounts()

	doc := map[string]any{
		"seedCreatedAt": cfg.Now.AddDate(0, 0, -seedAgeDays).UTC().Format(time.RFC3339),
		"actors":        data.actors,
		"demoSeed": map[string]any{
			"organizations": data.orgs,
			"customers":     data.customers,
			"properties":    data.properties,
			"spaces":        data.spaces,
			"units":         data.units,
			"contracts":     data.contracts,
			"invoices":      data.invoices,
			"jobs":          data.jobs,
			"offers":        data.offers,
			"consents":      data.consents,
		},
	}

	raw, err := json.Marshal(doc)
	if err != nil {
		return nil, apperr.Wrap(err, "json.Marshal")
	}

	var fixture seed.Fixture

	err = json.Unmarshal(raw, &fixture)
	if err != nil {
		return nil, apperr.Wrap(err, "json.Unmarshal")
	}

	data.manifest.Customers, data.manifest.Units, data.manifest.Jobs = cfg.Customers, len(
		data.units,
	), len(
		data.jobs,
	)

	return &Result{Fixture: &fixture, Manifest: data.manifest}, nil
}

func org(id, kind, name string) map[string]any {
	return map[string]any{
		keyID:       id,
		keyTenantID: tenant,
		keyKind:     kind,
		keyName:     name,
		keyStatus:   "active",
	}
}

// stamp is the time `daysAgo` days before now (after it when negative).
func (b *builder) stamp(daysAgo int) string {
	return b.cfg.Now.AddDate(0, 0, -daysAgo).UTC().Format(time.RFC3339)
}

// customer adds customer i with a property, two spaces, its units, one contract, three invoices and its jobs.
func (b *builder) customer(i int) {
	n := i + 1
	orgID, custID, propID := fmt.Sprintf(
		"load-org-c-%04d",
		n,
	), fmt.Sprintf(
		"load-cust-%04d",
		n,
	), fmt.Sprintf(
		"load-prop-%04d",
		n,
	)
	name := fmt.Sprintf("Load Customer %04d", n)
	b.orgs = append(b.orgs, org(orgID, "customer", name))
	b.customers = append(b.customers, map[string]any{
		keyID:            custID,
		keyTenantID:      tenant,
		"organizationId": orgID,
		keyName:          name,
		keyStatus:        "active",
		"serviceProfile": "rto",
	})
	b.properties = append(b.properties, map[string]any{
		keyID:            propID,
		keyCustomerOrgID: orgID,
		keyKind:          "office",
		keyName:          name + " office",
		"address": fmt.Sprintf(
			"%d Load Street, Kuala Lumpur (fictional)",
			n,
		),
		"accessInstructions": nil,
		"archived":           false,
	})
	floor, room := fmt.Sprintf("load-floor-%04d", n), fmt.Sprintf("load-room-%04d", n)
	b.spaces = append(
		b.spaces,
		map[string]any{
			keyID:           floor,
			keyPropertyID:   propID,
			"parentSpaceId": nil,
			keyKind:         "floor",
			keyName:         "1F",
		},
		map[string]any{
			keyID:           room,
			keyPropertyID:   propID,
			"parentSpaceId": floor,
			keyKind:         "room",
			keyName:         "Open office",
		},
	)

	unitIDs := b.customerUnitRecords(i, orgID, propID, room)
	b.customerUnits[i] = unitIDs
	b.contractAndInvoices(n, custID, orgID, unitIDs)
	b.jobsOfCustomer(i, n)
}

// customerUnitRecords adds the customer's units and answers their fixture IDs.
func (b *builder) customerUnitRecords(i int, orgID, propID, room string) []string {
	unitIDs := make([]string, 0, b.cfg.UnitsPerCustomer)

	for unit := range b.cfg.UnitsPerCustomer {
		id := fmt.Sprintf("load-unit-%04d-%d", i+1, unit+1)
		unitIDs = append(unitIDs, id)
		b.units = append(b.units, map[string]any{
			keyID:            id,
			keyVersion:       1,
			keyCustomerOrgID: orgID,
			keyPropertyID:    propID,
			"spaceId":        room,
			"displayName": fmt.Sprintf(
				"AC %d-%d",
				i+1,
				unit+1,
			),
			"modelId":           model,
			"capabilityVersion": modelVersion,
			"installedAt": b.stamp(
				unitInstalledDays,
			),
			"serviceScope":        []string{"indoor", "outdoor", "electrical"},
			"archived":            false,
			"observedState":       nil,
			"observedRestriction": nil,
			"warrantyEndsAt":      nil,
		})
	}

	return unitIDs
}

// contractAndInvoices adds the customer's contract over all its units and its last three monthly invoices: the older
// two paid, the latest unpaid and not yet due.
func (b *builder) contractAndInvoices(n int, custID, orgID string, unitIDs []string) {
	contract := fmt.Sprintf("load-contract-%04d", n)
	b.contracts = append(b.contracts, map[string]any{
		keyID:            contract,
		keyVersion:       1,
		"customerId":     custID,
		keyCustomerOrgID: orgID,
		keyUnitIDs:       unitIDs,
		"planType":       "rto",
		"startAt": b.stamp(
			contractStartDays,
		),
		"endAt":               b.stamp(-contractEndDays),
		"priceMinor":          invoiceMinor,
		"currency":            "MYR",
		"restrictionEligible": false,
		"rulesVersion":        "demo-v1",
	})

	for month := range invoicesPerCustomer {
		monthsLeft := invoicesPerCustomer - month // 3, 2, 1: the period ended that many months ago
		invoice := map[string]any{
			keyID: fmt.Sprintf(
				"load-invoice-%04d-%d",
				n,
				month+1,
			),
			keyVersion:        1,
			"contractId":      contract,
			"contractVersion": 1,
			"amountMinor":     invoiceMinor,
			"currency":        "MYR",
			"period": map[string]any{
				"from": b.stamp(monthDays * monthsLeft),
				"to":   b.stamp(monthDays * (monthsLeft - 1)),
			},
			"dueAt":         b.stamp(monthDays*(monthsLeft-1) - invoiceGraceDays),
			"status":        "unpaid",
			"paymentMethod": nil,
			"paymentStatus": nil,
			"paidAt":        nil,
		}

		if month < paidInvoices {
			invoice["status"], invoice["paymentMethod"], invoice["paymentStatus"] = "paid", "card", "succeeded"
			invoice["paidAt"] = b.stamp(monthDays * (monthsLeft - 1))
		}

		b.invoices = append(b.invoices, invoice)
	}
}

// jobsOfCustomer adds the customer's maintenance requests; the first one of every other customer is offered to a
// contractor, so the partner screens have jobs to list.
func (b *builder) jobsOfCustomer(i, n int) {
	for j := range jobsPerCustomer {
		id := fmt.Sprintf("load-job-%04d-%d", n, j+1)
		slot := map[string]any{"startAt": b.stamp(-slotStartDays), "endAt": b.stamp(-slotEndDays)}
		job := map[string]any{
			keyID:                 id,
			keyVersion:            1,
			"unitId":              b.customerUnits[i][j%b.cfg.UnitsPerCustomer],
			"type":                "reactive",
			"status":              "requested",
			"symptom":             "Load test: filter check requested",
			"contactWindow":       nil,
			"requestedSlot":       slot,
			"scheduledSlot":       nil,
			"dueAt":               b.stamp(-slotEndDays),
			"contractorOrgId":     nil,
			"assignmentId":        nil,
			"alertIds":            []string{},
			"origin":              "client_request",
			"preferredSlots":      []any{slot},
			"preferenceRound":     1,
			"slotProposal":        nil,
			"partnerSlotProposal": nil,
		}

		if j == 0 && i%offerEvery == 0 {
			contractor := b.contractorOf(i)
			job["status"], job["contractorOrgId"] = "offered", contractor
			b.offers = append(b.offers, map[string]any{
				keyID: "load-offer-" + strings.TrimPrefix(
					id,
					"load-job-",
				),
				"jobId":           id,
				"contractorOrgId": contractor,
				"termsVersion":    "terms-demo-v1",
				"offeredAt":       b.stamp(1),
				"offerExpiresAt":  b.stamp(-offerExpiryDays),
				"accessValidFrom": b.stamp(
					1,
				),
				"accessValidUntil": b.stamp(-offerAccessDays),
				"decision":         nil,
				"decidedBy":        nil,
				"decidedAt":        nil,
				"declineReason":    nil,
				"candidateSlots":   []any{slot},
			})
		}

		b.jobs = append(b.jobs, job)
		b.customerJobs[i] = append(b.customerJobs[i], id)
	}
}

// account is one synthetic account before it is added.
type account struct {
	group, name, role, orgID string
	permissions              []string
	employment               *string
	scopes                   []map[string]any
	customerFixtureID        string
	units, jobs              []string
}

// accounts adds the synthetic accounts: the owners of the first customers, the contractor companies' staff and HQ.
func (b *builder) accounts() {
	for i := range b.cfg.CustomerUsers {
		n := i + 1
		orgID := fmt.Sprintf("load-org-c-%04d", n)
		b.addAccount(account{
			group:       "customer",
			name:        fmt.Sprintf("load-customer-%04d", n),
			role:        roleClient,
			orgID:       orgID,
			permissions: []string{"control.execute"},
			employment:  nil,
			scopes:      []map[string]any{{keyKind: "organization", keyID: orgID}},
			customerFixtureID: fmt.Sprintf(
				"load-cust-%04d",
				n,
			),
			units: b.customerUnits[i],
			jobs:  b.customerJobs[i],
		})
	}

	for i := range b.cfg.PartnerUsers {
		contractor := fmt.Sprintf("load-org-k-%02d", i%b.cfg.Contractors)
		b.addAccount(account{
			group: "partner",
			name:  fmt.Sprintf("load-partner-%04d", i+1),
			role:  "contractor",
			orgID: contractor,
			permissions: []string{
				"partner.accept",
				"partner.assign",
				"partner.review",
			},
			employment:        nil,
			scopes:            []map[string]any{{keyKind: "organization", keyID: contractor}},
			customerFixtureID: "",
			units:             nil,
			jobs:              b.jobsOfContractor(i % b.cfg.Contractors),
		})
	}

	for i := range b.cfg.TechnicianUsers {
		units := b.techUnits(i)
		scopes := make([]map[string]any, 0, len(units))

		for _, unit := range units {
			scopes = append(scopes, map[string]any{keyKind: "unit", keyID: unit})
		}

		b.addAccount(account{
			group: "technician",
			name:  fmt.Sprintf("load-technician-%04d", i+1),
			role:  "technician",
			orgID: fmt.Sprintf("load-org-k-%02d", i%b.cfg.Contractors),
			permissions: []string{
				"control.diagnose",
				"device.maintain",
				"alert.read",
				"alert.resolve",
			},
			employment: new(
				"external",
			),
			scopes:            scopes,
			customerFixtureID: "",
			units:             units,
			jobs:              nil,
		})
	}

	for i := range b.cfg.HQUsers {
		b.addAccount(account{
			group: "hq", name: fmt.Sprintf("load-hq-%04d", i+1), role: "admin", orgID: operator,
			permissions: hqPermissions(), employment: nil,
			scopes:            []map[string]any{{keyKind: "tenant", keyID: tenant}},
			customerFixtureID: "", units: nil, jobs: nil,
		})
	}
}

// techUnits are the units a technician's access names: the units of four customers.
func (b *builder) techUnits(i int) []string {
	var units []string

	for k := range techCustomers {
		units = append(units, b.customerUnits[(i*techCustomers+k)%b.cfg.Customers]...)
	}

	return units
}

// jobsOfContractor are the jobs offered to contractor company k.
func (b *builder) jobsOfContractor(k int) []string {
	var jobs []string

	for i := 0; i < b.cfg.Customers; i += offerEvery {
		if i%b.cfg.Contractors == k {
			jobs = append(jobs, b.customerJobs[i][0])
		}
	}

	return jobs
}

// addAccount adds one actor and its manifest entry.
func (b *builder) addAccount(entry account) {
	validUntil := any(nil)
	if entry.employment != nil {
		validUntil = farFuture // an external technician's access must end (FR-A03)
	}

	var clientRole any
	if entry.role == roleClient {
		clientRole = "owner"
	}

	b.actors = append(b.actors, map[string]any{
		"membershipId":   entry.name,
		"userId":         "user-" + entry.name,
		"role":           entry.role,
		"permissions":    entry.permissions,
		"tenantId":       tenant,
		"validFrom":      b.stamp(1),
		"validUntil":     validUntil,
		"organizationId": entry.orgID,
		"scopeVersion":   1,
		"employment":     entry.employment,
		"clientRole":     clientRole,
		"displayName":    entry.name,
		"qualifications": []any{},
		"scopes":         entry.scopes,
	})

	user := User{
		Username: entry.name, Password: b.cfg.Password, Group: entry.group, Role: entry.role,
		UserID: seed.ID("user-" + entry.name).String(), MembershipID: seed.ID(entry.name).String(),
		TenantID: seed.ID(tenant).String(), OrgID: seed.ID(entry.orgID).String(), CustomerID: "",
		UnitIDs: ids(entry.units), JobIDs: ids(entry.jobs),
	}
	if entry.customerFixtureID != "" {
		user.CustomerID = seed.ID(entry.customerFixtureID).String()
	}

	b.manifest.Users = append(b.manifest.Users, user)

	// the customer overview reads the location consent, which the demo seed gives its customers
	if entry.role == roleClient {
		b.consents = append(b.consents, map[string]any{
			keyID:       "consent-" + entry.name + "-location",
			keyTenantID: tenant,
			keyVersion:  1,
			"createdAt": b.stamp(seedAgeDays),
			"updatedAt": b.stamp(
				seedAgeDays,
			),
			"membershipId": entry.name,
			"purpose":      "location_automation",
			"granted":      false,
			"grantedAt":    nil,
			"revokedAt":    nil,
		})
	}
}

// ids turns fixture IDs into the UUIDs the database holds.
func ids(fixtureIDs []string) []string {
	out := make([]string, 0, len(fixtureIDs))
	for _, id := range fixtureIDs {
		out = append(out, seed.ID(id).String())
	}

	return out
}
