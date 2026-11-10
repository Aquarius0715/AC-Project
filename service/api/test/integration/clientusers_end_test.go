package integration_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// A save of a customer's user that leaves the status as it is keeps the end HQ gave the membership (IR662): the
// membership's end is rewritten only when the user is disabled or enabled again.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestClientUserSaveKeepsTheEndHQSet(t *testing.T) {
	srv := server(t)
	cust := seed.ID("cust-b").String()
	membership, user := uuid.NewString(), uuid.NewString()
	end := clock().Add(30 * 24 * time.Hour)

	owner(
		t,
		`INSERT INTO identity.memberships (id, tenant_id, user_id, organization_id, role, client_role, valid_from,
			valid_until)
		SELECT $1, tenant_id, user_id, organization_id, 'client', 'member', valid_from, $3 FROM identity.memberships
		WHERE id = $2`,
		membership,
		seed.ID("customer-b"),
		end,
	)

	email := "end-" + uuid.NewString()[:8] + "@example.com"
	owner(
		t,
		`INSERT INTO identity.client_users (id, tenant_id, customer_id, membership_id, email, client_role, status, invited_at,
			invited_by_membership_id)
		VALUES ($1,$2,$3,$4,$5,'member','active',$6,$7)`,
		user,
		seed.ID("tenant-a"),
		cust,
		membership,
		email,
		clock().Add(-time.Hour),
		seed.ID("customer-b"),
	)

	t.Cleanup(func() {
		owner(t, `DELETE FROM identity.client_users WHERE id = $1`, user)
		owner(t, `DELETE FROM identity.memberships WHERE id = $1`, membership)
	})

	// a save that changes nothing but the reason: the status stays active
	if code, res := write(
		srv,
		hq(),
		"clientUsers.save",
		`{"id":"`+user+`","customerId":"`+cust+`","email":"`+email+`","clientRole":"member","reason":"admin change"}`,
		1,
	); code != 200 {
		t.Fatalf("save: %d %v", code, res)
	}

	var until *time.Time

	ownerScan(
		t,
		`SELECT valid_until FROM identity.memberships WHERE id = $1`,
		[]any{membership},
		&until,
	)

	if until == nil || !until.Equal(end) {
		t.Errorf("the end HQ set after a save that left the status: %v, want %v", until, end)
	}
}
