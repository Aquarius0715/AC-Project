package server

import (
	"slices"

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
	// IR183 step 2b adds the restriction ↔ equipment handlers here.
	add(ops.DomainEquipment, "equipment", map[string]events.Handler{})
	add(ops.DomainBilling, "billing", map[string]events.Handler{})
	return out
}
