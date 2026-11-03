package ops

import (
	"errors"
	"fmt"
	"sort"
)

// Business-domain microservices (IR180). Each Core API service serves the operations of its catalog modules; the
// gateway routes each REST route to the service that owns its operation. Phase A shares the PostgreSQL cluster: a
// service writes only its own schemas, and the remaining cross-domain reads are listed in IR180.
const (
	DomainIdentity    = "identity"    // identity-api
	DomainEquipment   = "equipment"   // equipment-api
	DomainMaintenance = "maintenance" // maintenance-api
	DomainBilling     = "billing"     // billing-api
	DomainEnergy      = "energy"      // energy-api
)

// Domains lists every domain in a stable order.
//
//nolint:gochecknoglobals // exported list that callers assign as a value (Config.Domains = ops.Domains)
var Domains = []string{
	DomainIdentity,
	DomainEquipment,
	DomainMaintenance,
	DomainBilling,
	DomainEnergy,
}

// The modules of operation-catalog.csv (its module column).
const (
	moduleIdentity      = "Identity & access"
	moduleNotifications = "Notifications"
	moduleAudit         = "Audit"
	moduleDemo          = "Demo"
	moduleAssets        = "Assets"
	moduleDevices       = "Devices"
	moduleControl       = "Control"
	moduleMonitoring    = "Monitoring & alerts"
	moduleReadModels    = "Read models"
	moduleMaintenance   = "Maintenance"
	moduleBilling       = "Billing"
	moduleRestrictions  = "Restrictions"
	moduleEnergy        = "Energy & carbon"
)

var errModulesWithoutDomain = errors.New("ops: catalog modules without a domain")

// moduleDomain returns the domain that owns an operation-catalog module ("" for an unknown module).
func moduleDomain(module string) string {
	switch module {
	case moduleIdentity, moduleNotifications, moduleAudit:
		return DomainIdentity
	case moduleDemo, // demo.trigger simulates device events (IR185)
		moduleAssets, moduleDevices, moduleControl, moduleMonitoring, moduleReadModels:
		return DomainEquipment
	case moduleMaintenance:
		return DomainMaintenance
	case moduleBilling, moduleRestrictions:
		return DomainBilling
	case moduleEnergy:
		return DomainEnergy
	}

	return ""
}

// DomainOf returns the domain that owns an operation ("" for an unknown operation).
func DomainOf(operation string) string {
	s, ok := SpecByName()[operation]
	if !ok {
		return ""
	}

	return moduleDomain(s.Module)
}

// CheckDomains verifies that every catalog module belongs to exactly one known domain (used by tests and start-up).
func CheckDomains() error {
	known := map[string]bool{}
	for _, d := range Domains {
		known[d] = true
	}

	var missing []string

	for _, s := range SpecByName() {
		if !known[moduleDomain(s.Module)] {
			missing = append(missing, s.Module)
		}
	}

	if len(missing) > 0 {
		sort.Strings(missing)

		return fmt.Errorf("%w: %v", errModulesWithoutDomain, missing)
	}

	return nil
}

// ServeDomains restricts the mounted routes to the operations of the given domains; other routes are not served
// (NOT_FOUND), so a request routed to the wrong service never runs. Nil serves every domain (tests, single-process
// runs).
func (r *Registry) ServeDomains(domains ...string) {
	if len(domains) == 0 {
		r.domains = nil

		return
	}

	r.domains = map[string]bool{}
	for _, d := range domains {
		r.domains[d] = true
	}
}

func (r *Registry) serves(operation string) bool {
	return r.domains == nil || r.domains[DomainOf(operation)]
}
