package maintenance

import (
	"maps"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// componentFilter is the first component of the unit the test drafts are written for.
const componentFilter = "filter"

// submitNow is when the test drafts are submitted.
func submitNow() time.Time { return time.Date(2026, 9, 15, 1, 0, 0, 0, time.UTC) }

// submitComponents are the components of the unit the test drafts are written for.
func submitComponents() []string { return []string{componentFilter, "drain"} }

// inHours is the time hours after submitNow.
func inHours(hours int) *time.Time {
	v := submitNow().Add(time.Duration(hours) * time.Hour)

	return &v
}

// draftItems record both components: the filter normal, the drain with a finding and its reason.
func draftItems() []Item {
	return []Item{
		{
			ComponentKey:   componentFilter,
			Result:         new(resultNormal),
			ID:             uuid.UUID{},
			ComponentGroup: "",
			Reason:         nil,
			EvidenceIDs:    nil,
			AuthorID:       uuid.UUID{},
			ObservedAt:     time.Time{},
		},
		{
			ComponentKey:   "drain",
			Result:         new("attention"),
			Reason:         new("slow flow"),
			ID:             uuid.UUID{},
			ComponentGroup: "",
			EvidenceIDs:    nil,
			AuthorID:       uuid.UUID{},
			ObservedAt:     time.Time{},
		},
	}
}

// validDraft passes every IR100 check: both components recorded, a work text, no next action, a part and a ready
// attachment.
func validDraft() Report {
	return Report{
		Items:      draftItems(),
		WorkText:   "Cleaned the filter and checked the drain.",
		NextAction: &NextAction{Kind: nextActionNone, Date: nil, Note: nil},
		Parts: []Part{
			{
				Name:                 "Filter",
				Quantity:             1,
				CatalogCode:          nil,
				Source:               "",
				LotSerial:            nil,
				ReplacesComponentKey: nil,
				OldPartDisposal:      nil,
				ReceiptAttachmentID:  nil,
			},
		},
		AttachmentRefs: []AttachmentRef{
			{
				Status:    "ready",
				ID:        uuid.UUID{},
				TenantID:  uuid.UUID{},
				Version:   0,
				CreatedAt: time.Time{},
				UpdatedAt: time.Time{},
				JobID:     uuid.UUID{},
				ReportID:  uuid.UUID{},
				BlobID:    uuid.UUID{},
				Name:      "",
				Mime:      "",
				Size:      0,
			},
		},
		ID:                 uuid.UUID{},
		TenantID:           uuid.UUID{},
		Version:            0,
		CreatedAt:          time.Time{},
		UpdatedAt:          time.Time{},
		ReviewAvailability: nil,
		JobID:              uuid.UUID{},
		AuthorID:           uuid.UUID{},
		Measurements:       nil,
		Refrigerant:        nil,
		SignOff:            nil,
		SubmittedAt:        nil,
		AcceptedAt:         nil,
		ReviewHistory:      nil,
		State:              "",
	}
}

// TestSubmitErrorsComplete: a draft with the components' results, a work text and a next action is complete, and so
// is one with a follow-up that has a future date and a note.
func TestSubmitErrorsComplete(t *testing.T) {
	t.Parallel()

	if fieldErrs := submitErrors(
		&Report{
			Items:              validDraft().Items,
			WorkText:           validDraft().WorkText,
			NextAction:         validDraft().NextAction,
			ID:                 uuid.UUID{},
			TenantID:           uuid.UUID{},
			Version:            0,
			CreatedAt:          time.Time{},
			UpdatedAt:          time.Time{},
			ReviewAvailability: nil,
			JobID:              uuid.UUID{},
			AuthorID:           uuid.UUID{},
			Measurements:       nil,
			Parts:              nil,
			Refrigerant:        nil,
			SignOff:            nil,
			AttachmentRefs:     nil,
			SubmittedAt:        nil,
			AcceptedAt:         nil,
			ReviewHistory:      nil,
			State:              "",
		},
		submitComponents(),
		submitNow(),
	); len(
		fieldErrs,
	) != 0 {
		t.Fatalf("a complete draft: %v", fieldErrs)
	}

	follow := validDraft()

	follow.NextAction = &NextAction{
		Kind: nextActionFollowUp,
		Date: inHours(48),
		Note: new("replace the drain pan"),
	}
	if fe := submitErrors(&follow, submitComponents(), submitNow()); len(fe) != 0 {
		t.Errorf("a follow-up with a date and a note: %v", fe)
	}
}

// TestSubmitErrors covers the IR100 checks jobs.submit applies to a draft: every component recorded once with a
// result (a reason unless normal), a work text of 10–4000 characters, a next action (a follow-up needs a future date
// and a note), part quantities 1–999, ready attachments and readings without a unit mismatch.
func TestSubmitErrors(t *testing.T) {
	t.Parallel()

	for name, testCase := range map[string]struct {
		change func(*Report)
		want   map[string]string
	}{
		"no result": {
			func(r *Report) { r.Items[0].Result = nil },
			map[string]string{fieldItems: "errors.result_required"},
		},
		"no reason": {
			func(r *Report) { r.Items[1].Reason = new("") },
			map[string]string{fieldItems: "errors.reason_required"},
		},
		"component missing": {
			func(r *Report) { r.Items = r.Items[:1] },
			map[string]string{fieldItems: "errors.components_mismatch"},
		},
		"component twice": {
			func(r *Report) { r.Items[1].ComponentKey = componentFilter },
			map[string]string{fieldItems: "errors.components_mismatch"},
		},
		"short work text": {func(r *Report) { r.WorkText = "Done." }, map[string]string{fieldWorkText: apperr.KeyLength}},
		"blank work text": {
			func(r *Report) { r.WorkText = strings.Repeat(" ", 12) },
			map[string]string{fieldWorkText: apperr.KeyLength},
		},
		"padded short work text": {
			func(r *Report) { r.WorkText = "  Done.     " },
			map[string]string{fieldWorkText: apperr.KeyLength},
		},
		"no next action": {func(r *Report) { r.NextAction = nil }, map[string]string{fieldNextAction: apperr.KeyRequired}},
		"follow-up no note": {
			func(r *Report) { r.NextAction = &NextAction{Kind: nextActionFollowUp, Date: inHours(48), Note: nil} },
			map[string]string{fieldNextAction: apperr.KeyInvalid},
		},
		"follow-up blank note": {
			func(r *Report) {
				r.NextAction = &NextAction{Kind: nextActionFollowUp, Date: inHours(48), Note: new("   ")}
			},
			map[string]string{fieldNextAction: apperr.KeyInvalid},
		},
		"follow-up past": {
			func(r *Report) {
				r.NextAction = &NextAction{Kind: nextActionFollowUp, Date: inHours(-1), Note: new("check again")}
			},
			map[string]string{fieldNextAction: apperr.KeyInvalid},
		},
		"zero parts":     {func(r *Report) { r.Parts[0].Quantity = 0 }, map[string]string{"parts": apperr.KeyRange}},
		"too many parts": {func(r *Report) { r.Parts[0].Quantity = 1000 }, map[string]string{"parts": apperr.KeyRange}},
		"photo processing": {
			func(r *Report) { r.AttachmentRefs[0].Status = "processing" },
			map[string]string{"attachmentIds": "errors.attachment_not_ready"},
		},
		"reading wrong unit": {func(r *Report) {
			r.Measurements = []ReportMeasurement{
				{
					QualityReason: new("unit_mismatch"), ID: uuid.UUID{}, TenantID: uuid.UUID{}, Version: 0,
					CreatedAt: time.Time{}, UpdatedAt: time.Time{}, BoundaryID: nil, RawUnit: nil, UnitID: uuid.UUID{},
					SensorID: uuid.UUID{}, ComponentKey: "", Metric: "", Value: nil, Unit: "", ObservedAt: time.Time{},
					ReceivedAt: time.Time{}, Origin: "", Quality: "", IsDemo: false, Sequence: 0, EventID: uuid.UUID{},
				},
			}
		}, map[string]string{"measurements": "errors.unit_mismatch"}},
	} {
		r := validDraft()
		testCase.change(&r)

		if fe := submitErrors(&r, submitComponents(), submitNow()); !maps.Equal(fe, testCase.want) {
			t.Errorf("%s: %v want %v", name, fe, testCase.want)
		}
	}
}
