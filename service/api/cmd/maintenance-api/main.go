// Command maintenance-api is a business-domain Core API service (IR180): maintenance jobs, offers, assignments, reports, partners and workforce.
// It serves POST /v1/ops/:operation for its domain only; the gateway routes each operation to its service.
package main

import (
	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/server"
)

func main() { server.Main("maintenance-api", ops.DomainMaintenance) }
