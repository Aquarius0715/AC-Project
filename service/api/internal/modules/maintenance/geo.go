package maintenance

import (
	"math"

	"github.com/pradita/ac-project/service/api/internal/platform/geo"
)

// The site check-in measured on the server (DEC-109, IR825): the device reports its GPS position and its accuracy,
// the property's pin is HQ's, and the distance between them decides.
const (
	// checkInRadius is how near the site a location check-in must be, in metres (IR111).
	checkInRadius = 200.0
	// maxAccuracy is the coarsest fix a location check-in takes, in metres: beyond it the device does not know where
	// it is well enough to tell 200 m.
	maxAccuracy = 100.0
)

// Position is jobs.checkIn's position: the device's fix in WGS 84 degrees and its accuracy radius in metres.
type Position struct {
	Latitude       float64 `json:"latitude"`
	Longitude      float64 `json:"longitude"`
	AccuracyMeters float64 `json:"accuracyMeters"`
}

// onGlobe reports whether the fix is a point on the globe with a non-negative accuracy.
func (p Position) onGlobe() bool {
	return geo.Point{Latitude: p.Latitude, Longitude: p.Longitude}.OnGlobe() &&
		p.AccuracyMeters >= 0
}

// distanceTo is the great-circle distance from the fix to the pin, in metres rounded to the metre.
func (p Position) distanceTo(pin SitePoint) float64 {
	return geo.Distance(
		geo.Point{Latitude: p.Latitude, Longitude: p.Longitude},
		geo.Point{Latitude: pin.Latitude, Longitude: pin.Longitude},
	)
}

// near reports whether the site may be within the check-in radius: the fix's own error counts in the device's favour,
// so a fix 250 m away with 60 m accuracy may be 190 m away and passes.
func near(distance, accuracy float64) bool {
	return math.Max(0, distance-accuracy) <= checkInRadius
}
