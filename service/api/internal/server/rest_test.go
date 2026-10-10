package server

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/ops"
)

// TestRESTRoutesResolve checks the route table of the assembled Core API (IR222) without a database (the pool
// connects lazily): every catalog operation has a handler, every route binds to its input type, and a request on a
// route — with IDs in the path — reaches that route's operation in the router, not a static or parameter neighbour.
func TestRESTRoutesResolve(t *testing.T) {
	srv, err := New(context.Background(), Config{DatabaseURL: "postgres://x:x@127.0.0.1:1/x"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.DB.Close()
	if n := len(srv.Registry.Operations()); n != len(ops.Catalog) {
		t.Fatalf("%d handlers for %d catalog operations", n, len(ops.Catalog))
	}
	routes := 0
	for _, s := range ops.Catalog {
		for _, rt := range s.Routes {
			routes++
			path := rt.Path
			for range ops.PathParams(rt.Path) {
				path = path[:strings.Index(path, "{")] + uuid.NewString() + path[strings.Index(path, "}")+1:]
			}
			c := srv.Echo.NewContext(httptest.NewRequest(rt.Method, path, nil), httptest.NewRecorder())
			srv.Echo.Router().Route(c)
			if got := c.RouteInfo().Name; got != s.Name {
				t.Errorf("%s %s resolves to %q, want %s", rt.Method, path, got, s.Name)
			}
		}
	}
	if routes != 221 {
		t.Fatalf("%d routes, want 221", routes)
	}
}
