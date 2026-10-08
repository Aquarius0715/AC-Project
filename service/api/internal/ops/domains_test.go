package ops

import "testing"

func TestEveryOperationHasOneDomain(t *testing.T) {
	if err := CheckDomains(); err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for name := range SpecByName() {
		counts[DomainOf(name)]++
	}
	total := 0
	for _, d := range Domains {
		if counts[d] == 0 {
			t.Errorf("domain %s owns no operation", d)
		}
		total += counts[d]
	}
	if total != len(SpecByName()) {
		t.Fatalf("operations covered %d of %d", total, len(SpecByName()))
	}
	if DomainOf("jobs.list") != DomainMaintenance || DomainOf("units.list") != DomainEquipment || DomainOf("nope") != "" {
		t.Fatal("DomainOf")
	}
}
