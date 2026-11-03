package ops_test

import (
	"testing"

	"github.com/pradita/ac-project/service/api/internal/ops"
)

func TestEveryOperationHasOneDomain(t *testing.T) {
	t.Parallel()

	err := ops.CheckDomains()
	if err != nil {
		t.Fatal(err)
	}

	counts := map[string]int{}
	for name := range ops.SpecByName() {
		counts[ops.DomainOf(name)]++
	}

	total := 0

	for _, domain := range ops.Domains {
		if counts[domain] == 0 {
			t.Errorf("domain %s owns no operation", domain)
		}

		total += counts[domain]
	}

	if total != len(ops.SpecByName()) {
		t.Fatalf("operations covered %d of %d", total, len(ops.SpecByName()))
	}

	if ops.DomainOf("jobs.list") != ops.DomainMaintenance ||
		ops.DomainOf("units.list") != ops.DomainEquipment ||
		ops.DomainOf("nope") != "" {
		t.Fatal("DomainOf")
	}
}
