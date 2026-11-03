package iotbridge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// errNoSnapshot is the preview's answer without the scenario clock's time.
var errNoSnapshot = errors.New("no snapshot time in the preview's answer")

// ScenarioNow reads the Core API's scenario clock: the snapshot time of the demo-only password-reset preview, which
// saves and sends nothing. The demo runs its own clock (a seed date that moves with real time), and readings and
// history must be stamped on it.
func ScenarioNow(client *http.Client, api string) (time.Time, error) {
	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		api+"/v1/auth/password-reset-preview",
		bytes.NewReader([]byte(`{"demoEmail":"clock.probe@iot-bridge.invalid"}`)),
	)
	if err != nil {
		return time.Time{}, apperr.Wrap(err, "http.NewRequestWithContext")
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return time.Time{}, apperr.Wrap(err, "http.Client.Do")
	}

	defer func() { _ = resp.Body.Close() }()

	var out struct {
		Meta struct {
			SnapshotAt time.Time `json:"snapshotAt"`
		} `json:"meta"`
	}

	decodeErr := json.NewDecoder(resp.Body).Decode(&out)
	if decodeErr != nil {
		return time.Time{}, apperr.Wrap(decodeErr, "json.Decoder.Decode")
	}

	if out.Meta.SnapshotAt.IsZero() {
		return time.Time{}, errNoSnapshot
	}

	return out.Meta.SnapshotAt, nil
}
