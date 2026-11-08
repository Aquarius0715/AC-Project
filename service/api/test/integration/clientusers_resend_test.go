package integration_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// A customer's owner cannot send the set-up e-mail again once the access has ended (IR461, IR508): the e-mail would
// open the sign-in account HQ turned off. Like partnerUsers.resendInvite, the answer is CONFLICT errors.access_expired.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestClientUserResendRefusedAfterTheAccessEnded(t *testing.T) {
	srv := server(t)
	cust := seed.ID("cust-b").String()
	email := "ended-" + uuid.NewString()[:8] + "@example.com"

	code, resp := write(
		srv,
		customerB(),
		"clientUsers.save",
		`{"customerId":"`+cust+`","email":"`+email+`","clientRole":"member","displayName":"Ended Member"}`,
		0,
	)
	if code != 200 {
		t.Fatalf("invite: %d %v", code, resp)
	}

	id, membership := asStr(t, data(resp)["id"]), asStr(t, data(resp)["membershipId"])
	t.Cleanup(func() {
		owner(t, `DELETE FROM identity.client_users WHERE id = $1`, id)
		owner(t, `DELETE FROM identity.consents WHERE membership_id = $1`, membership)
		owner(t, `DELETE FROM identity.memberships WHERE id = $1`, membership)
	})

	// HQ ends the access: the membership ends and the sign-in is turned off (members.save, IR508)
	owner(
		t,
		`UPDATE identity.memberships SET valid_from = $2::timestamptz - interval '1 hour', valid_until = $2, revoked_at = $2
		WHERE id = $1`,
		membership,
		clock().Add(-time.Minute),
	)
	owner(
		t,
		`UPDATE identity.client_users SET last_invite_sent_at = last_invite_sent_at - interval '10 minutes' WHERE id = $1`,
		id,
	)

	code, res := write(srv, customerB(), "clientUsers.resendInvite", `{"id":"`+id+`"}`, 0)
	if code != 409 || res["messageKey"] != keyAccessExpired {
		t.Errorf("resend after HQ ended the access: %d %v", code, res)
	}
}
