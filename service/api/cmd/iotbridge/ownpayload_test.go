package main

import (
	"testing"

	"github.com/pradita/ac-project/service/api/internal/devicesim"
)

// TestOwnPayloadTakesOnlyTheTopicsDevice: a simulated module's ack is taken on its own device's topic; one that names
// another device, or does not decode, is dropped before it reaches the API.
func TestOwnPayloadTakesOnlyTheTopicsDevice(t *testing.T) {
	t.Parallel()

	const (
		topic = "t/6a1b5e0e-2f4a-4c51-9d0e-1c2b3a4d5e6f/d/0d1c2b3a-4e5f-4a6b-8c7d-9e0f1a2b3c4d/cmd/ack"
		own   = `{"deviceId":"0d1c2b3a-4e5f-4a6b-8c7d-9e0f1a2b3c4d","commandId":"c1","result":"applied"}`
		other = `{"deviceId":"1e2d3c4b-5f6a-4b7c-9d8e-0f1a2b3c4d5e","commandId":"c1","result":"applied"}`
	)

	take := func(payload string) (devicesim.Ack, bool) {
		var ack devicesim.Ack

		ok := ownPayload(
			message(topic, payload),
			"ack",
			&ack,
			func() string { return ack.DeviceID },
		)

		return ack, ok
	}

	ack, ok := take(own)
	if !ok || ack.CommandID != "c1" {
		t.Errorf("own device: %+v, %v", ack, ok)
	}

	_, otherOK := take(other)
	if otherOK {
		t.Error("an ack naming another device was taken")
	}

	_, badOK := take("{")
	if badOK {
		t.Error("an ack that does not decode was taken")
	}
}
