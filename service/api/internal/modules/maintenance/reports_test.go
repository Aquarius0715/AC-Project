package maintenance

import (
	"maps"
	"testing"
	"time"
)

// TestSubmitErrors covers the IR100 checks jobs.submit applies to a draft: every component recorded once with a
// result (a reason unless normal), a work text of 10–4000 characters, a next action (a follow-up needs a future date
// and a note), part quantities 1–999, ready attachments and readings without a unit mismatch.
func TestSubmitErrors(t *testing.T) {
	now := time.Date(2026, 9, 15, 1, 0, 0, 0, time.UTC)
	str := func(s string) *string { return &s }
	at := func(h int) *time.Time { v := now.Add(time.Duration(h) * time.Hour); return &v }
	components := []string{"filter", "drain"}
	valid := func() Report {
		return Report{
			Items:          []Item{{ComponentKey: "filter", Result: str("normal")}, {ComponentKey: "drain", Result: str("attention"), Reason: str("slow flow")}},
			WorkText:       "Cleaned the filter and checked the drain.",
			NextAction:     &NextAction{Kind: "none"},
			Parts:          []Part{{Name: "Filter", Quantity: 1}},
			AttachmentRefs: []AttachmentRef{{Status: "ready"}},
		}
	}
	if fe := submitErrors(&Report{Items: valid().Items, WorkText: valid().WorkText, NextAction: valid().NextAction}, components, now); len(fe) != 0 {
		t.Fatalf("a complete draft: %v", fe)
	}
	for name, c := range map[string]struct {
		change func(*Report)
		want   map[string]string
	}{
		"no result":          {func(r *Report) { r.Items[0].Result = nil }, map[string]string{"items": "errors.result_required"}},
		"no reason":          {func(r *Report) { r.Items[1].Reason = str("") }, map[string]string{"items": "errors.reason_required"}},
		"component missing":  {func(r *Report) { r.Items = r.Items[:1] }, map[string]string{"items": "errors.components_mismatch"}},
		"component twice":    {func(r *Report) { r.Items[1].ComponentKey = "filter" }, map[string]string{"items": "errors.components_mismatch"}},
		"short work text":    {func(r *Report) { r.WorkText = "Done." }, map[string]string{"workText": "error.length"}},
		"no next action":     {func(r *Report) { r.NextAction = nil }, map[string]string{"nextAction": "error.required"}},
		"follow-up no note":  {func(r *Report) { r.NextAction = &NextAction{Kind: "follow_up", Date: at(48)} }, map[string]string{"nextAction": "error.invalid"}},
		"follow-up past":     {func(r *Report) { r.NextAction = &NextAction{Kind: "follow_up", Date: at(-1), Note: str("check again")} }, map[string]string{"nextAction": "error.invalid"}},
		"zero parts":         {func(r *Report) { r.Parts[0].Quantity = 0 }, map[string]string{"parts": "error.range"}},
		"too many parts":     {func(r *Report) { r.Parts[0].Quantity = 1000 }, map[string]string{"parts": "error.range"}},
		"photo processing":   {func(r *Report) { r.AttachmentRefs[0].Status = "processing" }, map[string]string{"attachmentIds": "errors.attachment_not_ready"}},
		"reading wrong unit": {func(r *Report) { r.Measurements = []ReportMeasurement{{QualityReason: str("unit_mismatch")}} }, map[string]string{"measurements": "errors.unit_mismatch"}},
	} {
		r := valid()
		c.change(&r)
		if fe := submitErrors(&r, components, now); !maps.Equal(fe, c.want) {
			t.Errorf("%s: %v want %v", name, fe, c.want)
		}
	}
	follow := valid()
	follow.NextAction = &NextAction{Kind: "follow_up", Date: at(48), Note: str("replace the drain pan")}
	if fe := submitErrors(&follow, components, now); len(fe) != 0 {
		t.Errorf("a follow-up with a date and a note: %v", fe)
	}
}
