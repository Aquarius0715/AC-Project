package devices

import (
	"maps"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

func TestWaveOf(t *testing.T) {
	waves := []Wave{{"pilot", 20}, {"half", 50}, {"all", 100}}

	got := []int{}
	for i := range 10 {
		got = append(got, waveOf(i, 10, waves))
	}

	want := []int{0, 0, 1, 1, 1, 2, 2, 2, 2, 2}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}

	if waveOf(0, 1, []Wave{{"all", 100}}) != 0 {
		t.Fatal("single device")
	}
}

// TestInWindow: the install window is Kuala Lumpur local time, from its start up to (not including) its end; a window
// that ends earlier than it starts runs over midnight (IR339).
func TestInWindow(t *testing.T) {
	kl := func(h, m int) time.Time { return time.Date(2026, 9, 14, h, m, 0, 0, time.UTC).Add(-8 * time.Hour) }
	for name, c := range map[string]struct {
		now      time.Time
		from, to string
		want     bool
	}{
		"start":          {kl(1, 0), "01:00", "05:00", true},
		"inside":         {kl(4, 59), "01:00", "05:00", true},
		"end":            {kl(5, 0), "01:00", "05:00", false},
		"before":         {kl(0, 59), "01:00", "05:00", false},
		"over midnight":  {kl(23, 30), "22:00", "02:00", true},
		"after midnight": {kl(1, 59), "22:00", "02:00", true},
		"midnight end":   {kl(2, 0), "22:00", "02:00", false},
		"daytime":        {kl(12, 0), "22:00", "02:00", false},
	} {
		if got := inWindow(c.now, c.from, c.to); got != c.want {
			t.Errorf("%s: %v", name, got)
		}
	}
}

// TestOutcome: a finished firmware operation becomes the device's campaign result; busy or offline at the start skips
// it, no answer or a reported failure fails it (it keeps its old version).
func TestOutcome(t *testing.T) {
	for _, c := range []struct{ status, code, result, key string }{
		{"succeeded", "", "succeeded", ""},
		{"failed", "CONFLICT", "skipped", "errors.device_busy"},
		{"failed", "OFFLINE", "skipped", "errors.device_offline"},
		{"failed", "TIMEOUT", "failed", "errors.install_timeout"},
		{"failed", "UNAVAILABLE", "failed", "errors.install_failed"},
	} {
		if r, k := outcome(c.status, c.code); r != c.result || k != c.key {
			t.Errorf("%s %s: %s %s", c.status, c.code, r, k)
		}
	}
}

// TestWavePassed: the next wave starts once 95 % of a wave's devices succeeded; an empty wave passes.
func TestWavePassed(t *testing.T) {
	for _, c := range []struct {
		w    waveCount
		want bool
	}{
		{waveCount{"succeeded": 19, "failed": 1}, true},
		{waveCount{"succeeded": 18, "skipped": 1, "pending": 1}, false},
		{waveCount{"succeeded": 4, "failed": 1}, false},
		{waveCount{"succeeded": 5}, true},
		{waveCount{}, true},
	} {
		if got := c.w.passed(); got != c.want {
			t.Errorf("%v: %v", c.w, got)
		}
	}
}

// TestControlInput: firmwareCampaigns.control pauses, resumes, aborts with a reason of 1–1000 characters or retries one
// device (IR111). Over REST the action and the device come from the route, so an unknown action or a retry without a
// device reaches this rule only from the other callers of the operation.
func TestControlInput(t *testing.T) {
	camp, dev := uuid.New(), uuid.New()

	reason, blank := "vendor recall", "   "
	for name, c := range map[string]struct {
		in   ControlInput
		want map[string]string
	}{
		"pause": {
			ControlInput{CampaignID: camp, Action: "pause", DeviceID: nil, Reason: nil},
			map[string]string{},
		},
		"resume": {
			ControlInput{CampaignID: camp, Action: "resume", DeviceID: nil, Reason: nil},
			map[string]string{},
		},
		"abort": {
			ControlInput{CampaignID: camp, Action: "abort", Reason: &reason, DeviceID: nil},
			map[string]string{},
		},
		"abort without reason": {
			ControlInput{CampaignID: camp, Action: "abort", DeviceID: nil, Reason: nil},
			map[string]string{"reason": apperr.KeyLength},
		},
		"abort blank reason": {
			ControlInput{CampaignID: camp, Action: "abort", Reason: &blank, DeviceID: nil},
			map[string]string{"reason": apperr.KeyLength},
		},
		"retry a device": {
			ControlInput{CampaignID: camp, Action: "retry_device", DeviceID: &dev, Reason: nil},
			map[string]string{},
		},
		"retry without device": {
			ControlInput{CampaignID: camp, Action: "retry_device", DeviceID: nil, Reason: nil},
			map[string]string{"deviceId": apperr.KeyRequired},
		},
		"unknown action": {
			ControlInput{CampaignID: camp, Action: "restart", DeviceID: nil, Reason: nil},
			map[string]string{"action": apperr.KeyInvalid},
		},
		"no campaign": {
			ControlInput{Action: "pause", CampaignID: uuid.UUID{}, DeviceID: nil, Reason: nil},
			map[string]string{"campaignId": apperr.KeyRequired},
		},
	} {
		in := c.in
		if got := in.Validate(); !maps.Equal(got, c.want) {
			t.Errorf("%s: %v, want %v", name, got, c.want)
		}
	}
}
