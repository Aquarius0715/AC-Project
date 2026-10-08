// Package voice implements voice.resolveIntent (D09, IR65): a fixed en/ms grammar over Space names.
package voice

import (
	"context"
	"encoding/json"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/core/modules/monitoring"
	"github.com/pradita/ac-project/service/core/ops"
	"github.com/pradita/ac-project/service/core/platform/apperr"
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
	fe := map[string]string{}
	in.Text = strings.TrimSpace(in.Text)
	if n := utf8.RuneCountInString(in.Text); n < 1 || n > 200 {
		fe["text"] = "error.length"
	}
	if in.Locale != "en" && in.Locale != "ms" {
		fe["locale"] = "error.invalid"
	}
	return fe
}

var grammar = map[string]struct{ temperature, set, help *regexp.Regexp }{
	"en": {regexp.MustCompile(`(?i)^temperature (.+)$`), regexp.MustCompile(`(?i)^set (.+) to (\d{1,3}) degrees$`), regexp.MustCompile(`(?i)^help$`)},
	"ms": {regexp.MustCompile(`(?i)^suhu (.+)$`), regexp.MustCompile(`(?i)^tetapkan (.+) kepada (\d{1,3}) darjah$`), regexp.MustCompile(`(?i)^bantuan$`)},
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
	Before          json.RawMessage         `json:"before,omitempty"`
	ExpectedVersion *int                    `json:"expectedVersion,omitempty"`
	measurementNull bool
}

// MarshalJSON keeps measurement:null for a temperature intent without readings.
func (x Intent) MarshalJSON() ([]byte, error) {
	type plain Intent
	b, err := json.Marshal(plain(x))
	if err != nil || !x.measurementNull {
		return b, err
	}
	return append(b[:len(b)-1], []byte(`,"measurement":null}`)...), nil
}

// candidates matches the room against not archived Space names (trimmed, ASCII case-insensitive, any kind) and returns
// the caller-readable not archived units placed directly in the matched spaces with their path labels.
func (v Voice) candidates(ctx context.Context, c *ops.Call, room string) ([]Candidate, error) {
	rows, err := c.Tx.Query(ctx, `WITH RECURSIVE path(id, label, parent) AS (
			SELECT s.id, s.name, s.parent_space_id FROM assets.spaces s WHERE NOT s.archived AND lower(btrim(s.name)) = lower(btrim($1))
			UNION ALL SELECT p.id, a.name || ' > ' || p.label, a.parent_space_id FROM path p JOIN assets.spaces a ON a.id = p.parent)
		SELECT u.id, pr.name || ' > ' || p.label || ' > ' || u.display_name FROM path p JOIN assets.spaces s ON s.id = p.id JOIN assets.properties pr ON pr.id = s.property_id
		JOIN assets.units u ON u.space_id = s.id AND NOT u.archived WHERE p.parent IS NULL ORDER BY 2, 1`, room)
	if err != nil {
		return nil, err
	}
	var all []Candidate
	for rows.Next() {
		var x Candidate
		if err := rows.Scan(&x.UnitID, &x.PathLabel); err != nil {
			rows.Close()
			return nil, err
		}
		all = append(all, x)
	}
	rows.Close()
	if err := rows.Err(); err != nil || len(all) == 0 {
		return []Candidate{}, err
	}
	ids := make([]uuid.UUID, len(all))
	for i, x := range all {
		ids[i] = x.UnitID
	}
	visible, err := v.Units.VisibleUnits(ctx, c, ids)
	if err != nil {
		return nil, err
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

func (v Voice) resolve(ctx context.Context, c *ops.Call, in *Input) (Intent, error) {
	g := grammar[in.Locale]
	if g.help.MatchString(in.Text) {
		return Intent{Kind: "help", MessageKey: "voice.help"}, nil
	}
	var room string
	var celsius *int
	if m := g.set.FindStringSubmatch(in.Text); m != nil {
		n, _ := strconv.Atoi(m[2])
		room, celsius = m[1], &n
	} else if m := g.temperature.FindStringSubmatch(in.Text); m != nil {
		room = m[1]
	} else {
		return Intent{Kind: "unsupported"}, nil
	}
	cands, err := v.candidates(ctx, c, room)
	if err != nil {
		return Intent{}, err
	}
	var unit uuid.UUID
	switch {
	case in.SelectedUnitID != nil: // must be one of the candidates of this request (IR65)
		found := false
		for _, x := range cands {
			found = found || x.UnitID == *in.SelectedUnitID
		}
		if !found {
			return Intent{}, apperr.E(apperr.NotFound, "error.notFound")
		}
		unit = *in.SelectedUnitID
	case len(cands) == 0:
		return Intent{Kind: "unsupported"}, nil
	case len(cands) > 1:
		return Intent{Kind: "candidates", Candidates: cands}, nil
	default:
		unit = cands[0].UnitID
	}
	if celsius == nil {
		meas, err := monitoring.LatestMeasurement(ctx, c, unit, "temperature")
		if err != nil {
			return Intent{}, err
		}
		return Intent{Kind: "temperature", UnitID: &unit, Measurement: meas, measurementNull: meas == nil}, nil
	}
	var before []byte
	var version int
	err = c.Tx.QueryRow(ctx, `SELECT observed_state, version FROM assets.units WHERE id = $1`, unit).Scan(&before, &version)
	if err == pgx.ErrNoRows {
		return Intent{}, apperr.E(apperr.NotFound, "error.notFound")
	}
	if err != nil {
		return Intent{}, err
	}
	return Intent{Kind: "change", UnitID: &unit, Celsius: celsius, Before: before, ExpectedVersion: &version}, nil
}

// Register binds voice.resolveIntent.
func Register(r *ops.Registry, v Voice) { ops.Register(r, "voice.resolveIntent", v.resolve) }
