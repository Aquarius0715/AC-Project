package integration_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/ops"
	apiserver "github.com/pradita/ac-project/service/api/internal/server"
)

// demo.* is served to a signed-in session (the demo panel) or to the service token's caller (the IoT bridge), never to
// an anonymous request (IR768, B-30); and a command result that names another device than the command's is refused.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestDemoOperationsNeedASessionOrTheService(t *testing.T) {
	srv := serverCfg(t, func(c *apiserver.Config) {
		c.DemoOps = true
		c.InternalToken = internalToken
	})
	backwards := `{"to":"2000-01-01T00:00:00Z"}` // reaches the handler without moving the clock: errors.clock_backwards

	for _, testCase := range []struct {
		name, token string
		want        int
	}{
		{"anonymous", "", http.StatusUnauthorized},
		{"a wrong service token", "not-the-token", http.StatusUnauthorized},
		{"the service token", internalToken, http.StatusUnprocessableEntity},
	} {
		code, body := demoCall(
			context.Background(),
			srv,
			"demo.advanceClock",
			backwards,
			testCase.token,
		)
		if code != testCase.want {
			t.Errorf(
				"%s: demo.advanceClock answered %d %s, want %d",
				testCase.name,
				code,
				body,
				testCase.want,
			)
		}
	}

	if code, res := write(
		srv,
		hq(),
		"demo.advanceClock",
		backwards,
		0,
	); code != http.StatusUnprocessableEntity ||
		fields(res) != "to=errors.clock_backwards" {
		t.Errorf("a signed-in session: %d %v", code, res)
	}

	unit := boundUnit(t, srv, "online")
	_, detail := post(srv, customerB(), "units.get", `{"id":"`+unit+`"}`)

	code, res := write(
		srv,
		customerB(),
		"commands.create",
		`{"unitId":"`+unit+`","action":{"kind":"set_power","power":true},"expectedUnitVersion":`+itoa(
			ver(t, detail),
		)+`}`,
		0,
	)
	if code != http.StatusOK {
		t.Fatalf("command: %d %v", code, res)
	}

	cmd := asStr(t, data(res)["id"])
	keepObserved(t, cmd)

	ack := `{"scenarioId":"t","eventId":"` + uuid.NewString() + `","occurredAt":"` +
		clock().Add(time.Second).
			Format(time.RFC3339) +
		`","eventType":"command_ack","commandId":"` + cmd +
		`","deviceId":"` + uuid.NewString() + `","sequence":2}`
	if code, res := write(
		srv,
		customerB(),
		"demo.trigger",
		ack,
		0,
	); code != http.StatusUnprocessableEntity ||
		fields(res) != "deviceId=errors.command_of_other_device" {
		t.Errorf("an acknowledgement naming another device: %d %v", code, res)
	}
}

// demoCall sends one demo operation without a user, with the service token when token is set, and returns the status
// and the body.
func demoCall(
	ctx context.Context,
	srv *apiserver.Server,
	operation, body, token string,
) (int, string) {
	method, target, reqBody := ops.ClientRequest(operation, body)
	req := httptest.NewRequestWithContext(ctx, method, target, reqBody)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.NewString())

	if token != "" {
		req.Header.Set(ops.InternalTokenHeader, token)
	}

	rec := httptest.NewRecorder()
	srv.Echo.ServeHTTP(rec, req)

	return rec.Code, rec.Body.String()
}
