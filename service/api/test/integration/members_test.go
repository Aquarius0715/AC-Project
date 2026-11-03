package integration_test

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

const (
	jsonAlertPermissions = `["alert.read","alert.resolve"]`
	jsonRoleAdmin        = `"admin"`
)

func newUser(t *testing.T) string {
	t.Helper()

	userID := uuid.NewString()
	owner(
		t,
		`INSERT INTO identity.users (id, email, display_name) VALUES ($1, $2, 'Test user')`,
		userID,
		userID+"@ac.local",
	)

	return userID
}

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestMembersSave(t *testing.T) {
	srv := server(t)
	user := newUser(t)
	operation, contractor, customer := seed.ID("org-operator-a").
		String(),
		seed.ID("org-contractor-a").
			String(),
		seed.ID("org-customer-a").
			String()
	unit := seed.ID("unit-non-rto").String()
	from := clock().Add(-time.Hour).Format(time.RFC3339)
	until := clock().Add(30 * 24 * time.Hour).Format(time.RFC3339)
	body := func(bodyFields map[string]string) string {
		base := map[string]string{
			fieldUserID:         `"` + user + `"`,
			fieldOrganizationID: `"` + operation + `"`,
			fieldRole:           jsonRoleTechnician,
			fieldEmployment:     `"internal"`,
			fieldPermissions:    `["alert.read"]`,
			fieldScopes:         `[{"kind":"unit","id":"` + unit + `"}]`,
			fieldValidFrom:      `"` + from + `"`,
			fieldValidUntil:     jsonNull,
			fieldReason:         `"onboarding"`,
		}

		for key, value := range bodyFields {
			if value == "" {
				delete(base, key)

				continue
			}

			base[key] = value
		}

		parts := make([]string, 0, len(base))
		for k, v := range base {
			parts = append(parts, `"`+k+`":`+v)
		}

		return "{" + strings.Join(parts, ",") + "}"
	}
	refused := map[string]string{ // the field each VALIDATION case is refused for
		"admin unit scope":       "scopes=error.invalid",
		"admin two tenants":      "scopes=error.invalid",
		caseBlankReason:          refusedReasonLength,
		"employment for admin":   "employment=error.notAllowed",
		"external without end":   "validUntil=errors.external_needs_end",
		"internal at contractor": "organizationId=errors.organization_kind",
		"no employment":          "employment=error.required",
		"technician permission":  "permissions=errors.write_needs_read",
		"unknown permission":     "permissions=error.invalid",
		"unknown property":       "scopes=errors.unknown_scope",
		"unknown scope":          "scopes=errors.unknown_scope",
		"until before from":      "validUntil=error.range",
		"write without read":     "permissions=errors.write_needs_read",
	}

	for name, testCase := range map[string]struct {
		f    map[string]string
		code int
	}{
		"no employment":         {map[string]string{fieldEmployment: jsonNull}, 422},
		"employment for admin":  {map[string]string{fieldRole: jsonRoleAdmin}, 422},
		"technician permission": {map[string]string{fieldPermissions: `["job.write"]`}, 422},
		"write without read": {
			map[string]string{
				fieldRole: jsonRoleAdmin, fieldEmployment: jsonNull, fieldPermissions: `["job.write"]`,
				fieldScopes: `[]`,
			}, 422,
		},
		"unknown permission": {
			map[string]string{
				fieldRole: jsonRoleAdmin, fieldEmployment: jsonNull, fieldPermissions: `["god.mode"]`,
				fieldScopes: `[]`,
			}, 422,
		},
		"admin unit scope": {
			map[string]string{fieldRole: jsonRoleAdmin, fieldEmployment: jsonNull, fieldPermissions: `[]`},
			422,
		},
		"admin two tenants": {
			map[string]string{
				fieldRole: jsonRoleAdmin, fieldEmployment: jsonNull, fieldPermissions: `[]`,
				fieldScopes: `[{"kind":"tenant","id":"` + seed.ID("tenant-a").String() + `"},{"kind":"tenant","id":"` +
					uuid.NewString() + `"}]`,
			}, 422,
		},
		"external without end": {
			map[string]string{fieldEmployment: `"external"`, fieldOrganizationID: `"` + contractor + `"`},
			422,
		},
		"until before from": {
			map[string]string{fieldValidUntil: `"` + clock().Add(-2*time.Hour).Format(time.RFC3339) + `"`},
			422,
		},
		caseBlankReason:          {map[string]string{fieldReason: jsonBlank}, 422},
		"internal at contractor": {map[string]string{fieldOrganizationID: `"` + contractor + `"`}, 422},
		"unknown scope":          {map[string]string{fieldScopes: `[{"kind":"unit","id":"` + uuid.NewString() + `"}]`}, 422},
		"unknown property": {map[string]string{fieldScopes: `[{"kind":"property",` +
			`"id":"` + uuid.NewString() + `"}]`}, 422},
		"unknown user":         {map[string]string{fieldUserID: `"` + uuid.NewString() + `"`}, 404},
		"unknown organization": {map[string]string{fieldOrganizationID: `"` + uuid.NewString() + `"`}, 404},
	} {
		code, m := write(srv, hq(), "members.save", body(testCase.f), 0)
		if w, ok := refused[name]; ok {
			refusal(t, name, code, m, w)
		} else if code != testCase.code {
			t.Errorf("save %s: %d want %d", name, code, testCase.code)
		}
	}

	code, resp := write(srv, hq(), "members.save", body(nil), 0)
	if code != 200 || data(resp)["role"] != roleTechnician ||
		len(asArr(t, data(resp)["scopes"])) != 1 ||
		asNum(t, data(resp)["scopeVersion"]) != 1 {
		t.Fatalf("create: %d %v", code, resp)
	}

	membershipID := asStr(t, data(resp)["id"])
	// update: permissions change bumps scopeVersion; role is fixed
	if code, res := write(
		srv,
		hq(),
		"members.save",
		body(
			map[string]string{
				"id":             `"` + membershipID + `"`,
				fieldPermissions: jsonAlertPermissions,
				fieldValidUntil:  `"` + until + `"`,
			},
		),
		1,
	); code != 200 ||
		asNum(t, data(res)["scopeVersion"]) != 2 ||
		asNum(t, data(res)["version"]) != 2 {
		t.Fatalf("update: %d %v", code, res)
	}

	if code, res := write(
		srv,
		hq(),
		"members.save",
		body(
			map[string]string{
				"id":             `"` + membershipID + `"`,
				fieldPermissions: jsonAlertPermissions,
				fieldReason:      `"period"`,
			},
		),
		2,
	); code != 200 ||
		asNum(t, data(res)["scopeVersion"]) != 2 {
		t.Fatalf("same permissions keep scopeVersion: %d %v", code, res)
	}

	if code, res := write(
		srv,
		hq(),
		"members.save",
		body(
			map[string]string{
				"id":             `"` + membershipID + `"`,
				fieldRole:        jsonRoleAdmin,
				fieldEmployment:  jsonNull,
				fieldPermissions: `[]`,
				fieldScopes:      `[]`,
			},
		),
		3,
	); code != 422 ||
		fields(res) != "role=errors.membership_fixed" {
		t.Errorf("role fixed: %d %s", code, fields(res))
	}
	// a technician may also be scoped to a whole property (the identity copy of the property, IR186)
	if code, res := write(
		srv,
		hq(),
		"members.save",
		body(
			map[string]string{
				"id":             `"` + membershipID + `"`,
				fieldPermissions: jsonAlertPermissions,
				fieldScopes: `[{"kind":"property","id":"` + seed.ID("property-home-a").
					String() +
					`"}]`,
				fieldReason: `"whole home"`,
			},
		),
		3,
	); code != 200 ||
		asObj(t, asArr(t, data(res)["scopes"])[0])["kind"] != kindProperty ||
		asNum(t, data(res)["scopeVersion"]) != 3 {
		t.Fatalf("property scope: %d %v", code, res)
	}

	if code, _ := write(
		srv,
		hq(),
		"members.save",
		body(map[string]string{"id": `"` + membershipID + `"`}),
		1,
	); code != 409 {
		t.Error("stale membership version")
	}
	// client memberships get clientRole=member and the initial consent
	code, resp = write(srv, hq(), "members.save", body(map[string]string{
		fieldOrganizationID: `"` + customer + `"`,
		fieldRole:           `"client"`,
		fieldEmployment:     jsonNull,
		fieldPermissions:    `[]`,
		fieldScopes:         `[{"kind":"organization","id":"` + customer + `"}]`,
	}), 0)
	if code != 200 || data(resp)["clientRole"] != clientRoleMember {
		t.Fatalf("client: %d %v", code, resp)
	}

	var consents int
	ownerScan(
		t,
		`SELECT count(*) FROM identity.consents WHERE membership_id = $1 AND granted = false`,
		[]any{data(resp)["id"]},
		&consents,
	)

	if consents != 1 {
		t.Error("initial consent")
	}

	if code, _ := write(srv, customerA(), "members.save", body(nil), 0); code != 403 {
		t.Error("client manages members")
	}

	// self promotion and the last identity.write holder
	_, resp = post(srv, hq(), "members.list", `{"filters":{"role":"admin"},"limit":100}`)

	var self map[string]any

	for _, it := range items(t, resp) {
		if it["id"] == seed.ID("hq-operator").String() {
			self = it
		}
	}

	perms := jsonBytes(t, self["permissions"])
	selfBody := func(p string) string {
		return `{"id":"` + asStr(
			t,
			self["id"],
		) + `","userId":"` + asStr(t, self["userId"]) + `","organizationId":"` + operation +
			`","role":"admin","employment":null,"permissions":` + p +
			`,"scopes":[{"kind":"tenant","id":"` + seed.ID("tenant-a").
			String() +
			`"}],"validFrom":"` + asStr(
			t,
			self["validFrom"],
		) + `","validUntil":null,"reason":"self"}`
	}

	withOverride := strings.Replace(string(perms), `[`, `["restriction.override",`, 1)
	if code, _ := write(
		srv,
		hq(),
		"members.save",
		selfBody(withOverride),
		int(asNum(t, self["version"])),
	); code != 403 {
		t.Error("self promotion")
	}

	owner(
		t,
		`UPDATE identity.memberships SET valid_until = $2 WHERE id = $1`,
		seed.ID("hq-restriction-manager"),
		clock().Add(-time.Minute),
	)
	t.Cleanup(func() {
		owner(
			t,
			`UPDATE identity.memberships SET valid_until = NULL WHERE id = $1`,
			seed.ID("hq-restriction-manager"),
		)
	})
	// tenant-a's own: other tenants' administrators (TestBootstrapAdmin's) are not this test's
	owner(
		t,
		`UPDATE identity.memberships SET valid_until = $2 WHERE id <> $1 AND role = 'admin' AND id IN (SELECT membership_id
			FROM identity.membership_permissions WHERE permission = 'identity.write')
		AND created_at > $3 AND tenant_id = $4`,
		seed.ID("hq-operator"),
		clock().Add(-time.Minute),
		clock().Add(-24*time.Hour),
		seed.ID("tenant-a"),
	)

	withoutIdentity := strings.Replace(
		strings.Replace(string(perms), `"identity.write",`, ``, 1),
		`,"identity.write"`,
		``,
		1,
	)
	if code, res := write(
		srv,
		hq(),
		"members.save",
		selfBody(withoutIdentity),
		int(asNum(t, self["version"])),
	); code != 409 {
		t.Errorf("last identity.write holder: %d %v", code, res)
	}
}

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestMembersList(t *testing.T) {
	srv := server(t)

	_, resp := post(srv, hq(), "members.list", `{"limit":100}`)
	for _, it := range items(t, resp) {
		if it["role"] == roleClient {
			t.Fatal("clients excluded by default")
		}
	}
	// IR172: every membership carries its user's display name (contractors list their technicians by name)
	_, resp = post(srv, contrA(), "members.list", `{"limit":100}`)
	if len(items(t, resp)) == 0 {
		t.Fatal("contractor sees its technicians")
	}

	for _, it := range items(t, resp) {
		if name, _ := it["displayName"].(string); name == "" {
			t.Fatalf("displayName missing: %v", it["id"])
		}
	}

	if _, res := post(
		srv,
		hq(),
		"members.list",
		`{"filters":{"role":"client"},"limit":100}`,
	); len(
		items(t, res),
	) < 2 {
		t.Error("role=client")
	}

	if code, res := post(
		srv,
		hq(),
		"members.list",
		`{"filters":{"activeOnly":false,"organizationId":"`+uuid.NewString()+`"}}`,
	); code != 200 ||
		len(items(t, res)) != 0 {
		t.Error("filters")
	}

	_, resp = post(srv, contrA(), "members.list", `{"limit":100}`)
	if len(items(t, resp)) == 0 {
		t.Fatal("contractor sees its technicians")
	}

	for _, it := range items(t, resp) {
		if it["role"] != roleTechnician ||
			it["organizationId"] != seed.ID("org-contractor-a").String() ||
			len(asArr(t, it["permissions"])) != 0 ||
			len(asArr(t, it["scopes"])) != 0 {
			t.Fatalf("contractor projection: %v", it)
		}
	}

	want := map[string]string{ // the field each case is refused for
		"{\"filters\":{\"role\":\"guest\"}}": refusedFiltersInvalid,
		bodyUnknownFilter:                    refusedUnknownFieldX,
		bodySortByName:                       refusedSortFieldInvalid,
	}

	for _, b := range []string{`{"filters":{"role":"guest"}}`, bodyUnknownFilter, `{"sort":{"field":"name",` +
		`"direction":"asc"}}`} {
		code, m := post(srv, hq(), "members.list", b)
		refusal(t, b, code, m, want[b])
	}

	if code, _ := post(srv, customerA(), "members.list", `{}`); code != 403 {
		t.Error("client lists members")
	}
}

// TestMembersListCarriesEachMembersOwn (IR496): members.list reads the permissions, scopes and qualification grants of
// a page of members in one query per table; each member still gets only its own, and each revoked grant keeps its own
// revocation time.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestMembersListCarriesEachMembersOwn(t *testing.T) {
	srv := server(t)

	var firstTech, secondTech string
	ownerScan(
		t,
		`SELECT (array_agg(id::text ORDER BY id))[1], (array_agg(id::text ORDER BY id))[2] FROM identity.memberships
		WHERE tenant_id = $1 AND role = 'technician'`,
		[]any{seed.ID("tenant-a")},
		&firstTech,
		&secondTech,
	)

	if firstTech == "" || secondTech == "" {
		t.Skip("the seed has fewer than two technicians")
	}

	revokedA, revokedB := time.Date(
		2026,
		9,
		1,
		1,
		0,
		0,
		0,
		time.UTC,
	), time.Date(
		2026,
		9,
		2,
		2,
		0,
		0,
		0,
		time.UTC,
	)
	owner(
		t,
		`INSERT INTO identity.qualification_grants (tenant_id, membership_id, code, valid_from, valid_until, revoked_at)
			VALUES
		($1, $2, 'test.ir496.a', '2026-01-01Z', '2027-01-01Z', $4), ($1, $3, 'test.ir496.b', '2026-01-01Z', '2027-01-01Z',
		$5)`,
		seed.ID("tenant-a"),
		firstTech,
		secondTech,
		revokedA,
		revokedB,
	)
	t.Cleanup(func() {
		owner(
			t,
			`DELETE FROM identity.qualification_grants WHERE code IN ('test.ir496.a', 'test.ir496.b')`,
		)
	})

	_, res := post(
		srv,
		hq(),
		"members.list",
		`{"filters":{"role":"technician","activeOnly":false},"limit":100}`,
	)
	grants := map[string]map[string]string{} // member → test grant code → revokedAt

	for _, it := range items(t, res) {
		membershipID := asStr(t, it["id"])
		for _, q := range asArr(t, it["qualifications"]) {
			grant := asObj(t, q)
			if code := asStr(t, grant["code"]); strings.HasPrefix(code, "test.ir496.") {
				if grants[membershipID] == nil {
					grants[membershipID] = map[string]string{}
				}

				grants[membershipID][code], _ = grant["revokedAt"].(string)
			}
		}
	}

	at := func(v time.Time) string { return v.Format(time.RFC3339) }
	if len(grants[firstTech]) != 1 || grants[firstTech]["test.ir496.a"] != at(revokedA) ||
		len(grants[secondTech]) != 1 ||
		grants[secondTech]["test.ir496.b"] != at(revokedB) {
		t.Fatalf("each member carries only its own grant with its own revocation: %v", grants)
	}
}
