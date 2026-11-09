package integration_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

// A unit with the name of a row of a previewed import, saved after the preview, stops the commit: the name stays unique
// among the siblings of its space (admin.md, DD-A02; CONFLICT error.duplicateSiblingName), as units.save keeps it.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestUnitsImportCommitKeepsSiblingNamesUnique(t *testing.T) {
	srv := server(t)
	cust := seed.ID("cust-b").String()
	org := seed.ID("org-customer-b").String()
	prop := "Race site " + uuid.NewString()[:6]

	code, resp := write(
		srv,
		hq(),
		"properties.save",
		`{"customerOrgId":"`+org+`","kind":"office","name":"`+prop+`","address":null,"accessInstructions":null}`,
		0,
	)
	if code != 200 {
		t.Fatalf("property: %d %v", code, resp)
	}

	propID := asStr(t, data(resp)["id"])
	csv := "Property,Unit,Model\n" + prop + ",Race AC,SPL-100\n"
	body := `{"customerId":"` + cust + `","fileName":"units.csv","csvText":` + jsonStr(t, csv) +
		`,"mapping":{"property":"Property","unit_name":"Unit","model_code":"Model"}}`

	code, resp = post(srv, hq(), "units.importPreview", body)
	if code != 200 || asNum(t, data(resp)["readyCount"]) != 1 {
		t.Fatalf("preview: %d %v", code, resp)
	}

	previewID := asStr(t, data(resp)["previewId"])

	// someone adds a unit with that name while the preview waits
	if code, res := write(
		srv,
		hq(),
		"units.save",
		`{"customerOrgId":"`+org+`","propertyId":"`+propID+`","spaceId":null,"displayName":"race ac",`+
			`"modelId":"`+seed.ID("ventilation-demo").
			String()+
			`","type":"split","installedAt":null,"serviceScope":["indoor"]}`,
		0,
	); code != 200 {
		t.Fatalf("units.save: %d %v", code, res)
	}

	code, res := write(
		srv,
		hq(),
		"units.importCommit",
		`{"previewId":"`+previewID+`","customerId":"`+cust+`"}`,
		0,
	)
	if code != 409 || res["messageKey"] != apperr.KeyDuplicateSiblingName {
		t.Errorf("commit with a name taken since the preview: %d %v", code, res)
	}
}

// An import is undone only while the places it created hold no unit it did not create (admin.md: archiving a property
// or space that still holds non-archived units is CONFLICT): a unit saved into the new property after the import
// would otherwise sit in an archived property.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestUnitsImportUndoKeepsPlacesThatHoldOtherUnits(t *testing.T) {
	srv := server(t)
	cust := seed.ID("cust-b").String()
	org := seed.ID("org-customer-b").String()
	prop := "Undo site " + uuid.NewString()[:6]
	csv := "Property,Floor,Unit,Model\n" + prop + ",2F,Undo AC,SPL-100\n"
	body := `{"customerId":"` + cust + `","fileName":"units.csv","csvText":` + jsonStr(t, csv) +
		`,"mapping":{"property":"Property","floor":"Floor","unit_name":"Unit","model_code":"Model"}}`

	code, resp := post(srv, hq(), "units.importPreview", body)
	if code != 200 {
		t.Fatalf("preview: %d %v", code, resp)
	}

	code, resp = write(
		srv,
		hq(),
		"units.importCommit",
		`{"previewId":"`+asStr(t, data(resp)["previewId"])+`","customerId":"`+cust+`"}`,
		0,
	)
	if code != 200 {
		t.Fatalf("commit: %d %v", code, resp)
	}

	imported := data(resp)
	propID := asStr(t, asArr(t, imported["createdPropertyIds"])[0])

	// a second unit is saved into the property the import created
	if code, res := write(
		srv,
		hq(),
		"units.save",
		`{"customerOrgId":"`+org+`","propertyId":"`+propID+`","spaceId":null,"displayName":"Added later",`+
			`"modelId":"`+seed.ID("ventilation-demo").
			String()+
			`","type":"split","installedAt":null,"serviceScope":["indoor"]}`,
		0,
	); code != 200 {
		t.Fatalf("units.save: %d %v", code, res)
	}

	code, res := write(
		srv,
		hq(),
		"units.importUndo",
		`{"importId":"`+asStr(t, imported["id"])+`","reason":"wrong file"}`,
		ver(t, resp),
	)
	if code != 409 || res["messageKey"] != "error.importPlaceInUse" {
		t.Errorf("undo with another unit in the new property: %d %v", code, res)
	}
}
