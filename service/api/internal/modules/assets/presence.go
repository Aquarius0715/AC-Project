package assets

import (
	"context"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// PinnedProperty is a customer's property with HQ's map pin (DEC-109): its away/home radius (nil for the default)
// and the installed units an arrival or a departure is about.
type PinnedProperty struct {
	ID           uuid.UUID
	Pin          GeoPoint
	RadiusMeters *int
	UnitIDs      []uuid.UUID
}

// PinnedProperties returns the customer organization's properties that have a pin, for the away/home geofence
// (presence.report, IR826). Archived properties and units, and units not installed yet, are left out.
func (m *Module) PinnedProperties(
	ctx context.Context,
	call *ops.Call,
	org uuid.UUID,
) ([]PinnedProperty, error) {
	rows, err := call.Tx.Query(
		ctx,
		`SELECT id, latitude, longitude, geofence_radius_m FROM assets.properties
		WHERE customer_org_id = $1 AND NOT archived AND latitude IS NOT NULL AND longitude IS NOT NULL ORDER BY id`,
		org,
	)
	if err != nil {
		return nil, apperr.Wrap(err, "select assets.properties")
	}
	defer rows.Close()

	out := []PinnedProperty{}

	for rows.Next() {
		var property PinnedProperty

		err := rows.Scan(
			&property.ID,
			&property.Pin.Latitude,
			&property.Pin.Longitude,
			&property.RadiusMeters,
		)
		if err != nil {
			return nil, apperr.Wrap(err, "scan assets.properties")
		}

		out = append(out, property)
	}

	err = rows.Err()
	if err != nil {
		return nil, apperr.Wrap(err, "select assets.properties")
	}

	rows.Close()

	for i := range out {
		out[i].UnitIDs, err = m.unitIDs(
			ctx,
			call,
			"u.property_id = $1 AND NOT u.archived AND u.install_state = 'installed'",
			[]any{out[i].ID},
		)
		if err != nil {
			return nil, err
		}
	}

	return out, nil
}
