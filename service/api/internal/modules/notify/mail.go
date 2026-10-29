package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"mime"
	"net/mail"
	"net/smtp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/db"
)

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
func (s SMTP) Send(_ context.Context, to, subject, body string) (string, error) {
	if strings.ContainsAny(to, "\r\n") || strings.ContainsAny(subject, "\r\n") { // no header injection
		return "", fmt.Errorf("mail: line break in a header")
	}
	id := uuid.NewString() + "@ac-project.local"
	// RFC 5322 needs a Date; a subject with non-ASCII text (a customer's or unit's name) is RFC 2047 encoded, as headers
	// are ASCII; the body's line ends become CRLF once
	msg := "From: " + s.From + "\r\nTo: " + to + "\r\nSubject: " + mime.QEncoding.Encode("UTF-8", subject) + "\r\nDate: " + time.Now().UTC().Format(time.RFC1123Z) +
		"\r\nMessage-ID: <" + id + ">\r\nMIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n" + strings.ReplaceAll(strings.ReplaceAll(body, "\r\n", "\n"), "\n", "\r\n")
	from, err := mail.ParseAddress(s.From) // the envelope takes the bare address ("Name <a@b>" → a@b)
	if err != nil {
		return "", fmt.Errorf("mail: sender %q: %w", s.From, err)
	}
	return id, smtp.SendMail(s.Addr, nil, from.Address, []string{to}, []byte(msg))
}

// Apps are the sign-in addresses of the web apps that e-mails link to.
type Apps struct{ Customer, Partner, Technician, Admin string }

// of is the app of a membership role: a client's customer app, a contractor's partner app, a technician's app, HQ's.
func (a Apps) of(role string) string {
	switch role {
	case "client":
		return a.Customer
	case "contractor":
		return a.Partner
	case "technician":
		return a.Technician
	default:
		return a.Admin
	}
}

// subjects of the notification e-mails by template key; the in-app texts of the web stay the reference.
var subjects = map[string]string{
	"alert": "Alert: %s", "quality": "Report quality: %s", "schedule_change": "Schedule change: %s", "report_return": "Report returned: %s",
	"completion": "Work completed: %s", "payment": "Payment: %s", "payment_reminder": "Payment reminder: %s", "restriction": "Service restriction: %s",
	"inquiry": "Inquiry: %s", "job_update": "Job update: %s", "device_operation": "Device operation: %s", "invite": "Invitation: %s",
	"certificate_expiry": "Certificate expiring: %s", "warranty_expiring": "Warranty ending: %s",
}

const footer = "\n\n— AC Project (demo). This message was sent by the demo stack; in development it only reaches the local Mailpit inbox.\n"

// notificationMail renders an e-mail notification: the subject from its template and target, the body with its details
// and a link to the recipient's app.
func notificationMail(templateKey string, params map[string]any, role string, apps Apps) (string, string) {
	name, _ := params["targetName"].(string)
	if name == "" {
		name = templateKey
	}
	format, ok := subjects[templateKey]
	if !ok {
		format = "Notification: %s"
	}
	subject := oneLine(fmt.Sprintf(format, name))
	var b strings.Builder
	b.WriteString(subject)
	for _, k := range []string{"status", "reason", "message"} {
		if v, ok := params[k].(string); ok && v != "" {
			fmt.Fprintf(&b, "\n%s: %s", strings.ToUpper(k[:1])+k[1:], v)
		}
	}
	fmt.Fprintf(&b, "\n\nOpen the app: %s/notifications", apps.of(role))
	b.WriteString(footer)
	return subject, b.String()
}

// oneLine keeps a subject on one line: a name with a line break (a unit or organization name) must not fail the send.
func oneLine(s string) string {
	return strings.Join(strings.Fields(strings.NewReplacer("\r", " ", "\n", " ").Replace(s)), " ")
}

// maxAttempts is how often a failed e-mail is tried before it stays failed.
const maxAttempts = 5

// retryDue is the SQL condition for a failed e-mail whose next attempt is due at $3 (now): 1, 2, 4 and 8 minutes after
// the attempts before (IR425), so an SMTP outage of a quarter of an hour loses nothing; the worker runs every 5 s.
const retryDue = `%[1]s.attempts < $1 AND %[1]s.%[2]s <= $3::timestamptz - interval '1 minute' * power(2, %[1]s.attempts - 1)`

// Deliver sends the tenant's pending e-mails in tx (the tenant is set on the transaction): the e-mail notifications
// without a sent delivery. Each attempt is recorded; a failure is retried on a growing pause (retryDue) up to
// maxAttempts. It answers how many were sent. A new account's set-up e-mail is the identity provider's (IR461, IR479).
func Deliver(ctx context.Context, tx pgx.Tx, s Sender, apps Apps, now time.Time, limit int) (int, error) {
	sent := 0
	type pending struct {
		id            uuid.UUID
		to, key, role string
		params        []byte
		attempts      int
		ended         bool // the recipient's membership has ended: the e-mail is suppressed, not sent
	}
	rows, err := tx.Query(ctx, `SELECT n.id, u.email::text, n.template_key, m.role, n.params, COALESCE(d.attempts, 0), m.valid_until IS NOT NULL AND m.valid_until <= $3
		FROM notify.notifications n JOIN identity.memberships m ON m.id = n.recipient_membership_id JOIN identity.users u ON u.id = m.user_id
		LEFT JOIN notify.deliveries d ON d.notification_id = n.id AND d.channel = 'email'
		WHERE n.channel = 'email' AND (d.id IS NULL OR (d.state = 'failed' AND `+fmt.Sprintf(retryDue, "d", "updated_at")+`))
		ORDER BY n.occurred_at LIMIT $2
		FOR UPDATE OF n SKIP LOCKED`, maxAttempts, limit, now) // a second worker takes other rows: no e-mail is sent twice
	if err != nil {
		return 0, err
	}
	var ns []pending
	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.id, &p.to, &p.key, &p.role, &p.params, &p.attempts, &p.ended); err != nil {
			rows.Close()
			return 0, err
		}
		ns = append(ns, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	for _, p := range ns {
		var params map[string]any
		_ = json.Unmarshal(p.params, &params)
		subject, body := notificationMail(p.key, params, p.role, apps)
		var msgID string
		var err error
		state, reason := "sent", (*string)(nil)
		switch {
		case p.ended: // a disabled or removed account gets no more e-mails (recorded, never retried)
			state, reason = "suppressed", ptr("membership ended")
		default:
			if msgID, err = s.Send(ctx, p.to, subject, body); err != nil {
				state, reason = "failed", ptr(err.Error())
			} else {
				sent++
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO notify.deliveries (tenant_id, notification_id, channel, state, provider_message_id, failure_reason, attempts, updated_at)
			VALUES (current_setting('app.tenant_id')::uuid, $1, 'email', $2, NULLIF($3, ''), $4, $5, $6)
			ON CONFLICT (notification_id, channel) DO UPDATE SET state = EXCLUDED.state, provider_message_id = EXCLUDED.provider_message_id,
				failure_reason = EXCLUDED.failure_reason, attempts = EXCLUDED.attempts, updated_at = EXCLUDED.updated_at`,
			p.id, state, msgID, reason, p.attempts+1, now); err != nil {
			return sent, err
		}
	}
	return sent, nil
}

func ptr(s string) *string { return &s }

// DeliverAll runs Deliver for every tenant, each in its own transaction with the tenant set (row-level security).
func DeliverAll(ctx context.Context, m *db.TxManager, s Sender, apps Apps, now time.Time, limit int) (int, error) {
	var tenants []uuid.UUID
	rows, err := m.Writer.Query(ctx, `SELECT id FROM platform.tenants ORDER BY id`)
	if err != nil {
		return 0, err
	}
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		tenants = append(tenants, id)
	}
	rows.Close()
	total := 0
	for _, t := range tenants {
		err := m.Run(ctx, false, &ops.Principal{TenantID: t}, func(tx pgx.Tx) error {
			n, err := Deliver(ctx, tx, s, apps, now, limit)
			total += n
			return err
		})
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

// Retention is how long the notifications and their deliveries are kept (database design §6, IR428).
const Retention = 2 * 365 * 24 * time.Hour

// Sweep deletes, in the tenant of tx, the notifications older than Retention with their deliveries. It answers how
// many rows went.
func Sweep(ctx context.Context, tx pgx.Tx, now time.Time) (int64, error) {
	cut := now.Add(-Retention)
	var n int64
	for _, q := range []string{
		`DELETE FROM notify.deliveries d USING notify.notifications x WHERE x.id = d.notification_id AND x.occurred_at < $1`,
		`DELETE FROM notify.notifications WHERE occurred_at < $1`,
	} {
		tag, err := tx.Exec(ctx, q, cut)
		if err != nil {
			return n, err
		}
		n += tag.RowsAffected()
	}
	return n, nil
}

// SweepAll runs Sweep for every tenant, each in its own transaction with the tenant set (row-level security).
func SweepAll(ctx context.Context, m *db.TxManager, now time.Time) (int64, error) {
	var tenants []uuid.UUID
	rows, err := m.Writer.Query(ctx, `SELECT id FROM platform.tenants ORDER BY id`)
	if err != nil {
		return 0, err
	}
	if tenants, err = pgx.CollectRows(rows, pgx.RowTo[uuid.UUID]); err != nil {
		return 0, err
	}
	var total int64
	for _, t := range tenants {
		if err := m.Run(ctx, false, &ops.Principal{TenantID: t}, func(tx pgx.Tx) error {
			n, err := Sweep(ctx, tx, now)
			total += n
			return err
		}); err != nil {
			return total, err
		}
	}
	return total, nil
}
