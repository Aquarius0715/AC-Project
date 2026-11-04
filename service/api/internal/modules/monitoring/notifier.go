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
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/authz"
)

// Notifier evaluates alert-policy notifications for automations.simulate / fire (SR21, SR25, SR28, SR12, IR155).
type Notifier struct {
	Notify notify.Store
}

type fact struct {
	UnitID     uuid.UUID       `json:"unitId"`
	Metric     string          `json:"metric"`
	Value      json.RawMessage `json:"value"      swaggertype:"object"`
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

func cmp(op string, value, threshold float64) bool {
	switch op {
	case "gt":
		return value > threshold
	case opGTE:
		return value >= threshold
	case "lt":
		return value < threshold
	}

	return value <= threshold
}

// inWindow reports whether when falls into the active window in tz (nil window: always). An end before the start runs
// past midnight and the weekdays are start days: 22:00–06:00 on Monday covers Monday 22:00 to Tuesday 06:00.
func inWindow(activeWindow *ActiveWindow, when time.Time, tz string) bool {
	if activeWindow == nil {
		return true
	}

	loc, err := time.LoadLocation(tz)
	if err != nil {
		return false
	}

	local := when.In(loc)

	weekday := int(local.Weekday())
	if weekday == 0 {
		weekday = 7
	}

	timeOfDay := local.Format("15:04")
	if activeWindow.StartLocal < activeWindow.EndLocal {
		return slices.Contains(activeWindow.Weekdays, weekday) &&
			timeOfDay >= activeWindow.StartLocal &&
			timeOfDay < activeWindow.EndLocal
	}

	if timeOfDay >= activeWindow.StartLocal {
		return slices.Contains(activeWindow.Weekdays, weekday)
	}

	prev := weekday - 1 // the morning part belongs to the window that started the day before
	if prev == 0 {
		prev = 7
	}

	return timeOfDay < activeWindow.EndLocal && slices.Contains(activeWindow.Weekdays, prev)
}

// policiesOf returns the notification candidates of a unit: attached alert policies plus the tenant default policy
// with the rules enabled for the unit's customer (SR25, IR108).
func policiesOf(ctx context.Context, call *ops.Call, unit, org uuid.UUID) ([]notifPolicy, error) {
	rows, err := call.Tx.Query(
		ctx,
		`SELECT p.id, p.kind = 'default_alert', p.enabled, p.owner_membership_id, p.condition, p.rules,
			p.recipient_membership_ids, p.channels,
		COALESCE(p.cooldown_minutes, 0), p.timezone FROM monitoring.alert_policies p
		WHERE p.id IN (SELECT policy_id FROM assets.unit_alert_policies WHERE unit_id = $1) OR p.kind = 'default_alert' ORDER
		BY p.id`,
		unit,
	)
	if err != nil {
		return nil, apperr.Wrap(err, "select monitoring.alert_policies")
	}

	var out []notifPolicy

	for rows.Next() {
		policy, err := scanPolicy(rows)
		if err != nil {
			rows.Close()

			return nil, err
		}

		out = append(out, policy)
	}

	rows.Close()

	rowsErr := rows.Err()
	if rowsErr != nil {
		return nil, apperr.Wrap(rowsErr, "select monitoring.alert_policies")
	}

	for idx := range out { // default rules switched off for this customer (owner-only settings, IR115)
		if !out[idx].isDefault {
			continue
		}

		err := withoutSwitchedOff(ctx, call, &out[idx], org)
		if err != nil {
			return nil, err
		}
	}

	return out, nil
}

// scanPolicy reads one policy row: an alert policy has its one condition, the default policy its rules, each with
// its rule key.
func scanPolicy(row pgx.Row) (notifPolicy, error) {
	var (
		policy      notifPolicy
		cond, rules []byte
	)

	err := row.Scan(
		&policy.id,
		&policy.isDefault,
		&policy.enabled,
		&policy.owner,
		&cond,
		&rules,
		&policy.recipients,
		&policy.channels,
		&policy.cooldown,
		&policy.timezone,
	)
	if err != nil {
		return policy, apperr.Wrap(err, "select monitoring.alert_policies")
	}

	if policy.isDefault {
		var defaultRules []DefaultAlertRule

		err := json.Unmarshal(rules, &defaultRules)
		if err != nil {
			return policy, apperr.Wrap(err, "decode monitoring.alert_policies rules")
		}

		for _, defaultAlertRule := range defaultRules {
			policy.conds = append(policy.conds, defaultAlertRule.AlertCondition)
			policy.ruleKeys = append(policy.ruleKeys, defaultAlertRule.RuleKey)
		}

		return policy, nil
	}

	var condition AlertCondition

	err = json.Unmarshal(cond, &condition)
	if err != nil {
		return policy, apperr.Wrap(err, "decode monitoring.alert_policies condition")
	}

	policy.conds, policy.ruleKeys = []AlertCondition{condition}, []string{""}

	return policy, nil
}

// withoutSwitchedOff drops the default policy's rules the unit's customer switched off (IR115). A failed read must
// not turn a switched-off rule back on, so it is an error.
func withoutSwitchedOff(
	ctx context.Context,
	call *ops.Call,
	policy *notifPolicy,
	org uuid.UUID,
) error {
	settingRows, err := call.Tx.Query(
		ctx,
		`SELECT s.rule_key FROM monitoring.default_rule_settings s JOIN assets.customers cu ON cu.id = s.customer_id
		WHERE s.policy_id = $1 AND cu.organization_id = $2 AND NOT s.enabled`,
		policy.id,
		org,
	)
	if err != nil {
		return apperr.Wrap(err, "select monitoring.default_rule_settings")
	}

	switchedOff, err := pgx.CollectRows(settingRows, pgx.RowTo[string])
	if err != nil {
		return apperr.Wrap(err, "select monitoring.default_rule_settings")
	}

	off := map[string]bool{}
	for _, k := range switchedOff {
		off[k] = true
	}

	var (
		conds []AlertCondition
		keys  []string
	)

	for j, k := range policy.ruleKeys {
		if !off[k] {
			conds, keys = append(conds, policy.conds[j]), append(keys, k)
		}
	}

	policy.conds, policy.ruleKeys = conds, keys

	return nil
}

// recipientsOf resolves the active recipients and channels: alert policies use their recipients (still active and
// able to read the unit's customer) and channels; the default policy notifies the customer's active clients in-app.
func recipientsOf(
	ctx context.Context,
	call *ops.Call,
	policy notifPolicy,
	org uuid.UUID,
) ([]uuid.UUID, []string, error) {
	if policy.isDefault {
		ids, err := ops.Query[[]uuid.UUID](
			ctx,
			call,
			identity.QueryMembers,
			identity.MembersInput{
				Role:           authz.RoleClient,
				OrganizationID: &org,
				IDs:            nil,
				Permission:     "",
				ClientRole:     "",
			},
		)

		return ids, []string{"inApp"}, err
	}

	if len(policy.recipients) == 0 {
		return []uuid.UUID{}, policy.channels, nil
	}

	clients, err := ops.Query[[]uuid.UUID](
		ctx,
		call,
		identity.QueryMembers,
		identity.MembersInput{
			IDs:            policy.recipients,
			Role:           authz.RoleClient,
			OrganizationID: &org,
			Permission:     "",
			ClientRole:     "",
		},
	)
	if err != nil {
		return nil, nil, err
	}

	admins, err := ops.Query[[]uuid.UUID](
		ctx,
		call,
		identity.QueryMembers,
		identity.MembersInput{
			IDs:            policy.recipients,
			Role:           "admin",
			Permission:     "alert.read",
			OrganizationID: nil,
			ClientRole:     "",
		},
	)
	if err != nil {
		return nil, nil, err
	}

	ids := slices.Concat(clients, admins)
	slices.SortFunc(
		ids,
		func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) },
	)

	return ids, policy.channels, nil
}

// freshValue is a fact's value when it can decide at the tick: valid quality, a number, observed at most 120 s before
// the tick and not after it (late or future observations decide nothing, D08).
func freshValue(f fact, at time.Time) (float64, bool) {
	var value float64
	if f.Quality != qualityValid || string(f.Value) == "null" ||
		json.Unmarshal(f.Value, &value) != nil ||
		f.ObservedAt.After(at) ||
		at.Sub(f.ObservedAt) > 120*time.Second {
		return 0, false
	}

	return value, true
}

// recovered reports whether v is past the recovery threshold: below it for a high limit, above it for a low one (D08).
func recovered(c AlertCondition, v float64) bool {
	if c.Operator == "gt" || c.Operator == opGTE {
		return v < c.RecoveryThreshold
	}

	return v > c.RecoveryThreshold
}

// recoverAlert resolves the open source Alert of the policy / rule on the unit after a sustained recovery — in the demo
// evaluation a fact is taken as sustained for durationSeconds — and keeps the recovering reading as its evidence: the
// reason names it and resolutionEvidenceIds holds the evaluation event (D08; only Alerts with a policy, IR66). An Alert
// detected after the tick is left alone.
func recoverAlert(
	ctx context.Context,
	call *ops.Call,
	unit, policy uuid.UUID,
	ruleKey string,
	cnd AlertCondition,
	value float64,
	when time.Time,
	eventID uuid.UUID,
) error {
	var key *string
	if ruleKey != "" {
		key = &ruleKey
	}

	dir := ">"
	if cnd.Operator == "gt" || cnd.Operator == opGTE {
		dir = "<"
	}

	reason := fmt.Sprintf(
		"Recovered: %s %s %s %s for %d s (D08)",
		cnd.Metric,
		strconv.FormatFloat(value, 'f', -1, 64),
		dir,
		strconv.FormatFloat(cnd.RecoveryThreshold, 'f', -1, 64),
		cnd.DurationSeconds,
	)

	rows, err := call.Tx.Query(
		ctx,
		`UPDATE monitoring.alerts a SET status = 'resolved', resolved_at = $4, resolution_reason = $5, resolution_evidence_ids
			= ARRAY[$6::uuid],
		version = version + 1, updated_at = $4
		WHERE a.unit_id = $1 AND a.policy_id = $2 AND a.rule_key IS NOT DISTINCT FROM $3 AND a.type = 'sensor' AND a.status <>
		'resolved' AND a.detected_at <= $4
		RETURNING a.id`,
		unit,
		policy,
		key,
		when,
		reason,
		eventID,
	)
	if err != nil {
		return apperr.Wrap(err, "update monitoring.alerts")
	}

	var ids []uuid.UUID

	for rows.Next() {
		var alertID uuid.UUID

		err := rows.Scan(&alertID)
		if err != nil {
			rows.Close()

			return apperr.Wrap(err, "update monitoring.alerts")
		}

		ids = append(ids, alertID)
	}

	rows.Close()

	rowsErr := rows.Err()
	if rowsErr != nil {
		return apperr.Wrap(rowsErr, "update monitoring.alerts")
	}

	for _, id := range ids {
		call.Emit(
			ops.Event{
				AggregateType: kindAlert,
				AggregateID:   id,
				Type:          "AlertResolved",
				Payload:       map[string]any{keyUnitID: unit, "recovered": true},
			},
		)
	}

	return nil
}

// Evaluate returns one outcome per notification policy of the unit, sorted by policyId. A policy matches when one of
// its conditions holds for the facts at the tick (facts are taken as sustained for durationSeconds in the demo
// evaluation); absent facts give not_due, invalid quality gives quality. With fire, a matching policy reuses or opens
// its source Alert and creates one Notification per recipient and channel; zero recipients add one DeliveryFailure
// (no_recipient, SR12) and do not advance the cooldown. A policy without a cooldown notifies once per open Alert
// (IR600). With fire, a condition whose fact is past its recovery threshold resolves the policy's open Alert first,
// whether the policy is enabled or not (D08).
func (n Notifier) Evaluate(
	ctx context.Context,
	call *ops.Call,
	unit uuid.UUID,
	factsJSON []byte,
	when time.Time,
	eventID uuid.UUID,
	fire bool,
) ([]json.RawMessage, error) {
	var facts []fact

	err := json.Unmarshal(factsJSON, &facts)
	if err != nil {
		return nil, apperr.Wrap(err, "decode the evaluation facts")
	}

	byMetric := map[string]fact{}

	for _, f := range facts {
		if f.UnitID == unit {
			byMetric[f.Metric] = f
		}
	}

	var org uuid.UUID

	err = call.Tx.QueryRow(ctx, `SELECT customer_org_id FROM assets.units WHERE id = $1`, unit).
		Scan(&org)
	if err != nil {
		return nil, apperr.Wrap(err, "select assets.units")
	}

	pols, err := policiesOf(ctx, call, unit, org)
	if err != nil {
		return nil, err
	}

	run := evaluation{
		notify:  n.Notify,
		call:    call,
		unit:    unit,
		org:     org,
		facts:   byMetric,
		when:    when,
		eventID: eventID,
		fire:    fire,
	}
	out := []json.RawMessage{}

	for _, policy := range pols {
		raw, err := run.policy(ctx, policy)
		if err != nil {
			return nil, err
		}

		out = append(out, raw)
	}

	return out, nil
}

// evaluation is one Evaluate run on a unit: its customer organization, its facts by metric, the tick, the evaluation
// event, and whether it fires (opens Alerts and creates Notifications) or only simulates.
type evaluation struct {
	notify  notify.Store
	call    *ops.Call
	unit    uuid.UUID
	org     uuid.UUID
	facts   map[string]fact
	when    time.Time
	eventID uuid.UUID
	fire    bool
}

// policyMatch is the policy's condition that holds at the tick with its rule key, or the reason none does: disabled,
// not_due, or quality when a fact cannot decide.
type policyMatch struct {
	cond    *AlertCondition
	ruleKey string
	reason  string
}

// policy evaluates one policy into its outcome: with fire, a sustained recovery first resolves its open Alert (D08);
// a matching policy that nothing suppresses is selected, or failed with no one to notify, and with fire it notifies.
func (run evaluation) policy(ctx context.Context, policy notifPolicy) (json.RawMessage, error) {
	if run.fire {
		err := run.resolveRecovered(ctx, policy)
		if err != nil {
			return nil, err
		}
	}

	match := run.match(policy)
	reason := match.reason

	if reason == "" {
		suppressed, err := run.suppressed(ctx, policy, match.ruleKey)
		if err != nil {
			return nil, err
		}

		reason = suppressed
	}

	var (
		recips   []uuid.UUID
		channels []string
	)

	if reason == "" {
		var err error

		recips, channels, err = recipientsOf(ctx, run.call, policy, run.org)
		if err != nil {
			return nil, err
		}
	}

	outcome := Outcome{
		UnitID:          run.unit,
		PolicyID:        policy.id,
		Decision:        "",
		Reason:          nil,
		NotificationIDs: nil,
		FailureIDs:      nil,
	}

	switch {
	case reason != "":
		outcome.Decision, outcome.Reason = "suppressed", &reason
	case len(recips) == 0 || len(channels) == 0:
		r := reasonNoRecipient
		outcome.Decision, outcome.Reason = outcomeFailed, &r
	default:
		outcome.Decision = "selected"
	}

	if run.fire && reason == "" {
		err := run.deliver(ctx, policy, match, recips, channels, &outcome)
		if err != nil {
			return nil, err
		}
	}

	return outcomeJSON(outcome, run.fire)
}

// resolveRecovered resolves the policy's open Alert of each condition whose fresh fact is past its recovery threshold
// (D08).
func (run evaluation) resolveRecovered(ctx context.Context, policy notifPolicy) error {
	for idx, cnd := range policy.conds {
		f, ok := run.facts[cnd.Metric]
		if !ok {
			continue
		}

		value, fresh := freshValue(f, run.when)
		if !fresh || !recovered(cnd, value) {
			continue
		}

		err := recoverAlert(
			ctx,
			run.call,
			run.unit,
			policy.id,
			policy.ruleKeys[idx],
			cnd,
			value,
			run.when,
			run.eventID,
		)
		if err != nil {
			return err
		}
	}

	return nil
}

// match finds the first of the policy's conditions that holds at the tick: its fact is fresh, past the threshold and
// within the condition's active window.
func (run evaluation) match(policy notifPolicy) policyMatch {
	if !policy.enabled {
		return policyMatch{cond: nil, ruleKey: "", reason: "disabled"}
	}

	reason := "not_due"

	for idx, cnd := range policy.conds {
		f, present := run.facts[cnd.Metric]
		if !present {
			continue
		}

		value, fresh := freshValue(f, run.when)
		if !fresh {
			if reason == "not_due" {
				reason = "quality"
			}

			continue
		}

		if cmp(cnd.Operator, value, cnd.Threshold) &&
			inWindow(cnd.ActiveWindow, run.when, policy.timezone) {
			hit := policy.conds[idx]

			return policyMatch{cond: &hit, ruleKey: policy.ruleKeys[idx], reason: ""}
		}
	}

	return policyMatch{cond: nil, ruleKey: "", reason: reason}
}

// suppressed is why a matching policy notifies no one now, or "": its owner is no longer active (D02), its cooldown
// since its last notification of the unit has not passed, or, without a cooldown, the open Alert it notified about is
// still open.
func (run evaluation) suppressed(
	ctx context.Context,
	policy notifPolicy,
	ruleKey string,
) (string, error) {
	owner, err := ops.Query[[]uuid.UUID](
		ctx,
		run.call,
		identity.QueryMembers,
		identity.MembersInput{
			IDs:            []uuid.UUID{policy.owner},
			Role:           "",
			OrganizationID: nil,
			Permission:     "",
			ClientRole:     "",
		},
	)
	if err != nil {
		return "", err
	}

	if len(owner) == 0 {
		return "owner_forbidden", nil
	}

	switch {
	case policy.cooldown > 0:
		return run.coolingDown(ctx, policy)
	case policy.cooldown == 0:
		return run.notifiedOpen(ctx, policy, ruleKey)
	}

	return "", nil
}

// coolingDown is cooldown while the policy's cooldown since its last notification of the unit has not passed.
func (run evaluation) coolingDown(ctx context.Context, policy notifPolicy) (string, error) {
	var last *time.Time

	err := run.call.Tx.QueryRow(ctx, `SELECT max(occurred_at) FROM monitoring.alert_notifications WHERE policy_id = $1 AND
		unit_id = $2`, policy.id, run.unit).
		Scan(&last)
	if err != nil {
		return "", apperr.Wrap(err, "select monitoring.alert_notifications")
	}

	if last != nil && run.when.Sub(*last) < time.Duration(policy.cooldown)*time.Minute {
		return "cooldown", nil
	}

	return "", nil
}

// notifiedOpen is cooldown while the Alert a policy without a cooldown (the default policy) notified about stays open:
// it notifies once per open Alert, and a recurrence after it resolved opens a new Alert and notifies again (IR600,
// DEC-90).
func (run evaluation) notifiedOpen(
	ctx context.Context,
	policy notifPolicy,
	ruleKey string,
) (string, error) {
	var key *string
	if ruleKey != "" {
		key = &ruleKey
	}

	var told bool

	err := run.call.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM monitoring.alerts a JOIN monitoring.alert_notifications
		n ON
		n.alert_id = a.id AND n.policy_id = a.policy_id
		WHERE a.unit_id = $1 AND a.policy_id = $2 AND a.rule_key IS NOT DISTINCT FROM $3 AND a.type = 'sensor' AND a.status
		<> 'resolved')`, run.unit, policy.id, key).
		Scan(&told)
	if err != nil {
		return "", apperr.Wrap(err, "select monitoring.alerts")
	}

	if told {
		return "cooldown", nil
	}

	return "", nil
}

// deliver reuses or opens the policy's source Alert and notifies each recipient on each channel; with no one to
// notify it records one DeliveryFailure (no_recipient, SR12) instead, which does not advance the cooldown.
func (run evaluation) deliver(
	ctx context.Context,
	policy notifPolicy,
	match policyMatch,
	recips []uuid.UUID,
	channels []string,
	outcome *Outcome,
) error {
	alert, err := sourceAlert(
		ctx,
		run.call,
		run.unit,
		run.org,
		policy.id,
		match.ruleKey,
		match.cond,
		run.when,
	)
	if err != nil {
		return err
	}

	if outcome.Decision == outcomeFailed {
		failureID, err := deliveryFailure(ctx, run.call, alert, policy.id, run.eventID, run.when)
		if err != nil {
			return err
		}

		outcome.NotificationIDs, outcome.FailureIDs = []uuid.UUID{}, []uuid.UUID{failureID}

		return nil
	}

	outcome.Decision, outcome.NotificationIDs, outcome.FailureIDs = "created", []uuid.UUID{}, []uuid.UUID{}
	typ := "fault" // IR104: the alert template takes its type from the source Alert (sensor → fault)

	for _, recipient := range recips {
		for _, channel := range channels {
			notificationID, err := run.notify.Create(ctx, run.call, notify.New{
				RecipientMembershipID: recipient,
				Channel:               channel,
				Type:                  typ,
				TemplateKey:           kindAlert,
				TargetKind:            "unit",
				TargetID:              run.unit,
				Severity:              match.cond.Severity,
				SourceAlertID:         &alert,
				Params:                map[string]any{"status": statusOpen},
			})
			if err != nil {
				return apperr.Wrap(err, "notify.Store.Create")
			}

			_, execErr := run.call.Tx.Exec(
				ctx,
				`INSERT INTO monitoring.alert_notifications (tenant_id, notification_id, alert_id, policy_id, unit_id,
					occurred_at)
				VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $4, $5)`,
				notificationID,
				alert,
				policy.id,
				run.unit,
				run.call.Now,
			)
			if execErr != nil {
				return apperr.Wrap(execErr, "insert monitoring.alert_notifications")
			}

			outcome.NotificationIDs = append(outcome.NotificationIDs, notificationID)
		}
	}

	return nil
}

// outcomeJSON is the outcome as Evaluate answers it: a fired evaluation's NotificationOutcome always carries both
// arrays, a simulated NotificationDecision neither.
func outcomeJSON(outcome Outcome, fire bool) (json.RawMessage, error) {
	if !fire {
		outcome.NotificationIDs, outcome.FailureIDs = nil, nil

		raw, err := json.Marshal(outcome)

		return raw, apperr.Wrap(err, "json.Marshal")
	}

	if outcome.NotificationIDs == nil {
		outcome.NotificationIDs, outcome.FailureIDs = []uuid.UUID{}, []uuid.UUID{}
	}

	raw, err := json.Marshal(struct {
		Outcome

		NotificationIDs []uuid.UUID `json:"notificationIds"`
		FailureIDs      []uuid.UUID `json:"failureIds"`
	}{outcome, outcome.NotificationIDs, outcome.FailureIDs})

	return raw, apperr.Wrap(err, "json.Marshal")
}

// sourceAlert reuses the open Alert of the policy/rule on the unit or opens one (alerts_one_open_per_rule) linked to
// the latest earlier Alert of the same policy / rule on the unit (a recurrence after it resolved, IR66).
func sourceAlert(
	ctx context.Context,
	call *ops.Call,
	unit, org, policy uuid.UUID,
	ruleKey string,
	cnd *AlertCondition,
	when time.Time,
) (uuid.UUID, error) {
	var key *string
	if ruleKey != "" {
		key = &ruleKey
	}

	var alertID uuid.UUID

	err := call.Tx.QueryRow(ctx, `SELECT id FROM monitoring.alerts WHERE unit_id = $1 AND policy_id = $2 AND rule_key IS
		NOT
		DISTINCT FROM $3 AND type = 'sensor' AND status <> 'resolved'`,
		unit, policy, key).
		Scan(&alertID)
	if err == nil {
		return alertID, nil
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		return alertID, apperr.Wrap(err, "select monitoring.alerts")
	}

	err = call.Tx.QueryRow(ctx, `INSERT INTO monitoring.alerts (tenant_id, unit_id, customer_org_id, policy_id, rule_key,
		type, severity, status, cause_code, evidence_kind, evidence_text,
		observed_at, detected_at, created_at, updated_at, previous_alert_id) VALUES (current_setting('app.tenant_id')::uuid,
		$1, $2, $3, $4, 'sensor', $5, 'open', 'unknown', 'demo_observation', $6, $7, $7, $7, $7,
		(SELECT p.id FROM monitoring.alerts p WHERE p.unit_id = $1 AND p.policy_id = $3 AND p.rule_key IS NOT DISTINCT FROM $4
		AND p.type = 'sensor' ORDER BY p.detected_at DESC, p.id DESC LIMIT 1))
		ON CONFLICT DO NOTHING RETURNING
		id`, unit, org, policy, key, cnd.Severity, "Demo event: "+cnd.Metric+" "+cnd.Operator+" threshold", when).
		Scan(&alertID)
	if errors.Is(
		err,
		pgx.ErrNoRows,
	) { // a concurrent evaluation opened it first (alerts_one_open_per_rule)
		err = call.Tx.QueryRow(ctx, `SELECT id FROM monitoring.alerts WHERE unit_id = $1 AND policy_id = $2 AND rule_key IS
			NOT
			DISTINCT FROM $3 AND type = 'sensor' AND status <> 'resolved'`,
			unit, policy, key).
			Scan(&alertID)
	}

	return alertID, apperr.Wrap(err, "select monitoring.alerts")
}

// deliveryFailure appends one no_recipient DeliveryFailure per alert/event/policy (SR12; repeats add none).
func deliveryFailure(
	ctx context.Context,
	call *ops.Call,
	alert, policy, eventID uuid.UUID,
	when time.Time,
) (uuid.UUID, error) {
	var raw []byte

	scanErr := call.Tx.QueryRow(ctx, `SELECT delivery_failures FROM monitoring.alerts WHERE id = $1 FOR UPDATE`, alert).
		Scan(&raw)
	if scanErr != nil {
		return uuid.Nil, apperr.Wrap(scanErr, "select monitoring.alerts")
	}

	// the entries stay maps: a field another writer adds to an entry is kept when the list is written back
	var failures []map[string]any

	err := json.Unmarshal(raw, &failures)
	if err != nil {
		return uuid.Nil, apperr.Wrap(err, "decode monitoring.alerts delivery_failures")
	}

	for _, failure := range failures {
		if failure["eventId"] == eventID.String() && failure[keyPolicyID] == policy.String() {
			text, _ := failure["id"].(string)
			id, err := uuid.Parse(text)

			return id, apperr.Wrap(err, "decode monitoring.alerts delivery_failures")
		}
	}

	failureID := uuid.New()
	added := map[string]any{
		"id":            failureID,
		keyPolicyID:     policy,
		"eventId":       eventID,
		"occurredAt":    when,
		"reason":        reasonNoRecipient,
		"deliveryState": outcomeFailed,
	}

	encoded, marshalErr := json.Marshal(append(failures, added))
	if marshalErr != nil {
		return uuid.Nil, apperr.Wrap(marshalErr, "json.Marshal")
	}

	_, err = call.Tx.Exec(
		ctx,
		`UPDATE monitoring.alerts SET delivery_failures = $2, updated_at = $3 WHERE id = $1`,
		alert,
		encoded,
		when,
	)

	return failureID, apperr.Wrap(err, "update monitoring.alerts")
}
