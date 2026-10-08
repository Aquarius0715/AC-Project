package server

import (
	"slices"

	"github.com/pradita/ac-project/service/api/internal/modules/assets"
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

// consumers returns the event subscribers of the served domains (all domains when none is given). Each consumer
// name is stable: it is the de-duplication key in platform.processed_events (IR183).
func consumers(m *db.TxManager, domains []string) []*events.Consumer {
	serves := func(d string) bool { return len(domains) == 0 || slices.Contains(domains, d) }
	var out []*events.Consumer
	add := func(domain, name string, handlers map[string]events.Handler) {
		if serves(domain) && len(handlers) > 0 {
			out = append(out, &events.Consumer{Name: name, DB: m, Handlers: handlers})
		}
	}
	add(ops.DomainEquipment, "equipment", merge(assets.EventHandlers(), monitoring.EventHandlers(), control.EventHandlers()))         // IR184, IR185
	add(ops.DomainBilling, "billing", restrictions.EventHandlers())                                                                   // IR185
	add(ops.DomainIdentity, "identity", merge(events.Handlers(notify.Replicas...), notify.EventHandlers(), identity.GrantHandlers())) // IR188
	add(ops.DomainMaintenance, "maintenance", events.Handlers(maintenance.Replicas...))                                               // IR192
	add(ops.DomainEnergy, "energy", events.Handlers(energy.Replicas...))                                                              // IR189
	return out
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
