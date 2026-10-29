package integration

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
	apiserver "github.com/pradita/ac-project/service/api/internal/server"
)

// accountMembership opens a membership of a new person for a user-list row of a test: every row has its account
// since IR461 (IR479). It is ended when the test ends.
func accountMembership(t *testing.T, org uuid.UUID, role string, clientRole any) string {
	t.Helper()
	id := uuid.NewString()
	owner(t, `INSERT INTO identity.memberships (id, tenant_id, user_id, organization_id, role, client_role, valid_from) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		id, seed.ID("tenant-a"), newUser(t), org, role, clientRole, clock.Add(-time.Hour))
	t.Cleanup(func() {
		owner(t, `UPDATE identity.memberships SET valid_until = $2 WHERE id = $1 AND (valid_until IS NULL OR valid_until > $2)`, id, clock.Add(-time.Minute))
	})
	return id
}

// race sends the two writes at once and expects one to pass and the other to be refused with the given message.
func race(t *testing.T, round int, want string, writes ...func() (int, map[string]any)) {
	t.Helper()
	raceRefused(t, round, want, false, writes...)
}

// raceRefused is race where the refused write may also be FORBIDDEN, when forbidden is true: a caller whose own right
// the other write took away is refused before the rule is reached (the split services authorize the second request
// after the first committed).
func raceRefused(t *testing.T, round int, want string, forbidden bool, writes ...func() (int, map[string]any)) {
	t.Helper()
	codes := make(chan int, len(writes))
	var wg sync.WaitGroup
	for _, w := range writes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			code, m := w()
			if code == 409 && m["messageKey"] != want {
				t.Errorf("round %d: refused for another reason: %v", round, m)
			}
			codes <- code
		}()
	}
	wg.Wait()
	close(codes)
	got := map[int]int{}
	for c := range codes {
		got[c]++
	}
	refused := got[409]
	if forbidden {
		refused += got[403]
	}
	if got[200] != 1 || refused != 1 {
		t.Fatalf("round %d: one change passes, the other keeps the last one (%s): %v", round, want, got)
	}
}

// TestLastOwnerAtOnce (IR471): a customer with two active owners; HQ demotes one and removes the other at the same
// moment. Each change alone keeps an owner, so without waiting for each other both passed and left none. The customer's
// other active owners (the seed's, and any another test added) are switched off for the test.
func TestLastOwnerAtOnce(t *testing.T) {
	s := server(t)
	cust := seed.ID("cust-b")
	var active []string
	ownerScan(t, `SELECT COALESCE(array_agg(id::text), '{}') FROM identity.client_users WHERE customer_id = $1 AND client_role = 'owner' AND status = 'active'`, []any{cust}, &active)
	owner(t, `UPDATE identity.client_users SET status = 'disabled' WHERE id = ANY($1::uuid[])`, active)
	t.Cleanup(func() {
		owner(t, `UPDATE identity.client_users SET status = 'active' WHERE id = ANY($1::uuid[])`, active)
	})
	for round := 0; round < 5; round++ {
		a, b := uuid.NewString(), uuid.NewString()
		for _, id := range []string{a, b} {
			owner(t, `INSERT INTO identity.client_users (id, tenant_id, customer_id, membership_id, email, client_role, status, invited_at, invited_by_membership_id)
				VALUES ($1, $2, $3, $4, $5, 'owner', 'active', $6, $7)`, id, seed.ID("tenant-a"), cust, accountMembership(t, seed.ID("org-customer-b"), "client", "owner"),
				"owner-"+id[:8]+"@example.com", clock, seed.ID("customer-b"))
		}
		t.Cleanup(func() { owner(t, `DELETE FROM identity.client_users WHERE id = ANY($1::uuid[])`, []string{a, b}) })
		race(t, round, "errors.last_owner",
			func() (int, map[string]any) {
				return write(s, &hq, "clientUsers.save", `{"id":"`+a+`","customerId":"`+cust.String()+`","email":"owner-`+a[:8]+`@example.com","clientRole":"member","status":"active","reason":"no longer an owner"}`, 1)
			},
			func() (int, map[string]any) {
				return write(s, &hq, "clientUsers.remove", `{"id":"`+b+`","reason":"left the company"}`, 1)
			})
		owner(t, `UPDATE identity.client_users SET status = 'disabled' WHERE id = ANY($1::uuid[]) AND client_role = 'owner'`, []string{a, b}) // the next round starts with no owner
	}
}

// TestLastContractorAtOnce (IR471): a company with two active contractor accounts; one is disabled and the other
// removed at the same moment. Each change alone keeps a contractor, so without waiting for each other both passed and
// left none. The company's other active contractor accounts (the caller's own, and any another test added) are
// switched off for the test.
func TestLastContractorAtOnce(t *testing.T) {
	s := server(t)
	org := seed.ID("org-contractor-a")
	var active []string
	ownerScan(t, `SELECT COALESCE(array_agg(id::text), '{}') FROM identity.partner_users WHERE organization_id = $1 AND partner_role = 'contractor' AND status = 'active'`, []any{org}, &active)
	owner(t, `UPDATE identity.partner_users SET status = 'disabled' WHERE id = ANY($1::uuid[])`, active)
	t.Cleanup(func() {
		owner(t, `UPDATE identity.partner_users SET status = 'active' WHERE id = ANY($1::uuid[])`, active)
	})
	for round := 0; round < 5; round++ {
		a, b := uuid.NewString(), uuid.NewString()
		for _, id := range []string{a, b} {
			owner(t, `INSERT INTO identity.partner_users (id, tenant_id, organization_id, membership_id, email, partner_role, status, invited_at, invited_by_membership_id)
				VALUES ($1, $2, $3, $4, $5, 'contractor', 'active', $6, $7)`, id, seed.ID("tenant-a"), org, accountMembership(t, org, "contractor", nil),
				"boss-"+id[:8]+"@example.com", clock, seed.ID("contractor-a"))
		}
		t.Cleanup(func() { owner(t, `DELETE FROM identity.partner_users WHERE id = ANY($1::uuid[])`, []string{a, b}) })
		race(t, round, "errors.last_contractor",
			func() (int, map[string]any) {
				return write(s, &contrA, "partnerUsers.save", `{"id":"`+a+`","email":"boss-`+a[:8]+`@example.com","partnerRole":"contractor","status":"disabled","reason":"on leave"}`, 1)
			},
			func() (int, map[string]any) {
				return write(s, &contrA, "partnerUsers.remove", `{"id":"`+b+`","reason":"left the company"}`, 1)
			})
		owner(t, `UPDATE identity.partner_users SET status = 'disabled' WHERE id = ANY($1::uuid[])`, []string{a, b}) // the next round starts with none active
	}
}

// identityAdmins leaves hq-operator and hq-restriction-manager the tenant's only identity.write holders for the test.
func identityAdmins(t *testing.T) {
	t.Helper()
	pair := []string{seed.ID("hq-operator").String(), seed.ID("hq-restriction-manager").String()}
	var others []string
	ownerScan(t, `WITH d AS (DELETE FROM identity.membership_permissions WHERE permission = 'identity.write' AND NOT (membership_id = ANY($1::uuid[])) RETURNING membership_id)
		SELECT COALESCE(array_agg(membership_id::text), '{}') FROM d`, []any{pair}, &others)
	t.Cleanup(func() {
		owner(t, `INSERT INTO identity.membership_permissions (tenant_id, membership_id, permission)
			SELECT tenant_id, id, 'identity.write' FROM identity.memberships WHERE id = ANY($1::uuid[]) ON CONFLICT DO NOTHING`, append(others, pair...))
	})
}

// adminSave is the members.save body that keeps an HQ membership as listed, with edit applied, and its version.
func adminSave(t *testing.T, s *apiserver.Server, id string, edit func(map[string]any)) (string, int) {
	t.Helper()
	_, m := post(s, &hq, "members.list", `{"filters":{"role":"admin"},"limit":100}`)
	for _, it := range items(m) {
		if it["id"] != id {
			continue
		}
		body := map[string]any{"id": id, "userId": it["userId"], "organizationId": it["organizationId"], "role": "admin", "employment": nil, "permissions": it["permissions"],
			"scopes": it["scopes"], "validFrom": it["validFrom"], "validUntil": it["validUntil"], "reason": "IR472 test"}
		edit(body)
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		return string(b), int(it["version"].(float64))
	}
	t.Fatalf("membership %s not listed", id)
	return "", 0
}

// withoutIdentityWrite takes identity.write from the body's permissions.
func withoutIdentityWrite(b map[string]any) {
	var kept []any
	for _, p := range b["permissions"].([]any) {
		if p != "identity.write" {
			kept = append(kept, p)
		}
	}
	b["permissions"] = kept
}

// TestLastIdentityAdminAtOnce (IR472): the tenant's last two identity.write holders take the permission from each other
// at the same moment. Each caller stays a holder in its own change, so without waiting for each other both passed and
// left no one who can manage access. The tenant's other holders lose the permission for the test.
func TestLastIdentityAdminAtOnce(t *testing.T) {
	s := server(t)
	identityAdmins(t)
	op, rm := seed.ID("hq-operator").String(), seed.ID("hq-restriction-manager").String()
	for round := 0; round < 5; round++ {
		opBody, opVer := adminSave(t, s, op, withoutIdentityWrite)
		rmBody, rmVer := adminSave(t, s, rm, withoutIdentityWrite)
		raceRefused(t, round, "errors.last_identity_admin", true, // each caller is one of the two: the second may have lost its right already
			func() (int, map[string]any) { return write(s, &hq, "members.save", rmBody, rmVer) },
			func() (int, map[string]any) { return write(s, &restrMgr, "members.save", opBody, opVer) })
		var holders int
		ownerScan(t, `SELECT count(*) FROM identity.membership_permissions WHERE membership_id = ANY($1::uuid[]) AND permission = 'identity.write'`, []any{[]string{op, rm}}, &holders)
		if holders != 1 {
			t.Fatalf("round %d: one of the two keeps identity.write: %d", round, holders)
		}
		owner(t, `INSERT INTO identity.membership_permissions (tenant_id, membership_id, permission)
			SELECT tenant_id, id, 'identity.write' FROM identity.memberships WHERE id = ANY($1::uuid[]) ON CONFLICT DO NOTHING`, []string{op, rm})
	}
}

// TestLastIdentityAdminStartLater (IR472): the last identity.write holder cannot move its own start later — it would stop
// being valid now, like ending it; another holder can.
func TestLastIdentityAdminStartLater(t *testing.T) {
	s := server(t)
	identityAdmins(t)
	op, rm := seed.ID("hq-operator").String(), seed.ID("hq-restriction-manager").String()
	var opFrom, rmFrom time.Time
	ownerScan(t, `SELECT (SELECT valid_from FROM identity.memberships WHERE id = $1), (SELECT valid_from FROM identity.memberships WHERE id = $2)`, []any{op, rm}, &opFrom, &rmFrom)
	t.Cleanup(func() {
		owner(t, `UPDATE identity.memberships SET valid_from = CASE id WHEN $1 THEN $2::timestamptz ELSE $4::timestamptz END WHERE id IN ($1, $3)`, op, opFrom, rm, rmFrom)
	})
	owner(t, `DELETE FROM identity.membership_permissions WHERE membership_id = $1 AND permission = 'identity.write'`, rm)
	later := func(b map[string]any) { b["validFrom"] = clock.Add(time.Hour).Format(time.RFC3339) }
	body, v := adminSave(t, s, op, later)
	if code, m := write(s, &hq, "members.save", body, v); code != 409 || m["messageKey"] != "errors.last_identity_admin" {
		t.Fatalf("the last holder starts later: %d %v", code, m)
	}
	owner(t, `INSERT INTO identity.membership_permissions (tenant_id, membership_id, permission) SELECT tenant_id, id, 'identity.write' FROM identity.memberships WHERE id = $1`, rm)
	body, v = adminSave(t, s, rm, later)
	if code, m := write(s, &hq, "members.save", body, v); code != 200 {
		t.Fatalf("another holder starts later: %d %v", code, m)
	}
}
