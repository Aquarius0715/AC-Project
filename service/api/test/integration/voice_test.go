package integration_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

func TestVoiceAndExport(t *testing.T) {
	srv := server(t)
	say := func(a *actor, text, locale, extra string) (int, map[string]any) {
		return post(
			srv,
			a,
			"voice.resolveIntent",
			`{"text":"`+text+`","locale":"`+locale+`"`+extra+`}`,
		)
	}
	bedroomA := seed.ID("unit-online-rto").String()

	code, res := say(customerA(), "  Temperature BEDROOM ", "en", "")
	if code != 200 || data(res)["kind"] != "temperature" || data(res)["unitId"] != bedroomA {
		t.Fatalf("temperature: %d %v", code, res)
	}

	if _, ok := data(res)["measurement"]; !ok {
		t.Error("measurement key present (object or null)")
	}

	code, res = say(customerA(), "set living room to 24 degrees", "en", "")
	if code != 200 || data(res)["kind"] != "change" || asNum(t, data(res)["celsius"]) != 24 ||
		data(res)["expectedVersion"] == nil ||
		data(res)["unitId"] != seed.ID("unit-non-rto").String() {
		t.Fatalf("change: %d %v", code, res)
	}

	if _, resp := say(
		customerA(),
		"tetapkan living room kepada 25 darjah",
		"ms",
		"",
	); data(resp)["kind"] != "change" {
		t.Errorf("ms change: %v", resp)
	}

	for text, kind := range map[string]string{
		"help": "help", "hello there": "unsupported",
		"temperature Lobby AC": "unsupported", "temperature garage": "unsupported",
	} {
		if _, m := say(customerA(), text, "en", ""); data(m)["kind"] != kind {
			t.Errorf("%s: %v", text, m)
		}
	}

	if _, resp := say(
		customerA(),
		"bantuan",
		"ms",
		"",
	); data(resp)["kind"] != "help" ||
		data(resp)["messageKey"] != "voice.help" {
		t.Errorf("bantuan: %v", resp)
	}

	if _, m := say(customerA(), "help", "ms", ""); data(m)["kind"] != "unsupported" {
		t.Error("English help in Malay grammar")
	}
	// HQ sees both customers' bedrooms → candidates, then a selection
	code, res = say(hq(), "temperature bedroom", "en", "")
	if code != 200 || data(res)["kind"] != "candidates" ||
		len(asArr(t, data(res)["candidates"])) < 2 {
		t.Fatalf("candidates: %d %v", code, res)
	}

	label := asStr(t, asObj(t, asArr(t, data(res)["candidates"])[0])["pathLabel"])
	if label == "" {
		t.Error("path label")
	}

	if _, resp := say(
		hq(),
		"temperature bedroom",
		"en",
		`,"selectedUnitId":"`+bedroomA+`"`,
	); data(resp)["kind"] != "temperature" ||
		data(resp)["unitId"] != bedroomA {
		t.Errorf("selected: %v", resp)
	}

	if code, _ := say(
		hq(),
		"temperature bedroom",
		"en",
		`,"selectedUnitId":"`+uuid.NewString()+`"`,
	); code != 404 {
		t.Error("selection outside the candidates")
	}

	if code, m := say(customerA(), " ", "en", ""); code != 422 || fields(m) != "text=error.length" {
		t.Errorf("blank text: %d %s", code, fields(m))
	}

	if code, resp := say(
		customerA(),
		"help",
		"jp",
		"",
	); code != 422 ||
		fields(resp) != "locale=error.invalid" {
		t.Errorf("bad locale: %d %s", code, fields(resp))
	}

	if code, _ := say(contrA(), "help", "en", ""); code != 403 {
		t.Error("contractor voice")
	}

	// monthly export
	exp := func(a *actor, month, props, sections, format string) (int, map[string]any) {
		return post(
			srv,
			a,
			"energy.exportReport",
			`{"month":"`+month+`","propertyIds":[`+props+`],"sections":[`+sections+`],"format":"`+format+`"}`,
		)
	}
	homeA := `"` + seed.ID("property-home-a").String() + `"`
	all := `"energy_cost","month_comparison","co2_offsets","alerts_maintenance"`

	code, res = exp(customerA(), "2026-08", homeA, all, "csv")
	if code != 200 || data(res)["fileName"] != "energy-report-2026-08.csv" ||
		data(res)["mime"] != "text/csv" ||
		asNum(t, data(res)["size"]) <= 0 {
		t.Fatalf("export: %d %v", code, res)
	}

	if _, resp := exp(
		customerA(),
		"2026-08",
		homeA,
		`"energy_cost"`,
		"pdf",
	); data(resp)["mime"] != "application/pdf" {
		t.Error("pdf export")
	}

	refused := map[string]string{ // the field each VALIDATION case is refused for
		"bad format":      "format=error.invalid",
		"bad month":       "month=error.invalid",
		"month not ended": "month=errors.month_not_ended",
		"no sections":     "sections=error.required",
	}

	for name, tc := range map[string][5]string{
		"month not ended": {"2026-09", homeA, all, "csv", "422"},
		"bad month":       {"2026-13", homeA, all, "csv", "422"},
		"no sections":     {"2026-08", homeA, "", "csv", "422"},
		"bad format":      {"2026-08", homeA, all, "xlsx", "422"},
		"other customer":  {"2026-08", `"` + seed.ID("property-home-b").String() + `"`, all, "csv", "404"},
	} {
		code, m := exp(customerA(), tc[0], tc[1], tc[2], tc[3])
		if w, ok := refused[name]; ok {
			refusal(t, name, code, m, w)
		} else if itoa(code) != tc[4] {
			t.Errorf("%s: %d", name, code)
		}
	}

	if code, _ := exp(hq(), "2026-08", homeA, all, "csv"); code != 403 {
		t.Error("HQ export")
	}
}
