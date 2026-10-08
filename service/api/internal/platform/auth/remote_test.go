package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestRemoteSource(t *testing.T) {
	tenant, member, good := uuid.New(), uuid.New(), uuid.New()
	calls, status := 0, http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != PrincipalPath || r.Header.Get("Authorization") != "Bearer internal" || r.URL.Query().Get("subject") != "sub-1" {
			http.Error(w, "bad", http.StatusBadRequest)
			return
		}
		if r.URL.Query().Get("membership") != member.String() {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		_ = json.NewEncoder(w).Encode(PrincipalDTO{UserID: good, TenantID: tenant, MembershipID: member, Role: "client", ClientRole: "owner",
			Permissions: []string{"control.execute"}, Scopes: map[string][]uuid.UUID{"unit": {good}}, ScopeVersion: 3})
	}))
	defer srv.Close()
	now := time.Date(2026, 9, 14, 1, 0, 0, 0, time.UTC)
	r := &RemoteSource{BaseURL: srv.URL, Token: "internal", TTL: 30 * time.Second, Now: func() time.Time { return now }}
	ctx := context.Background()
	p, err := r.Principal(ctx, "sub-1", tenant, member)
	if err != nil || p.Role != "client" || !p.Permissions["control.execute"] || p.ScopeVersion != 3 || len(p.Scopes["unit"]) != 1 || p.ClientRole != "owner" {
		t.Fatalf("principal %+v %v", p, err)
	}
	if _, err := r.Principal(ctx, "sub-1", tenant, member); err != nil || calls != 1 {
		t.Fatalf("cached call: calls=%d err=%v", calls, err)
	}
	now = now.Add(31 * time.Second)
	if _, err := r.Principal(ctx, "sub-1", tenant, member); err != nil || calls != 2 {
		t.Fatalf("expired cache: calls=%d", calls)
	}
	if _, err := r.Principal(ctx, "sub-1", tenant, uuid.New()); !errors.Is(err, ErrNoMembership) {
		t.Fatalf("unknown membership: %v", err)
	}
	status = http.StatusServiceUnavailable
	now = now.Add(time.Minute)
	if _, err := r.Principal(ctx, "sub-1", tenant, member); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("outage: %v", err)
	}
	status = http.StatusOK
	if _, err := r.Principal(ctx, "sub-1", tenant, member); err != nil {
		t.Fatalf("outage must not be cached: %v", err)
	}
	srv.Close()
	now = now.Add(time.Minute)
	if _, err := r.Principal(ctx, "sub-1", tenant, member); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("down: %v", err)
	}
}
