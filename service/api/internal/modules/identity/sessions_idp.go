package identity

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/platform/events"
)

// SessionEnder ends every session of a user at the identity provider (IR386): Keycloak's admin logout locally,
// Cognito's AdminUserGlobalSignOut in production.
type SessionEnder interface {
	EndSessions(ctx context.Context, subject string) error
}

// EventSessionsRevoked is the event members.revokeSessions emits (IR384).
const EventSessionsRevoked = "SessionsRevoked"

// SessionHandlers ends the identity provider's sessions of the user HQ signed out everywhere: their refresh tokens
// and single sign-on end, so the next sign-in asks for the password (IR386). It is best effort. The Core API
// already refuses every earlier sign-in (IR384), and a retried event would hold up the identity-accounts consumer's
// later events (IR511), so a provider that does not answer is logged and the event is done. No handler without a provider.
func SessionHandlers(ender SessionEnder) map[string]events.Handler {
	if ender == nil {
		return nil
	}
	return map[string]events.Handler{
		EventSessionsRevoked: func(ctx context.Context, tx pgx.Tx, e events.Event) error {
			var p struct {
				UserID uuid.UUID `json:"userId"`
			}
			if err := e.Decode(&p); err != nil {
				return err
			}
			var subject string // the provider's subject: Cognito's sub, or locally the user ID itself
			err := tx.QueryRow(ctx, `SELECT COALESCE(cognito_sub, id::text) FROM identity.users WHERE id = $1`, p.UserID).Scan(&subject)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			if err != nil {
				return err
			}
			cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			if err := ender.EndSessions(cctx, subject); err != nil {
				slog.WarnContext(ctx, "identity: the identity provider's sessions were not ended", "user", p.UserID, "err", err)
			}
			return nil
		},
	}
}
