package integration_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/seed"
	apiserver "github.com/pradita/ac-project/service/api/internal/server"
)

// TestDemoTriggersDoNotWaitForTheirOwnConnections: a trigger runs on the connection of its operation's transaction.
// When it asked the pool for another one while holding that one, as many triggers at once as the pool has connections
// all waited for each other until their callers gave up, and every other request of the service waited behind them
// (the load test's iot-design with a parallel bridge: reads took 10 s, and 0.1% of the readings were stored).
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestDemoTriggersDoNotWaitForTheirOwnConnections(t *testing.T) {
	if clusterMode() {
		t.Skip("the test limits the pool of a server of its own")
	}

	srv := serverCfg(t, func(c *apiserver.Config) {
		c.DemoOps = true
		c.DatabaseURL = testDB("ac_app_login") + "&pool_max_conns=2"
	})
	device, unit := seed.ID("device-limited"), seed.ID("unit-limited")

	var seen string

	ownerScan(
		t,
		`SELECT COALESCE(last_seen_at::text, '') FROM assets.units WHERE id = $1`,
		[]any{unit},
		&seen,
	)
	t.Cleanup(func() {
		owner(
			t,
			`UPDATE assets.units SET last_seen_at = NULLIF($2, '')::timestamptz WHERE id = $1`,
			unit,
			seen,
		)
	})

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	const calls = 8

	codes, bodies := make([]int, calls), make([]string, calls)

	var running sync.WaitGroup

	for i := range calls {
		running.Go(func() {
			codes[i], bodies[i] = anonymousTrigger(ctx, srv, device)
		})
	}

	running.Wait()

	for i, code := range codes {
		if code != 200 {
			t.Errorf("trigger %d of %d answered %d, want 200: %s", i+1, calls, code, bodies[i])
		}
	}
}

// anonymousTrigger sends one state report as the bridge does, without a user, and returns the status.
func anonymousTrigger(ctx context.Context, srv *apiserver.Server, device uuid.UUID) (int, string) {
	event := uuid.NewString()
	body := `{"scenarioId":"t","eventId":"` + event + `","occurredAt":"` +
		clock().Add(-time.Minute).Format(time.RFC3339) +
		`","eventType":"state","deviceId":"` + device.String() +
		`","state":{"power":null,"celsius":null,"mode":null,"fanLevel":null}}`

	method, target, reqBody := ops.ClientRequest("demo.trigger", body)
	req := httptest.NewRequestWithContext(ctx, method, target, reqBody)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", event)

	rec := httptest.NewRecorder()
	srv.Echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		return rec.Code, rec.Body.String()
	}

	return rec.Code, ""
}
