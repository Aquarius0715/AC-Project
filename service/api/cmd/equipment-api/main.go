// Command equipment-api is a business-domain Core API service (IR180): units and locations, devices, remote control, monitoring and alerts, dashboard read models.
// It serves POST /v1/ops/:operation for its domain only; the gateway routes each operation to its service.
package main

import (
	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/server"
)

func main() { server.Main("equipment-api", ops.DomainEquipment) }
