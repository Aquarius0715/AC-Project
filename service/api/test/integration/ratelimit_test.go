package integration

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/ratelimit"
	"github.com/pradita/ac-project/service/api/internal/seed"
	apiserver "github.com/pradita/ac-project/service/api/internal/server"
)

type downStore struct{}

func (downStore) Incr(context.Context, string, time.Duration) (int64, error) {
	return 0, errors.New("connection refused")
}

// TestWriteLimit: one membership's writes per minute are bounded, across every operation (network architecture §6,
// IR375). The write over the limit is RATE_LIMITED with the seconds to wait; other members, reads and a store that does
// not answer are not held up.
func TestWriteLimit(t *testing.T) {
	now := time.Date(2026, 10, 11, 9, 0, 20, 0, time.UTC)
	limited := func(store ratelimit.Store) *apiserver.Server {
		return serverCfg(t, func(c *apiserver.Config) {
			// one process for every domain, also in cluster mode: the cached split facade would not take this limit, and
			// the shared store, not the split, is what spans the services
			c.Domains = ops.Domains
			c.WriteLimit = &ratelimit.Window{Store: store, Limit: 3, Window: time.Minute, Prefix: "test:" + uuid.NewString() + ":", Now: func() time.Time { return now }}
		})
	}
	s := limited(&ratelimit.Memory{})
	prefs := `{"locale":"ms","timezone":"Asia/Singapore","monthlyReportEmail":true}`
	for i := 1; i <= 3; i++ {
		if code, m := write(s, &customerB, "preferences.update", prefs, 0); code != 200 {
			t.Fatalf("write %d: %d %v", i, code, m)
		}
	}
	// the fourth write of the minute, of any operation, waits until the minute ends (40 s after 09:00:20)
	req := httptest.NewRequest(ops.ClientRequest("consents.update", `{"purpose":"location_automation","granted":false}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+customerB.token)
	req.Header.Set("X-Tenant-Id", seed.ID("tenant-a").String())
	req.Header.Set("X-Membership-Id", seed.ID(customerB.membership).String())
	req.Header.Set("Idempotency-Key", "test-"+uuid.NewString())
	req.Header.Set("X-Expected-Version", "1")
	w := httptest.NewRecorder()
	s.Echo.ServeHTTP(w, req)
	if w.Code != 429 || w.Header().Get("Retry-After") != "40" {
		t.Fatalf("over the limit: %d Retry-After=%q %s", w.Code, w.Header().Get("Retry-After"), w.Body.String())
	}
	if code, m := write(s, &customerB, "preferences.update", prefs, 0); code != 429 || m["code"] != "RATE_LIMITED" || m["messageKey"] != "error.rateLimited" || m["retryAfterSeconds"] != float64(40) {
		t.Errorf("RATE_LIMITED body: %d %v", code, m)
	}
	// reads and other members go on
	if code, _ := post(s, &customerB, "session.get", `{}`); code != 200 {
		t.Errorf("read while limited: %d", code)
	}
	if code, m := write(s, &customerA, "preferences.update", `{"locale":"en","timezone":"Asia/Kuala_Lumpur","monthlyReportEmail":true}`, 0); code != 200 {
		t.Errorf("another member: %d %v", code, m)
	}
	// the next minute is a new window
	now = now.Add(time.Minute)
	if code, m := write(s, &customerB, "preferences.update", prefs, 0); code != 200 {
		t.Errorf("next minute: %d %v", code, m)
	}
	// a counter store that does not answer lets writes through (the edge limits still hold)
	down := limited(downStore{})
	for i := 1; i <= 5; i++ {
		if code, m := write(down, &customerB, "preferences.update", prefs, 0); code != 200 {
			t.Fatalf("store down, write %d: %d %v", i, code, m)
		}
	}
}
