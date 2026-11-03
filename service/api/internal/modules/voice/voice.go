// Package voice implements voice.resolveIntent (D09, IR65): a fixed en/ms grammar over Space names.
package voice

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/modules/monitoring"
	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// Units filters the units the caller may read (Assets).
type Units interface {
	VisibleUnits(ctx context.Context, c *ops.Call, units []uuid.UUID) ([]uuid.UUID, error)
}

// Voice is the voice operation.
type Voice struct {
	Units Units
}

// Input is voice.resolveIntent input.
type Input struct {
	Text           string     `json:"text"`
	Locale         string     `json:"locale"`
	SelectedUnitID *uuid.UUID `json:"selectedUnitId,omitempty"`
}

// Validate implements ops.Validator: 1–200 code points after trim, locale en|ms.
func (in *Input) Validate() map[string]string {
	fieldErrs := map[string]string{}

	in.Text = strings.TrimSpace(in.Text)
	if n := utf8.RuneCountInString(in.Text); n < 1 || n > 200 {
		fieldErrs["text"] = apperr.KeyLength
	}

	if in.Locale != "en" && in.Locale != "ms" {
		fieldErrs["locale"] = apperr.KeyInvalid
	}

	return fieldErrs
}

// grammar is the fixed command grammar of one locale.
type grammar struct{ temperature, set, help *regexp.Regexp }

var (
	enTemperature = regexp.MustCompile(`(?i)^temperature (.+)$`)
	enSet         = regexp.MustCompile(`(?i)^set (.+) to (\d{1,3}) degrees$`)
	enHelp        = regexp.MustCompile(`(?i)^help$`)
	msTemperature = regexp.MustCompile(`(?i)^suhu (.+)$`)
	msSet         = regexp.MustCompile(`(?i)^tetapkan (.+) kepada (\d{1,3}) darjah$`)
	msHelp        = regexp.MustCompile(`(?i)^bantuan$`)
)

// grammarOf returns the grammar of the locale: en or ms (Validate allows no other).
func grammarOf(locale string) grammar {
	switch locale {
	case "en":
		return grammar{temperature: enTemperature, set: enSet, help: enHelp}
	case "ms":
		return grammar{temperature: msTemperature, set: msSet, help: msHelp}
	default:
		return grammar{temperature: nil, set: nil, help: nil}
	}
}

// Candidate is a ResolvedIntent candidate.
type Candidate struct {
	UnitID    uuid.UUID `json:"unitId"`
	PathLabel string    `json:"pathLabel"`
}

// Intent is ResolvedIntent (one of its variants, chosen by kind).
type Intent struct {
	Kind            string                  `json:"kind"`
	MessageKey      string                  `json:"messageKey,omitempty"`
	Candidates      []Candidate             `json:"candidates,omitempty"`
	UnitID          *uuid.UUID              `json:"unitId,omitempty"`
	Measurement     *monitoring.Measurement `json:"measurement,omitempty"`
	Celsius         *int                    `json:"celsius,omitempty"`
	Before          json.RawMessage         `json:"before,omitempty"          swaggertype:"object"`
	ExpectedVersion *int                    `json:"expectedVersion,omitempty"`
	measurementNull bool
}

// MarshalJSON keeps measurement:null for a temperature intent without readings.
func (x Intent) MarshalJSON() ([]byte, error) {
	type plain Intent

	b, err := json.Marshal(plain(x))
	if err != nil || !x.measurementNull {
		return b, apperr.Wrap(err, "json.Marshal")
	}

	return append(b[:len(b)-1], []byte(`,"measurement":null}`)...), nil
}

// candidates matches the room against not archived Space names (trimmed, ASCII case-insensitive, any kind) and returns
// the caller-readable not archived units placed directly in the matched spaces with their path labels.
func (v Voice) candidates(ctx context.Context, call *ops.Call, room string) ([]Candidate, error) {
	rows, err := call.Tx.Query(ctx, `WITH RECURSIVE path(id, label, parent) AS (
			SELECT s.id, s.name, s.parent_space_id FROM assets.spaces s WHERE NOT s.archived AND lower(btrim(s.name)) =
			lower(btrim($1))
			UNION ALL SELECT p.id, a.name || ' > ' || p.label, a.parent_space_id FROM path p JOIN assets.spaces a ON a.id =
			p.parent)
		SELECT u.id, pr.name || ' > ' || p.label || ' > ' || u.display_name FROM path p JOIN assets.spaces s ON s.id = p.id
		JOIN assets.properties pr ON pr.id = s.property_id
		JOIN assets.units u ON u.space_id = s.id AND NOT u.archived WHERE p.parent IS NULL ORDER BY 2, 1`, room)
	if err != nil {
		return nil, apperr.Wrap(err, "select path")
	}

	var all []Candidate

	for rows.Next() {
		var candidate Candidate

		err := rows.Scan(&candidate.UnitID, &candidate.PathLabel)
		if err != nil {
			rows.Close()

			return nil, apperr.Wrap(err, "select path")
		}

		all = append(all, candidate)
	}

	rows.Close()

	rowsErr := rows.Err()
	if rowsErr != nil || len(all) == 0 {
		return []Candidate{}, apperr.Wrap(rowsErr, "select path")
	}

	return v.readable(ctx, call, all)
}

// readable keeps the candidates whose unit the caller may read, ordered by path label.
func (v Voice) readable(ctx context.Context, call *ops.Call, all []Candidate) ([]Candidate, error) {
	ids := make([]uuid.UUID, len(all))
	for i, x := range all {
		ids[i] = x.UnitID
	}

	visible, err := v.Units.VisibleUnits(ctx, call, ids)
	if err != nil {
		return nil, apperr.Wrap(err, "voice.Units.VisibleUnits")
	}

	keep := map[uuid.UUID]bool{}
	for _, id := range visible {
		keep[id] = true
	}

	out := []Candidate{}

	for _, x := range all {
		if keep[x.UnitID] {
			out = append(out, x)
		}
	}

	sort.SliceStable(out, func(i, j int) bool { return out[i].PathLabel < out[j].PathLabel })

	return out, nil
}

// resolve serves voice.resolveIntent.
//
//	@Summary		voice.resolveIntent (read)
//	@ID				voice.resolveIntent
//	@Description	Authorization: client:control.execute | technician:control.diagnose | admin:control.execute
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot; IR65 regex grammar \
//	@Description	and Space.name matching
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DDC-07
//	@Tags			voice
//	@Accept			json
//	@Produce		json
//	@Param			request	body		Input	true	"input"
//	@Success		200		{object}	ops.Envelope{data=Intent}
//	@Failure		401		{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403		{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404		{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409		{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422		{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429		{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503		{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504		{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/voice/resolve-intent [post]
func (v Voice) resolve(ctx context.Context, call *ops.Call, input *Input) (Intent, error) {
	patterns := grammarOf(input.Locale)
	if patterns.help.MatchString(input.Text) {
		return Intent{
			Kind:            "help",
			MessageKey:      "voice.help",
			Candidates:      nil,
			UnitID:          nil,
			Measurement:     nil,
			Celsius:         nil,
			Before:          nil,
			ExpectedVersion: nil,
			measurementNull: false,
		}, nil
	}

	room, celsius, understood := parseCommand(patterns, input.Text)
	if !understood {
		return unsupportedIntent(), nil
	}

	cands, err := v.candidates(ctx, call, room)
	if err != nil {
		return Intent{}, err
	}

	var unit uuid.UUID

	switch {
	case input.SelectedUnitID != nil: // must be one of the candidates of this request (IR65)
		if !slices.ContainsFunc(
			cands,
			func(x Candidate) bool { return x.UnitID == *input.SelectedUnitID },
		) {
			return Intent{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
		}

		unit = *input.SelectedUnitID
	case len(cands) == 0:
		return unsupportedIntent(), nil
	case len(cands) > 1:
		return Intent{
			Kind:            "candidates",
			Candidates:      cands,
			MessageKey:      "",
			UnitID:          nil,
			Measurement:     nil,
			Celsius:         nil,
			Before:          nil,
			ExpectedVersion: nil,
			measurementNull: false,
		}, nil
	default:
		unit = cands[0].UnitID
	}

	if celsius == nil {
		return temperatureIntent(ctx, call, unit)
	}

	return changeIntent(ctx, call, unit, celsius)
}

// parseCommand matches a set or a temperature command: it returns the room, the requested °C of a set command and
// whether the text is either command.
func parseCommand(patterns grammar, text string) (string, *int, bool) {
	if match := patterns.set.FindStringSubmatch(text); match != nil {
		celsius, _ := strconv.Atoi(match[2])

		return match[1], &celsius, true
	}

	if match := patterns.temperature.FindStringSubmatch(text); match != nil {
		return match[1], nil, true
	}

	return "", nil, false
}

func unsupportedIntent() Intent {
	return Intent{
		Kind:            "unsupported",
		MessageKey:      "",
		Candidates:      nil,
		UnitID:          nil,
		Measurement:     nil,
		Celsius:         nil,
		Before:          nil,
		ExpectedVersion: nil,
		measurementNull: false,
	}
}

// temperatureIntent reports the unit's latest temperature reading (measurement null without readings).
func temperatureIntent(ctx context.Context, call *ops.Call, unit uuid.UUID) (Intent, error) {
	meas, err := monitoring.LatestMeasurement(ctx, call, unit, "temperature")
	if err != nil {
		return Intent{}, apperr.Wrap(err, "monitoring.LatestMeasurement")
	}

	return Intent{
		Kind:            "temperature",
		UnitID:          &unit,
		Measurement:     meas,
		measurementNull: meas == nil,
		MessageKey:      "",
		Candidates:      nil,
		Celsius:         nil,
		Before:          nil,
		ExpectedVersion: nil,
	}, nil
}

// changeIntent proposes the set point change with the unit's observed state and version to confirm against.
func changeIntent(
	ctx context.Context,
	call *ops.Call,
	unit uuid.UUID,
	celsius *int,
) (Intent, error) {
	var (
		before  []byte
		version int
	)

	err := call.Tx.QueryRow(ctx, `SELECT observed_state, version FROM assets.units WHERE id = $1`, unit).
		Scan(&before, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return Intent{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return Intent{}, apperr.Wrap(err, "select assets.units")
	}

	return Intent{
		Kind:            "change",
		UnitID:          &unit,
		Celsius:         celsius,
		Before:          before,
		ExpectedVersion: &version,
		MessageKey:      "",
		Candidates:      nil,
		Measurement:     nil,
		measurementNull: false,
	}, nil
}

// Register binds voice.resolveIntent.
func Register(r *ops.Registry, v Voice) { ops.Register(r, "voice.resolveIntent", v.resolve) }
