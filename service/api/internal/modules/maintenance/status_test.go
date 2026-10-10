package maintenance

import (
	"testing"
	"time"
)

// TestReadStatus: a valid certificate reads as expiring within 30 days of its expiry and as expired from it on; a
// stored status other than valid reads as stored (IR133 item 2).
func TestReadStatus(t *testing.T) {
	now := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	day := 24 * time.Hour
	for name, c := range map[string]struct {
		stored  string
		expires time.Time
		want    string
	}{
		"valid":            {"valid", now.Add(31 * day), "valid"},
		"expiring at 30 d": {"valid", now.Add(30 * day), "expiring"},
		"expiring":         {"valid", now.Add(day), "expiring"},
		"expired at once":  {"valid", now, "expired"},
		"expired":          {"valid", now.Add(-day), "expired"},
		"rejected stays":   {"rejected", now.Add(31 * day), "rejected"},
		"pending stays":    {"pending_verification", now.Add(-day), "pending_verification"},
	} {
		if got := readStatus(c.stored, c.expires, now); got != c.want {
			t.Errorf("%s: %s, want %s", name, got, c.want)
		}
	}
}

// TestContentMatches: a file is what its leading bytes say — the PNG signature, the JPEG start of image, the PDF
// header — and a type outside the three never matches (IR308).
func TestContentMatches(t *testing.T) {
	png, jpeg, pdf := []byte("\x89PNG\r\n\x1a\nrest"), []byte{0xFF, 0xD8, 0xFF, 0xE0}, []byte("%PDF-1.7")
	for name, c := range map[string]struct {
		mime string
		b    []byte
		want bool
	}{
		"png":            {"image/png", png, true},
		"jpeg":           {"image/jpeg", jpeg, true},
		"pdf":            {"application/pdf", pdf, true},
		"pdf named png":  {"image/png", pdf, false},
		"png named jpeg": {"image/jpeg", png, false},
		"short":          {"image/png", png[:4], false},
		"other type":     {"image/gif", []byte("GIF89a"), false},
	} {
		if got := ContentMatches(c.mime, c.b); got != c.want {
			t.Errorf("%s: %v, want %v", name, got, c.want)
		}
	}
}
