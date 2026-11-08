package integration_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// A new preview removes the previews that have expired (database-design.md: "purged after expiry"): nothing else
// deletes them, so the table would keep every file anyone previewed.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestUnitsImportPreviewPurgesExpiredPreviews(t *testing.T) {
	srv := server(t)
	cust := seed.ID("cust-b").String()
	expired, live := uuid.NewString(), uuid.NewString()

	for id, expires := range map[string]time.Time{expired: clock().Add(-time.Hour), live: clock().Add(time.Hour)} {
		owner(
			t,
			`INSERT INTO assets.unit_import_previews (id, tenant_id, customer_id, membership_id, file_name, rows, expires_at)
			VALUES ($1, $2, $3, $4, 'old.csv', '[]', $5)`,
			id,
			seed.ID("tenant-a"),
			cust,
			seed.ID("hq-operator"),
			expires,
		)
	}

	t.Cleanup(func() {
		owner(
			t,
			`DELETE FROM assets.unit_import_previews WHERE id = ANY($1::uuid[])`,
			[]string{expired, live},
		)
	})

	csv := "Property,Unit,Model\nPurge site " + uuid.NewString()[:6] + ",Purge AC,SPL-100\n"
	body := `{"customerId":"` + cust + `","fileName":"units.csv","csvText":` + jsonStr(t, csv) +
		`,"mapping":{"property":"Property","unit_name":"Unit","model_code":"Model"}}`

	if code, resp := post(srv, hq(), "units.importPreview", body); code != 200 {
		t.Fatalf("preview: %d %v", code, resp)
	}

	var left int

	ownerScan(
		t,
		`SELECT count(*) FROM assets.unit_import_previews WHERE id = ANY($1::uuid[])`,
		[]any{[]string{expired, live}},
		&left,
	)

	if left != 1 {
		t.Errorf("previews left of an expired and a live one: %d, want 1 (the live one)", left)
	}
}
