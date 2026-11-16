package maintenance

import "testing"

// The check-in distance (DEC-109, IR825): the haversine in metres, and the accuracy counted in the device's favour.
func TestCheckInDistance(t *testing.T) {
	t.Parallel()

	klcc := SitePoint{Latitude: 3.1579, Longitude: 101.7116}
	fix := func(lat, lng float64) Position { return Position{Latitude: lat, Longitude: lng, AccuracyMeters: 0} }

	for name, example := range map[string]struct {
		from Position
		to   SitePoint
		want float64
	}{
		"same point":                {fix(3.1579, 101.7116), klcc, 0},
		"0.0018° north":             {fix(3.1597, 101.7116), klcc, 200},
		"0.0018° east":              {fix(3.1579, 101.7134), klcc, 200},
		"Kuala Lumpur to Singapore": {fix(3.139, 101.6869), SitePoint{Latitude: 1.3521, Longitude: 103.8198}, 309_253},
		"across the antimeridian":   {fix(0, 179.9995), SitePoint{Latitude: 0, Longitude: -179.9995}, 111},
	} {
		if got := example.from.distanceTo(example.to); got != example.want {
			t.Errorf("%s: %v m, want %v", name, got, example.want)
		}
	}

	for _, check := range []struct {
		distance, accuracy float64
		want               bool
	}{
		{200, 0, true}, {201, 0, false}, {250, 60, true}, {250, 49, false}, {30, 100, true},
	} {
		if near(check.distance, check.accuracy) != check.want {
			t.Errorf("near(%v, %v) = %v", check.distance, check.accuracy, !check.want)
		}
	}

	for _, position := range []Position{
		{Latitude: 91, Longitude: 0, AccuracyMeters: 0},
		{Latitude: 0, Longitude: -181, AccuracyMeters: 0},
		{Latitude: 0, Longitude: 0, AccuracyMeters: -1},
	} {
		if position.onGlobe() {
			t.Errorf("%+v is not a fix", position)
		}
	}
}
