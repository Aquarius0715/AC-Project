package integration

import (
	"testing"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	apiserver "github.com/pradita/ac-project/service/api/internal/server"
)

// IR180: a business-domain service answers only its own operations; others are NOT_FOUND as if unknown, so a request
// routed to the wrong service never runs. Data of other domains comes from their services (IR193).
func TestDomainServiceServesOnlyItsOperations(t *testing.T) {
	cluster := newCluster(t)

	billing := cluster.srv[ops.DomainBilling]
	if code, m := post(billing, &customerA, "invoices.list", `{"limit":1}`); code != 200 {
		t.Fatalf("billing-api invoices.list: %d %v", code, m)
	}

	if code, res := post(
		billing,
		&customerA,
		"units.list",
		`{"limit":1}`,
	); code != 404 ||
		res["messageKey"] != apperr.KeyUnknownOperation {
		t.Fatalf("billing-api units.list: %d %v", code, res)
	}

	two := serverCfg(t, func(c *apiserver.Config) {
		c.Domains, c.InternalToken = []string{
			ops.DomainEquipment,
			ops.DomainBilling,
		}, "test-internal"
		c.ServiceURLs = map[string]string{
			ops.DomainIdentity:    cluster.urls[ops.DomainIdentity],
			ops.DomainMaintenance: cluster.urls[ops.DomainMaintenance],
			ops.DomainEnergy:      cluster.urls[ops.DomainEnergy],
		}
	})
	if code, _ := post(two, &customerA, "units.list", `{"limit":1}`); code != 200 {
		t.Fatalf("two domains units.list: %d", code)
	}

	if code, _ := post(two, &customerA, "jobs.list", `{"limit":1}`); code != 404 {
		t.Fatalf("two domains jobs.list: %d", code)
	}
}
