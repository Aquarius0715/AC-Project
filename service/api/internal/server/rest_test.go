package server_test

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/modules/identity"
	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/server"
)

// assembled is the Core API assembled without a database: the URL is only parsed, as the pool connects lazily.
func assembled(t *testing.T) *server.Server {
	t.Helper()

	srv, err := server.New(
		t.Context(),
		server.Config{
			DatabaseURL:       "postgres://x@127.0.0.1:1/x",
			DatabaseReaderURL: "",
			Addr:              "",
			Clock:             nil,
			DemoOps:           false,
			DemoShared:        false,
			DemoStart:         time.Time{},
			Logger:            nil,
			Domains:           nil,
			IdentityURL:       "",
			InternalToken:     "",
			ServiceURLs:       nil,
			PrincipalTTL:      0,
			DemoClock:         nil,
			WriteLimit:        nil,
			DenyLimit:         nil,
			IdentityProvider:  nil,
			AccountApps: identity.AccountApps{
				Customer:   "",
				Partner:    "",
				Technician: "",
				Admin:      "",
			},
			AccountsPerDay: 0,
			WrapTx:         nil,
		},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(srv.DB.Close)

	return srv
}

// TestRESTRoutesResolve checks the route table of the assembled Core API (IR222): every catalog operation has a
// handler, every route binds to its input type, and a request on a route — with IDs in the path — reaches that
// route's operation in the router, not a static or parameter neighbour.
func TestRESTRoutesResolve(t *testing.T) {
	t.Parallel()

	srv := assembled(t)

	if n := len(srv.Registry.Operations()); n != len(ops.Catalog) {
		t.Fatalf("%d handlers for %d catalog operations", n, len(ops.Catalog))
	}

	routes := 0

	for _, spec := range ops.Catalog {
		for _, route := range spec.Routes {
			routes++

			path := route.Path
			for range ops.PathParams(route.Path) {
				before, rest, _ := strings.Cut(path, "{")
				_, after, _ := strings.Cut(rest, "}")
				path = before + uuid.NewString() + after
			}

			ectx := srv.Echo.NewContext(
				httptest.NewRequestWithContext(t.Context(), route.Method, path, nil),
				httptest.NewRecorder(),
			)
			srv.Echo.Router().Route(ectx)

			if got := ectx.RouteInfo().Name; got != spec.Name {
				t.Errorf("%s %s resolves to %q, want %s", route.Method, path, got, spec.Name)
			}
		}
	}

	if routes != 233 {
		t.Fatalf("%d routes, want 233", routes)
	}
}
