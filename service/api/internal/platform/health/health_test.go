package health_test

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/pradita/ac-project/service/api/internal/platform/health"
)

// serve answers /healthz with status (and any other path with 404) and returns the server's port.
func serve(t *testing.T, status int) string {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			w.WriteHeader(http.StatusNotFound)

			return
		}

		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)

	parsed, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}

	return parsed.Port()
}

// hangUp returns the port of a listener that closes every connection before answering (the port stays taken for the
// test, so no other process can answer on it).
func hangUp(t *testing.T) string {
	t.Helper()

	var config net.ListenConfig

	listener, err := config.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = listener.Close() })

	go func() {
		for {
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}

			_ = conn.Close()
		}
	}()

	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}

	return port
}

func TestCheck(t *testing.T) {
	t.Parallel()

	healthy := serve(t, http.StatusOK)
	failing := serve(t, http.StatusServiceUnavailable)
	gone := hangUp(t)

	cases := []struct {
		name string
		addr string
		want int
	}{
		{"port only, 200", ":" + healthy, 0},
		{"host and port, 200", "0.0.0.0:" + healthy, 0},
		{"503", ":" + failing, 1},
		{"no answer", ":" + gone, 1},
		{"no port", "8080", 1},
		{"empty port", "localhost:", 1},
	}

	for _, tc := range cases {
		got := health.Check(tc.addr)
		if got != tc.want {
			t.Errorf("%s (%q): %d, want %d", tc.name, tc.addr, got, tc.want)
		}
	}
}
