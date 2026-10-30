package integration

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/seed"
	apiserver "github.com/pradita/ac-project/service/api/internal/server"
)

// TestAccountConsumerApart (IR511): the sign-in accounts and sessions have their own consumer, "identity-accounts". An
// event the "identity" consumer cannot apply holds back only that consumer: an account is still turned off at once.
// An account event "identity" applied before the split counts as applied and is not applied again.
func TestAccountConsumerApart(t *testing.T) {
	fake := &fakeIdP{}
	s := serverCfg(t, func(c *apiserver.Config) { c.IdentityProvider = fake; c.Domains = ops.Domains })
	ctx := context.Background()
	if err := s.DrainEvents(ctx); err != nil {
		t.Fatal(err)
	}
	tenant := seed.ID("tenant-a")
	user, membership := uuid.New(), uuid.New()
	owner(t, `INSERT INTO identity.users (id, email, display_name, sign_in_membership_id) VALUES ($1, $2, 'Split Test', $3)`, user, "split-"+user.String()[:8]+"@example.com", membership)
	owner(t, `INSERT INTO identity.memberships (id, tenant_id, user_id, organization_id, role, employment, valid_from, valid_until) VALUES ($1,$2,$3,$4,'technician','external',$5,$6)`,
		membership, tenant, user, seed.ID("org-contractor-b"), clock.Add(-24*time.Hour), clock.Add(10*24*time.Hour))
	emit := func(kind, payload string) uuid.UUID {
		var id uuid.UUID
		ownerScan(t, `INSERT INTO platform.outbox (tenant_id, aggregate_type, aggregate_id, event_type, payload, correlation_id) VALUES ($1, 'membership', $2, $3, $4::jsonb, 'test-ir511') RETURNING id`,
			[]any{tenant, membership, kind, payload}, &id)
		return id
	}
	// applied by "identity" before the split: not again
	before := emit("AccountEnabled", `{"membershipId":"`+membership.String()+`","enabled":true}`)
	owner(t, `INSERT INTO platform.processed_events (consumer, event_id) VALUES ('identity', $1)`, before)
	// an audit entry "identity" cannot write (no such actor role): it keeps failing and holds that consumer back
	bad := uuid.New()
	ownerScan(t, `INSERT INTO platform.outbox (tenant_id, aggregate_type, aggregate_id, event_type, payload, correlation_id) VALUES ($1, 'audit', $2, 'AuditRecorded', $3::jsonb, 'test-ir511') RETURNING id`,
		[]any{tenant, bad, `{"id":"` + bad.String() + `","actorId":"split-test","actorRole":"nobody","action":"split.test","targetKind":"test","targetId":"x","occurredAt":"` + clock.Format(time.RFC3339) + `","correlationId":"test-ir511","result":"success"}`}, &bad)
	t.Cleanup(func() {
		owner(t, `INSERT INTO platform.processed_events (consumer, event_id) VALUES ('identity', $1) ON CONFLICT DO NOTHING`, bad)
		owner(t, `DELETE FROM platform.event_failures WHERE event_id = $1`, bad)
	})
	emit("AccountEnabled", `{"membershipId":"`+membership.String()+`","enabled":false}`)

	if err := s.DrainEvents(ctx); err == nil {
		t.Fatal("the audit entry that cannot be written was applied")
	}
	fake.mu.Lock()
	got := slices.Clone(fake.switched)
	fake.mu.Unlock()
	if !slices.Contains(got, user.String()+"=false") || slices.Contains(got, user.String()+"=true") {
		t.Errorf("the account events: %v (want only %s=false)", got, user)
	}
}
