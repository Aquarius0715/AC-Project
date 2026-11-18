package devices

import "testing"

// The restriction_bypass Alert names each fault kind; the web words these texts in the display language
// (shared/lib/evidence.ts, IR847), so a changed text would show in English.
func TestBypassFault(t *testing.T) {
	t.Parallel()

	want := map[string]string{
		signalCommunicationLost: "stopped reporting",
		signalPowerLost:         "lost power",
		signalTamper:            "reported its cover opened",
	}
	for kind, text := range want {
		if got := bypassFault(kind); got != text {
			t.Errorf("bypassFault(%q) = %q, want %q", kind, got, text)
		}
	}
}
