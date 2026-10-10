package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// IR268: the token's sign-in time (OIDC auth_time) makes the user's last sign-in, in business time, and the
// customer's Users list shows it. Further requests of that sign-in and the token of an older session leave it alone;
// a later sign-in moves it — directly on identity (session.get) and through another service, whose principal comes
// from identity-api in cluster mode (units.list).
func TestLastSignIn(t *testing.T) {
	s := server(t)
	ctx := context.Background()
	owner, err := pgx.Connect(ctx, testDB("postgres"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Close(ctx) }) // registered first, so it closes after the reset below
	user := seed.ID("user-customer-a")
	exec := func(sql string, args ...any) {
		if _, err := owner.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	read := func() (last, auth *time.Time) {
		if err := owner.QueryRow(ctx, `SELECT last_sign_in_at, sign_in_auth_time FROM identity.users WHERE id = $1`, user).Scan(&last, &auth); err != nil {
			t.Fatal(err)
		}
		return last, auth
	}
	reset := func() {
		exec(`UPDATE identity.users SET last_sign_in_at = NULL, sign_in_auth_time = NULL WHERE id = $1`, user)
	}
	reset()
	t.Cleanup(reset)
	earlier := clock.Add(-30 * 24 * time.Hour)
	mark := func() { exec(`UPDATE identity.users SET last_sign_in_at = $2 WHERE id = $1`, user, earlier) } // to see whether a request writes
	signedIn := func(at time.Time) *actor { return &actor{fmt.Sprintf("tok-a@%d", at.Unix()), "customer-a"} }
	request := func(a *actor, op string) {
		t.Helper()
		body := map[string]string{"session.get": `{}`, "units.list": `{"limit":1}`}[op]
		if code, m := post(s, a, op, body); code != 200 {
			t.Fatalf("%s: %d %v", op, code, m)
		}
	}

	t1 := clock.Add(-2 * time.Hour)
	request(signedIn(t1), "session.get")
	last, auth := read()
	if last == nil || last.Before(clock) || auth == nil || !auth.Equal(t1) {
		t.Fatalf("first sign-in: last %v, auth_time %v", last, auth)
	}
	_, m := post(s, &customerA, "clientUsers.list", `{"limit":100}`)
	shown := ""
	for _, it := range items(m) {
		if it["membershipId"] == seed.ID("customer-a").String() {
			shown, _ = it["lastSignInAt"].(string)
		}
	}
	if at, err := time.Parse(time.RFC3339Nano, shown); err != nil || !at.Equal(*last) {
		t.Fatalf("Users list: %q, want %v", shown, last)
	}

	mark()
	request(signedIn(t1), "session.get")                 // the same sign-in again
	request(signedIn(t1.Add(-time.Hour)), "session.get") // an older session's token
	request(&customerA, "session.get")                   // a token without a sign-in time
	if last, _ := read(); last == nil || !last.Equal(earlier) {
		t.Fatalf("must not move: %v", last)
	}
	t2 := t1.Add(time.Hour)
	request(signedIn(t2), "session.get")
	if last, auth := read(); last == nil || last.Equal(earlier) || auth == nil || !auth.Equal(t2) {
		t.Fatalf("a later sign-in: %v %v", last, auth)
	}
	mark()
	t3 := t2.Add(time.Minute)
	request(signedIn(t3), "units.list") // another service: identity-api records it through the principal call
	if last, auth := read(); last == nil || last.Equal(earlier) || auth == nil || !auth.Equal(t3) {
		t.Fatalf("through another service: %v %v", last, auth)
	}
}
