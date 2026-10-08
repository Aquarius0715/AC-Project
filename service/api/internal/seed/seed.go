// Package seed loads docs/04-agentic-sdlc/fixture-contract.json (actors + demoSeed) into PostgreSQL.
// Fixture string IDs map to deterministic UUIDv5 values (database design §3), so tests and the web demo can
// compute the database ID of any fixture ID with ID().
package seed

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/pradita/ac-project/service/api/internal/modules/billing"
	"github.com/pradita/ac-project/service/api/internal/modules/monitoring"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Namespace is the UUIDv5 namespace for fixture IDs.
var Namespace = uuid.MustParse("6f1c3a52-6d0b-5c1e-9a57-0d1b7a4c2e10")

// ID returns the database UUID of a fixture ID.
func ID(fixtureID string) uuid.UUID { return uuid.NewSHA1(Namespace, []byte(fixtureID)) }

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
			ID           string    `json:"id"`
			TenantID     string    `json:"tenantId"`
			Version      int       `json:"version"`
			CreatedAt    time.Time `json:"createdAt"`
			Region       string    `json:"region"`
			Year         int       `json:"year"`
			KgCO2ePerKWh float64   `json:"kgCO2ePerKWh"`
			Source       string    `json:"source"`
		} `json:"factors"`
		Baselines []struct {
			ID          string                       `json:"id"`
			TenantID    string                       `json:"tenantId"`
			Version     int                          `json:"version"`
			CreatedAt   time.Time                    `json:"createdAt"`
			UnitIDs     []string                     `json:"unitIds"`
			Period      struct{ From, To time.Time } `json:"period"`
			Method      string                       `json:"method"`
			BaselineKWh *float64                     `json:"baselineKWh"`
			Quality     json.RawMessage              `json:"quality"`
			BoundaryID  string                       `json:"boundaryId"`
			Boundary    string                       `json:"boundary"`
			Assumptions string                       `json:"assumptions"`
			Source      string                       `json:"source"`
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
		Consents      []struct {
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
			ID, TenantID, Kind, Name, Status string
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
			Temperature        json.RawMessage `json:"temperature"`
			Modes              []string        `json:"modes"`
			FanLevels          []string        `json:"fanLevels"`
			Ventilation        bool            `json:"ventilation"`
			VentilationLevels  []string        `json:"ventilationLevels"`
			Sensors            json.RawMessage `json:"sensors"`
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
			ServiceScope        []string        `json:"serviceScope"`
			Archived            bool            `json:"archived"`
			ObservedState       json.RawMessage `json:"observedState"`
			ObservedRestriction json.RawMessage `json:"observedRestriction"`
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
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f Fixture
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("fixture: %w", err)
	}
	return &f, nil
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
func Apply(ctx context.Context, tx pgx.Tx, f *Fixture) error {
	ex := func(sql string, args ...any) error {
		_, err := tx.Exec(ctx, sql, args...)
		return err
	}
	orgTenant := tenantOfOrg(f)
	for _, t := range f.DemoSeed.Tenants {
		if err := ex(`INSERT INTO platform.tenants (id, name) VALUES ($1,$2) ON CONFLICT DO NOTHING`, ID(t), t); err != nil {
			return fmt.Errorf("tenant %s: %w", t, err)
		}
	}
	for _, o := range f.DemoSeed.Organizations {
		kind := o.Kind
		if kind == "hq" || kind == "admin" {
			kind = "operator"
		}
		if err := ex(`INSERT INTO identity.organizations (id, tenant_id, name, kind, status) VALUES ($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`,
			ID(o.ID), ID(o.TenantID), o.Name, kind, o.Status); err != nil {
			return fmt.Errorf("organization %s: %w", o.ID, err)
		}
	}
	for _, a := range f.Actors {
		if err := ex(`INSERT INTO identity.users (id, email, display_name) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`,
			ID(a.UserID), a.MembershipID+"@ac.local", a.DisplayName); err != nil {
			return fmt.Errorf("user %s: %w", a.UserID, err)
		}
		if err := ex(`INSERT INTO identity.memberships (id, tenant_id, user_id, organization_id, role, employment, client_role, scope_version, valid_from, valid_until)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT DO NOTHING`,
			ID(a.MembershipID), ID(a.TenantID), ID(a.UserID), ID(a.OrganizationID), a.Role, a.Employment, a.ClientRole,
			max(a.ScopeVersion, 1), a.ValidFrom, a.ValidUntil); err != nil {
			return fmt.Errorf("membership %s: %w", a.MembershipID, err)
		}
		for _, p := range a.Permissions {
			if err := ex(`INSERT INTO identity.membership_permissions (tenant_id, membership_id, permission) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`,
				ID(a.TenantID), ID(a.MembershipID), p); err != nil {
				return fmt.Errorf("permission %s/%s: %w", a.MembershipID, p, err)
			}
		}
		for _, q := range a.Qualifications { // one grant per membership and code (UUIDv5 of both)
			if err := ex(`INSERT INTO identity.qualification_grants (id, tenant_id, membership_id, code, valid_from, valid_until, revoked_at)
				VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT DO NOTHING`, ID("grant-"+a.MembershipID+"-"+q.Code), ID(a.TenantID), ID(a.MembershipID), q.Code,
				q.ValidFrom, q.ValidUntil, q.RevokedAt); err != nil {
				return fmt.Errorf("grant %s/%s: %w", a.MembershipID, q.Code, err)
			}
		}
		for _, s := range a.Scopes {
			if err := ex(`INSERT INTO identity.membership_scopes (tenant_id, membership_id, kind, ref_id) VALUES ($1,$2,$3,$4) ON CONFLICT DO NOTHING`,
				ID(a.TenantID), ID(a.MembershipID), s.Kind, ID(s.ID)); err != nil {
				return fmt.Errorf("scope %s: %w", a.MembershipID, err)
			}
		}
	}
	for _, m := range f.DemoSeed.Capabilities {
		var temp any
		if len(m.Temperature) > 0 && string(m.Temperature) != "null" {
			temp = []byte(m.Temperature)
		}
		sensors := []byte(m.Sensors)
		if len(sensors) == 0 {
			sensors = []byte("[]")
		}
		nz := func(a []string) []string {
			if a == nil {
				return []string{}
			}
			return a
		}
		if err := ex(`INSERT INTO devices.capabilities (id, version, tenant_id, manufacturer, model, control, mode_control, fan_control, temperature,
			modes, fan_levels, ventilation, ventilation_levels, sensors, firmware_candidates, is_current)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,true) ON CONFLICT DO NOTHING`,
			ID(m.ID), max(m.Version, 1), ID(m.TenantID), m.Manufacturer, m.Model, m.Control, m.ModeControl, m.FanControl, temp,
			nz(m.Modes), nz(m.FanLevels), m.Ventilation, nz(m.VentilationLevels), sensors, nz(m.FirmwareCandidates)); err != nil {
			return fmt.Errorf("capability %s: %w", m.ID, err)
		}
	}
	for _, c := range f.DemoSeed.Customers {
		if err := ex(`INSERT INTO assets.customers (id, tenant_id, organization_id, name, status, service_profile) VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT DO NOTHING`,
			ID(c.ID), ID(c.TenantID), ID(c.OrganizationID), c.Name, c.Status, c.ServiceProfile); err != nil {
			return fmt.Errorf("customer %s: %w", c.ID, err)
		}
	}
	propTenant := map[string]string{}
	for _, p := range f.DemoSeed.Properties {
		t := orgTenant[p.CustomerOrgID]
		propTenant[p.ID] = t
		if err := ex(`INSERT INTO assets.properties (id, tenant_id, customer_org_id, kind, name, address, access_instructions, archived)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT DO NOTHING`,
			ID(p.ID), ID(t), ID(p.CustomerOrgID), p.Kind, p.Name, p.Address, p.AccessInstructions, p.Archived); err != nil {
			return fmt.Errorf("property %s: %w", p.ID, err)
		}
	}
	// parents before children
	pending := f.DemoSeed.Spaces
	done := map[string]bool{}
	for len(pending) > 0 {
		var next = pending[:0:0]
		for _, s := range pending {
			if s.ParentSpaceID != nil && !done[*s.ParentSpaceID] {
				next = append(next, s)
				continue
			}
			var parent *uuid.UUID
			if s.ParentSpaceID != nil {
				u := ID(*s.ParentSpaceID)
				parent = &u
			}
			if err := ex(`INSERT INTO assets.spaces (id, tenant_id, property_id, parent_space_id, kind, name) VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT DO NOTHING`,
				ID(s.ID), ID(propTenant[s.PropertyID]), ID(s.PropertyID), parent, s.Kind, s.Name); err != nil {
				return fmt.Errorf("space %s: %w", s.ID, err)
			}
			done[s.ID] = true
		}
		if len(next) == len(pending) {
			return fmt.Errorf("spaces with unknown parents: %d", len(next))
		}
		pending = next
	}
	for _, u := range f.DemoSeed.Units {
		var space *uuid.UUID
		if u.SpaceID != nil {
			s := ID(*u.SpaceID)
			space = &s
		}
		state := []byte(u.ObservedState)
		if len(state) == 0 || string(state) == "null" {
			state = []byte(`{"power":null,"celsius":null,"mode":null,"fanLevel":null,"observedAt":null}`)
		}
		var restr any
		if len(u.ObservedRestriction) > 0 && string(u.ObservedRestriction) != "null" {
			restr = []byte(u.ObservedRestriction)
		}
		scope := u.ServiceScope
		if len(scope) == 0 {
			scope = []string{"indoor", "outdoor", "electrical"}
		}
		if err := ex(`INSERT INTO assets.units (id, tenant_id, customer_org_id, property_id, space_id, display_name, model_id, installed_at,
			service_scope, archived, capability_version, observed_state, observed_restriction, version)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) ON CONFLICT DO NOTHING`,
			ID(u.ID), ID(orgTenant[u.CustomerOrgID]), ID(u.CustomerOrgID), ID(u.PropertyID), space, u.DisplayName, ID(u.ModelID),
			u.InstalledAt, scope, u.Archived, u.CapabilityVersion, state, restr, max(u.Version, 1)); err != nil {
			return fmt.Errorf("unit %s: %w", u.ID, err)
		}
	}
	unitTenant := map[string]string{}
	for _, u := range f.DemoSeed.Units {
		unitTenant[u.ID] = orgTenant[u.CustomerOrgID]
	}
	for _, d := range f.DemoSeed.Devices { // device, sensors, active binding; the unit mirrors the device connection
		tenant := ID(unitTenant[d.TargetUnitID])
		var unit, binding *uuid.UUID
		if d.UnitID != nil {
			u := ID(*d.UnitID)
			unit = &u
		}
		if d.BindingID != nil {
			b := ID(*d.BindingID)
			binding = &b
		}
		if err := ex(`INSERT INTO devices.devices (id, tenant_id, serial, binding_id, unit_id, target_unit_id, created_by_membership_id, connection, last_seen_at,
			firmware_version, power_signal, tamper) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) ON CONFLICT DO NOTHING`,
			ID(d.ID), tenant, d.Serial, binding, unit, ID(d.TargetUnitID), ID(d.CreatedByMembershipID), d.Connection, d.LastSeenAt, d.FirmwareVersion,
			d.PowerSignal, d.Tamper); err != nil {
			return fmt.Errorf("device %s: %w", d.ID, err)
		}
		for _, x := range d.Sensors {
			if err := ex(`INSERT INTO devices.sensors (id, tenant_id, device_id, metric, unit, boundary_id, stale_after_seconds, calibrated_at)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT DO NOTHING`, ID(x.ID), tenant, ID(d.ID), x.Metric, x.Unit, x.BoundaryID, x.StaleAfterSeconds, x.CalibratedAt); err != nil {
				return fmt.Errorf("sensor %s: %w", x.ID, err)
			}
		}
		if unit == nil || binding == nil {
			continue
		}
		if err := ex(`INSERT INTO devices.device_bindings (id, tenant_id, device_id, unit_id, customer_org_id, bound_at, reason, actor_membership_id)
			SELECT $1,$2,$3,$4,u.customer_org_id,$5,'demo seed',$6 FROM assets.units u WHERE u.id = $4 ON CONFLICT DO NOTHING`,
			*binding, tenant, ID(d.ID), *unit, f.SeedCreatedAt, ID(d.CreatedByMembershipID)); err != nil {
			return fmt.Errorf("binding %s: %w", d.ID, err)
		}
		if err := ex(`UPDATE assets.units SET connection = $2, last_seen_at = $3 WHERE id = $1`, *unit, d.Connection, d.LastSeenAt); err != nil {
			return fmt.Errorf("unit connection %s: %w", d.ID, err)
		}
	}
	for _, m := range f.DemoSeed.Measurements { // IR121: Measurement.id = eventId
		if err := ex(`INSERT INTO monitoring.measurements (tenant_id, unit_id, sensor_id, metric, observed_at, sequence, value, unit, origin, quality, boundary_id, received_at, event_id)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) ON CONFLICT DO NOTHING`,
			ID(unitTenant[m.UnitID]), ID(m.UnitID), ID(m.SensorID), m.Metric, m.ObservedAt, m.Sequence, m.Value, m.Unit, m.Origin, m.Quality, m.BoundaryID,
			m.ReceivedAt, ID(m.ID)); err != nil {
			return fmt.Errorf("measurement %s: %w", m.ID, err)
		}
	}
	for _, a := range f.DemoSeed.AllergenObservations {
		if err := ex(`INSERT INTO monitoring.allergen_observations (id, tenant_id, unit_id, availability, substance, value, unit, source_label, observed_at, evidence_text, created_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) ON CONFLICT DO NOTHING`,
			ID(a.ID), ID(unitTenant[a.UnitID]), ID(a.UnitID), a.Availability, a.Substance, a.Value, a.Unit, a.SourceLabel, a.ObservedAt, a.EvidenceText, a.CreatedAt); err != nil {
			return fmt.Errorf("allergen %s: %w", a.ID, err)
		}
	}
	hq := ID("hq-operator")
	for _, x := range f.DemoSeed.Factors {
		if err := ex(`INSERT INTO energy.emission_factors (id, version, tenant_id, region, year, kg_co2e_per_kwh, source, created_by, created_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT DO NOTHING`, ID(x.ID), x.Version, ID(x.TenantID), x.Region, x.Year, x.KgCO2ePerKWh, x.Source, hq, x.CreatedAt); err != nil {
			return fmt.Errorf("factor %s: %w", x.ID, err)
		}
	}
	for _, x := range f.DemoSeed.Baselines {
		units := make([]uuid.UUID, len(x.UnitIDs))
		for i, u := range x.UnitIDs {
			units[i] = ID(u)
		}
		if err := ex(`INSERT INTO energy.baselines (id, version, tenant_id, unit_ids, period, method, baseline_kwh, quality, boundary_id, boundary, assumptions, source, created_by, created_at)
			VALUES ($1,$2,$3,$4,tstzrange($5,$6),$7,$8,$9,$10,$11,$12,$13,$14,$15) ON CONFLICT DO NOTHING`, ID(x.ID), x.Version, ID(x.TenantID), units, x.Period.From, x.Period.To,
			x.Method, x.BaselineKWh, []byte(x.Quality), x.BoundaryID, x.Boundary, x.Assumptions, x.Source, hq, x.CreatedAt); err != nil {
			return fmt.Errorf("baseline %s: %w", x.ID, err)
		}
	}
	for _, x := range f.DemoSeed.Consents { // IR84 initial location consents
		if err := ex(`INSERT INTO identity.consents (id, tenant_id, membership_id, purpose, granted, granted_at, revoked_at, created_at, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$8) ON CONFLICT DO NOTHING`, ID(x.ID), ID(x.TenantID), ID(x.MembershipID), x.Purpose, x.Granted, x.GrantedAt, x.RevokedAt, x.CreatedAt); err != nil {
			return fmt.Errorf("consent %s: %w", x.ID, err)
		}
	}
	// client_users rows for the seed client memberships (owners, active; IR144)
	if err := ex(`INSERT INTO identity.client_users (id, tenant_id, customer_id, membership_id, email, display_name, client_role, status, invited_at, invited_by_membership_id, created_at, updated_at)
		SELECT m.id, m.tenant_id, cu.id, m.id, u.email, u.display_name, COALESCE(m.client_role, 'owner'), 'active', $1, m.id, $1, $1
		FROM identity.memberships m JOIN identity.users u ON u.id = m.user_id JOIN assets.customers cu ON cu.organization_id = m.organization_id
		WHERE m.role = 'client' AND m.id = ANY($2) ON CONFLICT DO NOTHING`, f.SeedCreatedAt, clientMemberships(f)); err != nil {
		return fmt.Errorf("client users: %w", err)
	}
	if err := applyBusiness(ex, f); err != nil {
		return err
	}
	return applyPolicies(ex, f)
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

// applyPolicies seeds the IR120 default policy of each tenant (owned by its first HQ actor) and the customer-a demo
// setting that switches the AC offline rule off.
func applyPolicies(ex func(string, ...any) error, f *Fixture) error {
	rules, err := json.Marshal(monitoring.DefaultRules)
	if err != nil {
		return err
	}
	done := map[string]bool{}
	for _, a := range f.Actors {
		if a.Role != "admin" || done[a.TenantID] {
			continue
		}
		done[a.TenantID] = true
		id := DefaultPolicyID(a.TenantID)
		if err := ex(`INSERT INTO monitoring.alert_policies (id, tenant_id, kind, customer_id, name, owner_membership_id, created_by_user_id, timezone, enabled, priority, rules)
			VALUES ($1,$2,'default_alert',NULL,'Default policy',$3,$4,'Asia/Kuala_Lumpur',true,0,$5) ON CONFLICT DO NOTHING`,
			id, ID(a.TenantID), ID(a.MembershipID), ID(a.UserID), rules); err != nil {
			return fmt.Errorf("default policy %s: %w", a.TenantID, err)
		}
		if a.TenantID == "tenant-a" {
			if err := ex(`INSERT INTO monitoring.default_rule_settings (tenant_id, policy_id, rule_key, customer_id, enabled, changed_by_membership_id, reason)
				VALUES ($1,$2,'ac_offline',$3,false,$4,'Demo: customer switched the offline rule off') ON CONFLICT DO NOTHING`,
				ID(a.TenantID), id, ID("cust-a"), ID(a.MembershipID)); err != nil {
				return fmt.Errorf("default rule setting: %w", err)
			}
		}
	}
	return nil
}

// DefaultPolicyID is the ID of a tenant's default alert policy (policy-default for tenant-a).
func DefaultPolicyID(tenant string) uuid.UUID {
	if tenant == "tenant-a" {
		return ID("policy-default")
	}
	return ID("policy-default-" + tenant)
}

// ---- demoSeed business records (contracts, invoices, restrictions, commands, alerts, notifications, jobs) ----

func str(m map[string]any, k string) string { v, _ := m[k].(string); return v }

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

func num(m map[string]any, k string) float64 { v, _ := m[k].(float64); return v }

func rng(m map[string]any, k, from, to string) (any, any) {
	r, _ := m[k].(map[string]any)
	if r == nil {
		return nil, nil
	}
	return r[from], r[to]
}

func ids(v any) []uuid.UUID {
	out := []uuid.UUID{}
	xs, _ := v.([]any)
	for _, x := range xs {
		out = append(out, ID(x.(string)))
	}
	return out
}

func raw(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

// fixtureIDs replaces fixture IDs inside an action / policy object by their database UUIDs.
func fixtureIDs(v any, keys ...string) []byte {
	m, _ := v.(map[string]any)
	out := map[string]any{}
	for k, x := range m {
		out[k] = x
	}
	for _, k := range keys {
		if s, ok := out[k].(string); ok {
			out[k] = ID(s).String()
		}
	}
	return raw(out)
}

// applyBusiness loads the demoSeed business collections with deterministic IDs so the API demo shows the fixture
// scenario (IR161). Tenants come from the referenced customers / units.
func applyBusiness(ex func(string, ...any) error, f *Fixture) error {
	tenantOf := map[string]string{}
	for _, c := range f.DemoSeed.Customers {
		tenantOf[c.ID] = c.TenantID
	}
	unitTenant := map[string]string{}
	unitOrg := map[string]string{}
	orgTenant := tenantOfOrg(f)
	for _, u := range f.DemoSeed.Units {
		unitTenant[u.ID], unitOrg[u.ID] = orgTenant[u.CustomerOrgID], u.CustomerOrgID
	}
	hq := ID("hq-operator")
	contractTenant := map[string]string{}
	for _, k := range f.DemoSeed.Contracts {
		t := tenantOf[str(k, "customerId")]
		contractTenant[str(k, "id")] = t
		if err := ex(`INSERT INTO billing.contracts (id, version, tenant_id, customer_id, customer_org_id, plan_type, term, price_minor, currency, restriction_eligible, rules_version, created_by, created_at)
			VALUES ($1,$2,$3,$4,$5,$6,tstzrange($7::timestamptz,$8::timestamptz),$9,$10,$11,$12,$13,$7::timestamptz) ON CONFLICT DO NOTHING`,
			ID(str(k, "id")), int(num(k, "version")), ID(t), ID(str(k, "customerId")), ID(str(k, "customerOrgId")), str(k, "planType"), str(k, "startAt"), str(k, "endAt"),
			int64(num(k, "priceMinor")), str(k, "currency"), k["restrictionEligible"], opt(k, "rulesVersion"), hq); err != nil {
			return fmt.Errorf("contract %s: %w", str(k, "id"), err)
		}
		for _, u := range ids(k["unitIds"]) {
			if err := ex(`INSERT INTO billing.contract_units (tenant_id, contract_id, contract_version, unit_id) VALUES ($1,$2,$3,$4) ON CONFLICT DO NOTHING`,
				ID(t), ID(str(k, "id")), int(num(k, "version")), u); err != nil {
				return fmt.Errorf("contract units %s: %w", str(k, "id"), err)
			}
		}
	}
	for i, inv := range f.DemoSeed.Invoices {
		t := contractTenant[str(inv, "contractId")]
		var customer string
		for _, k := range f.DemoSeed.Contracts {
			if str(k, "id") == str(inv, "contractId") {
				customer = str(k, "customerId")
			}
		}
		from, to := rng(inv, "period", "from", "to")
		start, err := time.Parse(time.RFC3339, str(inv["period"].(map[string]any), "from"))
		if err != nil {
			return fmt.Errorf("invoice %s period: %w", str(inv, "id"), err)
		}
		if err := ex(`INSERT INTO billing.invoices (id, tenant_id, number, contract_id, contract_version, customer_id, amount_minor, currency, period, due_at, status, paid_at, version)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,tstzrange($9::timestamptz,$10::timestamptz),$11,$12,$13,$14) ON CONFLICT DO NOTHING`,
			ID(str(inv, "id")), ID(t), billing.InvoiceNumber(start, i+1), ID(str(inv, "contractId")), int(num(inv, "contractVersion")),
			ID(customer), int64(num(inv, "amountMinor")), str(inv, "currency"), from, to, str(inv, "dueAt"), str(inv, "status"), opt(inv, "paidAt"), int(num(inv, "version"))); err != nil {
			return fmt.Errorf("invoice %s: %w", str(inv, "id"), err)
		}
	}
	for _, r := range f.DemoSeed.Restrictions {
		t := contractTenant[str(r, "contractId")]
		var customer string
		for _, k := range f.DemoSeed.Contracts {
			if str(k, "id") == str(r, "contractId") {
				customer = str(k, "customerId")
			}
		}
		if err := ex(`INSERT INTO restrictions.restrictions (id, tenant_id, contract_id, contract_version, customer_id, rules_version, notice_at, execute_after, reason, policy, state,
			notice_notification_ids, version, created_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$7,$7) ON CONFLICT DO NOTHING`,
			ID(str(r, "id")), ID(t), ID(str(r, "contractId")), int(num(r, "contractVersion")), ID(customer), str(r, "rulesVersion"), str(r, "noticeAt"), str(r, "executeAfter"),
			str(r, "reason"), raw(r["policy"]), str(r, "state"), ids(r["noticeNotificationIds"]), int(num(r, "version"))); err != nil {
			return fmt.Errorf("restriction %s: %w", str(r, "id"), err)
		}
		for _, inv := range ids(r["causeInvoiceIds"]) {
			if err := ex(`INSERT INTO restrictions.restriction_invoices (tenant_id, restriction_id, invoice_id) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`, ID(t), ID(str(r, "id")), inv); err != nil {
				return err
			}
		}
		pu, _ := r["perUnit"].([]any)
		for _, x := range pu {
			u := x.(map[string]any)
			obs := map[string]any{"restrictionId": ID(str(r, "id")).String(), "rulesVersion": str(r, "rulesVersion"), "policy": r["policy"], "observedAt": u["observedAt"]}
			var observed any
			if str(u, "applyState") == "applied" {
				observed = raw(obs)
			}
			if err := ex(`INSERT INTO restrictions.restriction_units (tenant_id, restriction_id, unit_id, apply_state, release_state, apply_command_ids, release_command_ids, observed_restriction,
				observed_at, pending_reason) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT DO NOTHING`, ID(t), ID(str(r, "id")), ID(str(u, "unitId")), str(u, "applyState"),
				str(u, "releaseState"), ids(u["applyCommandIds"]), ids(u["releaseCommandIds"]), observed, opt(u, "observedAt"), opt(u, "pendingReason")); err != nil {
				return fmt.Errorf("restriction unit: %w", err)
			}
			if observed != nil {
				if err := ex(`UPDATE assets.units SET observed_restriction = $2 WHERE id = $1`, ID(str(u, "unitId")), observed); err != nil {
					return err
				}
			}
		}
	}
	for _, c := range f.DemoSeed.Commands {
		a, _ := c["action"].(map[string]any)
		if err := ex(`INSERT INTO control.commands (id, tenant_id, unit_id, actor_membership_id, source, action, restriction_id, status, delivery, requested_at, sent_at, acknowledged_at,
			expires_at, correlation_id, created_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,'seed',$10,$10) ON CONFLICT DO NOTHING`,
			ID(str(c, "id")), ID(unitTenant[str(c, "unitId")]), ID(str(c, "unitId")), ID(str(c, "actorMembershipId")), map[bool]string{true: "restriction", false: "ui"}[a["restrictionId"] != nil],
			fixtureIDs(a, "restrictionId"), optID(a, "restrictionId"), str(c, "status"), str(c, "delivery"), str(c, "requestedAt"), opt(c, "sentAt"), opt(c, "acknowledgedAt"), str(c, "expiresAt")); err != nil {
			return fmt.Errorf("command %s: %w", str(c, "id"), err)
		}
		if a["restrictionId"] != nil { // billing's own record of the restriction command (IR194)
			status := str(c, "status")
			if status == "sent" {
				status = "requested"
			}
			kind, _ := a["kind"].(string)
			if err := ex(`INSERT INTO restrictions.restriction_commands (tenant_id, command_id, restriction_id, unit_id, kind, delivered, status, requested_at, updated_at)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$8) ON CONFLICT DO NOTHING`, ID(unitTenant[str(c, "unitId")]), ID(str(c, "id")), optID(a, "restrictionId"), ID(str(c, "unitId")),
				kind, str(c, "delivery") == "sent", status, str(c, "requestedAt")); err != nil {
				return fmt.Errorf("restriction command %s: %w", str(c, "id"), err)
			}
		}
	}
	for _, a := range f.DemoSeed.Alerts {
		u := str(a, "unitId")
		if err := ex(`INSERT INTO monitoring.alerts (id, tenant_id, unit_id, customer_org_id, policy_id, type, severity, status, cause_code, evidence_kind, evidence_text, observed_at, detected_at,
			previous_alert_id, created_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$13,$13) ON CONFLICT DO NOTHING`,
			ID(str(a, "id")), ID(unitTenant[u]), ID(u), ID(unitOrg[u]), optID(a, "policyId"), str(a, "type"), str(a, "severity"), str(a, "status"), str(a, "causeCode"),
			str(a, "evidenceKind"), str(a, "evidenceText"), str(a, "observedAt"), str(a, "detectedAt"), optID(a, "previousAlertId")); err != nil {
			return fmt.Errorf("alert %s: %w", str(a, "id"), err)
		}
	}
	memberTenant := map[string]string{}
	for _, a := range f.Actors {
		memberTenant[a.MembershipID] = a.TenantID
	}
	for _, n := range f.DemoSeed.Notifications {
		tg, _ := n["target"].(map[string]any)
		target := raw(map[string]any{"kind": tg["kind"], "id": ID(tg["id"].(string)).String()})
		if err := ex(`INSERT INTO notify.notifications (id, tenant_id, recipient_membership_id, scope_version_at_creation, type, channel, template_key, target, params, source_alert_id, severity,
			occurred_at, read_at, created_at) VALUES ($1,$2,$3,1,$4,$5,$6,$7,'{}',$8,$9,$10,$11,$10) ON CONFLICT DO NOTHING`,
			ID(str(n, "id")), ID(memberTenant[str(n, "recipientMembershipId")]), ID(str(n, "recipientMembershipId")), str(n, "type"), str(n, "channel"), str(n, "templateKey"), target,
			optID(n, "sourceAlertId"), str(n, "severity"), str(n, "occurredAt"), opt(n, "readAt")); err != nil {
			return fmt.Errorf("notification %s: %w", str(n, "id"), err)
		}
	}
	for _, j := range f.DemoSeed.Jobs {
		u := str(j, "unitId")
		rs, re := rng(j, "requestedSlot", "startAt", "endAt")
		ss, se := rng(j, "scheduledSlot", "startAt", "endAt")
		var sched any
		if ss != nil {
			sched = fmt.Sprintf("[%s,%s)", ss, se)
		}
		if err := ex(`INSERT INTO maintenance.jobs (id, tenant_id, unit_id, customer_org_id, type, status, origin, symptom, contact_window, requested_slot, preferred_slots, preference_round,
			scheduled_slot, due_at, contractor_org_id, assignment_id, version, created_at, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,tstzrange($10::timestamptz,$11::timestamptz),$12,$13,$14::tstzrange,$15,$16,$17,$18,$19,$19) ON CONFLICT DO NOTHING`,
			ID(str(j, "id")), ID(unitTenant[u]), ID(u), ID(unitOrg[u]), str(j, "type"), str(j, "status"), str(j, "origin"), str(j, "symptom"), opt(j, "contactWindow"), rs, re,
			raw(j["preferredSlots"]), int(num(j, "preferenceRound")), sched, str(j, "dueAt"), optID(j, "contractorOrgId"), optID(j, "assignmentId"), int(num(j, "version")), f.SeedCreatedAt); err != nil {
			return fmt.Errorf("job %s: %w", str(j, "id"), err)
		}
		for _, a := range ids(j["alertIds"]) {
			if err := ex(`INSERT INTO maintenance.job_alerts (tenant_id, job_id, alert_id) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`, ID(unitTenant[u]), ID(str(j, "id")), a); err != nil {
				return err
			}
		}
	}
	jobTenant := map[string]string{}
	for _, j := range f.DemoSeed.Jobs {
		jobTenant[str(j, "id")] = unitTenant[str(j, "unitId")]
	}
	for _, o := range f.DemoSeed.Offers {
		vs, ve := rng(o, "visitSlot", "startAt", "endAt")
		if err := ex(`INSERT INTO maintenance.offers (id, tenant_id, job_id, contractor_org_id, terms_version, visit_slot, offered_at, offer_expires_at, access_valid_from, access_valid_until,
			decision, decided_by, decided_at, decline_reason, created_at, updated_at)
			VALUES ($1,$2,$3,$4,$5,tstzrange($6::timestamptz,$7::timestamptz),$8,$9,$10,$11,$12,$13,$14,$15,$8,$8) ON CONFLICT DO NOTHING`,
			ID(str(o, "id")), ID(jobTenant[str(o, "jobId")]), ID(str(o, "jobId")), ID(str(o, "contractorOrgId")), str(o, "termsVersion"), vs, ve, str(o, "offeredAt"), str(o, "offerExpiresAt"),
			str(o, "accessValidFrom"), str(o, "accessValidUntil"), opt(o, "decision"), optID(o, "decidedBy"), opt(o, "decidedAt"), opt(o, "declineReason")); err != nil {
			return fmt.Errorf("offer %s: %w", str(o, "id"), err)
		}
	}
	for _, a := range f.DemoSeed.Assignments {
		if err := ex(`INSERT INTO maintenance.assignments (id, tenant_id, job_id, technician_membership_id, valid_from, valid_until, scheduled, status, reason, acknowledgement, acknowledged_at,
			cant_make_reason, created_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6,tstzrange($7::timestamptz,$8::timestamptz),$9,$10,$11,$12,$13,$14,$14) ON CONFLICT DO NOTHING`,
			ID(str(a, "id")), ID(jobTenant[str(a, "jobId")]), ID(str(a, "jobId")), ID(str(a, "technicianMembershipId")), str(a, "validFrom"), str(a, "validUntil"),
			str(a, "scheduledStart"), str(a, "scheduledEnd"), str(a, "status"), opt(a, "reason"), str(a, "acknowledgement"), opt(a, "acknowledgedAt"), opt(a, "cantMakeReason"),
			str(a, "createdAt")); err != nil {
			return fmt.Errorf("assignment %s: %w", str(a, "id"), err)
		}
	}
	return seedJobEvents(ex, f, jobTenant)
}

// seedJobEvents derives the job history (jobs.events) that the fixture rows imply: created, offered, the offer
// decision and the assignment. IDs are deterministic so re-seeding is idempotent; actors are only set where the
// fixture names them (decidedBy).
func seedJobEvents(ex func(string, ...any) error, f *Fixture, jobTenant map[string]string) error {
	ev := func(job, action string, actor, at any) error {
		return ex(`INSERT INTO maintenance.job_events (id, tenant_id, job_id, actor_user_id, action, occurred_at) VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT DO NOTHING`,
			ID("event:"+job+":"+action), ID(jobTenant[job]), ID(job), actor, action, at)
	}
	for _, j := range f.DemoSeed.Jobs {
		if err := ev(str(j, "id"), "job.created", nil, f.SeedCreatedAt); err != nil {
			return err
		}
	}
	for _, o := range f.DemoSeed.Offers {
		job := str(o, "jobId")
		if err := ev(job, "job.offered", nil, str(o, "offeredAt")); err != nil {
			return err
		}
		if d := opt(o, "decision"); d != nil && opt(o, "decidedAt") != nil {
			action := "offer.accepted"
			if *d == "decline" {
				action = "offer.declined"
			}
			if err := ev(job, action, optID(o, "decidedBy"), *opt(o, "decidedAt")); err != nil {
				return err
			}
		}
	}
	for _, a := range f.DemoSeed.Assignments {
		if err := ev(str(a, "jobId"), "job.assigned", nil, str(a, "createdAt")); err != nil {
			return err
		}
	}
	return nil
}
