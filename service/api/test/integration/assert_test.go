package integration

import "testing"

// asObj, asArr, asStr and asNum read a decoded JSON value as the JSON type the test expects. Anything else fails the
// test and shows the value, where a bare type assertion would panic (forcetypeassert).
func asObj(t testing.TB, v any) map[string]any {
	t.Helper()

	return as[map[string]any](t, v)
}

func asArr(t testing.TB, v any) []any {
	t.Helper()

	return as[[]any](t, v)
}

func asStr(t testing.TB, v any) string {
	t.Helper()

	return as[string](t, v)
}

func asNum(t testing.TB, v any) float64 {
	t.Helper()

	return as[float64](t, v)
}

// as reads v as T and fails the test when v holds something else.
func as[T any](t testing.TB, v any) T {
	t.Helper()

	x, ok := v.(T)
	if !ok {
		t.Fatalf("got %T %#v, want %T", v, v, x)
	}

	return x
}
