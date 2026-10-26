package integration

import (
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// race sends the two writes at once and expects one to pass and the other to be refused with the given message.
func race(t *testing.T, round int, want string, writes ...func() (int, map[string]any)) {
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
	if got[200] != 1 || got[409] != 1 {
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
			owner(t, `INSERT INTO identity.client_users (id, tenant_id, customer_id, email, client_role, status, invited_at, invited_by_membership_id)
				VALUES ($1, $2, $3, $4, 'owner', 'active', $5, $6)`, id, seed.ID("tenant-a"), cust, "owner-"+id[:8]+"@example.com", clock, seed.ID("customer-b"))
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
			owner(t, `INSERT INTO identity.partner_users (id, tenant_id, organization_id, email, partner_role, status, invited_at, invited_by_membership_id)
				VALUES ($1, $2, $3, $4, 'contractor', 'active', $5, $6)`, id, seed.ID("tenant-a"), org, "boss-"+id[:8]+"@example.com", clock, seed.ID("contractor-a"))
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
