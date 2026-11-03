package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/mail"
	"net/smtp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/authz"
	"github.com/pradita/ac-project/service/api/internal/platform/db"
)

// errHeaderLineBreak refuses a header value with a line break in it (header injection).
var errHeaderLineBreak = errors.New("mail: line break in a header")

// Sender sends one plain-text e-mail and answers the provider's message ID (IR418). In development it is SMTP to
// Mailpit, which keeps every message for its web inbox and delivers nothing; production sends through Amazon SES.
type Sender interface {
	Send(ctx context.Context, to, subject, body string) (string, error)
}

// SMTP sends through an SMTP server without authentication (Mailpit in the compose stack).
type SMTP struct {
	Addr string // host:port
	From string
}

// Send implements Sender.
func (s SMTP) Send(_ context.Context, recipient, subject, body string) (string, error) {
	if strings.ContainsAny(recipient, "\r\n") ||
		strings.ContainsAny(subject, "\r\n") { // no header injection
		return "", errHeaderLineBreak
	}

	id := uuid.NewString() + "@ac-project.local"
	// RFC 5322 needs a Date; a subject with non-ASCII text (a customer's or unit's name) is RFC 2047 encoded, as headers
	// are ASCII; the body's line ends become CRLF once
	msg := "From: " + s.From + "\r\nTo: " + recipient + "\r\nSubject: " + mime.QEncoding.Encode(
		"UTF-8",
		subject,
	) + "\r\nDate: " + time.Now().
		UTC().
		Format(time.RFC1123Z) +
		"\r\nMessage-ID: <" + id + ">\r\nMIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n" + strings.ReplaceAll(
		strings.ReplaceAll(body, "\r\n", "\n"),
		"\n",
		"\r\n",
	)

	from, err := mail.ParseAddress(
		s.From,
	) // the envelope takes the bare address ("Name <a@b>" → a@b)
	if err != nil {
		return "", fmt.Errorf("mail: sender %q: %w", s.From, err)
	}

	return id, apperr.Wrap(
		smtp.SendMail(s.Addr, nil, from.Address, []string{recipient}, []byte(msg)),
		"smtp.SendMail",
	)
}

// Apps are the sign-in addresses of the web apps that e-mails link to.
type Apps struct{ Customer, Partner, Technician, Admin string }

// of is the app of a membership role: a client's customer app, a contractor's partner app, a technician's app, HQ's.
func (a Apps) of(role string) string {
	switch role {
	case authz.RoleClient:
		return a.Customer
	case authz.RoleContractor:
		return a.Partner
	case authz.RoleTechnician:
		return a.Technician
	default:
		return a.Admin
	}
}

// subjectFormats are the subjects of the notification e-mails by template key; the in-app texts of the web stay the
// reference.
func subjectFormats() map[string]string {
	return map[string]string{
		noticeAlert:           "Alert: %s",
		noticeQuality:         "Report quality: %s",
		noticeScheduleChange:  "Schedule change: %s",
		noticeReportReturn:    "Report returned: %s",
		noticeCompletion:      "Work completed: %s",
		noticePayment:         "Payment: %s",
		noticePaymentReminder: "Payment reminder: %s",
		targetRestriction:     "Service restriction: %s",
		targetInquiry:         "Inquiry: %s",
		noticeJobUpdate:       "Job update: %s",
		noticeDeviceOperation: "Device operation: %s",
		"invite":              "Invitation: %s",
		"certificate_expiry":  "Certificate expiring: %s",
		"warranty_expiring":   "Warranty ending: %s",
	}
}

const footer = "\n\n— AC Project (demo). This message was sent by the demo stack; in development it only reaches the " +
	"local Mailpit inbox.\n"

// notificationMail renders an e-mail notification: the subject from its template and target, the body with its details
// and a link to the recipient's app.
func notificationMail(
	templateKey string,
	params map[string]any,
	role string,
	apps Apps,
) (string, string) {
	name, _ := params["targetName"].(string)
	if name == "" {
		name = templateKey
	}

	format, ok := subjectFormats()[templateKey]
	if !ok {
		format = "Notification: %s"
	}

	subject := oneLine(fmt.Sprintf(format, name))

	var builder strings.Builder
	builder.WriteString(subject)

	for _, k := range []string{keyStatus, "reason", "message"} {
		if v, ok := params[k].(string); ok && v != "" {
			fmt.Fprintf(&builder, "\n%s: %s", strings.ToUpper(k[:1])+k[1:], v)
		}
	}

	fmt.Fprintf(&builder, "\n\nOpen the app: %s/notifications", apps.of(role))
	builder.WriteString(footer)

	return subject, builder.String()
}

// oneLine keeps a subject on one line: a name with a line break (a unit or organization name) must not fail the send.
func oneLine(s string) string {
	return strings.Join(strings.Fields(strings.NewReplacer("\r", " ", "\n", " ").Replace(s)), " ")
}

// maxAttempts is how often a failed e-mail is tried before it stays failed.
const maxAttempts = 5

// retryDue is the SQL condition for a failed e-mail whose next attempt is due at $3 (now): 1, 2, 4 and 8 minutes after
// the attempts before (IR425), so an SMTP outage of a quarter of an hour loses nothing; the worker runs every 5 s.
const retryDue = `%[1]s.attempts < $1 AND %[1]s.%[2]s <= $3::timestamptz - interval '1 minute' * power(2, ` +
	`%[1]s.attempts - 1)`

// Deliver sends the tenant's pending e-mails in tx (the tenant is set on the transaction): the e-mail notifications
// without a sent delivery. Each attempt is recorded; a failure is retried on a growing pause (retryDue) up to
// maxAttempts. It answers how many were sent. A new account's set-up e-mail is the identity provider's (IR461, IR479).
func Deliver(
	ctx context.Context,
	txn pgx.Tx,
	sender Sender,
	apps Apps,
	now time.Time,
	limit int,
) (int, error) {
	due, err := dueMails(ctx, txn, now, limit)
	if err != nil {
		return 0, err
	}

	sent := 0

	for _, email := range due {
		delivered, err := deliverOne(ctx, txn, sender, apps, now, email)
		if err != nil {
			return sent, err
		}

		if delivered {
			sent++
		}
	}

	return sent, nil
}

// pendingMail is one e-mail notification waiting to be sent: its recipient's address and role, its template and
// parameters, the attempts so far, and whether the recipient's membership has ended.
type pendingMail struct {
	id            uuid.UUID
	to, key, role string
	params        []byte
	attempts      int
	ended         bool
}

// dueMails are the e-mails to send now, oldest first and at most limit: never delivered, or failed and due for a retry.
// They are locked, and a second worker skips them, so no e-mail is sent twice.
func dueMails(ctx context.Context, txn pgx.Tx, now time.Time, limit int) ([]pendingMail, error) {
	rows, err := txn.Query(
		ctx,
		`SELECT n.id, u.email::text, n.template_key, m.role, n.params, COALESCE(d.attempts, 0), m.valid_until IS NOT NULL AND
			m.valid_until <= $3
		FROM notify.notifications n JOIN identity.memberships m ON m.id = n.recipient_membership_id JOIN identity.users u ON
		u.id = m.user_id
		LEFT JOIN notify.deliveries d ON d.notification_id = n.id AND d.channel = 'email'
		WHERE n.channel = 'email' AND (d.id IS NULL OR (d.state = 'failed' AND `+fmt.Sprintf(
			retryDue,
			"d",
			"updated_at",
		)+`))
		ORDER BY n.occurred_at LIMIT $2
		FOR UPDATE OF n SKIP LOCKED`,
		maxAttempts,
		limit,
		now,
	) // a second worker takes other rows: no e-mail is sent twice
	if err != nil {
		return nil, apperr.Wrap(err, "select notify.notifications")
	}
	defer rows.Close()

	var due []pendingMail

	for rows.Next() {
		var email pendingMail

		err := rows.Scan(
			&email.id,
			&email.to,
			&email.key,
			&email.role,
			&email.params,
			&email.attempts,
			&email.ended,
		)
		if err != nil {
			return nil, apperr.Wrap(err, "select notify.notifications")
		}

		due = append(due, email)
	}

	return due, apperr.Wrap(rows.Err(), "select notify.notifications")
}

// deliverOne sends one e-mail and records its delivery, and reports whether it was sent. A recipient whose membership
// ended gets no more e-mails, and parameters that do not decode cannot be written: both are suppressed (recorded,
// never retried). A sender's error fails the delivery, which is retried.
func deliverOne(
	ctx context.Context,
	txn pgx.Tx,
	sender Sender,
	apps Apps,
	now time.Time,
	email pendingMail,
) (bool, error) {
	state, reason, msgID := "sent", (*string)(nil), ""

	var params map[string]any

	switch {
	case email.ended:
		state, reason = "suppressed", new("membership ended")
	case json.Unmarshal(email.params, &params) != nil:
		state, reason = "suppressed", new("parameters do not decode")
	default:
		subject, body := notificationMail(email.key, params, email.role, apps)

		var err error

		msgID, err = sender.Send(ctx, email.to, subject, body)
		if err != nil {
			state, reason = "failed", new(err.Error())
		}
	}

	_, err := txn.Exec(
		ctx,
		`INSERT INTO notify.deliveries (tenant_id, notification_id, channel, state, provider_message_id, failure_reason,
			attempts, updated_at)
		VALUES (current_setting('app.tenant_id')::uuid, $1, 'email', $2, NULLIF($3, ''), $4, $5, $6)
		ON CONFLICT (notification_id, channel) DO UPDATE SET state = EXCLUDED.state, provider_message_id =
		EXCLUDED.provider_message_id,
			failure_reason = EXCLUDED.failure_reason, attempts = EXCLUDED.attempts, updated_at = EXCLUDED.updated_at`,
		email.id,
		state,
		msgID,
		reason,
		email.attempts+1,
		now,
	)
	if err != nil {
		return false, apperr.Wrap(err, "insert notify.deliveries")
	}

	return state == "sent", nil
}

// DeliverAll runs Deliver for every tenant, each in its own transaction with the tenant set (row-level security).
func DeliverAll(
	ctx context.Context,
	txm *db.TxManager,
	sender Sender,
	apps Apps,
	now time.Time,
	limit int,
) (int, error) {
	var tenants []uuid.UUID

	rows, err := txm.Writer.Query(ctx, `SELECT id FROM platform.tenants ORDER BY id`)
	if err != nil {
		return 0, apperr.Wrap(err, "select platform.tenants")
	}

	for rows.Next() {
		var id uuid.UUID

		err := rows.Scan(&id)
		if err != nil {
			rows.Close()

			return 0, apperr.Wrap(err, "select platform.tenants")
		}

		tenants = append(tenants, id)
	}

	rows.Close()

	rowsErr := rows.Err()
	if rowsErr != nil {
		return 0, apperr.Wrap(rowsErr, "select platform.tenants")
	}

	total := 0

	for _, t := range tenants {
		err := txm.Run(
			ctx,
			false,
			&ops.Principal{
				TenantID:          t,
				Principal:         authz.Principal{Role: "", Permissions: nil, ClientRole: ""},
				MembershipID:      uuid.UUID{},
				UserID:            uuid.UUID{},
				OrgID:             uuid.UUID{},
				ScopeVersion:      0,
				Scopes:            nil,
				Employment:        "",
				Timezone:          "",
				SessionsRevokedAt: nil,
			},
			func(tx pgx.Tx) error {
				n, err := Deliver(ctx, tx, sender, apps, now, limit)
				total += n

				return err
			},
		)
		if err != nil {
			return total, apperr.Wrap(err, "db.TxManager.Run")
		}
	}

	return total, nil
}

// Retention is how long the notifications and their deliveries are kept (database design §6, IR428).
const Retention = 2 * 365 * 24 * time.Hour

// Sweep deletes, in the tenant of tx, the notifications older than Retention with their deliveries. It answers how
// many rows went.
func Sweep(ctx context.Context, txn pgx.Tx, now time.Time) (int64, error) {
	cut := now.Add(-Retention)

	var n int64

	for _, q := range []string{
		`DELETE FROM notify.deliveries d USING notify.notifications x WHERE x.id = d.notification_id AND x.occurred_at < $1`,
		`DELETE FROM notify.notifications WHERE occurred_at < $1`,
	} {
		tag, err := txn.Exec(ctx, q, cut)
		if err != nil {
			return n, apperr.Wrap(err, "exec (Sweep)")
		}

		n += tag.RowsAffected()
	}

	return n, nil
}

// SweepAll runs Sweep for every tenant, each in its own transaction with the tenant set (row-level security).
func SweepAll(ctx context.Context, txm *db.TxManager, now time.Time) (int64, error) {
	var tenants []uuid.UUID

	rows, err := txm.Writer.Query(ctx, `SELECT id FROM platform.tenants ORDER BY id`)
	if err != nil {
		return 0, apperr.Wrap(err, "select platform.tenants")
	}

	tenants, err = pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return 0, apperr.Wrap(err, "select platform.tenants")
	}

	var total int64

	for _, t := range tenants {
		err := txm.Run(
			ctx,
			false,
			&ops.Principal{
				TenantID:          t,
				Principal:         authz.Principal{Role: "", Permissions: nil, ClientRole: ""},
				MembershipID:      uuid.UUID{},
				UserID:            uuid.UUID{},
				OrgID:             uuid.UUID{},
				ScopeVersion:      0,
				Scopes:            nil,
				Employment:        "",
				Timezone:          "",
				SessionsRevokedAt: nil,
			},
			func(tx pgx.Tx) error {
				n, err := Sweep(ctx, tx, now)
				total += n

				return err
			},
		)
		if err != nil {
			return total, apperr.Wrap(err, "db.TxManager.Run")
		}
	}

	return total, nil
}
