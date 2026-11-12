package auth_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/auth"
)

// errConnRefused is what the pool says while the database is down.
var errConnRefused = errors.New("dial tcp: connection refused")

// failingRunner is a database that cannot be reached (or has no such row).
type failingRunner struct{ err error }

func (f failingRunner) Run(context.Context, bool, *ops.Principal, func(pgx.Tx) error) error {
	return f.err
}

// TestMiddlewareDatabaseOutage: a membership that cannot be read because the database is down is a 503 the client
// retries, not a 401 that tells the BFF the session is over (a sign-out for everybody while the database restarts).
// A row that is not there stays a 401.
func TestMiddlewareDatabaseOutage(t *testing.T) {
	t.Parallel()

	headers := membershipHeaders(tokenHQ, uuid.NewString(), uuid.NewString())
	cases := map[string]struct {
		err    error
		status int
	}{
		"connection refused":  {errConnRefused, http.StatusServiceUnavailable},
		"no such membership":  {pgx.ErrNoRows, http.StatusUnauthorized},
		"remote says unknown": {auth.ErrNoMembership, http.StatusUnauthorized},
	}

	for name, outcome := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			authenticator := &auth.Authenticator{
				Verifier: auth.StaticVerifier{tokenHQ: uuid.NewString()},
				DB:       failingRunner{err: outcome.err},
				Now:      scenarioClock,
				Source:   nil,
			}

			if w, _ := call(t, authenticator, headers); w.Code != outcome.status {
				t.Fatalf("status %d, want %d", w.Code, outcome.status)
			}
		})
	}
}
