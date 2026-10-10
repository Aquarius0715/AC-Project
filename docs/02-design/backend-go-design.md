---
document_id: DD-BACKEND-GO
version: 0.30.0
status: draft
owner: design-agent
consumers: [implementation-agent, test-agent, review-agent]
scope: production-target-implementation
---

# Backend Implementation Design (Go + Echo)

## 1. Purpose and decisions

This document turns the [backend architecture](backend-architecture.md) into an implementation design for the Core API, the webhook receiver, and the workers. The database is designed in the [database design](database-design.md) and [db/schema.sql](db/schema.sql). Status: `PROPOSED` (IR118, DEC-69). Phase 1A is unchanged.

| Decision | Choice |
|---|---|
| Language | Go 1.25 or later (one module `github.com/pradita/ac-project/service/api` (`service/api/go.mod`) in the official Go server layout (IR174): commands in `service/api/cmd/{api,worker,migrate,seed,gen}`, every package in `service/api/internal/`, DB integration tests in `service/api/test/integration`, Dockerfiles in `service/api/build/`; current pgx v5 and golang.org/x modules require Go ≥ 1.25) |
| HTTP framework | Echo v5 (`github.com/labstack/echo/v5`, checked with v5.4.0) for the Core API and the webhook receiver, structured as in the official Echo guide (IR173): handlers `func(c *echo.Context) error` return errors to one central `HTTPErrorHandler`; middleware from `echo/v5/middleware`; route group `/v1`; graceful shutdown with `echo.StartConfig`; `log/slog` as `e.Logger`; tests with `net/http/httptest` against `e.ServeHTTP` |
| Database driver | pgx v5 (`github.com/jackc/pgx/v5`, `pgxpool`); no ORM |
| Query code | sqlc generates typed Go from SQL in `db/queries/<schema>/*.sql`; hand-written SQL only for dynamic list filters |
| Migrations | `cmd/migrate` + `internal/migrate` + `internal/migrations` (IR167, IR174): embedded `migrations/NNNNNN_name.up.sql` files applied in order under an advisory lock, one transaction each (`-- migrate:no-transaction` for `CONCURRENTLY`), bookkeeping in the golang-migrate compatible `schema_migrations` table; `000001_init` is [db/schema.sql](db/schema.sql) |
| IDs | UUIDv7 from `github.com/google/uuid` (`uuid.NewV7`), time-ordered for index locality |
| Validation | Generated input structs + a `Validate() *DomainError` method per input (rules from the DD documents); `go-playground/validator` only for simple tags |
| Logging / tracing | `log/slog` JSON with a masking handler; OpenTelemetry SDK + `otelecho`, exported through the AWS Distro for OpenTelemetry collector |
| AWS | AWS SDK for Go v2 (SQS, SNS, Kinesis, IoT data plane, IoT, S3, SES, Secrets Manager, KMS) |
| Payments | `stripe-go` (current major version) with webhook signature verification |
| Authentication | Cognito access tokens verified with JWKS (`lestrrat-go/jwx`), cached keys |
| Configuration | Environment variables parsed into a typed struct (`caarlos0/env`); secrets fetched from Secrets Manager at start-up |
| Tests | `go test`, `testcontainers-go` (PostgreSQL 16, LocalStack), `httptest` for Echo, `golangci-lint` |
| Runtime | Everything runs in Docker ([container design](container-design.md), IR119): static binaries with `CGO_ENABLED=0` in a distroless image, linux/arm64 on ECS Fargate (Graviton), Docker Compose locally and in CI |

The core request pipeline in §4 and the example handler in §5 were compiled and run against PostgreSQL 16 with [db/schema.sql](db/schema.sql) on 2026-10-07 (create, update with version, stale version → 409 CONFLICT, validation → 422, unknown field → 422, cross-tenant create → 404, HQ user outside the company network → 403, outbox rows written in the same transaction).

## 2. Repository layout

```text
service/api/        (IR174, moved under service/api by IR179; the Next.js apps are the siblings service/web/* and service/web/shared, IR178)
  go.mod
  Makefile          gen, seed, test, test-unit, test-integration, cover, resetdb, testdb
  build/            api.Dockerfile, worker.Dockerfile, migrate.Dockerfile (context service/)
  scripts/          resetdb.sh, covermerge.py
  cmd/
    gateway/        Core API entry: routes the catalog's REST routes (IR222) to the domain service that owns each route's operation (IR180)
    identity-api/ equipment-api/ maintenance-api/ billing-api/ energy-api/
                    business-domain Core API services (Echo) — the REST routes of their domain's operations, /healthz, /readyz
    webhook/        Webhook receiver (Echo) — /stripe, /ses, /whatsapp
    worker/         one binary, --role=outbox|scheduler|notification|telemetry|iot|automation|importexport|rollup
    migrate/        applies internal/migrations (one-off ECS task before deploy, IR167)
    seed/           loads the demo fixture
    gen/            code generators (operations, contracts) — run in CI, output committed
  internal/
    server/         composition root and Echo layer (app.go wiring, http.go newEcho + middleware, demo.*, unit detail join)
    migrate/ migrations/ seed/ scheduler/   migration runner + embedded SQL, fixture loader, clock-driven transitions
    platform/
      config/       typed configuration
      apperr/       DomainError, codes, PostgreSQL error mapping
      httpx/        Echo setup, middleware, error handler, JSON envelope
      auth/         JWT verification, Principal, membership loader, HQ-network check
      authz/        policies compiled from the operation catalog authorization column
      db/           pgxpool (writer / reader), TxManager, RLS context
      idempotency/  Idempotency-Key store (platform.idempotency_keys)
      outbox/       outbox writer and relay
      audit/        audit writer (masking rules)
      events/       event envelope, SNS publisher, SQS consumer loop, processed_events de-duplication
      clock/ ids/ observability/ awsx/
    ops/            registry, dispatcher, catalog_gen.go (from operation-catalog.csv)
    contracts/      types_gen.go (from service-contracts.ts via JSON Schema)
    modules/
      identity/ assets/ devices/ control/ monitoring/ maintenance/
      billing/ restrictions/ energy/ notify/ audit/
        module.go     Register(reg, deps): binds operation names to handlers
        handlers.go   one function per operation (input → output)
        domain/       pure rules and state machines (no I/O)
        store/        sqlc output + small repository helpers
        consumers.go  event consumers for this module
  db/
    migrations/
    queries/<schema>/*.sql
  sqlc.yaml
  test/
    contract/       catalog coverage and DomainError mapping tests
    acceptance/     AT-* scenarios replayed against the API with fixture-contract.json seeds
  Dockerfile        targets runtime and dev (container design §3)
  Makefile
  (compose.yaml lives at the repository root and runs the whole system — container design §4)
```

Dependency rules (checked by `go-arch-lint` or import tests in CI):

1. `modules/<a>` never imports `modules/<b>`. Cross-module needs go through a small interface in the caller's package that `app/` wires (for example `control.UnitReader` implemented by assets), or through events.
2. `domain/` packages import only the standard library and `contracts/`.
3. Only `app/` constructs pools, AWS clients, and Echo. Handlers receive dependencies, never globals.
4. A module only queries its own schema (enforced by sqlc configuration per module and a CI grep for foreign schema names).

## 3. Code generation from the specification

| Source (docs) | Generator | Output | Check in CI |
|---|---|---|---|
| [operation-catalog.csv](operation-catalog.csv) | `cmd/gen/ops` | `internal/ops/catalog_gen.go`: 197 entries with name, mode, module, design IDs, authorization expression | Every catalog operation has exactly one registered handler; no extra handlers |
| [service-contracts.ts](service-contracts.ts) | `typescript-json-schema` → `go-jsonschema` | `internal/contracts/types_gen.go` (inputs, results, entities) | Regenerating produces no diff |
| [write-version-catalog.csv](write-version-catalog.csv) | `cmd/gen/ops` | expected-version requirement per write operation | Writes listed as `required` reject a missing version with VALIDATION |
| [fixture-contract.json](../04-agentic-sdlc/fixture-contract.json) | `test/acceptance/seed` | Seed loader for acceptance tests | AT scenarios pass |

The authorization column (for example `client:self-customer:owner | admin:alert.policy.write`) is parsed into a list of alternatives; each alternative is a role plus a permission and named scope predicates implemented in `platform/authz` (`self-customer`, `owner`, `assigned-valid-job`, `own-company`, `own-valid-offer`, `demo-only`, …). An unknown predicate fails generation, so the catalog and the code cannot drift.

## 4. Request pipeline (Core API)

Echo setup (`service/api/internal/server/http.go`, Echo guide: Quickstart, Routing, Error Handling, Cookbook › Graceful Shutdown, Testing): one `echo.New()`, the middleware below in order, `/healthz` and `/readyz`, the group `v1 := e.Group("/v1", auth)` with the REST routes of the catalog (`reg.MountREST(v1)`, IR222; `POST /v1/ops/:operation` was retired with IR223), and `e.HTTPErrorHandler = ops.HTTPErrorHandler`. Handlers and middleware never write error bodies themselves; they return a `*apperr.DomainError` (which implements `echo.HTTPStatusCoder`) and the central handler writes it with the correlation ID, maps Echo's own errors (unknown route or method → NOT_FOUND `error.unknownOperation`, 413 → VALIDATION `error.bodyTooLarge`, 503/504 → TIMEOUT) and turns any other error into UNAVAILABLE without internal detail, skipping responses already committed. `cmd/api` starts with `echo.StartConfig{GracefulTimeout: 25s}.Start(ctx, e)` on a SIGINT/SIGTERM context.

Echo middleware order:

| Order | Middleware | Behaviour |
|---|---|---|
| 1 | Recover | Panic → UNAVAILABLE (503), stack logged, no detail returned |
| 2 | Correlation ID | `middleware.RequestIDWithConfig` with a UUIDv7 generator (keeps an incoming `X-Request-Id`); echoed in `X-Request-Id`, logs, traces, events, audit |
| 3 | OpenTelemetry (`otelecho`) | Span per request, attributes `ac.operation`, `ac.tenant` |
| 4 | Access log | `middleware.RequestLoggerWithConfig` writing one `slog` JSON line (method, URI, status after the error handler, request ID, latency); bodies are never logged |
| 5 | Body limit / timeout | 1 MiB JSON checked by the dispatcher per operation, file operations more: attachments.add and jobs.reportProblem 8 MiB, reports.signOff 16 MiB (base64 files in the demo; production uploads go to S3, IR221), larger bodies VALIDATION `error.bodyTooLarge`; `middleware.ContextTimeout` 30 s on the request context used by the database (per-operation 5 s / 10 s deadlines remain the production target) |
| 6 | Authentication | Verifies the Cognito access token (issuer, audience = app client, expiry, `token_use=access`), loads the selected Membership (`X-Tenant-Id` + `X-Membership-Id` from the BFF session context; the membership must belong to the token subject, be inside its validity window and the user must be active, otherwise UNAUTHENTICATED) with permissions, scopes, client role and validity; requests without a token stay anonymous and reach only `public:` operations; caches it for 30 s keyed by membershipId + scopeVersion. A token whose `auth_time` is later than the user's recorded sign-in sets `last_sign_in_at` to the business time (IR268) |
| 7 | HQ network | Admin-role principals must carry a token from the admin app client (served only on `admin.<domain>`, IR117); otherwise FORBIDDEN |
| 8 | Dispatcher | The REST route of an operation: `ops.Binding` builds the JSON input from the path, the query string or the body, and the route's fixed field, then the pipeline below runs |

Dispatcher steps for one operation:

1. Look up the operation of the route (an unknown route or method → NOT_FOUND `error.unknownOperation`; an ID path parameter that is not a UUID → NOT_FOUND `error.notFound`, IR223). The route builds the JSON input first: path parameters named after the input fields they fill, then the query string for GET and DELETE (top-level fields, `object.field`, lists as repeated or comma-separated values with an empty value as the empty list, `cursor`, `limit`, `sort=field:direction`, the catalogued filters by name) or the JSON object body for POST, PUT and PATCH (no query parameters), then the fixed field (`payouts.transition` at `/approve` sets `action=approve`). Unknown or misplaced parameters are VALIDATION `error.notAllowed`, unreadable values `error.invalid`, a body that repeats a path parameter or the fixed field with another value `error.pathMismatch`. `NewBinding` rejects at start-up a route whose path parameter is not a scalar input field, a GET on a write, a DELETE on a read, or a query-string route of an input with lists of objects, maps or raw JSON.
2. Decode JSON strictly (`DisallowUnknownFields`) into the generated input type; run `Validate()` → VALIDATION with `fieldErrors`.
3. Authorize with the compiled policy → FORBIDDEN, or NOT_FOUND when the target is outside scope (D01). Denied attempts are written to the audit log in a separate short transaction.
4. Writes: require `Idempotency-Key`; insert `platform.idempotency_keys` (in_progress). A completed key with the same request hash returns the stored response; a different hash returns CONFLICT; an in-progress key returns CONFLICT with `retryAfterSeconds`.
5. Run the handler inside `TxManager.WithTx`: writer pool for writes, reader pool and read-only transaction for reads. The transaction starts with `set_config('app.tenant_id', …, true)` and `set_config('app.membership_id', …, true)` so row-level security applies to every statement.
6. The handler returns the result and records events (`call.Emit`) and audit entries (`call.Audit`). Before commit the dispatcher writes the audit rows and outbox rows in the same transaction.
7. Commit; store the response in the idempotency record; return `ServiceResult{data, meta}`.

```go
// internal/platform/db — one transaction per operation with RLS context and retry on serialization failure
func (m *TxManager) WithTx(ctx context.Context, readOnly bool, fn func(pgx.Tx) error) error {
	p := PrincipalFrom(ctx)
	pool, opts := m.Writer, pgx.TxOptions{IsoLevel: pgx.ReadCommitted}
	if readOnly {
		pool, opts = m.Reader, pgx.TxOptions{AccessMode: pgx.ReadOnly}
	}
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		err = pgx.BeginTxFunc(ctx, pool, opts, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true), set_config('app.membership_id', $2, true)",
				p.TenantID.String(), p.MembershipID.String()); err != nil {
				return err
			}
			return fn(tx)
		})
		var pg *pgconn.PgError
		if !(errors.As(err, &pg) && (pg.Code == "40001" || pg.Code == "40P01")) {
			break
		}
	}
	if err != nil {
		var de *DomainError
		if errors.As(err, &de) {
			return de
		}
		return FromPG(err)
	}
	return nil
}
```

```go
// internal/ops — typed registration keeps handlers free of JSON and HTTP concerns
func Register[I any, O any] (r *Registry, name string, mode Mode, pol Policy,
	h func(context.Context, *Call, *I) (O, error)) {
	r.ops[name] = &Operation{
		Name: name, Mode: mode, Authorize: pol,
		NewInput: func() any { return new(I) },
		Handle: func(ctx context.Context, c *Call, in any) (any, error) { return h(ctx, c, in.(*I)) },
	}
}

func NewServer(reg *Registry, authn echo.MiddlewareFunc) *echo.Echo {
	e := echo.New()
	e.HideBanner, e.HidePort = true, true
	e.HTTPErrorHandler = ErrorHandler // DomainError body + fixed status mapping (§6)
	e.Use(echoRecover(), requestID())
	e.GET("/healthz", func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	v1 := e.Group("/v1", authn)
	v1.POST("/ops/:operation", reg.Dispatch)
	return e
}
```

Reads that must see the caller's own write immediately (for example the detail reload after a save) are marked `consistency: writer` in the generated catalog and run on the writer pool; all other reads use the Aurora reader endpoint.

## 5. Handler pattern

Handlers are plain functions; business rules live in `domain/` and are unit-tested without a database.

```go
func RegisterAssets(r *ops.Registry) {
	ops.Register(r, "units.save", ops.Write, authz.Require("asset.write"),
		func(ctx context.Context, c *ops.Call, in *UnitSaveInput) (Unit, error) {
			if in.ID == nil {
				id := uuid.Must(uuid.NewV7())
				tag, err := c.Tx.Exec(ctx, `INSERT INTO assets.units (id, tenant_id, customer_org_id, property_id, space_id, display_name, model_id, capability_version)
					SELECT $1, p.tenant_id, p.customer_org_id, p.id, $3, $4, $5, 1 FROM assets.properties p WHERE p.id = $2 AND NOT p.archived`,
					id, in.PropertyID, in.SpaceID, in.DisplayName, in.ModelID)
				if err != nil {
					return Unit{}, apperr.FromPG(err)
				}
				if tag.RowsAffected() == 0 { // property archived or not visible to this tenant / scope (D01)
					return Unit{}, apperr.E(apperr.NotFound, "error.notFound")
				}
				c.Emit(ops.Event{AggregateType: "unit", AggregateID: id, Type: "UnitChanged"})
				c.Audit(ops.AuditEntry{Action: "units.save", TargetKind: "unit", TargetID: id.String(), NextVersion: ptr(1)})
				return Unit{ID: id, Version: 1}, nil
			}
			var v int
			err := c.Tx.QueryRow(ctx, `UPDATE assets.units SET display_name = $3, space_id = $4, version = version + 1, updated_at = now()
				WHERE id = $1 AND version = $2 AND NOT archived RETURNING version`,
				*in.ID, *in.ExpectedVersion, in.DisplayName, in.SpaceID).Scan(&v)
			if errors.Is(err, pgx.ErrNoRows) {
				return Unit{}, apperr.E(apperr.Conflict, "error.versionConflict")
			}
			if err != nil {
				return Unit{}, apperr.FromPG(err)
			}
			c.Emit(ops.Event{AggregateType: "unit", AggregateID: *in.ID, Type: "UnitChanged"})
			c.Audit(ops.AuditEntry{Action: "units.save", TargetKind: "unit", TargetID: in.ID.String(), PreviousVersion: in.ExpectedVersion, NextVersion: &v})
			return Unit{ID: *in.ID, Version: v}, nil
		})
}
```

Rules every handler follows:

1. **Visibility first.** A target that does not exist or is outside the caller's scope is NOT_FOUND, never FORBIDDEN (D01). Zero affected rows on a scoped insert or update means NOT_FOUND unless a version mismatch is confirmed, which is CONFLICT.
2. **Versions.** Updates use `WHERE id = $1 AND version = $2` and `version = version + 1`; aggregates with child rows lock the parent with `SELECT … FOR UPDATE` first.
3. **State machines.** Status transitions call `domain.<Aggregate>.Transition(from, event)`; illegal transitions are CONFLICT with a message key from the DD tables (for example Command, MaintenanceJob, Restriction in [common design §5](common.md#5-state-transitions-and-consistency)).
4. **Time.** `c.Now` (UTC, from `platform/clock`) is the only clock; tests inject a fixed clock. Pools scan `timestamptz` in UTC so every Instant is serialized with `Z`, independent of the host time zone.
5. **Side effects.** No HTTP calls to AWS or Stripe inside a transaction, except creating a Stripe Checkout Session, which is made idempotent with the operation's key and stored before commit. Everything else is an outbox event handled by a worker.
6. **Audit and events.** Every successful write records exactly one audit entry per changed aggregate and the events listed in backend architecture §8.

## 6. Errors

`DomainError` follows `service-contracts.ts`: `{code, messageKey, fieldErrors, correlationId, retryAfterSeconds}`.

| Code | HTTP | Typical source |
|---|---|---|
| VALIDATION | 422 | Input rules, CHECK violation (23514), malformed JSON, unknown field |
| UNAUTHENTICATED | 401 | Missing or expired token, revoked session |
| FORBIDDEN | 403 | Permission or role missing, HQ user outside the company network, owner-only action by a member |
| NOT_FOUND | 404 | Missing or out-of-scope target, RLS violation (42501), foreign key to an invisible row (23503) |
| CONFLICT | 409 | Version mismatch, unique / exclusion violation (23505 / 23P01), illegal state transition, idempotency key reuse |
| OFFLINE | 409 | Command to an offline device (rejected before sending) |
| TIMEOUT | 504 | Context deadline exceeded |
| RATE_LIMITED | 429 | Per-session write limit, Stripe or provider throttling surfaced to the user |
| UNAVAILABLE | 503 | Retries exhausted (40001 / 40P01), dependency down, panic; `retryAfterSeconds` set when known |

## 7. Workers

All workers share `internal/platform/events` (SNS FIFO publish, SQS long-poll consume, visibility extension, dead-letter after 5 receives) and record consumed event IDs in `platform.processed_events` inside the same transaction as their effect.

| Role (`--role=`) | Input | Work | Notes |
|---|---|---|---|
| outbox | `platform.outbox` | `SELECT … FOR UPDATE SKIP LOCKED LIMIT 100` every 200 ms (and on `LISTEN outbox`), publish to SNS FIFO (MessageGroupId = aggregate ID, deduplication ID = event ID), set `published_at` | Several instances can run; ordering is per aggregate |
| scheduler | `platform.scheduled_items` | Claims due items with a 60 s lease via `FOR UPDATE SKIP LOCKED`, runs the handler for the item kind (backend architecture §10) in a tenant transaction, sets `done_at` | Sweeps that are not item-based (command expiry every 5 s, stale sensors every 60 s) loop over tenants and `SET LOCAL app.tenant_id` per tenant |
| iot | SQS `command-requested`, SQS `device-ack` (IoT rule), SQS `device-event` | Publishes `/cmd` via the IoT data plane (QoS 1) and marks the Command sent; matches acknowledgements by command ID and accepts them only before `expires_at` (late acks recorded); writes device events and connection changes | Never retries a command after `expires_at` |
| telemetry | Kinesis stream (IoT rule) | Shard leases and checkpoints in `platform.stream_checkpoints`; batches of up to 500 records written with `pgx.CopyFrom` into `monitoring.measurements`; quality evaluation; alert policy evaluation → AlertOpened | Raw copies also go to S3 Parquet through a second IoT rule action (Firehose) |
| rollup | timer | Every 5 minutes upserts `measurements_15m` and, hourly, `measurements_1h` for closed buckets (`INSERT … ON CONFLICT DO UPDATE`) | Re-runnable for late data within 35 days |
| automation | EventBridge Scheduler ticks, TelemetryReceived, weather polls | Evaluates schedules and conditions, writes `control.automation_runs` (fired / skipped with reason), creates Commands through the Control module's command service | Same Command policy as the UI path |
| notification | SQS `notification-requested` | Applies preferences, consents and the recipient's scope version, renders en / ms templates, sends via SES or the WhatsApp provider, records `notify.deliveries` | Provider errors are retried with backoff; permanent failures surface on the alert (`delivery_failures`) |
| importexport | SQS `importexport-jobs` | CSV unit import preview / commit / undo (FR-A18), monthly energy report PDF (FR-C16), MRV and audit exports, attachment scan results | Reads and writes S3 through VPC endpoints |

The webhook receiver (`cmd/webhook`) is a separate Echo service: `/stripe` verifies `Stripe-Signature` with a 5-minute tolerance, stores the event in `billing.stripe_events` (primary key = Stripe event ID, so replays are no-ops), retrieves the PaymentIntent, and then runs the billing confirmation in one transaction that emits PaymentConfirmed. It answers 2xx only after the event is stored.

## 8. Security in the code

- Secrets (database credentials via IAM authentication tokens or Secrets Manager, Stripe restricted key, provider keys) are loaded at start-up and refreshed on rotation; they are never logged (the slog handler masks keys named `*secret*`, `*token*`, `*password*`, `authorization`, e-mail and phone values).
- The database role used by the services (`ac_app` / `ac_worker`) has no `BYPASSRLS` and no DDL rights; migrations run with a separate owner role.
- All SQL is parameterised (sqlc); dynamic list filters are built from an allowlist of column names.
- Request bodies are size-limited and strictly decoded; outbound HTTP clients have timeouts and only allowlisted hosts (enforced again by the Network Firewall).
- `govulncheck` and image scanning run in CI; dependency updates weekly.

## 9. Testing strategy

| Level | Tool | Scope |
|---|---|---|
| Domain unit tests | `go test` | State machines, calculations (energy, coverage, emissions), validation, authorization predicates — table-driven, no I/O |
| Store tests | `testcontainers-go` PostgreSQL 16 + migrations | sqlc queries, RLS isolation (a second tenant sees nothing), constraints (overlap, one pending command, append-only audit) |
| API tests | `httptest` + Echo | Status mapping, idempotency replay, version conflicts, HQ-network rule, owner-only rules |
| Contract tests | generated from the catalog | Every operation registered once, mode and authorization match the catalog, inputs reject unknown fields |
| Acceptance tests | `test/acceptance` | AT-* cases from the requirement documents replayed against the API with fixture-contract.json seeds and a fixed clock (the same cases that drive the Phase 1A mock) |
| Integration | LocalStack, Stripe CLI, IoT device simulator | Outbox → SNS → SQS flows, Stripe webhooks, MQTT command round trip in staging |
| Load | k6 | Design capacity in backend architecture §13 |
| Web unit tests | Vitest (`service/web/shared`, `npm run test`; config as in the Next.js docs with the React plugin and jsdom) | The pure mappers of `@ac/web/lib` that the Server Components and client views share (device tiles and events, quality review evidence check, payout lines and PDF, command history, action messages); `npm run typecheck` includes the shared package and its tests (IR219) |

## 10. Build, run, and deploy

- `make gen` (sqlc, operation and contract generators), `make swagger` (Swagger 2.0 from the swag annotations of the handlers, served by the gateway at `/docs` and `/swagger.json`, IR220), `make lint`, `make test`, `make cover` (merged coverage of all packages), `make resetdb` (rebuild the local database from schema.sql and reseed), `make test` / `make test-cluster` (each on a freshly rebuilt test database, `ac_test` / `ac_test_cluster`), `make test-all` (both suites at once, about a minute and a quarter, IR225), `make up` (`docker compose --profile full up`), `make down`.
- Dockerfile: multi-stage `golang:1.25` builder → `gcr.io/distroless/static-debian12:nonroot` runtime, read-only root filesystem, `healthcheck` subcommand, graceful SIGTERM drain ([container design §3](container-design.md#3-dockerfile-standards)).
- One image, several ECS services: `api`, `webhook`, and one service per worker role (independent autoscaling: API on CPU and request count, workers on SQS backlog and Kinesis iterator age).
- Deploy order: migrations (expand) → workers → API → BFF; contract steps of migrations ship one release later (database design §9).
- Health: `/healthz` (process), `/readyz` (database and required AWS clients reachable); ALB health checks use `/readyz`.

## 11. Open items

| ID | Open item |
|---|---|
| OPEN-GO-01 | Final choice of the TypeScript → Go contract generator (output reviewed for union types such as `UnitAction` and `Policy`) |
| OPEN-GO-02 | WhatsApp provider SDK (depends on OPEN-BE-04) |

Additional contracts for current version 0.30.0: Read IR01–139 in the [Re-review Correction Contracts](review-resolution-contracts.md). They take priority over older text on the same topic; follow IR72 for conflict precedence.
