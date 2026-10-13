package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/iotbridge"
	"github.com/pradita/ac-project/service/api/internal/ops"
)

// TestFetchWaitingReadsTheRequestedCommands: the internal query is called with the token and the tenant, and its rows
// come back as the waiting commands; a refused token, a bad answer and a dead API are errors.
func TestFetchWaitingReadsTheRequestedCommands(t *testing.T) {
	id := uuid.New()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/v1/system/queries/control.requestedCommands" || r.Header.Get(ops.InternalTokenHeader) != "tok" || r.Header.Get("X-Tenant-Id") != "t1" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"id": id, "deviceId": uuid.New(), "unitId": uuid.New(), "action": map[string]any{"kind": "set_temperature", "celsius": 26}, "expiresAt": time.Now().Add(time.Minute)}}})
	}))
	defer srv.Close()
	c := &http.Client{Timeout: 5 * time.Second}
	got, err := fetchWaiting(c, srv.URL, "tok", "t1")
	if err != nil || len(got) != 1 || got[0].ID != id {
		t.Fatalf("waiting: %v %v", got, err)
	}
	if _, err := fetchWaiting(c, srv.URL, "wrong", "t1"); err == nil {
		t.Error("a refused token is not an error")
	}
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`not json`)) }))
	defer bad.Close()
	if _, err := fetchWaiting(c, bad.URL, "tok", "t1"); err == nil {
		t.Error("a bad answer is not an error")
	}
	if _, err := fetchWaiting(c, "http://127.0.0.1:1", "tok", "t1"); err == nil {
		t.Error("a dead API is not an error")
	}
}

// TestDemoClockFollowsTheScenarioClock: the offset is the scenario time minus the real time of the preview's answer;
// an answer without a snapshot time is an error and keeps the old offset.
func TestDemoClockFollowsTheScenarioClock(t *testing.T) {
	scenario := time.Date(2026, 9, 14, 22, 43, 0, 0, time.UTC)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/auth/password-reset-preview" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"meta": map[string]any{"snapshotAt": scenario}})
	}))
	defer srv.Close()
	d := &demoClock{api: srv.URL, c: &http.Client{Timeout: 5 * time.Second}}
	if err := d.refresh(); err != nil {
		t.Fatal(err)
	}
	want := time.Until(scenario)
	if diff := d.offset() - want; diff > time.Minute || diff < -time.Minute {
		t.Errorf("offset %v, want about %v", d.offset(), want)
	}
	empty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"meta":{}}`)) }))
	defer empty.Close()
	keep := d.offset()
	d.api = empty.URL
	if err := d.refresh(); err == nil {
		t.Error("an answer without a snapshot time is accepted")
	}
	if d.offset() != keep {
		t.Error("a failed refresh changed the offset")
	}
}

// TestSendReportsRefusals: a trigger the API accepts is sent with the idempotency key; a refused one is an error.
func TestSendReportsRefusals(t *testing.T) {
	var key string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key = r.Header.Get("Idempotency-Key")
		if key == uuid.Nil.String() { // the API refuses a reading it cannot key
			w.WriteHeader(http.StatusUnprocessableEntity)
		}
	}))
	defer srv.Close()
	c := &http.Client{Timeout: 5 * time.Second}
	tr := iotbridge.Trigger{EventID: uuid.New(), EventType: "telemetry"}
	if err := send(c, srv.URL, tr); err != nil {
		t.Fatalf("accepted trigger: %v", err)
	}
	if key != tr.EventID.String() {
		t.Errorf("idempotency key %q, want the event ID", key)
	}
	if err := send(c, srv.URL, iotbridge.Trigger{EventID: uuid.Nil}); err == nil {
		t.Error("a refused trigger is not an error")
	}
	if err := send(c, "http://127.0.0.1:1", tr); err == nil {
		t.Error("a dead API is not an error")
	}
}

// TestSendCommandStopsBeforePublishingABadCommand: a command with no action kind is refused before anything is published
// or reported sent.
func TestSendCommandStopsBeforePublishingABadCommand(t *testing.T) {
	w := iotbridge.Waiting{ID: uuid.New(), Action: json.RawMessage(`{"celsius":26}`), ExpiresAt: time.Now().Add(time.Minute)}
	if err := sendCommand(nil, &http.Client{}, "http://127.0.0.1:1", uuid.New().String(), w, 0); err == nil {
		t.Error("a command without an action kind is sent")
	}
}

func TestEnvAndTokenHelpers(t *testing.T) {
	t.Setenv("IOT_TEST_VALUE", "x")
	if envOr("IOT_TEST_VALUE", "d") != "x" || envOr("IOT_TEST_UNSET", "d") != "d" {
		t.Error("envOr")
	}
	t.Setenv("INTERNAL_API_TOKEN", "tok")
	if internalToken() != "tok" {
		t.Error("internalToken")
	}
	_ = context.Background()
}
