package geo_test

import (
	"testing"

	"github.com/pradita/ac-project/service/api/internal/platform/geo"
)

// Distances computed independently (haversine on the same sphere), from a few metres to across the antimeridian.
func TestDistance(t *testing.T) {
	t.Parallel()

	klcc := geo.Point{Latitude: 3.1579, Longitude: 101.7116}
	kualaLumpur := geo.Point{Latitude: 3.139, Longitude: 101.6869}
	singapore := geo.Point{Latitude: 1.3521, Longitude: 103.8198}
	east, west := geo.Point{
		Latitude:  0,
		Longitude: 179.9995,
	}, geo.Point{
		Latitude:  0,
		Longitude: -179.9995,
	}

	for name, example := range map[string]struct {
		from, to geo.Point
		want     float64
	}{
		"same point":                {klcc, klcc, 0},
		"0.0018° north":             {geo.Point{Latitude: 3.1597, Longitude: 101.7116}, klcc, 200},
		"0.0018° east":              {geo.Point{Latitude: 3.1579, Longitude: 101.7134}, klcc, 200},
		"Kuala Lumpur to Singapore": {kualaLumpur, singapore, 309_253},
		"across the antimeridian":   {east, west, 111},
	} {
		if got := geo.Distance(example.from, example.to); got != example.want {
			t.Errorf("%s: %v m, want %v", name, got, example.want)
		}
	}

	for _, point := range []geo.Point{{Latitude: 91, Longitude: 0}, {Latitude: 0, Longitude: -181}} {
		if point.OnGlobe() {
			t.Errorf("%+v is off the globe", point)
		}
	}
}
