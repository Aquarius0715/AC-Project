package seed

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

const fixturePath = "../../../../docs/04-agentic-sdlc/fixture-contract.json"

func ownerConn(t *testing.T) *pgx.Conn {
	t.Helper()
	url := os.Getenv("AC_TEST_OWNER_DATABASE_URL")
	if url == "" {
		url = "postgres://postgres:local@localhost:5432/ac?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	c, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Skip("database not available:", err)
	}
	t.Cleanup(func() { c.Close(context.Background()) })
	return c
}

func TestIDDeterministic(t *testing.T) {
	if ID("unit-online-rto") != ID("unit-online-rto") || ID("a") == ID("b") || ID("x").Version() != 5 {
		t.Fatal("UUIDv5 mapping")
	}
}

func TestLoadErrors(t *testing.T) {
	if _, err := Load("nope.json"); err == nil {
		t.Fatal("missing file")
	}
	p := t.TempDir() + "/bad.json"
	_ = os.WriteFile(p, []byte("{"), 0o600)
	if _, err := Load(p); err == nil {
		t.Fatal("bad json")
	}
}

// Applies the fixture twice in a rolled-back transaction and checks the row counts.
func TestApplyFixture(t *testing.T) {
	f, err := Load(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	c := ownerConn(t)
	ctx := context.Background()
	tx, err := c.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	for i := 0; i < 2; i++ {
		if err := Apply(ctx, tx, f); err != nil {
			t.Fatalf("apply %d: %v", i, err)
		}
	}
	ids := func(n int, get func(int) string) []any {
		out := make([]any, n)
		for i := range out {
			out[i] = ID(get(i))
		}
		return out
	}
	counts := map[string][]any{
		"identity.memberships": ids(len(f.Actors), func(i int) string { return f.Actors[i].MembershipID }),
		"assets.customers":     ids(len(f.DemoSeed.Customers), func(i int) string { return f.DemoSeed.Customers[i].ID }),
		"assets.properties":    ids(len(f.DemoSeed.Properties), func(i int) string { return f.DemoSeed.Properties[i].ID }),
		"assets.spaces":        ids(len(f.DemoSeed.Spaces), func(i int) string { return f.DemoSeed.Spaces[i].ID }),
		"assets.units":         ids(len(f.DemoSeed.Units), func(i int) string { return f.DemoSeed.Units[i].ID }),
		"devices.capabilities": ids(len(f.DemoSeed.Capabilities), func(i int) string { return f.DemoSeed.Capabilities[i].ID }),
	}
	for table, want := range counts {
		var got int
		if err := tx.QueryRow(ctx, "SELECT count(DISTINCT id) FROM "+table+" WHERE id = ANY($1::uuid[])", want).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != len(want) || got == 0 {
			t.Errorf("%s: got %d want %d", table, got, len(want))
		}
	}
	var perms int
	_ = tx.QueryRow(ctx, `SELECT count(*) FROM identity.membership_permissions WHERE membership_id=$1`, ID("hq-operator")).Scan(&perms)
	if perms == 0 {
		t.Error("hq-operator has no permissions")
	}
	var name string
	if err := tx.QueryRow(ctx, `SELECT display_name FROM assets.units WHERE id=$1`, ID("unit-online-rto")).Scan(&name); err != nil || name != "Bedroom AC" {
		t.Fatalf("unit-online-rto: %v %q", err, name)
	}
}
