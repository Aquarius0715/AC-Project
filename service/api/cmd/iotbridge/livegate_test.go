package main

import (
	"testing"
	"time"
)

// TestLiveReadingsMinuteReopens: the minute of a node's readings of now is taken by the first message only when its
// readings reach the API. A message the API does not take, one without readings and one that is not JSON leave the
// minute open, so the node's next message of the same minute is recorded.
func TestLiveReadingsMinuteReopens(t *testing.T) {
	t.Parallel()

	const topic = "smarthome/ac/node_2/sensors"

	reading := `{"node_id":"node_2","temp":25.6,"humidity":43.8,"pm25":55}`

	cases := []struct {
		name   string
		first  string
		refuse int
	}{
		{name: "the API is away", first: reading, refuse: 1},
		{name: "no readings", first: `{"node_id":"node_2"}`, refuse: 0},
		{name: "not JSON", first: `{"node_id":`, refuse: 0},
	}

	for _, step := range cases {
		t.Run(step.name, func(t *testing.T) {
			t.Parallel()

			live := newLiveFixture(t)
			live.api.refuse = step.refuse

			start := time.Now()

			live.handler.handle(t.Context(), message(topic, step.first))

			if n := len(live.api.types()); n != 0 {
				t.Fatalf("the first message recorded %d triggers", n)
			}

			live.handler.handle(t.Context(), message(topic, reading))

			if !time.Now().Add(-clockMargin).Truncate(time.Minute).
				Equal(start.Add(-clockMargin).Truncate(time.Minute)) {
				t.Skip("the minute turned during the test")
			}

			if n := len(live.api.types()); n != 5 {
				t.Errorf(
					"the next message of the minute gave %d triggers, want its 3 readings, evaluation and heartbeat",
					n,
				)
			}
		})
	}
}
