package control

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/modules/identity"
	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/geo"
)

// The customer's away/home geofence (DEC-109, IR826). A member's device reports its position; the server compares it
// with the pins HQ set for the customer's properties and keeps, per member and property, only whether the device is
// inside. A property's household arrives when its first member comes inside and departs when its last one leaves:
// that change is a location fact for the property's units, evaluated as the scheduler evaluates a schedule.
const (
	// DefaultGeofenceRadius is the radius of a property that has none, in metres (Property.geofenceRadiusMeters).
	DefaultGeofenceRadius = 150
	// presenceHysteresis is how far beyond the radius a device must be before it counts as gone, in metres: a fix
	// jitters, and a device standing at the edge must not arrive and depart every minute.
	presenceHysteresis = 50.0
	// maxPresenceAccuracy is the coarsest fix a report takes, in metres (as the site check-in, IR825).
	maxPresenceAccuracy = 100.0
	locationConsent     = "location_automation"
	eventArrival        = "arrival"
	eventDeparture      = "departure"
)

// Sites are the customer's pinned properties (Assets).
type Sites interface {
	PinnedSites(ctx context.Context, c *ops.Call, org uuid.UUID) ([]Site, error)
}

// Site is a pinned property: its pin, its away/home radius in metres and its installed units.
type Site struct {
	PropertyID   uuid.UUID
	Pin          geo.Point
	RadiusMeters int
	UnitIDs      []uuid.UUID
}

// PresenceInput is presence.report input: the device's fix in WGS 84 degrees and its accuracy in metres. The fix is
// compared and dropped; it is never stored.
type PresenceInput struct {
	Latitude       float64 `json:"latitude"`
	Longitude      float64 `json:"longitude"`
	AccuracyMeters float64 `json:"accuracyMeters"`
}

// Validate implements ops.Validator: a fix on the globe that knows where it is to 100 m.
func (in *PresenceInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if !(geo.Point{Latitude: in.Latitude, Longitude: 0}).OnGlobe() {
		fieldErrs["latitude"] = apperr.KeyRange
	}

	if !(geo.Point{Latitude: 0, Longitude: in.Longitude}).OnGlobe() {
		fieldErrs["longitude"] = apperr.KeyRange
	}

	switch {
	case in.AccuracyMeters < 0:
		fieldErrs["accuracyMeters"] = apperr.KeyRange
	case in.AccuracyMeters > maxPresenceAccuracy:
		fieldErrs["accuracyMeters"] = "errors.position_inaccurate"
	}

	return fieldErrs
}

// PresenceTransition is one property whose household arrived or departed with this report, and the Commands its
// location automations requested.
type PresenceTransition struct {
	PropertyID uuid.UUID   `json:"propertyId"`
	Event      string      `json:"event"`
	CommandIDs []uuid.UUID `json:"commandIds"`
}

// PresenceResult is PresenceResult of service-contracts.ts.
type PresenceResult struct {
	Transitions []PresenceTransition `json:"transitions"`
}

// reportPresence serves presence.report.
//
//	@Summary		presence.report (write)
//	@ID				presence.report
//	@Description	Authorization: client:own-membership
//	@Description	Validation: D01; input constraints in the corresponding DD; DEC-109 the member's location_automation \
//	@Description	consent (else CONFLICT); accuracy at most 100 m; only inside/outside per pinned property is kept
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-C05
//	@Tags			presence
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key	header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			request			body		PresenceInput	true	"input"
//	@Success		200				{object}	ops.Envelope{data=PresenceResult}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/presence [post]
func (m Automations) reportPresence(
	ctx context.Context,
	call *ops.Call,
	input *PresenceInput,
) (PresenceResult, error) {
	out := PresenceResult{Transitions: []PresenceTransition{}}

	granted, err := identity.ConsentGranted(ctx, call, call.Principal.MembershipID, locationConsent)
	if err != nil {
		return out, apperr.Wrap(err, "identity.ConsentGranted")
	}

	if !granted { // SR02: the device's position is used only with the member's location consent
		return out, apperr.E(apperr.Conflict, "errors.consent_required")
	}

	sites, err := m.Sites.PinnedSites(ctx, call, call.Principal.OrgID)
	if err != nil {
		return out, apperr.Wrap(err, "control.Sites.PinnedSites")
	}

	fix := geo.Point{Latitude: input.Latitude, Longitude: input.Longitude}

	for _, site := range sites {
		event, err := m.settle(ctx, call, site, geo.Distance(fix, site.Pin))
		if err != nil || event == "" {
			if err != nil {
				return out, err
			}

			continue
		}

		commands, err := m.fireLocation(ctx, call, site, event)
		if err != nil {
			return out, err
		}

		out.Transitions = append(out.Transitions, PresenceTransition{
			PropertyID: site.PropertyID,
			Event:      event,
			CommandIDs: commands,
		})
	}

	return out, nil
}

// settle records whether the member is inside the site and answers the household's change: arrival when the first
// member came inside, departure when the last one left, "" otherwise. Between the radius and the hysteresis band the
// member keeps the state they had. The property's rows are taken under a lock, so two members' reports at once see
// each other.
func (Automations) settle(
	ctx context.Context,
	call *ops.Call,
	site Site,
	distance float64,
) (string, error) {
	_, err := call.Tx.Exec(
		ctx,
		`SELECT pg_advisory_xact_lock(hashtext('control.presence_states:' || $1::text))`,
		site.PropertyID,
	)
	if err != nil {
		return "", apperr.Wrap(err, "select pg_advisory_xact_lock")
	}

	err = forgetEndedHousemates(ctx, call, site.PropertyID)
	if err != nil {
		return "", err
	}

	var before, own, known bool

	err = call.Tx.QueryRow(ctx, `SELECT COALESCE(bool_or(inside), false), COALESCE(bool_or(inside) FILTER (WHERE
		membership_id = $2), false), count(*) FILTER (WHERE membership_id = $2) > 0 FROM control.presence_states WHERE
		property_id = $1`, site.PropertyID, call.Principal.MembershipID).
		Scan(&before, &own, &known)
	if err != nil {
		return "", apperr.Wrap(err, "select control.presence_states")
	}

	inside := own

	switch radius := float64(site.RadiusMeters); {
	case distance <= radius:
		inside = true
	case distance > radius+presenceHysteresis:
		inside = false
	case !known:
		return "", nil // in the band with no state yet: nothing to say
	}

	_, err = call.Tx.Exec(
		ctx,
		`INSERT INTO control.presence_states (tenant_id, membership_id, property_id, inside,
		changed_at, reported_at) VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $4, $4)
		ON CONFLICT (tenant_id, membership_id, property_id) DO UPDATE SET inside = EXCLUDED.inside,
		changed_at = CASE WHEN control.presence_states.inside <> EXCLUDED.inside THEN EXCLUDED.changed_at
			ELSE control.presence_states.changed_at END,
		reported_at = EXCLUDED.reported_at`,
		call.Principal.MembershipID,
		site.PropertyID,
		inside,
		call.Now,
	)
	if err != nil {
		return "", apperr.Wrap(err, "insert control.presence_states")
	}

	var after bool

	err = call.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM control.presence_states WHERE property_id = $1 AND
		inside)`, site.PropertyID).
		Scan(&after)
	if err != nil {
		return "", apperr.Wrap(err, "select control.presence_states")
	}

	switch {
	case !before && after:
		return eventArrival, nil
	case before && !after:
		return eventDeparture, nil
	}

	return "", nil
}

// fireLocation evaluates the site's units with the household's location fact and answers the Commands the location
// automations requested. Each rule's owner must still hold the location consent (evaluate, SR02).
func (m Automations) fireLocation(
	ctx context.Context,
	call *ops.Call,
	site Site,
	event string,
) ([]uuid.UUID, error) {
	commands := []uuid.UUID{}
	if len(site.UnitIDs) == 0 {
		return commands, nil
	}

	value, err := json.Marshal(event)
	if err != nil {
		return commands, apperr.Wrap(err, "json.Marshal")
	}

	input := &EvaluationInput{
		EventID:    uuid.Must(uuid.NewV7()),
		OccurredAt: call.Now,
		Phase:      keyCondition,
		UnitIDs:    site.UnitIDs,
		Facts:      make([]Fact, 0, len(site.UnitIDs)),
	}

	for _, unit := range site.UnitIDs {
		input.Facts = append(input.Facts, Fact{
			UnitID:     unit,
			Metric:     condLocation,
			Value:      value,
			Unit:       unitEvent,
			ObservedAt: call.Now,
			Quality:    qualityValid,
		})
	}

	result, err := m.evaluate(ctx, call, input, true, false)
	if err != nil {
		return commands, err
	}

	for _, decision := range result.Results {
		if decision.Decision == decisionRequested && decision.CommandID != nil && decision.fresh {
			commands = append(commands, *decision.CommandID)
		}
	}

	call.Audit(ops.AuditEntry{
		Action:          "presence.report",
		TargetKind:      "property",
		TargetID:        site.PropertyID.String(),
		Reason:          event,
		PreviousVersion: nil,
		NextVersion:     nil,
		Result:          "",
		Before:          nil,
		After:           nil,
	})

	return commands, nil
}

// forgetEndedHousemates drops the inside states of the property's other members whose access ended (disabled, removed,
// ended by HQ or run out): no event says so, and their stale inside would hold the household home for good.
func forgetEndedHousemates(ctx context.Context, call *ops.Call, property uuid.UUID) error {
	rows, err := call.Tx.Query(
		ctx,
		`SELECT membership_id FROM control.presence_states WHERE property_id = $1 AND inside
		AND membership_id <> $2`,
		property,
		call.Principal.MembershipID,
	)
	if err != nil {
		return apperr.Wrap(err, "select control.presence_states")
	}

	housemates, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return apperr.Wrap(err, "select control.presence_states")
	}

	if len(housemates) == 0 { // an empty IDs filter would ask identity for every membership
		return nil
	}

	active, err := identity.Members(ctx, call, identity.MembersInput{
		IDs: housemates, Role: "", OrganizationID: nil, Permission: "", ClientRole: "",
	})
	if err != nil {
		return apperr.Wrap(err, "identity.Members")
	}

	_, err = call.Tx.Exec(
		ctx,
		`DELETE FROM control.presence_states WHERE property_id = $1 AND membership_id = ANY($2) AND NOT membership_id =
			ANY($3)`,
		property,
		housemates,
		active,
	)

	return apperr.Wrap(err, "delete control.presence_states")
}

// forgetPresence deletes a member's presence when their location consent is withdrawn (IR53); it fires nothing.
func forgetPresence(ctx context.Context, txn pgx.Tx, membership uuid.UUID) error {
	_, err := txn.Exec(
		ctx,
		`DELETE FROM control.presence_states WHERE membership_id = $1`,
		membership,
	)

	return apperr.Wrap(err, "delete control.presence_states")
}
