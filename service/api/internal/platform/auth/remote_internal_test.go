package auth

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestRemoteSourceCacheDropsStaleEntries: identity-api's answers (also "no such membership") are kept for the TTL
// only, so a caller that sends one made-up membership ID after another does not grow the cache without end.
func TestRemoteSourceCacheDropsStaleEntries(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)

	clock := time.Date(2026, 9, 14, 1, 0, 0, 0, time.UTC)
	source := &RemoteSource{
		BaseURL: server.URL,
		Token:   "internal",
		Client:  server.Client(),
		TTL:     30 * time.Second,
		Now:     func() time.Time { return clock },
		mu:      sync.Mutex{},
		cache:   nil,
	}

	for range 2 * cacheSweepAt {
		principal, err := source.Principal(t.Context(), "sub", uuid.New(), uuid.New(), time.Time{})
		if err == nil || principal != nil {
			t.Fatal("an unknown membership is refused")
		}
	}

	clock = clock.Add(31 * time.Second)

	_, err := source.Principal(t.Context(), "sub", uuid.New(), uuid.New(), time.Time{})
	if err == nil {
		t.Fatal("an unknown membership is refused")
	}

	if len(source.cache) != 1 {
		t.Fatalf("cache holds %d entries after the TTL, want 1", len(source.cache))
	}
}
