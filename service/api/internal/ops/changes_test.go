package ops

import "testing"

func TestChanges(t *testing.T) {
	s := func(v string) *string { return &v }
	b, a := Changes(map[string]any{"name": "Old", "modes": []string{"cool"}, "same": 1, "gone": "x", "nil": nil},
		map[string]any{"name": "New", "modes": []string{"cool", "dry"}, "same": 1, "added": true, "nil": nil})
	want := map[string][2]*string{"name": {s("Old"), s("New")}, "modes": {s(`["cool"]`), s(`["cool","dry"]`)}, "gone": {s("x"), nil}, "added": {nil, s("true")}}
	if len(b) != len(want) || len(a) != len(want) {
		t.Fatalf("changed keys: before %v after %v", b, a)
	}
	for k, w := range want {
		if (b[k] == nil) != (w[0] == nil) || (b[k] != nil && *b[k] != *w[0]) || (a[k] == nil) != (w[1] == nil) || (a[k] != nil && *a[k] != *w[1]) {
			t.Errorf("%s: %v → %v", k, b[k], a[k])
		}
	}
	if b, a := Changes(nil, nil); len(b) != 0 || len(a) != 0 {
		t.Error("no change")
	}
}

func TestMasked(t *testing.T) {
	s := func(v string) *string { return &v }
	m := Masked(map[string]*string{"contactPhone": s("+60 12"), "Email": s("a@b"), "name": s("Ali"), "apiToken": nil})
	if *m["contactPhone"] != "***masked***" || *m["Email"] != "***masked***" || *m["name"] != "Ali" || m["apiToken"] != nil {
		t.Fatalf("masked: %v", m)
	}
	if Masked(nil) != nil {
		t.Error("nil stays nil")
	}
}
