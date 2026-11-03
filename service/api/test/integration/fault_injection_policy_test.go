package integration

import (
	"testing"
)

// TestAutomationPolicyEditRollsBackWhenItCannotBeWritten: an automation policy whose edit cannot be written keeps its
// name and version. The operation fails as a server error, and the policy reads back as it was, because the database
// rolls the transaction back (policies.save on an existing policy; the policy write fails).
func TestAutomationPolicyEditRollsBackWhenItCannotBeWritten(t *testing.T) {
	srv := server(t)

	code, created := write(
		srv,
		&hq,
		"policies.save",
		autoPolicyBody(map[string]string{"name": `"Before the edit"`}),
		0,
	)
	if code != 200 {
		t.Fatalf("create: %d %v", code, created)
	}

	id := asStr(t, data(created)["id"])

	t.Cleanup(func() {
		owner(t, `DELETE FROM control.automation_policy_units WHERE policy_id = $1`, id)
		owner(t, `DELETE FROM control.automation_policies WHERE id = $1`, id)
	})

	_, before := post(srv, &hq, "policies.get", `{"policyId":"`+id+`"}`)

	broken := injectAt(t, "UPDATE control.automation_policies SET name = ")

	code, res := write(
		broken,
		&hq,
		"policies.save",
		autoPolicyBody(map[string]string{"id": `"` + id + `"`, "name": `"Renamed"`}),
		ver(before),
	)
	if code < 500 {
		t.Fatalf("edit with a failing policy write: %d %v, want a server error", code, res)
	}

	_, after := post(srv, &hq, "policies.get", `{"policyId":"`+id+`"}`)
	if data(after)["name"] != "Before the edit" || ver(after) != ver(before) {
		t.Errorf("a failed edit changed the policy: before %v, after %v", before, after)
	}
}
