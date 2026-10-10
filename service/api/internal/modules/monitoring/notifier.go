package monitoring

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/modules/identity"
	"github.com/pradita/ac-project/service/api/internal/modules/notify"
	"github.com/pradita/ac-project/service/api/internal/ops"
)

// Notifier evaluates alert-policy notifications for automations.simulate / fire (SR21, SR25, SR28, SR12, IR155).
type Notifier struct {
	Notify notify.Store
}

type fact struct {
	UnitID     uuid.UUID       `json:"unitId"`
	Metric     string          `json:"metric"`
	Value      json.RawMessage `json:"value" swaggertype:"object"`
	ObservedAt time.Time       `json:"observedAt"`
	Quality    string          `json:"quality"`
}

// Outcome is NotificationDecision / NotificationOutcome.
type Outcome struct {
	UnitID          uuid.UUID   `json:"unitId"`
	PolicyID        uuid.UUID   `json:"policyId"`
	Decision        string      `json:"decision"`
	Reason          *string     `json:"reason"`
	NotificationIDs []uuid.UUID `json:"notificationIds,omitempty"`
	FailureIDs      []uuid.UUID `json:"failureIds,omitempty"`
}

type notifPolicy struct {
	id         uuid.UUID
	isDefault  bool
	enabled    bool
	owner      uuid.UUID
	conds      []AlertCondition
	ruleKeys   []string
	recipients []uuid.UUID
	channels   []string
	cooldown   int
	timezone   string
}

func cmp(op string, a, b float64) bool {
	switch op {
	case "gt":
		return a > b
	case "gte":
		return a >= b
	case "lt":
		return a < b
	}
	return a <= b
}

// inWindow reports whether at falls into the active window in tz (nil window: always). An end before the start runs
// past midnight and the weekdays are start days: 22:00–06:00 on Monday covers Monday 22:00 to Tuesday 06:00.
func inWindow(w *ActiveWindow, at time.Time, tz string) bool {
	if w == nil {
		return true
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return false
	}
	l := at.In(loc)
	wd := int(l.Weekday())
	if wd == 0 {
		wd = 7
	}
	hm := l.Format("15:04")
	if w.StartLocal < w.EndLocal {
		return slices.Contains(w.Weekdays, wd) && hm >= w.StartLocal && hm < w.EndLocal
	}
	if hm >= w.StartLocal {
		return slices.Contains(w.Weekdays, wd)
	}
	prev := wd - 1 // the morning part belongs to the window that started the day before
	if prev == 0 {
		prev = 7
	}
	return hm < w.EndLocal && slices.Contains(w.Weekdays, prev)
}

// policiesOf returns the notification candidates of a unit: attached alert policies plus the tenant default policy
// with the rules enabled for the unit's customer (SR25, IR108).
func policiesOf(ctx context.Context, c *ops.Call, unit, org uuid.UUID) ([]notifPolicy, error) {
	rows, err := c.Tx.Query(ctx, `SELECT p.id, p.kind = 'default_alert', p.enabled, p.owner_membership_id, p.condition, p.rules, p.recipient_membership_ids, p.channels,
		COALESCE(p.cooldown_minutes, 0), p.timezone FROM monitoring.alert_policies p
		WHERE p.id IN (SELECT policy_id FROM assets.unit_alert_policies WHERE unit_id = $1) OR p.kind = 'default_alert' ORDER BY p.id`, unit)
	if err != nil {
		return nil, err
	}
	var out []notifPolicy
	for rows.Next() {
		var p notifPolicy
		var cond, rules []byte
		if err := rows.Scan(&p.id, &p.isDefault, &p.enabled, &p.owner, &cond, &rules, &p.recipients, &p.channels, &p.cooldown, &p.timezone); err != nil {
			rows.Close()
			return nil, err
		}
		if p.isDefault {
			var rs []DefaultAlertRule
			_ = json.Unmarshal(rules, &rs)
			for _, r := range rs {
				p.conds, p.ruleKeys = append(p.conds, r.AlertCondition), append(p.ruleKeys, r.RuleKey)
			}
		} else {
			var a AlertCondition
			_ = json.Unmarshal(cond, &a)
			p.conds, p.ruleKeys = []AlertCondition{a}, []string{""}
		}
		out = append(out, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out { // default rules switched off for this customer (owner-only settings, IR115)
		if !out[i].isDefault {
			continue
		}
		off := map[string]bool{}
		rs, err := c.Tx.Query(ctx, `SELECT s.rule_key FROM monitoring.default_rule_settings s JOIN assets.customers cu ON cu.id = s.customer_id
			WHERE s.policy_id = $1 AND cu.organization_id = $2 AND NOT s.enabled`, out[i].id, org)
		if err != nil {
			return nil, err
		}
		for rs.Next() {
			var k string
			_ = rs.Scan(&k)
			off[k] = true
		}
		rs.Close()
		var conds []AlertCondition
		var keys []string
		for j, k := range out[i].ruleKeys {
			if !off[k] {
				conds, keys = append(conds, out[i].conds[j]), append(keys, k)
			}
		}
		out[i].conds, out[i].ruleKeys = conds, keys
	}
	return out, nil
}

// recipientsOf resolves the active recipients and channels: alert policies use their recipients (still active and
// able to read the unit's customer) and channels; the default policy notifies the customer's active clients in-app.
func recipientsOf(ctx context.Context, c *ops.Call, p notifPolicy, org uuid.UUID) ([]uuid.UUID, []string, error) {
	if p.isDefault {
		ids, err := ops.Query[[]uuid.UUID](ctx, c, identity.QueryMembers, identity.MembersInput{Role: "client", OrganizationID: &org})
		return ids, []string{"inApp"}, err
	}
	if len(p.recipients) == 0 {
		return []uuid.UUID{}, p.channels, nil
	}
	clients, err := ops.Query[[]uuid.UUID](ctx, c, identity.QueryMembers, identity.MembersInput{IDs: p.recipients, Role: "client", OrganizationID: &org})
	if err != nil {
		return nil, nil, err
	}
	admins, err := ops.Query[[]uuid.UUID](ctx, c, identity.QueryMembers, identity.MembersInput{IDs: p.recipients, Role: "admin", Permission: "alert.read"})
	if err != nil {
		return nil, nil, err
	}
	ids := append(clients, admins...)
	slices.SortFunc(ids, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
	return ids, p.channels, nil
}

// freshValue is a fact's value when it can decide at the tick: valid quality, a number, observed at most 120 s before
// the tick and not after it (late or future observations decide nothing, D08).
func freshValue(f fact, at time.Time) (float64, bool) {
	var v float64
	if f.Quality != "valid" || string(f.Value) == "null" || json.Unmarshal(f.Value, &v) != nil || f.ObservedAt.After(at) || at.Sub(f.ObservedAt) > 120*time.Second {
		return 0, false
	}
	return v, true
}

// recovered reports whether v is past the recovery threshold: below it for a high limit, above it for a low one (D08).
func recovered(c AlertCondition, v float64) bool {
	if c.Operator == "gt" || c.Operator == "gte" {
		return v < c.RecoveryThreshold
	}
	return v > c.RecoveryThreshold
}

// recoverAlert resolves the open source Alert of the policy / rule on the unit after a sustained recovery — in the demo
// evaluation a fact is taken as sustained for durationSeconds — and keeps the recovering reading as its evidence: the
// reason names it and resolutionEvidenceIds holds the evaluation event (D08; only Alerts with a policy, IR66). An Alert
// detected after the tick is left alone.
func recoverAlert(ctx context.Context, c *ops.Call, unit, policy uuid.UUID, ruleKey string, cnd AlertCondition, v float64, at time.Time, eventID uuid.UUID) error {
	var key *string
	if ruleKey != "" {
		key = &ruleKey
	}
	dir := ">"
	if cnd.Operator == "gt" || cnd.Operator == "gte" {
		dir = "<"
	}
	reason := fmt.Sprintf("Recovered: %s %s %s %s for %d s (D08)", cnd.Metric, strconv.FormatFloat(v, 'f', -1, 64), dir, strconv.FormatFloat(cnd.RecoveryThreshold, 'f', -1, 64), cnd.DurationSeconds)
	rows, err := c.Tx.Query(ctx, `UPDATE monitoring.alerts a SET status = 'resolved', resolved_at = $4, resolution_reason = $5, resolution_evidence_ids = ARRAY[$6::uuid],
		version = version + 1, updated_at = $4
		WHERE a.unit_id = $1 AND a.policy_id = $2 AND a.rule_key IS NOT DISTINCT FROM $3 AND a.type = 'sensor' AND a.status <> 'resolved' AND a.detected_at <= $4
		RETURNING a.id`, unit, policy, key, at, reason, eventID)
	if err != nil {
		return err
	}
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range ids {
		c.Emit(ops.Event{AggregateType: "alert", AggregateID: id, Type: "AlertResolved", Payload: map[string]any{"unitId": unit, "recovered": true}})
	}
	return nil
}

// Evaluate returns one outcome per notification policy of the unit, sorted by policyId. A policy matches when one of
// its conditions holds for the facts at the tick (facts are taken as sustained for durationSeconds in the demo
// evaluation); absent facts give not_due, invalid quality gives quality. With fire, a matching policy reuses or opens
// its source Alert and creates one Notification per recipient and channel; zero recipients add one DeliveryFailure
// (no_recipient, SR12) and do not advance the cooldown. With fire, a condition whose fact is past its recovery
// threshold resolves the policy's open Alert first, whether the policy is enabled or not (D08).
func (n Notifier) Evaluate(ctx context.Context, c *ops.Call, unit uuid.UUID, factsJSON []byte, at time.Time, eventID uuid.UUID, fire bool) ([]json.RawMessage, error) {
	var facts []fact
	_ = json.Unmarshal(factsJSON, &facts)
	byMetric := map[string]fact{}
	for _, f := range facts {
		if f.UnitID == unit {
			byMetric[f.Metric] = f
		}
	}
	var org uuid.UUID
	if err := c.Tx.QueryRow(ctx, `SELECT customer_org_id FROM assets.units WHERE id = $1`, unit).Scan(&org); err != nil {
		return nil, err
	}
	pols, err := policiesOf(ctx, c, unit, org)
	if err != nil {
		return nil, err
	}
	out := []json.RawMessage{}
	for _, p := range pols {
		if fire { // sustained recovery resolves the policy's open Alert (D08)
			for i, cnd := range p.conds {
				if f, ok := byMetric[cnd.Metric]; ok {
					if v, ok := freshValue(f, at); ok && recovered(cnd, v) {
						if err := recoverAlert(ctx, c, unit, p.id, p.ruleKeys[i], cnd, v, at, eventID); err != nil {
							return nil, err
						}
					}
				}
			}
		}
		o := Outcome{UnitID: unit, PolicyID: p.id}
		reason := ""
		var hit *AlertCondition
		hitKey := ""
		if !p.enabled {
			reason = "disabled"
		} else {
			reason = "not_due"
			for i, cnd := range p.conds {
				f, ok := byMetric[cnd.Metric]
				if !ok {
					continue
				}
				v, ok := freshValue(f, at)
				if !ok {
					if reason == "not_due" {
						reason = "quality"
					}
					continue
				}
				if cmp(cnd.Operator, v, cnd.Threshold) && inWindow(cnd.ActiveWindow, at, p.timezone) {
					cc := p.conds[i]
					hit, hitKey, reason = &cc, p.ruleKeys[i], ""
					break
				}
			}
		}
		if reason == "" { // owner still active (D02)
			owner, err := ops.Query[[]uuid.UUID](ctx, c, identity.QueryMembers, identity.MembersInput{IDs: []uuid.UUID{p.owner}})
			if err != nil {
				return nil, err
			}
			if len(owner) == 0 {
				reason = "owner_forbidden"
			}
		}
		if reason == "" && p.cooldown > 0 { // cooldown since the last created notification of this policy and unit
			var last *time.Time
			if err := c.Tx.QueryRow(ctx, `SELECT max(occurred_at) FROM monitoring.alert_notifications WHERE policy_id = $1 AND unit_id = $2`, p.id, unit).Scan(&last); err != nil {
				return nil, err
			}
			if last != nil && at.Sub(*last) < time.Duration(p.cooldown)*time.Minute {
				reason = "cooldown"
			}
		}
		var recips []uuid.UUID
		var channels []string
		if reason == "" {
			if recips, channels, err = recipientsOf(ctx, c, p, org); err != nil {
				return nil, err
			}
		}
		switch {
		case reason != "":
			o.Decision, o.Reason = "suppressed", &reason
		case len(recips) == 0 || len(channels) == 0:
			r := "no_recipient"
			o.Decision, o.Reason = "failed", &r
		default:
			o.Decision = "selected"
		}
		if fire && reason == "" {
			alert, err := sourceAlert(ctx, c, unit, org, p.id, hitKey, hit, at)
			if err != nil {
				return nil, err
			}
			if o.Decision == "failed" {
				fid, err := deliveryFailure(ctx, c, alert, p.id, eventID, at)
				if err != nil {
					return nil, err
				}
				o.NotificationIDs, o.FailureIDs = []uuid.UUID{}, []uuid.UUID{fid}
			} else {
				o.Decision, o.NotificationIDs, o.FailureIDs = "created", []uuid.UUID{}, []uuid.UUID{}
				typ := "fault" // IR104: the alert template takes its type from the source Alert (sensor → fault)
				for _, r := range recips {
					for _, ch := range channels {
						id, err := n.Notify.Create(ctx, c, notify.New{RecipientMembershipID: r, Channel: ch, Type: typ, TemplateKey: "alert", TargetKind: "unit", TargetID: unit,
							Severity: hit.Severity, SourceAlertID: &alert, Params: map[string]any{"status": "open"}})
						if err != nil {
							return nil, err
						}
						if _, err := c.Tx.Exec(ctx, `INSERT INTO monitoring.alert_notifications (tenant_id, notification_id, alert_id, policy_id, unit_id, occurred_at)
							VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $4, $5)`, id, alert, p.id, unit, c.Now); err != nil {
							return nil, err
						}
						o.NotificationIDs = append(o.NotificationIDs, id)
					}
				}
			}
		}
		var raw []byte
		if fire { // NotificationOutcome always carries both arrays
			if o.NotificationIDs == nil {
				o.NotificationIDs, o.FailureIDs = []uuid.UUID{}, []uuid.UUID{}
			}
			raw, _ = json.Marshal(struct {
				Outcome
				NotificationIDs []uuid.UUID `json:"notificationIds"`
				FailureIDs      []uuid.UUID `json:"failureIds"`
			}{o, o.NotificationIDs, o.FailureIDs})
		} else {
			o.NotificationIDs, o.FailureIDs = nil, nil
			raw, _ = json.Marshal(o)
		}
		out = append(out, raw)
	}
	return out, nil
}

// sourceAlert reuses the open Alert of the policy/rule on the unit or opens one (alerts_one_open_per_rule) linked to the
// latest earlier Alert of the same policy / rule on the unit (a recurrence after it resolved, IR66).
func sourceAlert(ctx context.Context, c *ops.Call, unit, org, policy uuid.UUID, ruleKey string, cnd *AlertCondition, at time.Time) (uuid.UUID, error) {
	var key *string
	if ruleKey != "" {
		key = &ruleKey
	}
	var id uuid.UUID
	err := c.Tx.QueryRow(ctx, `SELECT id FROM monitoring.alerts WHERE unit_id = $1 AND policy_id = $2 AND rule_key IS NOT DISTINCT FROM $3 AND type = 'sensor' AND status <> 'resolved'`,
		unit, policy, key).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return id, err
	}
	err = c.Tx.QueryRow(ctx, `INSERT INTO monitoring.alerts (tenant_id, unit_id, customer_org_id, policy_id, rule_key, type, severity, status, cause_code, evidence_kind, evidence_text,
		observed_at, detected_at, created_at, updated_at, previous_alert_id) VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $4, 'sensor', $5, 'open', 'unknown', 'demo_observation', $6, $7, $7, $7, $7,
		(SELECT p.id FROM monitoring.alerts p WHERE p.unit_id = $1 AND p.policy_id = $3 AND p.rule_key IS NOT DISTINCT FROM $4 AND p.type = 'sensor' ORDER BY p.detected_at DESC, p.id DESC LIMIT 1))
		RETURNING id`, unit, org, policy, key, cnd.Severity, "Demo event: "+cnd.Metric+" "+cnd.Operator+" threshold", at).Scan(&id)
	return id, err
}

// deliveryFailure appends one no_recipient DeliveryFailure per alert/event/policy (SR12; repeats add none).
func deliveryFailure(ctx context.Context, c *ops.Call, alert, policy, eventID uuid.UUID, at time.Time) (uuid.UUID, error) {
	var raw []byte
	if err := c.Tx.QueryRow(ctx, `SELECT delivery_failures FROM monitoring.alerts WHERE id = $1 FOR UPDATE`, alert).Scan(&raw); err != nil {
		return uuid.Nil, err
	}
	var fs []map[string]any
	_ = json.Unmarshal(raw, &fs)
	for _, f := range fs {
		if f["eventId"] == eventID.String() && f["policyId"] == policy.String() {
			id, _ := uuid.Parse(f["id"].(string))
			return id, nil
		}
	}
	id := uuid.New()
	fs = append(fs, map[string]any{"id": id, "policyId": policy, "eventId": eventID, "occurredAt": at, "reason": "no_recipient", "deliveryState": "failed"})
	b, _ := json.Marshal(fs)
	_, err := c.Tx.Exec(ctx, `UPDATE monitoring.alerts SET delivery_failures = $2, updated_at = $3 WHERE id = $1`, alert, b, at)
	return id, err
}
