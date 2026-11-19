package auth

import (
	"slices"
	"testing"
)

// OIDC_APP_CLIENT_ID lists one client per app (IR790): blanks and empty entries are dropped.
func TestCommaList(t *testing.T) {
	t.Parallel()

	got := commaList(" ac-customer-web, ac-partner-web ,,ac-technician-web ")
	want := []string{"ac-customer-web", "ac-partner-web", "ac-technician-web"}

	if !slices.Equal(got, want) {
		t.Fatalf("commaList = %q, want %q", got, want)
	}

	if len(commaList("")) != 0 {
		t.Fatal("an unset list has no entries")
	}
}
