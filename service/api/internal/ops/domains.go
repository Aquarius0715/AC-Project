package ops

import (
	"fmt"
	"sort"
)

// Business-domain microservices (IR180). Each Core API service serves the operations of its catalog modules; the
// gateway routes each REST route to the service that owns its operation. Phase A shares the PostgreSQL cluster: a service
// writes only its own schemas, and the remaining cross-domain reads are listed in IR180.
const (
	DomainIdentity    = "identity"    // identity-api
	DomainEquipment   = "equipment"   // equipment-api
	DomainMaintenance = "maintenance" // maintenance-api
	DomainBilling     = "billing"     // billing-api
	DomainEnergy      = "energy"      // energy-api
)

// Domains lists every domain in a stable order.
var Domains = []string{DomainIdentity, DomainEquipment, DomainMaintenance, DomainBilling, DomainEnergy}

// moduleDomain maps operation-catalog modules to their owning domain.
var moduleDomain = map[string]string{
	"Identity & access":   DomainIdentity,
	"Notifications":       DomainIdentity,
	"Audit":               DomainIdentity,
	"Demo":                DomainEquipment, // demo.trigger simulates device events (IR185)
	"Assets":              DomainEquipment,
	"Devices":             DomainEquipment,
	"Control":             DomainEquipment,
	"Monitoring & alerts": DomainEquipment,
	"Read models":         DomainEquipment,
	"Maintenance":         DomainMaintenance,
	"Billing":             DomainBilling,
	"Restrictions":        DomainBilling,
	"Energy & carbon":     DomainEnergy,
}

// DomainOf returns the domain that owns an operation ("" for an unknown operation).
func DomainOf(operation string) string {
	s, ok := SpecByName()[operation]
	if !ok {
		return ""
	}
	return moduleDomain[s.Module]
}

// CheckDomains verifies that every catalog module belongs to exactly one known domain (used by tests and start-up).
func CheckDomains() error {
	known := map[string]bool{}
	for _, d := range Domains {
		known[d] = true
	}
	var missing []string
	for _, s := range SpecByName() {
		if !known[moduleDomain[s.Module]] {
			missing = append(missing, s.Module)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("ops: catalog modules without a domain: %v", missing)
	}
	return nil
}

// ServeDomains restricts the mounted routes to the operations of the given domains; other routes are not served
// (NOT_FOUND), so a request routed to the wrong service never runs. Nil serves every domain (tests, single-process runs).
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
