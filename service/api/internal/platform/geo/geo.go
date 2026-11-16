// Package geo measures distances between WGS 84 points: the technician's site check-in and the customer's away/home
// geofence against the pin HQ set for a property (DEC-109).
package geo

import "math"

const (
	// earthRadius is the mean Earth radius in metres (IUGG), the sphere the distance is measured on.
	earthRadius = 6_371_008.8
	// radiansPerDegree converts the degrees of a point.
	radiansPerDegree = math.Pi / 180
)

// Point is a position in WGS 84 degrees.
type Point struct {
	Latitude  float64
	Longitude float64
}

// OnGlobe reports whether the point has a latitude in −90..90 and a longitude in −180..180.
func (p Point) OnGlobe() bool {
	return p.Latitude >= -90 && p.Latitude <= 90 && p.Longitude >= -180 && p.Longitude <= 180
}

// Distance is the great-circle distance between two points in metres, rounded to the metre: the angle between their
// unit vectors, atan2(|a×b|, a·b), which stays exact for a few metres and for half the globe.
func Distance(from, to Point) float64 {
	a := unitVector(from)
	b := unitVector(to)
	cross := math.Hypot(math.Hypot(a[1]*b[2]-a[2]*b[1], a[2]*b[0]-a[0]*b[2]), a[0]*b[1]-a[1]*b[0])
	dot := a[0]*b[0] + a[1]*b[1] + a[2]*b[2]

	return math.Round(earthRadius * math.Atan2(cross, dot))
}

// unitVector is the point on the unit sphere.
func unitVector(p Point) [3]float64 {
	lat, lng := p.Latitude*radiansPerDegree, p.Longitude*radiansPerDegree

	return [3]float64{math.Cos(lat) * math.Cos(lng), math.Cos(lat) * math.Sin(lng), math.Sin(lat)}
}
