// Package health is the container healthcheck subcommand that every service binary shares (container design §3).
package health

import (
	"context"
	"net/http"
	"time"
)

// timeout bounds the healthcheck's request (compose.yaml gives the healthcheck 3 s).
const timeout = 3 * time.Second

// Check asks the process's own /healthz on 127.0.0.1 at the port of addr (":8080", as ADDR sets it) and returns the
// exit code: 0 when the answer is 200 OK, 1 otherwise.
func Check(addr string) int {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		"http://127.0.0.1"+addr+"/healthz",
		nil,
	)
	if err != nil {
		return 1
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 1
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return 1
	}

	return 0
}
