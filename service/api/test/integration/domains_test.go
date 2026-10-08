package integration

import (
	"testing"

	"github.com/pradita/ac-project/service/api/internal/ops"
)

// IR180: a business-domain service answers only its own operations; others are NOT_FOUND as if unknown, so a request
// routed to the wrong service never runs.
func TestDomainServiceServesOnlyItsOperations(t *testing.T) {
	s := server(t)
	s.Registry.ServeDomains(ops.DomainBilling)
	if code, _ := post(s, &customerA, "invoices.list", `{"limit":1}`); code != 200 {
		t.Fatalf("billing-api invoices.list: %d", code)
	}
	if code, m := post(s, &customerA, "units.list", `{"limit":1}`); code != 404 || m["messageKey"] != "error.unknownOperation" {
		t.Fatalf("billing-api units.list: %d %v", code, m)
	}
	s.Registry.ServeDomains(ops.DomainEquipment, ops.DomainBilling)
	if code, _ := post(s, &customerA, "units.list", `{"limit":1}`); code != 200 {
		t.Fatalf("two domains units.list: %d", code)
	}
	if code, _ := post(s, &customerA, "jobs.list", `{"limit":1}`); code != 404 {
		t.Fatalf("two domains jobs.list: %d", code)
	}
}
