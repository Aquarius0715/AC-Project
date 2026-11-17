package integration_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// TestPartnerAccessUntil (IR461, IR421): the contractor moves a technician's access end within the allowed range;
// while the account is disabled the new end is kept and only reaches the membership when the account is enabled
// again; a booked technician's access end is not shortened.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestPartnerAccessUntil(t *testing.T) {
	srv := server(t)
	membership, partnerUserID := uuid.NewString(), uuid.NewString()
	email := partnerUserID[:8] + "@example.com"

	owner(
		t,
		`INSERT INTO identity.memberships (id, tenant_id, user_id, organization_id, role, employment, valid_from, valid_until)
		VALUES ($1, $2, $3, $4, 'technician', 'external', $5, $6)`,
		membership,
		seed.ID("tenant-a"),
		newUser(t),
		seed.ID("org-contractor-a"),
		clock().Add(-time.Hour),
		clock().Add(30*24*time.Hour),
	)
	owner(
		t,
		`INSERT INTO identity.partner_users (id, tenant_id, organization_id, membership_id, email, partner_role, status,
			invited_at, invited_by_membership_id)
		VALUES ($1, $2, $3, $4, $5, 'technician', 'active', $6, $7)`,
		partnerUserID,
		seed.ID("tenant-a"),
		seed.ID("org-contractor-a"),
		membership,
		email,
		clock(),
		seed.ID("contractor-a"),
	)
	t.Cleanup(func() {
		owner(t, `DELETE FROM identity.partner_users WHERE id = $1`, partnerUserID)
		owner(
			t,
			`UPDATE identity.memberships SET valid_until = $2 WHERE id = $1`,
			membership,
			clock(),
		)
	})

	save := func(id, extra string, version int) (int, map[string]any) {
		return write(
			srv,
			contrA(),
			opPartnerUsersSave,
			`{"id":"`+id+`","email":"`+email+`","partnerRole":"technician","reason":"contract"`+extra+`}`,
			version,
		)
	}
	until := func(at time.Time) string { return `,"accessUntil":"` + at.Format(time.RFC3339) + `"` }
	validUntil := func() time.Time {
		var end time.Time
		ownerScan(
			t,
			`SELECT valid_until FROM identity.memberships WHERE id = $1`,
			[]any{membership},
			&end,
		)

		return end
	}

	for name, at := range map[string]time.Time{
		"access end passed":      clock().Add(-time.Hour),
		"access end over a year": clock().Add(367 * 24 * time.Hour),
	} {
		code, res := save(partnerUserID, until(at), 1)
		refusal(t, name, code, res, "accessUntil=error.range")
	}

	if code, res := save(partnerUserID, `,"status":"disabled"`, 1); code != 200 {
		t.Fatalf("disable: %d %v", code, res)
	}
	// disabled: the list shows the new end, the membership stays closed
	later := clock().Add(60 * 24 * time.Hour).Truncate(time.Second)

	code, res := save(partnerUserID, until(later), 2)
	if code != 200 || data(res)["accessUntil"] != later.UTC().Format(time.RFC3339) {
		t.Fatalf("move the end while disabled: %d %v", code, res)
	}

	if end := validUntil(); !end.Equal(clock()) {
		t.Errorf("membership while disabled: %v, want %v", end, clock())
	}

	if code, res := save(partnerUserID, `,"status":"active"`, 3); code != 200 {
		t.Fatalf("enable: %d %v", code, res)
	}

	if end := validUntil(); !end.Equal(later) {
		t.Errorf("membership after enabling: %v, want %v", end, later)
	}
	// the seed technician holds an assigned job: its access end is not shortened (IR421)
	var (
		booked        string
		bookedVersion int
		bookedEnd     *time.Time
	)
	ownerScan(
		t,
		`SELECT x.id::text, x.version, m.valid_until FROM identity.partner_users x JOIN identity.memberships m ON m.id =
			x.membership_id WHERE x.membership_id = $1`,
		[]any{seed.ID("tech-external-a")},
		&booked,
		&bookedVersion,
		&bookedEnd,
	)
	owner(
		t,
		`UPDATE identity.memberships SET valid_until = $2 WHERE id = $1`,
		seed.ID("tech-external-a"),
		clock().Add(90*24*time.Hour),
	)
	t.Cleanup(func() {
		owner(
			t,
			`UPDATE identity.memberships SET valid_until = $2 WHERE id = $1`,
			seed.ID("tech-external-a"),
			bookedEnd,
		)
	})

	code, res = write(
		srv,
		contrA(),
		opPartnerUsersSave,
		`{"id":"`+booked+`","email":"tech-external-a@ac.local","partnerRole":"technician","reason":"contract"`+
			until(
				clock().Add(24*time.Hour),
			)+`}`,
		bookedVersion,
	)
	if code != 409 || res["messageKey"] != keyTechnicianBooked {
		t.Errorf("shorten a booked technician's access: %d %v", code, res)
	}
}
