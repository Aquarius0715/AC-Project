package monitoring

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/modules/identity"
	"github.com/pradita/ac-project/service/api/internal/modules/notify"
	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/authz"
)

// statusEscalated is the status word of an escalation notice.
const statusEscalated = "escalated"

// Escalation is the equipment scheduler job of the policies' escalation time (DD-A05, IR784): an alert opened by a
// policy that is still not acknowledged escalateAfterMinutes after it was detected tells the HQ members who hold
// alert.policy.write, once.
type Escalation struct {
	Notify notify.Store
}

// Run escalates the alerts that are due and returns how many it escalated. An alert is marked first (escalated_at, no
// version bump, so a person acknowledging it at the same time is not refused), so that it is escalated once however
// many ticks follow; an alert acknowledged or resolved before its time is never escalated.
func (escalation Escalation) Run(ctx context.Context, call *ops.Call) (int, error) {
	rows, err := call.Tx.Query(
		ctx,
		`UPDATE monitoring.alerts a SET escalated_at = $1 FROM monitoring.alert_policies p
		WHERE p.id = a.policy_id AND a.status = 'open' AND a.escalated_at IS NULL AND p.escalate_after_minutes > 0
		AND a.detected_at + make_interval(mins => p.escalate_after_minutes) <= $1
		RETURNING a.id, a.unit_id, a.severity`,
		call.Now,
	)
	if err != nil {
		return 0, apperr.Wrap(err, "update monitoring.alerts")
	}

	type due struct {
		id, unit uuid.UUID
		severity string
	}

	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (due, error) {
		var alert due

		return alert, row.Scan(&alert.id, &alert.unit, &alert.severity)
	})
	if err != nil || len(list) == 0 {
		return 0, apperr.Wrap(err, "update monitoring.alerts")
	}

	recipients, err := identity.Members(
		ctx,
		call,
		identity.MembersInput{
			Role:           authz.RoleAdmin,
			Permission:     "alert.policy.write",
			IDs:            nil,
			OrganizationID: nil,
			ClientRole:     "",
		},
	)
	if err != nil {
		return 0, apperr.Wrap(err, "identity.Members")
	}

	for _, alert := range list {
		for _, recipient := range recipients {
			_, err := escalation.Notify.Create(ctx, call, notify.New{
				RecipientMembershipID: recipient,
				Type:                  "fault",
				TemplateKey:           kindAlert,
				TargetKind:            "unit",
				TargetID:              alert.unit,
				Severity:              alert.severity,
				SourceAlertID:         &alert.id,
				Params:                map[string]any{"status": statusEscalated},
				Channel:               "",
			})
			if err != nil {
				return 0, apperr.Wrap(err, "notify.Store.Create")
			}
		}

		call.Audit(
			ops.AuditEntry{
				Action:          "alerts.escalate",
				TargetKind:      kindAlert,
				TargetID:        alert.id.String(),
				PreviousVersion: nil,
				NextVersion:     nil,
				Reason:          "",
				Result:          "",
				Before:          nil,
				After:           nil,
			},
		)
	}

	return len(list), nil
}
