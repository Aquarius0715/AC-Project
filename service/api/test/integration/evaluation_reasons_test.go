package integration

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// TestAutomationDecideReasons covers the D02 skip reasons decided after a rule's own trigger matched: a saved action
// the unit's capability no longer supports (invalid_capability), an owner who lost control of the unit
// (owner_forbidden) and a location rule whose owner's consent is gone (consent_revoked). Each rule is removed before
// the next so the decision names its reason.
func TestAutomationDecideReasons(t *testing.T) {
	s := server(t)
	u := boundUnit(t, s, "online")
	at := clock.Format(time.RFC3339)
	rule := func(cond, action string) (string, int) {
		t.Helper()
		code, m := write(s, &customerB, "automations.save", `{"name":"reason","unitIds":["`+u+`"],"timezone":"Asia/Kuala_Lumpur","enabled":true,"priority":10,"kind":"event","condition":`+cond+`,"action":`+action+`}`, 0)
		if code != 200 {
			t.Fatalf("rule: %d %v", code, m)
		}
		return data(m)["id"].(string), ver(m)
	}
	remove := func(id string, v int) {
		t.Helper()
		if code, m := write(s, &customerB, "automations.delete", `{"id":"`+id+`"}`, v); code != 200 {
			t.Fatalf("delete rule: %d %v", code, m)
		}
	}
	reason := func(facts string) string {
		t.Helper()
		code, m := post(s, &customerB, "automations.simulate", `{"eventId":"`+uuid.NewString()+`","occurredAt":"`+at+`","unitIds":["`+u+`"],"facts":[`+facts+`],"phase":"condition"}`)
		if code != 200 {
			t.Fatalf("simulate: %d %v", code, m)
		}
		d := data(m)["results"].([]any)[0].(map[string]any)
		if d["decision"] != "suppressed" {
			t.Fatalf("decision %v", d)
		}
		r, _ := d["reason"].(string)
		return r
	}
	fact := func(metric, value, unitName string) string {
		return `{"unitId":"` + u + `","metric":"` + metric + `","value":` + value + `,"unit":"` + unitName + `","observedAt":"` + at + `","quality":"valid"}`
	}
	occupied := fact("occupied", "true", "boolean")

	// invalid_capability: the saved action is patched to a fan level the model does not offer
	id, v := rule(`{"type":"occupancy","occupied":true}`, `{"kind":"set_fan","fanLevel":"high"}`)
	owner(t, `UPDATE control.automations SET definition = jsonb_set(definition, '{action}', '{"kind":"set_fan","fanLevel":"turbo"}') WHERE id = $1`, id)
	if r := reason(occupied); r != "invalid_capability" {
		t.Errorf("unsupported action: %q", r)
	}
	remove(id, v)

	// owner_forbidden: the owner is a membership of another organization (no control.execute on this unit)
	id, v = rule(`{"type":"occupancy","occupied":true}`, `{"kind":"set_temperature","celsius":24}`)
	owner(t, `UPDATE control.automations SET owner_membership_id = $2 WHERE id = $1`, id, seed.ID("customer-a"))
	if r := reason(occupied); r != "owner_forbidden" {
		t.Errorf("foreign owner: %q", r)
	}
	owner(t, `UPDATE control.automations SET owner_membership_id = $2 WHERE id = $1`, id, seed.ID("customer-b"))
	remove(id, v)

	// consent_revoked: a location rule saved with consent whose consent row was flipped without the withdrawal event
	_, c := post(s, &customerB, "consents.get", `{"purpose":"location_automation"}`)
	if data(c)["granted"] != true {
		if code, m := write(s, &customerB, "consents.update", `{"purpose":"location_automation","granted":true}`, ver(c)); code != 200 {
			t.Fatalf("consent: %d %v", code, m)
		}
	}
	id, v = rule(`{"type":"location","event":"arrival"}`, `{"kind":"set_power","power":true}`)
	owner(t, `UPDATE identity.consents SET granted = false WHERE membership_id = $1 AND purpose = 'location_automation'`, seed.ID("customer-b"))
	if r := reason(fact("location", `"arrival"`, "event")); r != "consent_revoked" {
		t.Errorf("consent gone: %q", r)
	}
	owner(t, `UPDATE identity.consents SET granted = true WHERE membership_id = $1 AND purpose = 'location_automation'`, seed.ID("customer-b"))
	if r := reason(fact("location", `"departure"`, "event")); r != "no_match" {
		t.Errorf("other location event: %q", r)
	}
	remove(id, v)
}
