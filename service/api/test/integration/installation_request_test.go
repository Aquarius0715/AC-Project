package integration_test

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/seed"
	apiserver "github.com/pradita/ac-project/service/api/internal/server"
)

const (
	fieldRequestID  = "installationRequestId"
	stateSubmitted  = "submitted"
	stateApproved   = "approved"
	typeInstallJSON = "installation"
)

// requestBody is an installationRequests.create body for the property.
func requestBody(prop string, quantity int, slots string) string {
	return `{"propertyId":"` + prop + `","quantity":` + strconv.Itoa(
		quantity,
	) + `,"modelId":null,"withIot":true,` +
		`"preferredSlots":` + slots + `,"note":"two rooms on the second floor"}`
}

func threeSlots() string {
	return "[" + slotJSON(25, 2) + "," + slotJSON(49, 2) + "," + slotJSON(73, 2) + "]"
}

// plannedUnit has HQ plan a unit for the request's customer and answers its ID.
func plannedUnit(t *testing.T, srv *apiserver.Server, org, prop, model string) string {
	t.Helper()

	code, res := write(
		srv,
		hq(),
		"units.save",
		`{"customerOrgId":"`+org+`","propertyId":"`+prop+`","spaceId":null,"displayName":"Requested AC `+
			uuid.NewString()[:8]+`","modelId":"`+model+`","type":"split","serviceScope":["indoor","outdoor"],`+
			`"installState":"planned","installedAt":null}`,
		0,
	)
	if code != http.StatusOK {
		t.Fatalf("plan a unit: %d %v", code, res)
	}

	return asStr(t, data(res)["id"])
}

// requestRow is the request in the caller's installationRequests.list (nil when it is not listed).
func requestRow(t *testing.T, srv *apiserver.Server, who *actor, id string) map[string]any {
	t.Helper()

	code, res := post(srv, who, "installationRequests.list", `{"limit":100}`)
	if code != http.StatusOK {
		t.Fatalf("installationRequests.list: %d %v", code, res)
	}

	for _, row := range items(t, res) {
		if row["id"] == id {
			return row
		}
	}

	return nil
}

// TestInstallationRequests (DEC-107, IR794): a customer owner asks for new aircons; HQ declines the request, or
// creates the planned units and the installation jobs for it.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestInstallationRequests(t *testing.T) {
	srv := server(t)
	org, prop, model := installBase(t)

	create := func(who *actor, body string) (int, map[string]any) {
		return write(srv, who, "installationRequests.create", body, 0)
	}

	code, res := create(customerA(), requestBody(prop, 0, threeSlots()))
	refusal(t, "no units", code, res, "quantity="+apperr.KeyRange)

	code, res = create(customerA(), requestBody(prop, 11, threeSlots()))
	refusal(t, "eleven units", code, res, "quantity="+apperr.KeyRange)

	code, res = create(
		customerA(),
		requestBody(prop, 1, "["+slotJSON(25, 2)+","+slotJSON(49, 2)+"]"),
	)
	refusal(t, "two slots", code, res, "preferredSlots="+apperr.KeyCount)

	code, res = create(
		customerA(),
		requestBody(prop, 1, "["+slotJSON(25, 2)+","+slotJSON(25, 2)+","+slotJSON(73, 2)+"]"),
	)
	refusal(t, "the same slot twice", code, res, "preferredSlots="+apperr.KeySlotRules)

	code, res = create(
		customerA(),
		requestBody(prop, 1, "["+slotJSON(1, 2)+","+slotJSON(49, 2)+","+slotJSON(73, 2)+"]"),
	)
	refusal(t, "a slot on the same day", code, res, "preferredSlots="+apperr.KeySlotRules)

	if code, _ = create(
		customerB(),
		requestBody(prop, 1, threeSlots()),
	); code != http.StatusNotFound {
		t.Errorf("another customer's property: %d", code)
	}

	if code, _ = create(hq(), requestBody(prop, 1, threeSlots())); code != http.StatusForbidden {
		t.Errorf("HQ creating a client's request: %d", code)
	}

	// a request is submitted, listed for its owner and for HQ, and not for another customer
	code, res = create(customerA(), requestBody(prop, 2, threeSlots()))
	if code != http.StatusOK || data(res)["status"] != stateSubmitted ||
		data(res)["quantity"] != float64(2) {
		t.Fatalf("create a request: %d %v", code, res)
	}

	request := asStr(t, data(res)["id"])

	if requestRow(t, srv, customerA(), request) == nil || requestRow(t, srv, hq(), request) == nil {
		t.Error("the owner and HQ see the request")
	}

	if requestRow(t, srv, customerB(), request) != nil {
		t.Error("another customer sees the request")
	}

	// HQ links jobs: a client cannot, and a request takes at most quantity unfinished jobs
	code, res = write(
		srv,
		customerA(),
		"jobs.create",
		jobBody(seed.ID("unit-non-rto").String(), map[string]string{
			fieldRequestID: `"` + request + `"`,
		}),
		0,
	)
	refusal(t, "a client links a request", code, res, fieldRequestID+"="+apperr.KeyNotAllowed)

	link := func(unit, id string) (int, map[string]any) {
		return write(srv, hq(), "jobs.create", jobBody(unit, map[string]string{
			fieldType: installationType, fieldRequestID: `"` + id + `"`,
		}), 0)
	}

	first := plannedUnit(t, srv, org, prop, model)

	code, res = link(first, request)
	if code != http.StatusOK || data(res)["origin"] != "client_request" ||
		data(res)["type"] != typeInstallJSON {
		t.Fatalf("the job for the request: %d %v", code, res)
	}

	row := requestRow(t, srv, customerA(), request)

	var jobIDs []any

	if row != nil {
		jobIDs, _ = row["jobIds"].([]any)
	}

	if row == nil || row["status"] != stateApproved || len(jobIDs) != 1 {
		t.Fatalf("the request after its first job: %v", row)
	}

	code, res = link(plannedUnit(t, srv, org, prop, model), request)
	if code != http.StatusOK {
		t.Fatalf("the second job: %d %v", code, res)
	}

	code, res = link(plannedUnit(t, srv, org, prop, model), request)
	if code != http.StatusConflict {
		t.Errorf("a third job for two units: %d %v", code, res)
	}

	// an approved request cannot be cancelled or declined
	approved := requestRow(t, srv, hq(), request)
	version := int(asNum(t, approved["version"]))

	if code, _ = write(
		srv,
		customerA(),
		"installationRequests.cancel",
		`{"id":"`+request+`"}`,
		version,
	); code != http.StatusConflict {
		t.Errorf("cancel an approved request: %d", code)
	}

	// decline: a reason of 5–500 characters, and only a submitted request
	declined := create2(t, srv, create, prop)

	code, res = write(
		srv,
		hq(),
		"installationRequests.decline",
		`{"id":"`+declined+`","reason":"no"}`,
		1,
	)
	refusal(t, "a short reason", code, res, "reason="+apperr.KeyLength)

	code, res = write(
		srv,
		hq(),
		"installationRequests.decline",
		`{"id":"`+declined+`","reason":"No capacity this month"}`,
		1,
	)
	if code != http.StatusOK || data(res)["status"] != "declined" ||
		data(res)["declineReason"] != "No capacity this month" {
		t.Fatalf("decline: %d %v", code, res)
	}

	if code, _ = write(
		srv,
		hq(),
		"installationRequests.decline",
		`{"id":"`+declined+`","reason":"No capacity again"}`,
		2,
	); code != http.StatusConflict {
		t.Errorf("decline twice: %d", code)
	}

	if code, _ = link(
		plannedUnit(t, srv, org, prop, model),
		declined,
	); code != http.StatusConflict {
		t.Errorf("a job for a declined request: %d", code)
	}

	// the owner cancels a submitted request, not another customer's
	cancelled := create2(t, srv, create, prop)

	if code, _ = write(
		srv,
		customerB(),
		"installationRequests.cancel",
		`{"id":"`+cancelled+`"}`,
		1,
	); code != http.StatusNotFound {
		t.Errorf("another customer cancels: %d", code)
	}

	code, res = write(srv, customerA(), "installationRequests.cancel", `{"id":"`+cancelled+`"}`, 1)
	if code != http.StatusOK || data(res)["status"] != "cancelled" {
		t.Fatalf("cancel: %d %v", code, res)
	}
}

// create2 makes a one-unit request of the owner and answers its ID.
func create2(
	t *testing.T,
	srv *apiserver.Server,
	create func(*actor, string) (int, map[string]any),
	prop string,
) string {
	t.Helper()

	_ = srv

	code, res := create(customerA(), requestBody(prop, 1, threeSlots()))
	if code != http.StatusOK {
		t.Fatalf("create a request: %d %v", code, res)
	}

	return asStr(t, data(res)["id"])
}
