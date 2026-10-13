package iotbridge

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestScenarioNowReadsThePreviewSnapshot: the scenario time is the preview's snapshot time; an answer without one, a bad
// answer and a dead API are errors.
func TestScenarioNowReadsThePreviewSnapshot(t *testing.T) {
	scenario := time.Date(2026, 9, 15, 0, 53, 0, 0, time.UTC)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/auth/password-reset-preview" || r.Method != http.MethodPost {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"meta": map[string]any{"snapshotAt": scenario}})
	}))
	defer srv.Close()
	c := &http.Client{Timeout: 5 * time.Second}
	got, err := ScenarioNow(c, srv.URL)
	if err != nil || !got.Equal(scenario) {
		t.Fatalf("scenario now %v, err %v", got, err)
	}
	for name, body := range map[string]string{"no snapshot": `{"meta":{}}`, "not json": `not json`} {
		bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) }))
		if _, err := ScenarioNow(c, bad.URL); err == nil {
			t.Errorf("%s: accepted", name)
		}
		bad.Close()
	}
	if _, err := ScenarioNow(c, "http://127.0.0.1:1"); err == nil {
		t.Error("a dead API is not an error")
	}
}
