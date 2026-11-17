package iotbridge_test

import (
	"testing"

	"github.com/pradita/ac-project/service/api/internal/iotbridge"
)

// TestOwnTopic: a module's message is taken only on its own device's topic (network-architecture F09); a payload that
// names another device, a topic of another shape or an unparsable device is refused.
func TestOwnTopic(t *testing.T) {
	t.Parallel()

	const (
		tenant = "6a1b5e0e-2f4a-4c51-9d0e-1c2b3a4d5e6f"
		device = "0d1c2b3a-4e5f-4a6b-8c7d-9e0f1a2b3c4d"
		other  = "1e2d3c4b-5f6a-4b7c-9d8e-0f1a2b3c4d5e"
		upper  = "0D1C2B3A-4E5F-4A6B-8C7D-9E0F1A2B3C4D"
		bad    = "bad-id"
	)

	cases := []struct {
		topic, device string
		ok            bool
	}{
		{"t/" + tenant + "/d/" + device + "/telemetry", device, true},
		{"t/" + tenant + "/d/" + device + "/cmd/ack", device, true},
		{"t/" + tenant + "/d/" + upper + "/heartbeat", device, true},
		{"t/" + tenant + "/d/" + device + "/telemetry", other, false},
		{"t/" + tenant + "/d/" + device + "/cmd/ack", "", false},
		{"t/" + tenant + "/d/" + device + "/cmd/ack", bad, false},
		{"t/" + tenant + "/d/" + bad + "/telemetry", bad, false},
		{"t/" + tenant + "/x/" + device + "/telemetry", device, false},
		{"x/" + tenant + "/d/" + device + "/telemetry", device, false},
		{"t/" + tenant + "/d/" + device, device, false},
		{device, device, false},
	}

	for _, tc := range cases {
		err := iotbridge.OwnTopic(tc.topic, tc.device)
		if (err == nil) != tc.ok {
			t.Errorf("OwnTopic(%q, %q) = %v, want ok %v", tc.topic, tc.device, err, tc.ok)
		}
	}
}
