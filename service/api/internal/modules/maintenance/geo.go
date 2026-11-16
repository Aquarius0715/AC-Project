package maintenance

import "math"

// The site check-in measured on the server (DEC-109, IR825): the device reports its GPS position and its accuracy,
// the property's pin is HQ's, and the distance between them decides.
const (
	// earthRadius is the mean Earth radius in metres (IUGG), the haversine's sphere.
	earthRadius = 6_371_008.8
	// checkInRadius is how near the site a location check-in must be, in metres (IR111).
	checkInRadius = 200.0
	// maxAccuracy is the coarsest fix a location check-in takes, in metres: beyond it the device does not know where
	// it is well enough to tell 200 m.
	maxAccuracy = 100.0
	// radiansPerDegree converts the WGS 84 degrees of a fix and a pin.
	radiansPerDegree = math.Pi / 180
)

// Position is jobs.checkIn's position: the device's fix in WGS 84 degrees and its accuracy radius in metres.
type Position struct {
	Latitude       float64 `json:"latitude"`
	Longitude      float64 `json:"longitude"`
	AccuracyMeters float64 `json:"accuracyMeters"`
}

// onGlobe reports whether the fix is a point on the globe with a non-negative accuracy.
func (p Position) onGlobe() bool {
	return p.Latitude >= -90 && p.Latitude <= 90 && p.Longitude >= -180 && p.Longitude <= 180 &&
		p.AccuracyMeters >= 0
}

// distanceTo is the great-circle distance from the fix to the pin, in metres rounded to the metre: the angle between
// the two points' unit vectors, atan2(|a×b|, a·b), which stays exact for a few metres and for half the globe.
func (p Position) distanceTo(pin SitePoint) float64 {
	a := unitVector(p.Latitude, p.Longitude)
	b := unitVector(pin.Latitude, pin.Longitude)
	cross := math.Hypot(math.Hypot(a[1]*b[2]-a[2]*b[1], a[2]*b[0]-a[0]*b[2]), a[0]*b[1]-a[1]*b[0])
	dot := a[0]*b[0] + a[1]*b[1] + a[2]*b[2]

	return math.Round(earthRadius * math.Atan2(cross, dot))
}

// unitVector is the point on the unit sphere at a latitude and longitude in degrees.
func unitVector(latitude, longitude float64) [3]float64 {
	lat, lng := latitude*radiansPerDegree, longitude*radiansPerDegree

	return [3]float64{math.Cos(lat) * math.Cos(lng), math.Cos(lat) * math.Sin(lng), math.Sin(lat)}
}

// near reports whether the site may be within the check-in radius: the fix's own error counts in the device's favour,
// so a fix 250 m away with 60 m accuracy may be 190 m away and passes.
func near(distance, accuracy float64) bool {
	return math.Max(0, distance-accuracy) <= checkInRadius
}
