package logging

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// IR369: what must never reach a log line is masked, whatever the code passes; IDs, dates and paths stay readable.
func TestMasking(t *testing.T) {
	var b bytes.Buffer
	log := New(&b, "test")
	log.Info("sign-in for aisha@example.com failed", "access_token", "eyJ.a.b", "Authorization", "Bearer x", "client_secret", "s3cr3t",
		"SESSION_SECRET", "k", "set_cookie", "c=1", "phone", "call +60 12-345 6789", "user", "01a12a68-3a58-720f-9409-a80b5e45cf73",
		"uri", "/v1/energy/summary?from=2026-09-01T00:00:00Z&to=2026-10-01T00:00:00Z", "latency_ms", 12,
		"err", errors.New(`connect postgres://ac_app_login:hunter2@db:5432/ac failed`))
	out := b.String()
	for _, leaked := range []string{"aisha@example.com", "eyJ.a.b", "Bearer x", "s3cr3t", `"k"`, "c=1", "+60 12-345 6789", "hunter2"} {
		if strings.Contains(out, leaked) {
			t.Errorf("%q reached the log: %s", leaked, out)
		}
	}
	for _, kept := range []string{`"service":"test"`, "01a12a68-3a58-720f-9409-a80b5e45cf73", "from=2026-09-01T00:00:00Z", `"latency_ms":12`,
		"postgres://ac_app_login:[masked]@db:5432/ac", "sign-in for [masked] failed", `"access_token":"[masked]"`, `"phone":"call [masked]"`} {
		if !strings.Contains(out, kept) {
			t.Errorf("%q missing from the log: %s", kept, out)
		}
	}
}
