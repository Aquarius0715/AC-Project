package integration

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

// TestRequestedCommandsForTheGateway: the IoT gateway reads the commands waiting to be sent with the internal service
// token and the tenant; a command leaves the list once it is reported sent, and a request without the token is refused.
func TestRequestedCommandsForTheGateway(t *testing.T) {
	cl := newCluster(t)
	tenant := seed.ID("tenant-a").String()
	unit := seed.ID("unit-online-rto")
	device := seed.ID("device-online-rto")
	command := uuid.NewString()
	owner(t, `INSERT INTO control.commands (id, tenant_id, unit_id, device_id, actor_membership_id, source, action, status, delivery, requested_at, expires_at, correlation_id, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,'ui',$6,'requested','not_sent',$7,$8,'corr-requested',$7,$7)`,
		command, seed.ID("tenant-a"), unit, device, seed.ID("hq-operator"), `{"kind":"set_temperature","celsius":26}`, clock, clock.Add(5*time.Minute))

	askBody := func(token, input string) (int, []map[string]any) {
		req, _ := http.NewRequest(http.MethodPost, cl.urls[ops.DomainEquipment]+"/internal/v1/system/queries/control.requestedCommands", strings.NewReader(input))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set(ops.InternalTokenHeader, token)
		}
		req.Header.Set("X-Tenant-Id", tenant)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var body struct {
			Data []map[string]any `json:"data"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&body)
		return resp.StatusCode, body.Data
	}
	ask := func(token string) (int, []map[string]any) { return askBody(token, `{"limit":50}`) }
	find := func(rows []map[string]any) map[string]any {
		for _, r := range rows {
			if r["id"] == command {
				return r
			}
		}
		return nil
	}

	if code, _ := ask(""); code != 401 {
		t.Fatalf("without the internal token: %d", code)
	}
	// the query's input is validated as an operation's is (IR431): 1–100 commands
	for _, body := range []string{`{"limit":0}`, `{"limit":1000000}`} {
		if code, _ := askBody("test-internal", body); code != 422 {
			t.Errorf("%s: %d, want 422", body, code)
		}
	}
	code, rows := ask("test-internal")
	if code != 200 {
		t.Fatalf("requested commands: %d", code)
	}
	got := find(rows)
	if got == nil || got["deviceId"] != device.String() || got["unitId"] != unit.String() {
		t.Fatalf("the requested command is not listed for its device: %v", rows)
	}

	// reported sent (the gateway's command_sent): it leaves the list
	owner(t, `UPDATE control.commands SET status = 'sent', delivery = 'sent', sent_at = $2 WHERE id = $1`, command, clock)
	if _, rows = ask("test-internal"); find(rows) != nil {
		t.Fatalf("a sent command is still waiting: %v", rows)
	}
}
