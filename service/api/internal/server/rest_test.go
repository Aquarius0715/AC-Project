package server

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/modules/identity"
	"github.com/pradita/ac-project/service/api/internal/ops"
)

// TestRESTRoutesResolve checks the route table of the assembled Core API (IR222) without a database (the pool
// connects lazily): every catalog operation has a handler, every route binds to its input type, and a request on a
// route — with IDs in the path — reaches that route's operation in the router, not a static or parameter neighbour.
func TestRESTRoutesResolve(t *testing.T) {
	srv, err := New(
		context.Background(),
		Config{
			DatabaseURL:       "postgres://x:x@127.0.0.1:1/x",
			DatabaseReaderURL: "",
			Addr:              "",
			Clock:             nil,
			DemoOps:           false,
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
	defer srv.DB.Close()

	if n := len(srv.Registry.Operations()); n != len(ops.Catalog) {
		t.Fatalf("%d handlers for %d catalog operations", n, len(ops.Catalog))
	}

	routes := 0

	for _, spec := range ops.Catalog {
		for _, route := range spec.Routes {
			routes++

			path := route.Path
			for range ops.PathParams(route.Path) {
				path = path[:strings.Index(path, "{")] + uuid.NewString() + path[strings.Index(path, "}")+1:]
			}

			ectx := srv.Echo.NewContext(
				httptest.NewRequest(route.Method, path, nil),
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
