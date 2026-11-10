package server

import (
	"slices"

	"github.com/pradita/ac-project/service/api/internal/modules/assets"
	"github.com/pradita/ac-project/service/api/internal/modules/audit"
	"github.com/pradita/ac-project/service/api/internal/modules/billing"
	"github.com/pradita/ac-project/service/api/internal/modules/control"
	"github.com/pradita/ac-project/service/api/internal/modules/energy"
	"github.com/pradita/ac-project/service/api/internal/modules/identity"
	"github.com/pradita/ac-project/service/api/internal/modules/maintenance"
	"github.com/pradita/ac-project/service/api/internal/modules/monitoring"
	"github.com/pradita/ac-project/service/api/internal/modules/notify"
	"github.com/pradita/ac-project/service/api/internal/modules/restrictions"
	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/db"
	"github.com/pradita/ac-project/service/api/internal/platform/events"
)

// consumerSet collects the event subscribers of the served domains (all domains when none is given).
type consumerSet struct {
	txm     *db.TxManager
	domains []string
	out     []*events.Consumer
}

// add adds the consumer name with domain's handlers when the process serves the domain and there are handlers.
func (set *consumerSet) add(
	domain, name string,
	handlers map[string]events.Handler,
) *events.Consumer {
	if len(handlers) == 0 || (len(set.domains) > 0 && !slices.Contains(set.domains, domain)) {
		return nil
	}

	consumer := &events.Consumer{
		Name:      name,
		DB:        set.txm,
		Handlers:  handlers,
		Batch:     0,
		Logf:      nil,
		Now:       nil,
		SplitFrom: "",
	}
	set.out = append(set.out, consumer)

	return consumer
}

// consumers returns the event subscribers of the served domains (all domains when none is given). Each consumer
// name is stable: it is the de-duplication key in platform.processed_events (IR183). provider ends a user's sessions
// at the identity provider when HQ signs them out everywhere (nil: none, IR386).
func consumers(
	txm *db.TxManager,
	domains []string,
	provider identity.Provider,
	apps identity.AccountApps,
) []*events.Consumer {
	set := &consumerSet{txm: txm, domains: domains, out: nil}

	set.add(
		ops.DomainEquipment,
		"equipment",
		merge(assets.EventHandlers(), monitoring.EventHandlers(), control.EventHandlers()),
	) // IR184, IR185
	set.add(
		ops.DomainBilling,
		"billing",
		merge(restrictions.EventHandlers(), events.Handlers(billing.Replicas()...)),
	) // IR185
	set.add(
		ops.DomainIdentity,
		"identity",
		merge(
			events.Handlers(notify.Replicas()...),
			notify.EventHandlers(),
			identity.GrantHandlers(),
			audit.EventHandlers(),
		),
	) // IR188
	// the sign-in accounts and sessions in their own consumer (IR511): a notification, audit or copy event that keeps
	// failing holds back only "identity", not turning an account off or ending sessions (IR386, IR461); the events
	// "identity" applied before the split count as applied, so no set-up e-mail or account change is repeated
	if c := set.add(
		ops.DomainIdentity,
		"identity-accounts",
		merge(identity.SessionHandlers(provider), identity.AccountHandlers(provider, apps)),
	); c != nil {
		c.SplitFrom = "identity"
	}

	set.add(
		ops.DomainMaintenance,
		"maintenance",
		merge(
			events.Handlers(maintenance.Replicas()...),
			maintenance.NoteHandlers(),
			maintenance.CustomerHandlers(),
		),
	) // IR192
	set.add(
		ops.DomainEnergy,
		"energy",
		events.Handlers(energy.Replicas()...),
	) // IR189

	return set.out
}

func merge(ms ...map[string]events.Handler) map[string]events.Handler {
	out := map[string]events.Handler{}

	for _, m := range ms {
		for k, h := range m {
			if _, dup := out[k]; dup {
				panic("events: two handlers for " + k + " in one consumer")
			}

			out[k] = h
		}
	}

	return out
}
