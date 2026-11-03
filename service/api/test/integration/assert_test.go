package integration_test

import (
	"encoding/json"
	"testing"
)

// jsonBytes encodes v as JSON and fails the test when it cannot.
func jsonBytes(t *testing.T, v any) []byte {
	t.Helper()

	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}

	return b
}

// asObj, asArr, asStr and asNum read a decoded JSON value as the JSON type the test expects. Anything else fails the
// test and shows the value, where a bare type assertion would panic (forcetypeassert).
func asObj(tb testing.TB, v any) map[string]any {
	tb.Helper()

	return as[map[string]any](tb, v)
}

func asArr(tb testing.TB, v any) []any {
	tb.Helper()

	return as[[]any](tb, v)
}

func asStr(tb testing.TB, v any) string {
	tb.Helper()

	return as[string](tb, v)
}

func asNum(tb testing.TB, v any) float64 {
	tb.Helper()

	return as[float64](tb, v)
}

// as reads v as T and fails the test when v holds something else.
func as[T any](tb testing.TB, v any) T {
	tb.Helper()

	x, ok := v.(T)
	if !ok {
		tb.Fatalf("got %T %#v, want %T", v, v, x)
	}

	return x
}
