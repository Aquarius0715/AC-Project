package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// A set-up e-mail is sent again only for an invitation that is still pending and whose access has not ended, both read
// under the locks the changes take (IR757): the invited account row (a disable locks it) and the membership (HQ's end
// of the access locks it). A change half done makes the re-send wait and then refuse, instead of opening the sign-in
// of an account that was disabled or whose access ended meanwhile (IR461, IR664).
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestResendInviteWaitsForAChangeOfTheAccount(t *testing.T) {
	srv := server(t)
	ctx := context.Background()

	for _, testCase := range []struct {
		name, change, key string
	}{
		{
			"disabled meanwhile",
			`UPDATE identity.client_users SET status = 'disabled' WHERE id = $1`,
			"errors.invite_not_pending",
		},
		{
			"access ended meanwhile",
			`UPDATE identity.memberships SET valid_from = $2, valid_until = $3
				WHERE id = (SELECT membership_id FROM identity.client_users WHERE id = $1)`,
			"errors.access_expired",
		},
	} {
		email := "resend-" + uuid.NewString()[:8] + "@example.com"

		code, resp := write(
			srv,
			customerB(),
			"clientUsers.save",
			`{"customerId":"`+seed.ID("cust-b").String()+`","email":"`+email+
				`","clientRole":"member","displayName":"Resend Race"}`,
			0,
		)
		if code != 200 {
			t.Fatalf("%s: invite: %d %v", testCase.name, code, resp)
		}

		id := asStr(t, data(resp)["id"])
		owner(
			t,
			`UPDATE identity.client_users SET last_invite_sent_at = last_invite_sent_at - interval '10 minutes'
			WHERE id = $1`,
			id,
		)

		conn, err := pgx.Connect(ctx, testDB("postgres"))
		if err != nil {
			t.Skip(err)
		}

		tx, err := conn.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}

		args := []any{id}
		if testCase.key == "errors.access_expired" {
			args = append(args, clock().Add(-2*time.Hour), clock().Add(-time.Hour))
		}

		_, err = tx.Exec(ctx, testCase.change, args...)
		if err != nil {
			t.Fatal(err)
		}

		done := make(chan saveResult, 1)

		go func() {
			status, reply := write(
				srv,
				customerB(),
				"clientUsers.resendInvite",
				`{"id":"`+id+`"}`,
				0,
			)
			done <- saveResult{status, reply}
		}()

		waited := false

		var got saveResult
		select {
		case got = <-done:
			t.Errorf(
				"%s: the re-send did not wait for the change: %d %v",
				testCase.name,
				got.code,
				got.resp,
			)
		case <-time.After(700 * time.Millisecond):
			waited = true
		}

		err = tx.Commit(ctx)
		if err != nil {
			t.Fatal(err)
		}

		_ = conn.Close(ctx)

		if waited {
			got = <-done
		}

		if got.code != 409 || got.resp["messageKey"] != testCase.key {
			t.Errorf("%s: re-send after the change: %d %v", testCase.name, got.code, got.resp)
		}
	}
}
