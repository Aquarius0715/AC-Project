package apperr

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestStatusMapping(t *testing.T) {
	want := map[Code]int{
		Validation: 422, Unauthenticated: 401, Forbidden: 403, NotFound: 404, Conflict: 409,
		Offline: 409, Timeout: 504, RateLimited: 429, Unavailable: 503,
	}
	for _, c := range Codes {
		if got := E(c, "k").HTTPStatus(); got != want[c] {
			t.Errorf("%s: got %d want %d", c, got, want[c])
		}
	}
}

// The code list must equal the ErrorCode union in service-contracts.ts.
func TestCodesMatchServiceContracts(t *testing.T) {
	b, err := os.ReadFile("../../../../../docs/02-design/service-contracts.ts")
	if err != nil {
		t.Skip("contracts not available:", err)
	}
	m := regexp.MustCompile(`export type ErrorCode = ([^;]+);`).FindSubmatch(b)
	if m == nil {
		t.Fatal("ErrorCode not found")
	}
	var got []string
	for _, c := range Codes {
		got = append(got, "'"+string(c)+"'")
	}
	if strings.Join(got, "|") != string(m[1]) {
		t.Fatalf("codes drift: %s vs %s", strings.Join(got, "|"), m[1])
	}
}

func TestFrom(t *testing.T) {
	cases := []struct {
		err  error
		want Code
	}{
		{&pgconn.PgError{Code: "23514"}, Validation},
		{&pgconn.PgError{Code: "22P02"}, Validation},
		{&pgconn.PgError{Code: "42501", Message: "new row violates row-level security policy"}, NotFound},
		{&pgconn.PgError{Code: "42501", Message: "permission denied for table units"}, Unavailable},
		{&pgconn.PgError{Code: "23503"}, NotFound},
		{&pgconn.PgError{Code: "23505"}, Conflict},
		{&pgconn.PgError{Code: "23P01"}, Conflict},
		{&pgconn.PgError{Code: "40001"}, Unavailable},
		{&pgconn.PgError{Code: "40P01"}, Unavailable},
		{&pgconn.PgError{Code: "57014"}, Timeout},
		{&pgconn.PgError{Code: "XX000"}, Unavailable},
		{context.DeadlineExceeded, Timeout},
		{pgx.ErrNoRows, NotFound},
		{fmt.Errorf("wrap: %w", E(Offline, "error.offline")), Offline},
		{errors.New("boom"), Unavailable},
	}
	for _, c := range cases {
		if got := From(c.err).Code; got != c.want {
			t.Errorf("%v: got %s want %s", c.err, got, c.want)
		}
	}
	if From(nil) != nil {
		t.Error("nil must stay nil")
	}
	if r := From(&pgconn.PgError{Code: "40001"}).RetryAfterSeconds; r == nil || *r != 1 {
		t.Error("serialization failure must carry retryAfterSeconds")
	}
}

func TestFieldsAndError(t *testing.T) {
	e := Fields(map[string]string{"name": "error.required"})
	if e.Code != Validation || e.FieldErrors["name"] != "error.required" || e.HTTPStatus() != http.StatusUnprocessableEntity {
		t.Fatal("Fields")
	}
	if e.Error() != "VALIDATION: error.validation" {
		t.Fatal(e.Error())
	}
}
