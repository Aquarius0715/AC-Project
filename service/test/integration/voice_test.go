package integration

import (
	"testing"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/internal/seed"
)

func TestVoiceAndExport(t *testing.T) {
	s := server(t)
	say := func(a *actor, text, locale, extra string) (int, map[string]any) {
		return post(s, a, "voice.resolveIntent", `{"text":"`+text+`","locale":"`+locale+`"`+extra+`}`)
	}
	bedroomA := seed.ID("unit-online-rto").String()
	code, m := say(&customerA, "  Temperature BEDROOM ", "en", "")
	if code != 200 || data(m)["kind"] != "temperature" || data(m)["unitId"] != bedroomA {
		t.Fatalf("temperature: %d %v", code, m)
	}
	if _, ok := data(m)["measurement"]; !ok {
		t.Error("measurement key present (object or null)")
	}
	code, m = say(&customerA, "set living room to 24 degrees", "en", "")
	if code != 200 || data(m)["kind"] != "change" || data(m)["celsius"].(float64) != 24 || data(m)["expectedVersion"] == nil || data(m)["unitId"] != seed.ID("unit-non-rto").String() {
		t.Fatalf("change: %d %v", code, m)
	}
	if _, m := say(&customerA, "tetapkan living room kepada 25 darjah", "ms", ""); data(m)["kind"] != "change" {
		t.Errorf("ms change: %v", m)
	}
	for text, kind := range map[string]string{"help": "help", "hello there": "unsupported", "temperature Lobby AC": "unsupported", "temperature garage": "unsupported"} {
		if _, m := say(&customerA, text, "en", ""); data(m)["kind"] != kind {
			t.Errorf("%s: %v", text, m)
		}
	}
	if _, m := say(&customerA, "bantuan", "ms", ""); data(m)["kind"] != "help" || data(m)["messageKey"] != "voice.help" {
		t.Errorf("bantuan: %v", m)
	}
	if _, m := say(&customerA, "help", "ms", ""); data(m)["kind"] != "unsupported" {
		t.Error("English help in Malay grammar")
	}
	// HQ sees both customers' bedrooms → candidates, then a selection
	code, m = say(&hq, "temperature bedroom", "en", "")
	if code != 200 || data(m)["kind"] != "candidates" || len(data(m)["candidates"].([]any)) < 2 {
		t.Fatalf("candidates: %d %v", code, m)
	}
	label := data(m)["candidates"].([]any)[0].(map[string]any)["pathLabel"].(string)
	if label == "" {
		t.Error("path label")
	}
	if _, m := say(&hq, "temperature bedroom", "en", `,"selectedUnitId":"`+bedroomA+`"`); data(m)["kind"] != "temperature" || data(m)["unitId"] != bedroomA {
		t.Errorf("selected: %v", m)
	}
	if code, _ := say(&hq, "temperature bedroom", "en", `,"selectedUnitId":"`+uuid.NewString()+`"`); code != 404 {
		t.Error("selection outside the candidates")
	}
	if code, _ := say(&customerA, " ", "en", ""); code != 422 {
		t.Error("blank text")
	}
	if code, _ := say(&customerA, "help", "jp", ""); code != 422 {
		t.Error("bad locale")
	}
	if code, _ := say(&contrA, "help", "en", ""); code != 403 {
		t.Error("contractor voice")
	}

	// monthly export
	exp := func(a *actor, month, props, sections, format string) (int, map[string]any) {
		return post(s, a, "energy.exportReport", `{"month":"`+month+`","propertyIds":[`+props+`],"sections":[`+sections+`],"format":"`+format+`"}`)
	}
	homeA := `"` + seed.ID("property-home-a").String() + `"`
	all := `"energy_cost","month_comparison","co2_offsets","alerts_maintenance"`
	code, m = exp(&customerA, "2026-08", homeA, all, "csv")
	if code != 200 || data(m)["fileName"] != "energy-report-2026-08.csv" || data(m)["mime"] != "text/csv" || data(m)["size"].(float64) <= 0 {
		t.Fatalf("export: %d %v", code, m)
	}
	if _, m := exp(&customerA, "2026-08", homeA, `"energy_cost"`, "pdf"); data(m)["mime"] != "application/pdf" {
		t.Error("pdf export")
	}
	for name, tc := range map[string][5]string{
		"month not ended": {"2026-09", homeA, all, "csv", "422"},
		"bad month":       {"2026-13", homeA, all, "csv", "422"},
		"no sections":     {"2026-08", homeA, "", "csv", "422"},
		"bad format":      {"2026-08", homeA, all, "xlsx", "422"},
		"other customer":  {"2026-08", `"` + seed.ID("property-home-b").String() + `"`, all, "csv", "404"},
	} {
		if code, _ := exp(&customerA, tc[0], tc[1], tc[2], tc[3]); itoa(code) != tc[4] {
			t.Errorf("%s: %d", name, code)
		}
	}
	if code, _ := exp(&hq, "2026-08", homeA, all, "csv"); code != 403 {
		t.Error("HQ export")
	}
}
