// Command identity-api is a business-domain Core API service (IR180): identity, memberships and sessions, notifications, audit, demo operations.
// It serves the REST routes of its domain's operations only (IR222); the gateway routes each route to its service.
package main

import (
	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/server"
)

func main() { server.Main("identity-api", ops.DomainIdentity) }
