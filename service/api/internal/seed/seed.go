// Package seed loads docs/04-agentic-sdlc/fixture-contract.json (actors + demoSeed) into PostgreSQL.
// Fixture string IDs map to deterministic UUIDv5 values (database design §3), so tests and the web demo can
// compute the database ID of any fixture ID with ID().
package seed

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/modules/billing"
	"github.com/pradita/ac-project/service/api/internal/modules/monitoring"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

var (
	// errUnknownParents stops the seed when spaces name parents the fixture does not have.
	errUnknownParents = errors.New("spaces with unknown parents")
	// errPerUnitEntry stops the seed at a restriction's per-unit state that is not an object.
	errPerUnitEntry = errors.New("a perUnit entry is not an object")
	// errNoTarget stops the seed at a notification whose target has no ID.
	errNoTarget = errors.New("the target has no id")
)

// namespace is the UUIDv5 namespace of the fixture IDs.
const namespace = "6f1c3a52-6d0b-5c1e-9a57-0d1b7a4c2e10"

// Namespace is the UUIDv5 namespace of the fixture IDs.
func Namespace() uuid.UUID { return uuid.MustParse(namespace) }

// ID returns the database UUID of a fixture ID.
func ID(fixtureID string) uuid.UUID { return uuid.NewSHA1(Namespace(), []byte(fixtureID)) }

// Fixture is the subset of fixture-contract.json the loader uses.
type Fixture struct {
	Clock         time.Time `json:"clock"`
	SeedCreatedAt time.Time `json:"seedCreatedAt"`
	Actors        []struct {
		MembershipID   string     `json:"membershipId"`
		UserID         string     `json:"userId"`
		Role           string     `json:"role"`
		Permissions    []string   `json:"permissions"`
		TenantID       string     `json:"tenantId"`
		ValidFrom      time.Time  `json:"validFrom"`
		ValidUntil     *time.Time `json:"validUntil"`
		OrganizationID string     `json:"organizationId"`
		ScopeVersion   int        `json:"scopeVersion"`
		Employment     *string    `json:"employment"`
		ClientRole     *string    `json:"clientRole"`
		DisplayName    string     `json:"displayName"`
		Qualifications []struct {
			Code       string     `json:"code"`
			ValidFrom  time.Time  `json:"validFrom"`
			ValidUntil time.Time  `json:"validUntil"`
			RevokedAt  *time.Time `json:"revokedAt"`
		} `json:"qualifications"`
		Scopes []struct {
			Kind string `json:"kind"`
			ID   string `json:"id"`
		} `json:"scopes"`
	} `json:"actors"`
	DemoSeed struct {
		Factors []struct {
			ID        string    `json:"id"`
			TenantID  string    `json:"tenantId"`
			Version   int       `json:"version"`
			CreatedAt time.Time `json:"createdAt"`
			Region    string    `json:"region"`
			Year      int       `json:"year"`
			//nolint:tagliatelle // the fixture's key, as the contract's EmissionFactor names it (CO2)
			KgCO2ePerKWh float64 `json:"kgCO2ePerKWh"`
			Source       string  `json:"source"`
		} `json:"factors"`
		Baselines []struct {
			ID        string    `json:"id"`
			TenantID  string    `json:"tenantId"`
			Version   int       `json:"version"`
			CreatedAt time.Time `json:"createdAt"`
			UnitIDs   []string  `json:"unitIds"`
			Period    struct {
				From time.Time `json:"from"`
				To   time.Time `json:"to"`
			} `json:"period"`
			Method      string          `json:"method"`
			BaselineKWh *float64        `json:"baselineKWh"`
			Quality     json.RawMessage `json:"quality"     swaggertype:"object"`
			BoundaryID  string          `json:"boundaryId"`
			Boundary    string          `json:"boundary"`
			Assumptions string          `json:"assumptions"`
			Source      string          `json:"source"`
		} `json:"baselines"`
		Contracts     []map[string]any `json:"contracts"`
		Invoices      []map[string]any `json:"invoices"`
		Restrictions  []map[string]any `json:"restrictions"`
		Commands      []map[string]any `json:"commands"`
		Alerts        []map[string]any `json:"alerts"`
		Notifications []map[string]any `json:"notifications"`
		Jobs          []map[string]any `json:"jobs"`
		Offers        []map[string]any `json:"offers"`
		Assignments   []map[string]any `json:"assignments"`
		RateCards     []map[string]any `json:"rateCards"`
		// FilterCleanings are customers' own cleanings (FR-C18): Filter care counts from the latest one (IR316)
		FilterCleanings []map[string]any `json:"filterCleanings"`
		Consents        []struct {
			ID           string     `json:"id"`
			TenantID     string     `json:"tenantId"`
			MembershipID string     `json:"membershipId"`
			Purpose      string     `json:"purpose"`
			Granted      bool       `json:"granted"`
			GrantedAt    *time.Time `json:"grantedAt"`
			RevokedAt    *time.Time `json:"revokedAt"`
			CreatedAt    time.Time  `json:"createdAt"`
		} `json:"consents"`
		Tenants       []string `json:"tenants"`
		Organizations []struct {
			ID       string `json:"id"`
			TenantID string `json:"tenantId"`
			Kind     string `json:"kind"`
			Name     string `json:"name"`
			Status   string `json:"status"`
		} `json:"organizations"`
		Capabilities []struct {
			ID                 string          `json:"id"`
			TenantID           string          `json:"tenantId"`
			Version            int             `json:"version"`
			Manufacturer       string          `json:"manufacturer"`
			Model              string          `json:"model"`
			Control            bool            `json:"control"`
			ModeControl        bool            `json:"modeControl"`
			FanControl         bool            `json:"fanControl"`
			Temperature        json.RawMessage `json:"temperature"        swaggertype:"object"`
			Modes              []string        `json:"modes"`
			FanLevels          []string        `json:"fanLevels"`
			Ventilation        bool            `json:"ventilation"`
			VentilationLevels  []string        `json:"ventilationLevels"`
			Sensors            json.RawMessage `json:"sensors"            swaggertype:"object"`
			FirmwareCandidates []string        `json:"firmwareCandidates"`
		} `json:"capabilities"`
		Customers []struct {
			ID             string `json:"id"`
			TenantID       string `json:"tenantId"`
			OrganizationID string `json:"organizationId"`
			Name           string `json:"name"`
			Status         string `json:"status"`
			ServiceProfile string `json:"serviceProfile"`
		} `json:"customers"`
		Properties []struct {
			ID                 string  `json:"id"`
			CustomerOrgID      string  `json:"customerOrgId"`
			Kind               string  `json:"kind"`
			Name               string  `json:"name"`
			Address            *string `json:"address"`
			AccessInstructions *string `json:"accessInstructions"`
			Archived           bool    `json:"archived"`
		} `json:"properties"`
		Spaces []struct {
			ID            string  `json:"id"`
			PropertyID    string  `json:"propertyId"`
			ParentSpaceID *string `json:"parentSpaceId"`
			Kind          string  `json:"kind"`
			Name          string  `json:"name"`
		} `json:"spaces"`
		Units []struct {
			ID                  string          `json:"id"`
			Version             int             `json:"version"`
			CustomerOrgID       string          `json:"customerOrgId"`
			PropertyID          string          `json:"propertyId"`
			SpaceID             *string         `json:"spaceId"`
			DisplayName         string          `json:"displayName"`
			ModelID             string          `json:"modelId"`
			CapabilityVersion   int             `json:"capabilityVersion"`
			InstalledAt         *time.Time      `json:"installedAt"`
			WarrantyEndsAt      *time.Time      `json:"warrantyEndsAt"`
			ServiceScope        []string        `json:"serviceScope"`
			Archived            bool            `json:"archived"`
			ObservedState       json.RawMessage `json:"observedState"       swaggertype:"object"`
			ObservedRestriction json.RawMessage `json:"observedRestriction" swaggertype:"object"`
		} `json:"units"`
		Devices []struct {
			ID                    string     `json:"id"`
			UnitID                *string    `json:"unitId"`
			BindingID             *string    `json:"bindingId"`
			Serial                string     `json:"serial"`
			Connection            string     `json:"connection"`
			LastSeenAt            *time.Time `json:"lastSeenAt"`
			FirmwareVersion       string     `json:"firmwareVersion"`
			PowerSignal           string     `json:"powerSignal"`
			Tamper                string     `json:"tamper"`
			TargetUnitID          string     `json:"targetUnitId"`
			CreatedByMembershipID string     `json:"createdByMembershipId"`
			Sensors               []struct {
				ID                string     `json:"id"`
				Metric            string     `json:"metric"`
				Unit              string     `json:"unit"`
				StaleAfterSeconds int        `json:"staleAfterSeconds"`
				BoundaryID        *string    `json:"boundaryId"`
				CalibratedAt      *time.Time `json:"calibratedAt"`
			} `json:"sensors"`
		} `json:"devices"`
		Measurements []struct {
			ID         string    `json:"id"`
			UnitID     string    `json:"unitId"`
			SensorID   string    `json:"sensorId"`
			Metric     string    `json:"metric"`
			Value      *float64  `json:"value"`
			Unit       string    `json:"unit"`
			ObservedAt time.Time `json:"observedAt"`
			ReceivedAt time.Time `json:"receivedAt"`
			Origin     string    `json:"origin"`
			Quality    string    `json:"quality"`
			Sequence   int64     `json:"sequence"`
			BoundaryID *string   `json:"boundaryId"`
		} `json:"measurements"`
		AllergenObservations []struct {
			ID           string     `json:"id"`
			UnitID       string     `json:"unitId"`
			Availability string     `json:"availability"`
			Substance    *string    `json:"substance"`
			Value        *float64   `json:"value"`
			Unit         *string    `json:"unit"`
			SourceLabel  *string    `json:"sourceLabel"`
			ObservedAt   *time.Time `json:"observedAt"`
			EvidenceText *string    `json:"evidenceText"`
			CreatedAt    time.Time  `json:"createdAt"`
		} `json:"allergenObservations"`
	} `json:"demoSeed"`
}

// Load reads a fixture file.
func Load(path string) (*Fixture, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, apperr.Wrap(err, "os.ReadFile")
	}

	var fixture Fixture

	unmarshalErr := json.Unmarshal(data, &fixture)
	if unmarshalErr != nil {
		return nil, fmt.Errorf("fixture: %w", unmarshalErr)
	}

	return &fixture, nil
}

func tenantOfOrg(f *Fixture) map[string]string {
	m := map[string]string{}
	for _, o := range f.DemoSeed.Organizations {
		m[o.ID] = o.TenantID
	}

	return m
}

// Apply inserts the fixture inside tx. It must run as the schema owner (seeding bypasses row-level security)
// and is idempotent: rows that already exist are left unchanged.
func Apply(ctx context.Context, txn pgx.Tx, fixture *Fixture) error {
	exec := func(sql string, args ...any) error {
		_, err := txn.Exec(ctx, sql, args...)

		return apperr.Wrap(err, "exec (Apply)")
	}
	// IR91 item 2: a row without createdAt / updatedAt gets fixture.seedCreatedAt. The columns default to
	// platform.app_now(), so the transaction's app.now is that time while the fixture is applied; otherwise the
	// seed would carry the wall clock, a date after the demo clock (IR318: "customer since Oct 2026").
	if !fixture.SeedCreatedAt.IsZero() {
		err := exec(
			`SELECT set_config('app.now', $1, true)`,
			fixture.SeedCreatedAt.UTC().Format(time.RFC3339Nano),
		)
		if err != nil {
			return err
		}

		defer func() { _, _ = txn.Exec(ctx, `SELECT set_config('app.now', '', true)`) }()
	}

	run := newSeeding(exec, fixture)
	// the records a row names come before it; the business records (IR161) follow the demo's places and devices, and
	// the default alert policies come last
	for _, step := range []func() error{
		run.tenants, run.organizations, run.actors, run.capabilities, run.customers, run.properties, run.spaces,
		run.units, run.devices, run.measurements, run.allergens, run.factors, run.baselines, run.consents,
		run.companyUsers, run.contracts, run.invoices, run.restrictions, run.commands, run.alerts, run.notifications,
		run.jobs, run.offers, run.assignments, run.rateCards, run.filterCleanings, run.jobEvents, run.policies,
	} {
		err := step()
		if err != nil {
			return err
		}
	}

	return nil
}

// execSQL runs one statement of the seed.
type execSQL func(sql string, args ...any) error

// seeding is one run of Apply: the statement runner, the fixture, HQ's operator membership (the author of the
// factors, baselines and contracts), and by fixture ID the tenant of each organization, property, unit, customer,
// contract, job and membership, the customer organization of each unit and the customer of each contract. The business
// records take their tenants from the customers and units they name (IR161).
type seeding struct {
	exec    execSQL
	fixture *Fixture
	hq      uuid.UUID

	orgTenant, propertyTenant, unitTenant, unitOrg, memberTenant map[string]string
	customerTenant, contractTenant, contractCustomer, jobTenant  map[string]string
}

// newSeeding prepares a run of Apply on the fixture.
func newSeeding(exec execSQL, fixture *Fixture) *seeding {
	run := &seeding{
		exec:             exec,
		fixture:          fixture,
		hq:               ID("hq-operator"),
		orgTenant:        tenantOfOrg(fixture),
		propertyTenant:   map[string]string{},
		unitTenant:       map[string]string{},
		unitOrg:          map[string]string{},
		memberTenant:     map[string]string{},
		customerTenant:   map[string]string{},
		contractTenant:   map[string]string{},
		contractCustomer: map[string]string{},
		jobTenant:        map[string]string{},
	}

	for _, property := range fixture.DemoSeed.Properties {
		run.propertyTenant[property.ID] = run.orgTenant[property.CustomerOrgID]
	}

	for _, unit := range fixture.DemoSeed.Units {
		run.unitTenant[unit.ID], run.unitOrg[unit.ID] = run.orgTenant[unit.CustomerOrgID], unit.CustomerOrgID
	}

	for _, actor := range fixture.Actors {
		run.memberTenant[actor.MembershipID] = actor.TenantID
	}

	for _, customer := range fixture.DemoSeed.Customers {
		run.customerTenant[customer.ID] = customer.TenantID
	}

	for _, contract := range fixture.DemoSeed.Contracts {
		id := str(contract, "id")
		run.contractTenant[id] = run.customerTenant[str(contract, "customerId")]
		run.contractCustomer[id] = str(contract, "customerId")
	}

	for _, job := range fixture.DemoSeed.Jobs {
		run.jobTenant[str(job, "id")] = run.unitTenant[str(job, "unitId")]
	}

	return run
}

// tenants inserts the fixture's tenants.
func (s *seeding) tenants() error {
	for _, tenant := range s.fixture.DemoSeed.Tenants {
		err := s.exec(
			`INSERT INTO platform.tenants (id, name) VALUES ($1,$2) ON CONFLICT DO NOTHING`,
			ID(tenant),
			tenant,
		)
		if err != nil {
			return fmt.Errorf("tenant %s: %w", tenant, err)
		}
	}

	return nil
}

// organizations inserts the organizations; the HQ and admin kinds are the operator's.
func (s *seeding) organizations() error {
	for _, org := range s.fixture.DemoSeed.Organizations {
		kind := org.Kind
		if kind == "hq" || kind == "admin" {
			kind = "operator"
		}

		err := s.exec(
			`INSERT INTO identity.organizations (id, tenant_id, name, kind, status) VALUES ($1,$2,$3,$4,$5) ON CONFLICT DO
				NOTHING`,
			ID(org.ID),
			ID(org.TenantID),
			org.Name,
			kind,
			org.Status,
		)
		if err != nil {
			return fmt.Errorf("organization %s: %w", org.ID, err)
		}
	}

	return nil
}

// actors inserts each actor's person and membership with its permissions, qualification grants and scopes.
func (s *seeding) actors() error {
	for _, actor := range s.fixture.Actors {
		// the realm'scope account of the actor names this membership in its claims (IR462)
		err := s.exec(
			`INSERT INTO identity.users (id, email, display_name, sign_in_membership_id) VALUES ($1,$2,$3,$4) ON CONFLICT DO
				NOTHING`,
			ID(
				actor.UserID,
			),
			actor.MembershipID+"@ac.local",
			actor.DisplayName,
			ID(actor.MembershipID),
		)
		if err != nil {
			return fmt.Errorf("user %s: %w", actor.UserID, err)
		}

		err = s.exec(
			`INSERT INTO identity.memberships (id, tenant_id, user_id, organization_id, role, employment, client_role,
				scope_version, valid_from, valid_until)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT DO NOTHING`,
			ID(
				actor.MembershipID,
			),
			ID(actor.TenantID),
			ID(actor.UserID),
			ID(actor.OrganizationID),
			actor.Role,
			actor.Employment,
			actor.ClientRole,
			max(actor.ScopeVersion, 1),
			actor.ValidFrom,
			actor.ValidUntil,
		)
		if err != nil {
			return fmt.Errorf("membership %s: %w", actor.MembershipID, err)
		}

		for _, permission := range actor.Permissions {
			err := s.exec(
				`INSERT INTO identity.membership_permissions (tenant_id, membership_id, permission) VALUES ($1,$2,$3) ON CONFLICT DO
					NOTHING`,
				ID(actor.TenantID),
				ID(actor.MembershipID),
				permission,
			)
			if err != nil {
				return fmt.Errorf("permission %s/%s: %w", actor.MembershipID, permission, err)
			}
		}

		for _, qualification := range actor.Qualifications { // one grant per membership and code (UUIDv5 of both)
			err := s.exec(
				`INSERT INTO identity.qualification_grants (id, tenant_id, membership_id, code, valid_from, valid_until, revoked_at)
				VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT DO NOTHING`,
				ID("grant-"+actor.MembershipID+"-"+qualification.Code),
				ID(actor.TenantID),
				ID(actor.MembershipID),
				qualification.Code,
				qualification.ValidFrom,
				qualification.ValidUntil,
				qualification.RevokedAt,
			)
			if err != nil {
				return fmt.Errorf("grant %s/%s: %w", actor.MembershipID, qualification.Code, err)
			}
		}

		for _, scope := range actor.Scopes {
			err := s.exec(
				`INSERT INTO identity.membership_scopes (tenant_id, membership_id, kind, ref_id) VALUES ($1,$2,$3,$4) ON CONFLICT DO
					NOTHING`,
				ID(actor.TenantID),
				ID(actor.MembershipID),
				scope.Kind,
				ID(scope.ID),
			)
			if err != nil {
				return fmt.Errorf("scope %s: %w", actor.MembershipID, err)
			}
		}
	}

	return nil
}

// capabilities inserts the models' capabilities, each the current version.
func (s *seeding) capabilities() error {
	for _, capability := range s.fixture.DemoSeed.Capabilities {
		var temp any
		if len(capability.Temperature) > 0 && string(capability.Temperature) != jsonNull {
			temp = []byte(capability.Temperature)
		}

		sensors := []byte(capability.Sensors)
		if len(sensors) == 0 {
			sensors = []byte("[]")
		}

		err := s.exec(
			`INSERT INTO devices.capabilities (id, version, tenant_id, manufacturer, model, control, mode_control, fan_control,
				temperature,
			modes, fan_levels, ventilation, ventilation_levels, sensors, firmware_candidates, is_current)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,true) ON CONFLICT DO NOTHING`,
			ID(
				capability.ID,
			),
			max(capability.Version, 1),
			ID(capability.TenantID),
			capability.Manufacturer,
			capability.Model,
			capability.Control,
			capability.ModeControl,
			capability.FanControl,
			temp,
			nonNil(
				capability.Modes,
			),
			nonNil(capability.FanLevels),
			capability.Ventilation,
			nonNil(capability.VentilationLevels),
			sensors,
			nonNil(capability.FirmwareCandidates),
		)
		if err != nil {
			return fmt.Errorf("capability %s: %w", capability.ID, err)
		}
	}

	return nil
}

// customers inserts the customers.
func (s *seeding) customers() error {
	for _, customer := range s.fixture.DemoSeed.Customers {
		err := s.exec(
			`INSERT INTO assets.customers (id, tenant_id, organization_id, name, status, service_profile) VALUES ($1,$2,$3,$4,$5,
				$6) ON CONFLICT DO NOTHING`,
			ID(
				customer.ID,
			),
			ID(customer.TenantID),
			ID(customer.OrganizationID),
			customer.Name,
			customer.Status,
			customer.ServiceProfile,
		)
		if err != nil {
			return fmt.Errorf("customer %s: %w", customer.ID, err)
		}
	}

	return nil
}

// properties inserts the properties in the tenants of their customer organizations.
func (s *seeding) properties() error {
	for _, property := range s.fixture.DemoSeed.Properties {
		tenant := s.orgTenant[property.CustomerOrgID]

		err := s.exec(
			`INSERT INTO assets.properties (id, tenant_id, customer_org_id, kind, name, address, access_instructions, archived)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT DO NOTHING`,
			ID(
				property.ID,
			),
			ID(tenant),
			ID(property.CustomerOrgID),
			property.Kind,
			property.Name,
			property.Address,
			property.AccessInstructions,
			property.Archived,
		)
		if err != nil {
			return fmt.Errorf("property %s: %w", property.ID, err)
		}
	}

	return nil
}

// spaces inserts the spaces, parents before children.
func (s *seeding) spaces() error {
	// parents before children
	pending := s.fixture.DemoSeed.Spaces
	done := map[string]bool{}

	for len(pending) > 0 {
		next := pending[:0:0]
		for _, space := range pending {
			if space.ParentSpaceID != nil && !done[*space.ParentSpaceID] {
				next = append(next, space)

				continue
			}

			var parent *uuid.UUID

			if space.ParentSpaceID != nil {
				parentID := ID(*space.ParentSpaceID)
				parent = &parentID
			}

			err := s.exec(
				`INSERT INTO assets.spaces (id, tenant_id, property_id, parent_space_id, kind, name) VALUES ($1,$2,$3,$4,$5,$6) ON
					CONFLICT DO NOTHING`,
				ID(
					space.ID,
				),
				ID(s.propertyTenant[space.PropertyID]),
				ID(space.PropertyID),
				parent,
				space.Kind,
				space.Name,
			)
			if err != nil {
				return fmt.Errorf("space %s: %w", space.ID, err)
			}

			done[space.ID] = true
		}

		if len(next) == len(pending) {
			return fmt.Errorf("%w: %d", errUnknownParents, len(next))
		}

		pending = next
	}

	return nil
}

// units inserts the units with their observed state, observed restriction and service scope.
func (s *seeding) units() error {
	for _, unit := range s.fixture.DemoSeed.Units {
		var space *uuid.UUID

		if unit.SpaceID != nil {
			spaceID := ID(*unit.SpaceID)
			space = &spaceID
		}

		state := []byte(unit.ObservedState)
		if len(state) == 0 || string(state) == jsonNull {
			state = []byte(
				`{"power":null,"celsius":null,"mode":null,"fanLevel":null,"observedAt":null}`,
			)
		}

		var restr any
		if len(unit.ObservedRestriction) > 0 && string(unit.ObservedRestriction) != jsonNull {
			restr = []byte(unit.ObservedRestriction)
		}

		scope := unit.ServiceScope
		if len(scope) == 0 {
			scope = []string{"indoor", "outdoor", "electrical"}
		}

		err := s.exec(
			`INSERT INTO assets.units (id, tenant_id, customer_org_id, property_id, space_id, display_name, model_id,
				installed_at,
			service_scope, archived, capability_version, observed_state, observed_restriction, version, warranty_ends_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15) ON CONFLICT DO NOTHING`,
			ID(
				unit.ID,
			),
			ID(s.orgTenant[unit.CustomerOrgID]),
			ID(unit.CustomerOrgID),
			ID(unit.PropertyID),
			space,
			unit.DisplayName,
			ID(unit.ModelID),
			unit.InstalledAt,
			scope,
			unit.Archived,
			unit.CapabilityVersion,
			state,
			restr,
			max(unit.Version, 1),
			unit.WarrantyEndsAt,
		)
		if err != nil {
			return fmt.Errorf("unit %s: %w", unit.ID, err)
		}
	}

	return nil
}

// devices inserts each device with its sensors and active binding; the bound unit mirrors the device's connection.
func (s *seeding) devices() error {
	for _, device := range s.fixture.DemoSeed.Devices {
		tenant := ID(s.unitTenant[device.TargetUnitID])

		var unit, binding *uuid.UUID

		if device.UnitID != nil {
			unitID := ID(*device.UnitID)
			unit = &unitID
		}

		if device.BindingID != nil {
			bindingID := ID(*device.BindingID)
			binding = &bindingID
		}

		err := s.exec(
			`INSERT INTO devices.devices (id, tenant_id, serial, binding_id, unit_id, target_unit_id, created_by_membership_id,
				connection, last_seen_at,
			firmware_version, power_signal, tamper) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) ON CONFLICT DO NOTHING`,
			ID(
				device.ID,
			),
			tenant,
			device.Serial,
			binding,
			unit,
			ID(device.TargetUnitID),
			ID(device.CreatedByMembershipID),
			device.Connection,
			device.LastSeenAt,
			device.FirmwareVersion,
			device.PowerSignal,
			device.Tamper,
		)
		if err != nil {
			return fmt.Errorf("device %s: %w", device.ID, err)
		}

		for _, sensor := range device.Sensors {
			err := s.exec(
				`INSERT INTO devices.sensors (id, tenant_id, device_id, metric, unit, boundary_id, stale_after_seconds,
					calibrated_at)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT DO NOTHING`,
				ID(sensor.ID),
				tenant,
				ID(device.ID),
				sensor.Metric,
				sensor.Unit,
				sensor.BoundaryID,
				sensor.StaleAfterSeconds,
				sensor.CalibratedAt,
			)
			if err != nil {
				return fmt.Errorf("sensor %s: %w", sensor.ID, err)
			}
		}

		if unit == nil || binding == nil {
			continue
		}

		err = s.exec(
			`INSERT INTO devices.device_bindings (id, tenant_id, device_id, unit_id, customer_org_id, bound_at, reason,
				actor_membership_id)
			SELECT $1,$2,$3,$4,u.customer_org_id,$5,'demo seed',$6 FROM assets.units u WHERE u.id = $4 ON CONFLICT DO NOTHING`,
			*binding,
			tenant,
			ID(device.ID),
			*unit,
			s.fixture.SeedCreatedAt,
			ID(device.CreatedByMembershipID),
		)
		if err != nil {
			return fmt.Errorf("binding %s: %w", device.ID, err)
		}

		err = s.exec(
			`UPDATE assets.units SET connection = $2, last_seen_at = $3 WHERE id = $1`,
			*unit,
			device.Connection,
			device.LastSeenAt,
		)
		if err != nil {
			return fmt.Errorf("unit connection %s: %w", device.ID, err)
		}
	}

	return nil
}

// measurements inserts the readings; a measurement's ID is its event ID (IR121).
func (s *seeding) measurements() error {
	for _, measurement := range s.fixture.DemoSeed.Measurements {
		err := s.exec(
			`INSERT INTO monitoring.measurements (tenant_id, unit_id, sensor_id, metric, observed_at, sequence, value, unit,
				origin, quality, boundary_id, received_at, event_id)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) ON CONFLICT DO NOTHING`,
			ID(
				s.unitTenant[measurement.UnitID],
			),
			ID(measurement.UnitID),
			ID(measurement.SensorID),
			measurement.Metric,
			measurement.ObservedAt,
			measurement.Sequence,
			measurement.Value,
			measurement.Unit,
			measurement.Origin,
			measurement.Quality,
			measurement.BoundaryID,
			measurement.ReceivedAt,
			ID(measurement.ID),
		)
		if err != nil {
			return fmt.Errorf("measurement %s: %w", measurement.ID, err)
		}
	}

	return nil
}

// allergens inserts the allergen observations.
func (s *seeding) allergens() error {
	for _, observation := range s.fixture.DemoSeed.AllergenObservations {
		err := s.exec(
			`INSERT INTO monitoring.allergen_observations (id, tenant_id, unit_id, availability, substance, value, unit,
				source_label, observed_at, evidence_text, created_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) ON CONFLICT DO NOTHING`,
			ID(
				observation.ID,
			),
			ID(s.unitTenant[observation.UnitID]),
			ID(observation.UnitID),
			observation.Availability,
			observation.Substance,
			observation.Value,
			observation.Unit,
			observation.SourceLabel,
			observation.ObservedAt,
			observation.EvidenceText,
			observation.CreatedAt,
		)
		if err != nil {
			return fmt.Errorf("allergen %s: %w", observation.ID, err)
		}
	}

	return nil
}

// factors inserts the emission factors, created by HQ.
func (s *seeding) factors() error {
	for _, factor := range s.fixture.DemoSeed.Factors {
		err := s.exec(
			`INSERT INTO energy.emission_factors (id, version, tenant_id, region, year, kg_co2e_per_kwh, source, created_by,
				created_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT DO NOTHING`,
			ID(factor.ID),
			factor.Version,
			ID(factor.TenantID),
			factor.Region,
			factor.Year,
			factor.KgCO2ePerKWh,
			factor.Source,
			s.hq,
			factor.CreatedAt,
		)
		if err != nil {
			return fmt.Errorf("factor %s: %w", factor.ID, err)
		}
	}

	return nil
}

// baselines inserts the energy baselines, created by HQ.
func (s *seeding) baselines() error {
	for _, baseline := range s.fixture.DemoSeed.Baselines {
		units := make([]uuid.UUID, len(baseline.UnitIDs))
		for i, unitID := range baseline.UnitIDs {
			units[i] = ID(unitID)
		}

		err := s.exec(
			`INSERT INTO energy.baselines (id, version, tenant_id, unit_ids, period, method, baseline_kwh, quality, boundary_id,
				boundary, assumptions, source, created_by, created_at)
			VALUES ($1,$2,$3,$4,tstzrange($5,$6),$7,$8,$9,$10,$11,$12,$13,$14,$15) ON CONFLICT DO NOTHING`,
			ID(baseline.ID),
			baseline.Version,
			ID(baseline.TenantID),
			units,
			baseline.Period.From,
			baseline.Period.To,
			baseline.Method,
			baseline.BaselineKWh,
			[]byte(baseline.Quality),
			baseline.BoundaryID,
			baseline.Boundary,
			baseline.Assumptions,
			baseline.Source,
			s.hq,
			baseline.CreatedAt,
		)
		if err != nil {
			return fmt.Errorf("baseline %s: %w", baseline.ID, err)
		}
	}

	return nil
}

// consents inserts the initial location consents (IR84).
func (s *seeding) consents() error {
	for _, consent := range s.fixture.DemoSeed.Consents {
		err := s.exec(
			`INSERT INTO identity.consents (id, tenant_id, membership_id, purpose, granted, granted_at, revoked_at, created_at,
				updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$8) ON CONFLICT DO NOTHING`,
			ID(consent.ID),
			ID(consent.TenantID),
			ID(consent.MembershipID),
			consent.Purpose,
			consent.Granted,
			consent.GrantedAt,
			consent.RevokedAt,
			consent.CreatedAt,
		)
		if err != nil {
			return fmt.Errorf("consent %s: %w", consent.ID, err)
		}
	}

	return nil
}

// companyUsers inserts the customers' and the contractor companies' user lists of the seed accounts.
func (s *seeding) companyUsers() error {
	// client_users rows for the seed client memberships (owners, active; IR144)
	err := s.exec(
		`INSERT INTO identity.client_users (id, tenant_id, customer_id, membership_id, email, display_name, client_role,
			status, invited_at, invited_by_membership_id, created_at, updated_at)
		SELECT m.id, m.tenant_id, cu.id, m.id, u.email, u.display_name, COALESCE(m.client_role, 'owner'), 'active', $1, m.id,
		$1, $1
		FROM identity.memberships m JOIN identity.users u ON u.id = m.user_id JOIN assets.customers cu ON cu.organization_id =
		m.organization_id
		WHERE m.role = 'client' AND m.id = ANY($2) ON CONFLICT DO NOTHING`,
		s.fixture.SeedCreatedAt,
		clientMemberships(s.fixture),
	)
	if err != nil {
		return fmt.Errorf("client users: %w", err)
	}
	// partner_users rows for the seed contractor company accounts (contractors and their technicians, active; FR-P11)
	err = s.exec(
		`INSERT INTO identity.partner_users (id, tenant_id, organization_id, membership_id, email, display_name, partner_role,
			status, invited_at, invited_by_membership_id, created_at, updated_at)
		SELECT m.id, m.tenant_id, m.organization_id, m.id, u.email, u.display_name, m.role, 'active', $1, m.id, $1, $1
		FROM identity.memberships m JOIN identity.users u ON u.id = m.user_id JOIN identity.organizations o ON o.id =
		m.organization_id
		WHERE o.kind = 'contractor' AND m.role IN ('contractor','technician') ON CONFLICT DO NOTHING`,
		s.fixture.SeedCreatedAt,
	)
	if err != nil {
		return fmt.Errorf("partner users: %w", err)
	}

	return nil
}

func clientMemberships(f *Fixture) []uuid.UUID {
	out := []uuid.UUID{}

	for _, a := range f.Actors {
		if a.Role == "client" {
			out = append(out, ID(a.MembershipID))
		}
	}

	return out
}

// policies seeds the IR120 default policy of each tenant (owned by its first HQ actor) and the customer-actor demo
// setting that switches the AC offline rule off.
func (s *seeding) policies() error {
	rules, err := json.Marshal(monitoring.DefaultRules())
	if err != nil {
		return apperr.Wrap(err, "json.Marshal")
	}

	done := map[string]bool{}
	for _, actor := range s.fixture.Actors {
		if actor.Role != "admin" || done[actor.TenantID] {
			continue
		}

		done[actor.TenantID] = true

		id := DefaultPolicyID(actor.TenantID)

		err := s.exec(
			`INSERT INTO monitoring.alert_policies (id, tenant_id, kind, customer_id, name, owner_membership_id,
				created_by_user_id, timezone, enabled, priority, rules)
			VALUES ($1,$2,'default_alert',NULL,'Default policy',$3,$4,'Asia/Kuala_Lumpur',true,0,$5) ON CONFLICT DO NOTHING`,
			id,
			ID(actor.TenantID),
			ID(actor.MembershipID),
			ID(actor.UserID),
			rules,
		)
		if err != nil {
			return fmt.Errorf("default policy %s: %w", actor.TenantID, err)
		}

		if actor.TenantID == demoTenant {
			err := s.exec(
				`INSERT INTO monitoring.default_rule_settings (tenant_id, policy_id, rule_key, customer_id, enabled,
					changed_by_membership_id, reason)
				VALUES ($1,$2,'ac_offline',$3,false,$4,'Demo: customer switched the offline rule off') ON CONFLICT DO NOTHING`,
				ID(actor.TenantID),
				id,
				ID("cust-a"),
				ID(actor.MembershipID),
			)
			if err != nil {
				return fmt.Errorf("default rule setting: %w", err)
			}
		}
	}

	return nil
}

// DefaultPolicyID is the ID of a tenant's default alert policy (policy-default for tenant-a).
func DefaultPolicyID(tenant string) uuid.UUID {
	if tenant == demoTenant {
		return ID("policy-default")
	}

	return ID("policy-default-" + tenant)
}

// ---- demoSeed business records (contracts, invoices, restrictions, commands, alerts, notifications, jobs) ----

func str(m map[string]any, k string) string {
	v, _ := m[k].(string)

	return v
}

func opt(m map[string]any, k string) *string {
	if v, ok := m[k].(string); ok {
		return &v
	}

	return nil
}

func optID(m map[string]any, k string) *uuid.UUID {
	if v, ok := m[k].(string); ok {
		id := ID(v)

		return &id
	}

	return nil
}

func num(m map[string]any, k string) float64 {
	v, _ := m[k].(float64)

	return v
}

// rng is the two ends of the range object at m[k], by their keys lower and upper; nil and nil without one.
func rng(m map[string]any, k, lower, upper string) (any, any) {
	r, _ := m[k].(map[string]any)
	if r == nil {
		return nil, nil
	}

	return r[lower], r[upper]
}

// ids is the database IDs of a list of fixture IDs; an entry that is not a string is left out, as str reads it.
func ids(v any) []uuid.UUID {
	xs, _ := v.([]any)

	out := make([]uuid.UUID, 0, len(xs))
	for _, x := range xs {
		if fixtureID, ok := x.(string); ok {
			out = append(out, ID(fixtureID))
		}
	}

	return out
}

// nonNil is values, or an empty list for none: the array columns are NOT NULL.
func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}

	return values
}

// raw is v as JSON. Fixture values come from the fixture's own JSON, so they always encode again; one that did not
// would go in as SQL NULL, which the NOT NULL columns refuse, so the seed stops.
func raw(v any) []byte {
	encoded, err := json.Marshal(v)
	if err != nil {
		return nil
	}

	return encoded
}

// fixtureIDs replaces fixture IDs inside an action / policy object by their database UUIDs.
func fixtureIDs(v any, keys ...string) []byte {
	m, _ := v.(map[string]any)

	out := map[string]any{}
	maps.Copy(out, m)

	for _, k := range keys {
		if s, ok := out[k].(string); ok {
			out[k] = ID(s).String()
		}
	}

	return raw(out)
}

// The demoSeed business records (contracts, invoices, restrictions, commands, alerts, notifications, jobs) have
// deterministic IDs, so the API demo shows the fixture scenario (IR161).

// contracts inserts the contracts and the units each covers, in the tenants of their customers.
func (s *seeding) contracts() error {
	for _, contract := range s.fixture.DemoSeed.Contracts {
		tenant := s.customerTenant[str(contract, "customerId")]

		err := s.exec(
			`INSERT INTO billing.contracts (id, version, tenant_id, customer_id, customer_org_id, plan_type, term, price_minor,
				currency, restriction_eligible, rules_version, created_by, created_at)
			VALUES ($1,$2,$3,$4,$5,$6,tstzrange($7::timestamptz,$8::timestamptz),$9,$10,$11,$12,$13,$7::timestamptz) ON CONFLICT
			DO NOTHING`,
			ID(
				str(contract, "id"),
			),
			int(num(contract, "version")),
			ID(tenant),
			ID(str(contract, "customerId")),
			ID(str(contract, "customerOrgId")),
			str(contract, "planType"),
			str(contract, "startAt"),
			str(contract, "endAt"),
			int64(
				num(contract, "priceMinor"),
			),
			str(contract, "currency"),
			contract["restrictionEligible"],
			opt(contract, "rulesVersion"),
			s.hq,
		)
		if err != nil {
			return fmt.Errorf("contract %s: %w", str(contract, "id"), err)
		}

		for _, unit := range ids(contract["unitIds"]) {
			err := s.exec(
				`INSERT INTO billing.contract_units (tenant_id, contract_id, contract_version, unit_id) VALUES ($1,$2,$3,$4) ON
					CONFLICT DO NOTHING`,
				ID(tenant),
				ID(str(contract, "id")),
				int(num(contract, "version")),
				unit,
			)
			if err != nil {
				return fmt.Errorf("contract units %s: %w", str(contract, "id"), err)
			}
		}
	}

	return nil
}

// invoices inserts the invoices, numbered in fixture order by their period's start.
func (s *seeding) invoices() error {
	for idx, invoice := range s.fixture.DemoSeed.Invoices {
		tenant := s.contractTenant[str(invoice, "contractId")]

		customer := s.contractCustomer[str(invoice, "contractId")]
		periodFrom, periodTo := rng(invoice, "period", "from", "to")

		fromText, _ := periodFrom.(string)

		start, err := time.Parse(time.RFC3339, fromText)
		if err != nil {
			return fmt.Errorf("invoice %s period: %w", str(invoice, "id"), err)
		}

		exErr := s.exec(
			`INSERT INTO billing.invoices (id, tenant_id, number, contract_id, contract_version, customer_id, amount_minor,
				currency, period, due_at, status, paid_at, version)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,tstzrange($9::timestamptz,$10::timestamptz),$11,$12,$13,$14) ON CONFLICT DO NOTHING`,
			ID(
				str(invoice, "id"),
			),
			ID(tenant),
			billing.InvoiceNumber(start, idx+1),
			ID(str(invoice, "contractId")),
			int(num(invoice, "contractVersion")),
			ID(
				customer,
			),
			int64(num(invoice, "amountMinor")),
			str(invoice, "currency"),
			periodFrom,
			periodTo,
			str(invoice, "dueAt"),
			str(invoice, "status"),
			opt(invoice, "paidAt"),
			int(num(invoice, "version")),
		)
		if exErr != nil {
			return fmt.Errorf("invoice %s: %w", str(invoice, "id"), exErr)
		}
	}

	return nil
}

// restrictions inserts the restrictions with the invoices that caused them and their units.
func (s *seeding) restrictions() error {
	for _, restriction := range s.fixture.DemoSeed.Restrictions {
		tenant := s.contractTenant[str(restriction, "contractId")]

		customer := s.contractCustomer[str(restriction, "contractId")]

		err := s.exec(
			`INSERT INTO restrictions.restrictions (id, tenant_id, contract_id, contract_version, customer_id, rules_version,
				notice_at, execute_after, reason, policy, state,
			notice_notification_ids, version, created_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$7,$7)
			ON CONFLICT DO NOTHING`,
			ID(
				str(restriction, "id"),
			),
			ID(tenant),
			ID(str(restriction, "contractId")),
			int(num(restriction, "contractVersion")),
			ID(customer),
			str(restriction, "rulesVersion"),
			str(restriction, "noticeAt"),
			str(restriction, "executeAfter"),
			str(
				restriction,
				"reason",
			),
			raw(restriction["policy"]),
			str(restriction, "state"),
			ids(restriction["noticeNotificationIds"]),
			int(num(restriction, "version")),
		)
		if err != nil {
			return fmt.Errorf("restriction %s: %w", str(restriction, "id"), err)
		}

		for _, inv := range ids(restriction["causeInvoiceIds"]) {
			err := s.exec(
				`INSERT INTO restrictions.restriction_invoices (tenant_id, restriction_id, invoice_id) VALUES ($1,$2,$3) ON CONFLICT
					DO NOTHING`,
				ID(tenant),
				ID(str(restriction, "id")),
				inv,
			)
			if err != nil {
				return err
			}
		}

		err = s.restrictionUnits(restriction, tenant)
		if err != nil {
			return err
		}
	}

	return nil
}

// restrictionUnits inserts the per-unit states of a restriction; a unit where it is applied observes it.
func (s *seeding) restrictionUnits(restriction map[string]any, tenant string) error {
	entries, _ := restriction["perUnit"].([]any)
	for _, entry := range entries {
		unit, ok := entry.(map[string]any)
		if !ok {
			return fmt.Errorf("restriction %s: %w", str(restriction, "id"), errPerUnitEntry)
		}

		obs := map[string]any{
			"restrictionId": ID(str(restriction, "id")).String(),
			"rulesVersion":  str(restriction, "rulesVersion"),
			"policy":        restriction["policy"],
			keyObservedAt:   unit[keyObservedAt],
		}

		var observed any
		if str(unit, "applyState") == "applied" {
			observed = raw(obs)
		}

		err := s.exec(
			`INSERT INTO restrictions.restriction_units (tenant_id, restriction_id, unit_id, apply_state, release_state,
				apply_command_ids, release_command_ids, observed_restriction,
			observed_at, pending_reason) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT DO NOTHING`,
			ID(tenant),
			ID(str(restriction, "id")),
			ID(str(unit, "unitId")),
			str(unit, "applyState"),
			str(
				unit,
				"releaseState",
			),
			ids(unit["applyCommandIds"]),
			ids(unit["releaseCommandIds"]),
			observed,
			opt(unit, keyObservedAt),
			opt(unit, "pendingReason"),
		)
		if err != nil {
			return fmt.Errorf("restriction unit: %w", err)
		}

		if observed != nil {
			err := s.exec(
				`UPDATE assets.units SET observed_restriction = $2 WHERE id = $1`,
				ID(str(unit, "unitId")),
				observed,
			)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// commands inserts the unit commands, and billing's own record of each restriction command (IR194).
func (s *seeding) commands() error {
	for _, command := range s.fixture.DemoSeed.Commands {
		action, _ := command["action"].(map[string]any)

		err := s.exec(
			`INSERT INTO control.commands (id, tenant_id, unit_id, actor_membership_id, source, action, restriction_id, status,
				delivery, requested_at, sent_at, acknowledged_at,
			expires_at, correlation_id, created_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,'seed',$10,
			$10) ON CONFLICT DO NOTHING`,
			ID(
				str(command, "id"),
			),
			ID(s.unitTenant[str(command, "unitId")]),
			ID(str(command, "unitId")),
			ID(str(command, "actorMembershipId")),
			map[bool]string{true: "restriction", false: "ui"}[action["restrictionId"] != nil],
			fixtureIDs(
				action,
				"restrictionId",
			),
			optID(action, "restrictionId"),
			str(command, "status"),
			str(command, "delivery"),
			str(command, "requestedAt"),
			opt(command, "sentAt"),
			opt(command, "acknowledgedAt"),
			str(command, "expiresAt"),
		)
		if err != nil {
			return fmt.Errorf("command %s: %w", str(command, "id"), err)
		}

		if action["restrictionId"] != nil { // billing's own record of the restriction command (IR194)
			status := str(command, "status")
			if status == "sent" {
				status = "requested"
			}

			kind, _ := action["kind"].(string)

			err := s.exec(
				`INSERT INTO restrictions.restriction_commands (tenant_id, command_id, restriction_id, unit_id, kind, delivered,
					status, requested_at, updated_at)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$8) ON CONFLICT DO NOTHING`,
				ID(s.unitTenant[str(command, "unitId")]),
				ID(str(command, "id")),
				optID(action, "restrictionId"),
				ID(str(command, "unitId")),
				kind,
				str(command, "delivery") == "sent",
				status,
				str(command, "requestedAt"),
			)
			if err != nil {
				return fmt.Errorf("restriction command %s: %w", str(command, "id"), err)
			}
		}
	}

	return nil
}

// alerts inserts the alerts in the tenants and customer organizations of their units.
func (s *seeding) alerts() error {
	for _, alert := range s.fixture.DemoSeed.Alerts {
		unit := str(alert, "unitId")

		err := s.exec(
			`INSERT INTO monitoring.alerts (id, tenant_id, unit_id, customer_org_id, policy_id, type, severity, status,
				cause_code, evidence_kind, evidence_text, observed_at, detected_at,
			previous_alert_id, created_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$13,$13) ON
			CONFLICT DO NOTHING`,
			ID(
				str(alert, "id"),
			),
			ID(s.unitTenant[unit]),
			ID(unit),
			ID(s.unitOrg[unit]),
			optID(alert, "policyId"),
			str(alert, "type"),
			str(alert, "severity"),
			str(alert, "status"),
			str(alert, "causeCode"),
			str(
				alert,
				"evidenceKind",
			),
			str(alert, "evidenceText"),
			str(alert, keyObservedAt),
			str(alert, "detectedAt"),
			optID(alert, "previousAlertId"),
		)
		if err != nil {
			return fmt.Errorf("alert %s: %w", str(alert, "id"), err)
		}
	}

	return nil
}

// notifications inserts the notifications in their recipients' tenants, with the targets' database IDs.
func (s *seeding) notifications() error {
	for _, notification := range s.fixture.DemoSeed.Notifications {
		ref, _ := notification["target"].(map[string]any)

		targetID, ok := ref["id"].(string)
		if !ok {
			return fmt.Errorf("notification %s: %w", str(notification, "id"), errNoTarget)
		}

		target := raw(map[string]any{"kind": ref["kind"], "id": ID(targetID).String()})

		err := s.exec(
			`INSERT INTO notify.notifications (id, tenant_id, recipient_membership_id, scope_version_at_creation, type, channel,
				template_key, target, params, source_alert_id, severity,
			occurred_at, read_at, created_at) VALUES ($1,$2,$3,1,$4,$5,$6,$7,'{}',$8,$9,$10,$11,$10) ON CONFLICT DO NOTHING`,
			ID(
				str(notification, "id"),
			),
			ID(s.memberTenant[str(notification, "recipientMembershipId")]),
			ID(str(notification, "recipientMembershipId")),
			str(notification, "type"),
			str(notification, "channel"),
			str(notification, "templateKey"),
			target,
			optID(
				notification,
				"sourceAlertId",
			),
			str(notification, "severity"),
			str(notification, "occurredAt"),
			opt(notification, "readAt"),
		)
		if err != nil {
			return fmt.Errorf("notification %s: %w", str(notification, "id"), err)
		}
	}

	return nil
}

// jobs inserts the maintenance jobs with the alerts each answers.
func (s *seeding) jobs() error {
	for _, job := range s.fixture.DemoSeed.Jobs {
		unit := str(job, "unitId")
		requestedStart, requestedEnd := rng(job, "requestedSlot", "startAt", "endAt")
		scheduledStart, scheduledEnd := rng(job, "scheduledSlot", "startAt", "endAt")

		var sched any
		if scheduledStart != nil {
			sched = fmt.Sprintf("[%s,%s)", scheduledStart, scheduledEnd)
		}

		err := s.exec(
			`INSERT INTO maintenance.jobs (id, tenant_id, unit_id, customer_org_id, type, status, origin, symptom,
				contact_window, requested_slot, preferred_slots, preference_round,
			scheduled_slot, due_at, contractor_org_id, assignment_id, version, created_at, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,tstzrange($10::timestamptz,$11::timestamptz),$12,$13,$14::tstzrange,$15,$16,$17,
			$18,$19,$19) ON CONFLICT DO NOTHING`,
			ID(
				str(job, "id"),
			),
			ID(s.unitTenant[unit]),
			ID(unit),
			ID(s.unitOrg[unit]),
			str(job, "type"),
			str(job, "status"),
			str(job, "origin"),
			str(job, "symptom"),
			opt(job, "contactWindow"),
			requestedStart,
			requestedEnd,
			raw(
				job["preferredSlots"],
			),
			int(num(job, "preferenceRound")),
			sched,
			str(job, "dueAt"),
			optID(job, "contractorOrgId"),
			optID(job, "assignmentId"),
			int(num(job, "version")),
			s.fixture.SeedCreatedAt,
		)
		if err != nil {
			return fmt.Errorf("job %s: %w", str(job, "id"), err)
		}

		for _, alert := range ids(job["alertIds"]) {
			err := s.exec(
				`INSERT INTO maintenance.job_alerts (tenant_id, job_id, alert_id) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`,
				ID(s.unitTenant[unit]),
				ID(str(job, "id")),
				alert,
			)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// offers inserts the contractors' offers.
func (s *seeding) offers() error {
	for _, offer := range s.fixture.DemoSeed.Offers {
		visitStart, visitEnd := rng(offer, "visitSlot", "startAt", "endAt")

		err := s.exec(
			`INSERT INTO maintenance.offers (id, tenant_id, job_id, contractor_org_id, terms_version, visit_slot, offered_at,
				offer_expires_at, access_valid_from, access_valid_until,
			decision, decided_by, decided_at, decline_reason, created_at, updated_at)
			VALUES ($1,$2,$3,$4,$5,tstzrange($6::timestamptz,$7::timestamptz),$8,$9,$10,$11,$12,$13,$14,$15,$8,$8) ON CONFLICT DO
			NOTHING`,
			ID(
				str(offer, "id"),
			),
			ID(s.jobTenant[str(offer, "jobId")]),
			ID(str(offer, "jobId")),
			ID(str(offer, "contractorOrgId")),
			str(offer, "termsVersion"),
			visitStart,
			visitEnd,
			str(offer, "offeredAt"),
			str(offer, "offerExpiresAt"),
			str(
				offer,
				"accessValidFrom",
			),
			str(offer, "accessValidUntil"),
			opt(offer, "decision"),
			optID(offer, "decidedBy"),
			opt(offer, "decidedAt"),
			opt(offer, "declineReason"),
		)
		if err != nil {
			return fmt.Errorf("offer %s: %w", str(offer, "id"), err)
		}
	}

	return nil
}

// assignments inserts the technicians' assignments.
func (s *seeding) assignments() error {
	for _, assignment := range s.fixture.DemoSeed.Assignments {
		err := s.exec(
			`INSERT INTO maintenance.assignments (id, tenant_id, job_id, technician_membership_id, valid_from, valid_until,
				scheduled, status, reason, acknowledgement, acknowledged_at,
			cant_make_reason, created_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6,tstzrange($7::timestamptz,$8::timestamptz),$9,
			$10,$11,$12,$13,$14,$14) ON CONFLICT DO NOTHING`,
			ID(
				str(assignment, "id"),
			),
			ID(s.jobTenant[str(assignment, "jobId")]),
			ID(str(assignment, "jobId")),
			ID(str(assignment, "technicianMembershipId")),
			str(assignment, "validFrom"),
			str(assignment, "validUntil"),
			str(
				assignment,
				"scheduledStart",
			),
			str(assignment, "scheduledEnd"),
			str(assignment, "status"),
			opt(assignment, "reason"),
			str(assignment, "acknowledgement"),
			opt(assignment, "acknowledgedAt"),
			opt(assignment, "cantMakeReason"),
			str(assignment, "createdAt"),
		)
		if err != nil {
			return fmt.Errorf("assignment %s: %w", str(assignment, "id"), err)
		}
	}

	return nil
}

// rateCards inserts the contractors' rate cards, their payout prices (FR-A22, FR-P10).
func (s *seeding) rateCards() error {
	for _, card := range s.fixture.DemoSeed.RateCards {
		lines, marshalErr := json.Marshal(card["lines"])
		if marshalErr != nil {
			return apperr.Wrap(marshalErr, "json.Marshal")
		}

		err := s.exec(
			`INSERT INTO maintenance.rate_cards (id, tenant_id, contractor_org_id, effective_from, currency, lines, version,
				created_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT DO NOTHING`,
			ID(
				str(card, "id"),
			),
			ID(str(card, "tenantId")),
			ID(str(card, "contractorOrgId")),
			str(card, "effectiveFrom"),
			str(card, "currency"),
			lines,
			card["version"],
			str(card, "createdAt"),
		)
		if err != nil {
			return fmt.Errorf("rate card %s: %w", str(card, "id"), err)
		}
	}

	return nil
}

// filterCleanings inserts the customers' own filter cleanings (FR-C18).
func (s *seeding) filterCleanings() error {
	for _, cleaning := range s.fixture.DemoSeed.FilterCleanings {
		err := s.exec(
			`INSERT INTO maintenance.filter_cleanings (id, tenant_id, unit_id, cleaned_at, marked_by_membership_id) VALUES ($1,
				$2,$3,$4,$5) ON CONFLICT DO NOTHING`,
			ID(
				str(cleaning, "id"),
			),
			ID(s.unitTenant[str(cleaning, "unitId")]),
			ID(str(cleaning, "unitId")),
			str(cleaning, "cleanedAt"),
			ID(str(cleaning, "markedByMembershipId")),
		)
		if err != nil {
			return fmt.Errorf("filter cleaning %s: %w", str(cleaning, "id"), err)
		}
	}

	return nil
}

// jobEvents derives the job history (jobs.events) that the fixture rows imply: created, offered, the offer
// decision and the assignment. IDs are deterministic so re-seeding is idempotent; actors are only set where the
// fixture names them (decidedBy).
func (s *seeding) jobEvents() error {
	event := func(job, action string, actor, occurredAt any) error {
		return s.exec(
			`INSERT INTO maintenance.job_events (id, tenant_id, job_id, actor_user_id, action, occurred_at) VALUES ($1,$2,$3,$4,
				$5,$6) ON CONFLICT DO NOTHING`,
			ID("event:"+job+":"+action),
			ID(s.jobTenant[job]),
			ID(job),
			actor,
			action,
			occurredAt,
		)
	}

	for _, job := range s.fixture.DemoSeed.Jobs {
		err := event(str(job, "id"), "job.created", nil, s.fixture.SeedCreatedAt)
		if err != nil {
			return err
		}
	}

	for _, offer := range s.fixture.DemoSeed.Offers {
		job := str(offer, "jobId")

		err := event(job, "job.offered", nil, str(offer, "offeredAt"))
		if err != nil {
			return err
		}

		if decision := opt(offer, "decision"); decision != nil && opt(offer, "decidedAt") != nil {
			action := "offer.accepted"
			if *decision == "decline" {
				action = "offer.declined"
			}

			err := event(job, action, optID(offer, "decidedBy"), *opt(offer, "decidedAt"))
			if err != nil {
				return err
			}
		}
	}

	for _, assignment := range s.fixture.DemoSeed.Assignments {
		err := event(str(assignment, "jobId"), "job.assigned", nil, str(assignment, "createdAt"))
		if err != nil {
			return err
		}
	}

	return nil
}
