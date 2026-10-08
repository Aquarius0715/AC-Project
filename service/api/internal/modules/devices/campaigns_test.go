package devices

import "testing"

func TestWaveOf(t *testing.T) {
	waves := []Wave{{"pilot", 20}, {"half", 50}, {"all", 100}}
	got := []int{}
	for i := 0; i < 10; i++ {
		got = append(got, waveOf(i, 10, waves))
	}
	want := []int{0, 0, 1, 1, 1, 2, 2, 2, 2, 2}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
	if waveOf(0, 1, []Wave{{"all", 100}}) != 0 {
		t.Fatal("single device")
	}
}
