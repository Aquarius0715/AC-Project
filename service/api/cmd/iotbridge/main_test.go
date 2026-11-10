package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/iotbridge"
	"github.com/pradita/ac-project/service/api/internal/ops"
)

// TestFetchWaitingReadsTheRequestedCommands: the internal query is called with the token and the tenant, and its rows
// come back as the waiting commands; a refused token, a bad answer and a dead API are errors.
func TestFetchWaitingReadsTheRequestedCommands(t *testing.T) {
	t.Parallel()

	commandID := uuid.New()

	srv := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/v1/system/queries/control.requestedCommands" ||
			r.Header.Get(ops.InternalTokenHeader) != "tok" ||
			r.Header.Get("X-Tenant-Id") != "t1" {
			writer.WriteHeader(http.StatusUnauthorized)

			return
		}

		_ = json.NewEncoder(writer).
			Encode(
				map[string]any{
					"data": []map[string]any{
						{
							"id":        commandID,
							"deviceId":  uuid.New(),
							"unitId":    uuid.New(),
							"action":    map[string]any{"kind": "set_temperature", "celsius": 26},
							"expiresAt": time.Now().Add(time.Minute),
						},
					},
				})
	}))
	defer srv.Close()

	client := &http.Client{Timeout: 5 * time.Second, Transport: nil, CheckRedirect: nil, Jar: nil}

	got, err := fetchWaiting(t.Context(), client, srv.URL, "tok", "t1")
	if err != nil || len(got) != 1 || got[0].ID != commandID {
		t.Fatalf("waiting: %v %v", got, err)
	}

	_, fetchWaitingErr := fetchWaiting(t.Context(), client, srv.URL, "wrong", "t1")
	if fetchWaitingErr == nil {
		t.Error("a refused token is not an error")
	}

	bad := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`not json`)) },
		),
	)
	defer bad.Close()

	_, fetchWaitingErr = fetchWaiting(t.Context(), client, bad.URL, "tok", "t1")
	if fetchWaitingErr == nil {
		t.Error("a bad answer is not an error")
	}

	_, fetchWaitingErr = fetchWaiting(t.Context(), client, "http://127.0.0.1:1", "tok", "t1")
	if fetchWaitingErr == nil {
		t.Error("a dead API is not an error")
	}
}

// TestDemoClockFollowsTheScenarioClock: the offset is the scenario time minus the real time of the preview's answer;
// an answer without a snapshot time is an error and keeps the old offset.
func TestDemoClockFollowsTheScenarioClock(t *testing.T) {
	t.Parallel()

	scenario := time.Date(2026, 9, 14, 22, 43, 0, 0, time.UTC)

	srv := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/auth/password-reset-preview" {
			writer.WriteHeader(http.StatusNotFound)

			return
		}

		_ = json.NewEncoder(writer).
			Encode(map[string]any{"meta": map[string]any{"snapshotAt": scenario}})
	}))
	defer srv.Close()

	clock := &demoClock{
		api: srv.URL,
		c:   &http.Client{Timeout: 5 * time.Second, Transport: nil, CheckRedirect: nil, Jar: nil},
		mu:  sync.Mutex{},
		off: 0,
	}

	err := clock.refresh(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	want := time.Until(scenario)
	if diff := clock.offset() - want; diff > time.Minute || diff < -time.Minute {
		t.Errorf("offset %v, want about %v", clock.offset(), want)
	}

	empty := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"meta":{}}`)) },
		),
	)
	defer empty.Close()

	keep := clock.offset()

	clock.api = empty.URL

	err = clock.refresh(t.Context())
	if err == nil {
		t.Error("an answer without a snapshot time is accepted")
	}

	if clock.offset() != keep {
		t.Error("a failed refresh changed the offset")
	}
}

// TestSendReportsRefusals: a trigger the API accepts is sent with the idempotency key; a refused one is an error.
func TestSendReportsRefusals(t *testing.T) {
	t.Parallel()

	var key string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key = r.Header.Get("Idempotency-Key")
		if key == uuid.Nil.String() { // the API refuses a reading it cannot key
			w.WriteHeader(http.StatusUnprocessableEntity)
		}
	}))
	defer srv.Close()

	client := &http.Client{Timeout: 5 * time.Second, Transport: nil, CheckRedirect: nil, Jar: nil}

	trigger := iotbridge.Trigger{
		EventID:     uuid.New(),
		EventType:   eventTelemetry,
		ScenarioID:  "",
		OccurredAt:  time.Time{},
		CommandID:   nil,
		Measurement: nil,
		DeviceID:    nil,
		State:       nil,
		Event:       nil,
		BindingID:   nil,
		Sequence:    nil,
		Kind:        "",
		Recovery:    nil,
	}

	err := send(t.Context(), client, srv.URL, trigger)
	if err != nil {
		t.Fatalf("accepted trigger: %v", err)
	}

	if key != trigger.EventID.String() {
		t.Errorf("idempotency key %q, want the event ID", key)
	}

	err = send(
		t.Context(),
		client,
		srv.URL,
		iotbridge.Trigger{
			EventID:     uuid.Nil,
			ScenarioID:  "",
			OccurredAt:  time.Time{},
			EventType:   "",
			CommandID:   nil,
			Measurement: nil,
			DeviceID:    nil,
			State:       nil,
			Event:       nil,
			BindingID:   nil,
			Sequence:    nil,
			Kind:        "",
			Recovery:    nil,
		},
	)
	if err == nil {
		t.Error("a refused trigger is not an error")
	}

	err = send(t.Context(), client, "http://127.0.0.1:1", trigger)
	if err == nil {
		t.Error("a dead API is not an error")
	}
}

// TestSendCommandStopsBeforePublishingABadCommand: a command with no action kind is refused before anything is
// published or reported sent.
func TestSendCommandStopsBeforePublishingABadCommand(t *testing.T) {
	t.Parallel()

	waiting := iotbridge.Waiting{
		ID:        uuid.New(),
		Action:    json.RawMessage(`{"celsius":26}`),
		ExpiresAt: time.Now().Add(time.Minute), DeviceID: uuid.UUID{}, UnitID: uuid.UUID{},
	}

	err := sendCommand(
		t.Context(),
		nil,
		&http.Client{Transport: nil, CheckRedirect: nil, Jar: nil, Timeout: 0},
		"http://127.0.0.1:1",
		uuid.New().String(),
		waiting,
		0,
	)
	if err == nil {
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

// TestDeviceKeyIsTheDeviceOfTheTopic: telemetry, heartbeat and acknowledgement topics of one device give the same key,
// so its messages stay in order on one worker; a topic of another shape is its own key.
func TestDeviceKeyIsTheDeviceOfTheTopic(t *testing.T) {
	t.Parallel()

	const device = "dev-9"

	for topic, want := range map[string]string{
		"t/tenant-1/d/dev-9/telemetry": device,
		"t/tenant-1/d/dev-9/heartbeat": device,
		"t/tenant-1/d/dev-9/cmd/ack":   device,
		"t/tenant-1/d":                 "t/tenant-1/d",
		"":                             "",
	} {
		if got := deviceKey(topic); got != want {
			t.Errorf("deviceKey(%q) = %q, want %q", topic, got, want)
		}
	}
}

// TestSendReusesItsConnections: sending is many calls a second from the workers, and a client that opens a connection
// for each one runs out of local ports (connect: cannot assign requested address) and loses readings and
// acknowledgements. So the workers share a few connections, however many calls they make.
func TestSendReusesItsConnections(t *testing.T) {
	t.Parallel()

	var opened atomic.Int64

	api := httptest.NewUnstartedServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"data":{}}`))
		}),
	)
	api.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			opened.Add(1)
		}
	}
	api.Start()

	defer api.Close()

	const (
		senders = 8
		each    = 50
	)

	client := newHTTPClient()

	var (
		running sync.WaitGroup
		trigger iotbridge.Trigger
	)

	for range senders {
		running.Go(func() {
			for range each {
				err := send(t.Context(), client, api.URL, trigger)
				if err != nil {
					t.Error(err)
				}
			}
		})
	}

	running.Wait()

	if got := opened.Load(); got > senders*2 {
		t.Errorf("%d connections opened for %d calls from %d senders", got, senders*each, senders)
	}
}
