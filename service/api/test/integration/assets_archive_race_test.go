package integration

import (
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// IR478: archiving a space while a unit is added to it, at the same moment, never leaves an active unit in an archived
// space. The archive checked for active units and then archived; the unit save checked that the space was not archived
// and then inserted; neither locked the space, so both could pass. Eight rounds of the two sent at once.
func TestSpaceArchiveRacesAUnitAdd(t *testing.T) {
	srv := server(t)

	prop := seed.ID("property-home-b").String()
	for round := range 8 {
		code, res := write(
			srv,
			&hq,
			"spaces.save",
			`{"propertyId":"`+prop+`","parentSpaceId":null,"kind":"room","name":"Race `+uuid.NewString()[:8]+`"}`,
			0,
		)
		if code != 200 {
			t.Fatalf("round %d: space: %d %v", round, code, res)
		}

		space := asStr(t, data(res)["id"])

		var (
			archived, added int
			unit            string
			group           sync.WaitGroup
		)

		group.Add(2)
		go func() {
			defer group.Done()

			archived, _ = write(
				srv,
				&hq,
				"spaces.archive",
				`{"id":"`+space+`","reason":"closed"}`,
				1,
			)
		}()
		go func() {
			defer group.Done()

			var resp map[string]any

			added, resp = write(
				srv,
				&hq,
				"units.save",
				`{"customerOrgId":"`+seed.ID("org-customer-b").
					String()+
					`","propertyId":"`+prop+`","spaceId":"`+space+`","displayName":"Race `+
					`AC `+uuid.NewString()[:8]+`","modelId":"`+seed.ID("ventilation-demo").
					String()+
					`","type":"split","installedAt":null,"serviceScope":["indoor"]}`,
				0,
			)
			if added == 200 {
				unit = data(resp)["id"].(string)
			}
		}()

		group.Wait()

		if archived == 200 && added == 200 {
			t.Fatalf("round %d: the space was archived and unit %s added to it", round, unit)
		}

		if archived != 200 && added != 200 {
			t.Fatalf("round %d: neither passed: archive %d, unit %d", round, archived, added)
		}
	}
}
