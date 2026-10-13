package iotbridge

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

// ScenarioNow reads the Core API's scenario clock: the snapshot time of the demo-only password-reset preview, which
// saves and sends nothing. The demo runs its own clock (a seed date that moves with real time), and readings and history
// must be stamped on it.
func ScenarioNow(c *http.Client, api string) (time.Time, error) {
	req, err := http.NewRequest(http.MethodPost, api+"/v1/auth/password-reset-preview", bytes.NewReader([]byte(`{"demoEmail":"clock.probe@iot-bridge.invalid"}`)))
	if err != nil {
		return time.Time{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		return time.Time{}, err
	}
	defer resp.Body.Close()
	var out struct {
		Meta struct {
			SnapshotAt time.Time `json:"snapshotAt"`
		} `json:"meta"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return time.Time{}, err
	}
	if out.Meta.SnapshotAt.IsZero() {
		return time.Time{}, errors.New("no snapshot time in the preview's answer")
	}
	return out.Meta.SnapshotAt, nil
}
