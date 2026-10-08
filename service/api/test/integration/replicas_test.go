package integration

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/modules/energy"
	"github.com/pradita/ac-project/service/api/internal/modules/notify"
	"github.com/pradita/ac-project/service/api/internal/platform/events"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

// IR188, IR189: every notify and energy reference copy holds exactly the captured columns of its source rows, and follows updates and
// deletes made by any write path.
func TestReplicas(t *testing.T) {
	_ = server(t)
	same := func() {
		t.Helper()
		drainEvents(t)
		for _, r := range append(append([]events.Replica{}, notify.Replicas...), energy.Replicas...) {
			cols := strings.Join(append(append([]string{}, r.Keys...), r.Cols...), ", ")
			source := r.Source
			if source == "monitoring.measurements" { // only minute-slot power samples are captured
				source = "(SELECT * FROM monitoring.measurements WHERE metric = 'power' AND observed_at = date_trunc('minute', observed_at)) m"
			}
			var diff int
			ownerScan(t, `SELECT count(*) FROM ((SELECT `+cols+` FROM `+source+` EXCEPT SELECT `+cols+` FROM `+r.Table+`)
				UNION ALL (SELECT `+cols+` FROM `+r.Table+` EXCEPT SELECT `+cols+` FROM `+source+`)) d`, nil, &diff)
			if diff != 0 {
				t.Fatalf("%s and %s differ in %d rows", r.Source, r.Table, diff)
			}
		}
	}
	same()
	unit := seed.ID("unit-other-customer")
	var name string
	ownerScan(t, `SELECT display_name FROM assets.units WHERE id = $1`, []any{unit}, &name)
	owner(t, `UPDATE assets.units SET display_name = display_name || ' (renamed)' WHERE id = $1`, unit)
	same()
	owner(t, `UPDATE assets.units SET display_name = $2 WHERE id = $1`, unit, name)
	rid := uuid.New()
	owner(t, `INSERT INTO restrictions.restrictions (id, tenant_id, contract_id, contract_version, customer_id, rules_version, notice_at, execute_after, reason, policy, state)
		VALUES ($1,$2,$3,1,$4,'r',$5,$6,'demo','{"kind":"power_off"}','applied')`, rid, seed.ID("tenant-a"), uuid.New(), seed.ID("cust-a"), clock, clock.Add(25*time.Hour))
	owner(t, `INSERT INTO restrictions.restriction_units (tenant_id, restriction_id, unit_id, apply_state, release_state) VALUES ($1,$2,$3,'applied','none')`, seed.ID("tenant-a"), rid, unit)
	powerSeries(t, unit.String(), clock.Add(-10*time.Minute), "measured", kw(1), kw(2))
	same()
	owner(t, `DELETE FROM restrictions.restriction_units WHERE restriction_id = $1`, rid)
	owner(t, `DELETE FROM restrictions.restrictions WHERE id = $1`, rid)
	same()
}
