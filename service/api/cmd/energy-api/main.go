// Command energy-api is a business-domain Core API service (IR180): energy, baselines, MRV and offsets.
// It serves the REST routes of its domain's operations only (IR222); the gateway routes each route to its service.
package main

import (
	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/server"
)

func main() { server.Main("energy-api", ops.DomainEnergy) }
