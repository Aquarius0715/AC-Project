---
document_id: DD-BACKEND-GO
version: 0.47.0
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
| Query code | As built (IR363): pgx with the SQL written in each operation function; list filters and sort columns come from allowlists (`paging.OrderBy`, filter maps), and the sort direction is one of two SQL keywords (`paging.Direction`, IR365). sqlc, planned first, is not adopted (§2) |
| Migrations | `cmd/migrate` + `internal/migrate` + `internal/migrations` (IR167, IR174; `migrate bootstrap-admin` with `internal/bootstrap` creates a tenant's first HQ administrator, IR509): embedded `migrations/NNNNNN_name.up.sql` files applied in order under an advisory lock, one transaction each (`-- migrate:no-transaction` for `CONCURRENTLY`), bookkeeping in the golang-migrate compatible `schema_migrations` table; `000001_init` is [db/schema.sql](db/schema.sql) |
| IDs | UUIDv7 from `github.com/google/uuid` (`uuid.NewV7`), time-ordered for index locality |
| Validation | Generated input structs + a `Validate() *DomainError` method per input (rules from the DD documents); `go-playground/validator` only for simple tags |
| Logging / tracing | `log/slog` JSON with a masking handler; OpenTelemetry SDK + `otelecho`, exported through the AWS Distro for OpenTelemetry collector |
| AWS | AWS SDK for Go v2 (SQS, SNS, Kinesis, IoT data plane, IoT, S3, SES, Secrets Manager, KMS) |
| Payments | `stripe-go` (current major version) with webhook signature verification |
| Authentication | Cognito access tokens verified with JWKS (`lestrrat-go/jwx`), cached keys |
| Configuration | Environment variables parsed into a typed struct (`caarlos0/env`); secrets fetched from Secrets Manager at start-up |
| Tests | `go test` against a local PostgreSQL 18 test database (`make testdb`, IR441), `httptest` for Echo, `gofmt` and `go vet`. As built (IR363), `testcontainers-go` and `golangci-lint`, planned first, are not used |
| Runtime | Everything runs in Docker ([container design](container-design.md), IR119): static binaries with `CGO_ENABLED=0` in a distroless image, linux/arm64 on ECS Fargate (Graviton), Docker Compose locally and in CI |

The core request pipeline in §4 and the example handler in §5 were compiled and run against PostgreSQL 16 with [db/schema.sql](db/schema.sql) on 2026-10-07 (create, update with version, stale version → 409 CONFLICT, validation → 422, unknown field → 422, cross-tenant create → 404, HQ user outside the company network → 403, outbox rows written in the same transaction).

## 2. Repository layout

As built (IR363; the first design's plan is listed at the end of this section):

```text
service/api/        (IR174; under service/api since IR179; the Next.js apps are service/web/*, IR178)
  go.mod  Makefile  gen, swagger, seed, test, test-all, test-cluster, cover, resetdb, testdb
  build/            service.Dockerfile (gateway and domain services), worker.Dockerfile, migrate.Dockerfile
  scripts/          resetdb.sh, covermerge.py, swag_annotate.py
  cmd/
    gateway/        Core API entry: forwards each REST route (IR222) to the domain service that owns its operation (IR180)
    identity-api/ equipment-api/ maintenance-api/ billing-api/ energy-api/   the five domain services (Echo)
    worker/         one binary, --role=outbox|scheduler|notification|… (IR166, IR195)
    migrate/  seed/  gen/ (ops: catalog, REST routes, contracts; restdoc: the route list for the swag annotations)
  internal/
    server/         composition root: app.go wiring, http.go Echo + middleware, event consumers, cross-domain wiring, demo
    gateway/        the gateway's route table, the embedded swagger.json and Swagger UI
    ops/            the operation pipeline: registry (catalog_gen.go), REST binding (rest.go), validation, authorization,
                    idempotency, expected version, transaction, events and audit (Call), internal queries between services
    platform/       apperr (DomainError), auth (OIDC / JWT, Principal), authz, db (pgx pools, TxManager, RLS context),
                    events (envelope, publish, replica consumers), paging, blob (file store), democlock, unitscope,
                    logging (slog handler), ratelimit (the write limit, IR375), idp (the identity provider's logout, IR386)
    modules/        one package per business area: identity assets devices control monitoring maintenance billing
                    restrictions energy notify audit summaries voice writes
    scheduler/      the clock-driven transitions per domain (IR195, IR356)
    migrate/ migrations/ seed/
  test/integration/ the Core API against PostgreSQL, run in one process and with the split services
```

**A module.** One file per feature (for example `modules/maintenance/reports.go`) holds:
- the input types, with `Validate()` for the field rules (run before authorization);
- the result types (the JSON of `service-contracts.ts`);
- one function per operation, for example `func (m Reports) saveDraft(ctx context.Context, c *ops.Call, in *DraftInput) (Report, error)`. It checks scope and versions, applies the rules, runs its SQL with pgx in the call's transaction, and records events and audit entries — a transaction script per operation;
- a `Register…` function that binds each catalog name to its function (`ops.Register(r, "jobs.saveDraft", m.saveDraft)`). Mode and authorization come from the generated catalog. A name that is not in the catalog panics at start-up.

**Dependency rules as built:**
1. Each domain service runs with its own database role and owns its schemas. It reads another domain's data only through that domain's internal queries (IR182–IR192) or its `ref_*` copies, and changes it only through events. The single-process build wires the same interfaces locally.
2. A module may import another module's package for its types and query helpers (for example `identity.LoadMembers`). The database roles, not import rules, keep each service in its own schemas.
3. Only `internal/server` builds pools, the Echo instance and clients. Operation functions receive them through `ops.Call` and their module struct.
4. Small interfaces sit in the package that uses them, for example `control.JobVersions`, which maintenance implements.

**Planned in the first design, not built:**
- per-module `domain/` (pure state machines) and `store/` (sqlc) packages, with `db/queries` and `sqlc.yaml`;
- `contracts/types_gen.go`;
- the packages `platform/config`, `httpx`, `idempotency`, `outbox`, `audit` and `clock`. Their work lives in `ops`, `db`, `events` and `server`;
- the webhook receiver (compose's `planned` profile);
- the `test/contract` and `test/acceptance` suites, whose checks the integration and E2E suites carry.

SQL stays inline with pgx. sqlc remains an option if the SQL should be type-checked at build time. pgx reports a query's server error only through `Rows.Err` once `Next` returns false, so rows are read with `pgx.CollectRows` / `pgx.ForEachRow`, or a manual `for rows.Next()` loop is followed by a `rows.Err()` check; `TestRowLoopsCheckErr` (`internal/platform/db`) fails on a loop without one (IR484).

## 3. Code generation from the specification

| Source (docs) | Generator | Output | Check in CI |
|---|---|---|---|
| [operation-catalog.csv](operation-catalog.csv) | `cmd/gen/ops` | `internal/ops/catalog_gen.go`: 210 entries with name, mode, module, design IDs, authorization expression | Every catalog operation has exactly one registered handler; no extra handlers |
| [service-contracts.ts](service-contracts.ts) | `cmd/gen/ops` | The web's `contracts.gen.ts` (IR313). The Go inputs and results are hand-written structs with JSON tags; the planned `types_gen.go` is not built (IR363) | The web's contract tests (IR312, IR314) and the integration tests read the API's JSON against the contract |
| [write-version-catalog.csv](write-version-catalog.csv) | `cmd/gen/ops` | expected-version requirement per write operation | Writes listed as `required` reject a missing version with VALIDATION |
| [fixture-contract.json](../04-agentic-sdlc/fixture-contract.json) | `internal/seed` (`cmd/seed`, `make seed`) | The demo fixture of the integration tests and the local stack | The integration and E2E suites pass on it |

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
| 6 | Authentication | Verifies the Cognito access token (issuer, audience = app client, expiry, `token_use=access`; as built, IR367: Cognito's `token_use=access` or Keycloak's `typ=Bearer`, so an ID or refresh token is UNAUTHENTICATED), loads the selected Membership (`X-Tenant-Id` + `X-Membership-Id` from the BFF session context; the membership must belong to the token subject, be inside its validity window and the user must be active, otherwise UNAUTHENTICATED) with permissions, scopes, client role and validity; requests without a token stay anonymous and reach only `public:` operations; caches it for 30 s keyed by membershipId + scopeVersion. A token whose `auth_time` is later than the user's recorded sign-in sets `last_sign_in_at` to the business time (IR268). As built (IR384): the principal carries the membership's `sessions_revoked_at`, and a token whose `auth_time` is earlier, or that has none, is UNAUTHENTICATED `errors.session_revoked`. Each service compares it with its own token, so the 30-second principal cache of the other services delays a revocation by at most 30 seconds |
| 7 | HQ network | Admin-role principals must carry a token from the admin app client (served only on `admin.<domain>`, IR117); otherwise FORBIDDEN |
| 8 | Dispatcher | The REST route of an operation: `ops.Binding` builds the JSON input from the path, the query string or the body, and the route's fixed field, then the pipeline below runs |

Dispatcher steps for one operation:

1. Look up the operation of the route (an unknown route or method → NOT_FOUND `error.unknownOperation`; an ID path parameter that is not a UUID → NOT_FOUND `error.notFound`, IR223). The route builds the JSON input first: path parameters named after the input fields they fill, then the query string for GET and DELETE (top-level fields, `object.field`, lists as repeated or comma-separated values with an empty value as the empty list, `cursor`, `limit`, `sort=field:direction`, the catalogued filters by name) or the JSON object body for POST, PUT and PATCH (no query parameters), then the fixed field (`payouts.transition` at `/approve` sets `action=approve`). Unknown or misplaced parameters are VALIDATION `error.notAllowed`, unreadable values `error.invalid`, a body that repeats a path parameter or the fixed field with another value `error.pathMismatch`. `NewBinding` rejects at start-up a route whose path parameter is not a scalar input field, a GET on a write, a DELETE on a read, or a query-string route of an input with lists of objects, maps or raw JSON.
2. Decode JSON strictly (`DisallowUnknownFields`) into the generated input type; run `Validate()` → VALIDATION with `fieldErrors`.
3. Authorize with the compiled policy → FORBIDDEN, or NOT_FOUND when the target is outside scope (D01). Denied attempts are written to the audit log in a separate short transaction.
   - As built (IR375): a write then counts against its membership's write limit (`ops.WriteLimiter`, `internal/platform/ratelimit`). The limit is 60 per minute by default, in a fixed window kept in the shared cache (`RATE_LIMIT_URL`), so every instance and service counts together. Over the limit the answer is RATE_LIMITED with `retryAfterSeconds` and `Retry-After`. A cache that does not answer lets the write through, logged.
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

Operation functions are plain functions, registered by name (§2). Each holds its rules and its SQL together. The few rules that need no database are plain helpers with unit tests: campaign waves, payout months, certificate and warranty stages, threshold operators. Every SQL statement runs in at least one integration test (IR311). The example is the shape of `units.save`:

```go
func RegisterAssets(r *ops.Registry) {
	// the mode (write) and the authorization (admin:asset.write) come from the generated catalog
	ops.Register(r, "units.save",
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

1. **Visibility first.** A target that does not exist or is outside the caller's scope is NOT_FOUND, never FORBIDDEN (D01). Zero affected rows on a scoped insert or update means NOT_FOUND unless a version mismatch is confirmed, which is CONFLICT. As built (IR366): the scope comes before the expected version and before any stored result. A lock that a scope check needs only loads the row (`Delivery.load`, `lockDevice`); the handler checks the scope, then the version. A caller outside the scope gets the same refusal for any version and input.
2. **Versions.** Updates use `WHERE id = $1 AND version = $2` and `version = version + 1`; aggregates with child rows lock the parent with `SELECT … FOR UPDATE` first.
3. **State machines.** A status change is an update guarded by the states it may leave (`WHERE status IN (…)`). An illegal or concurrent transition changes no row and answers CONFLICT with a message key from the DD tables (for example Command, MaintenanceJob, Restriction in [common design §5](common.md#5-state-transitions-and-consistency)).
4. **Time.** `c.Now` (UTC) is the only clock: the business clock, which `platform/democlock` shifts in demo runs; tests inject a fixed clock. Pools scan `timestamptz` in UTC so every Instant is serialized with `Z`, independent of the host time zone.
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
| RATE_LIMITED | 429 | Per-session write limit (as built per membership, IR375), Stripe or provider throttling surfaced to the user; `retryAfterSeconds` and `Retry-After` say how long to wait |
| UNAVAILABLE | 503 | Retries exhausted (40001 / 40P01), dependency down, panic; `retryAfterSeconds` set when known |

## 7. Workers

All workers share `internal/platform/events` (SNS FIFO publish, SQS long-poll consume, visibility extension, dead-letter after 5 receives) and record consumed event IDs in `platform.processed_events` inside the same transaction as their effect. As built (IR183, IR427): each consumer polls `platform.outbox` in `seq` order; a failing event is retried 1, 2, 4 and 8 minutes after the attempt before while the events after it wait, and after the fifth failure it is dead-lettered (`platform.event_failures.dead_at`, logged) and skipped. The SNS/SQS relay replaces the polling with the same handler contract.

| Role (`--role=`) | Input | Work | Notes |
|---|---|---|---|
| outbox | `platform.outbox` | `SELECT … FOR UPDATE SKIP LOCKED LIMIT 100` every 200 ms (and on `LISTEN outbox`), publish to SNS FIFO (MessageGroupId = aggregate ID, deduplication ID = event ID), set `published_at`. As built (IR428): the relay is planned; the role keeps the platform tables to their retention every hour (`events.Housekeep`) | Several instances can run; ordering is per aggregate |
| scheduler | the domain's tables (`platform.scheduled_items` reserved) | As built: one sweep per domain every second of business time. Each tenant transaction first takes the advisory lock of its domain and tenant: `pg_try_advisory_xact_lock` in the background loop, which skips a held tenant; `pg_advisory_xact_lock` in a demo clock jump, which waits (IR356). Then come the set-based transitions guarded by the state they leave (backend architecture §10). Item-based scheduling would claim `platform.scheduled_items` with `FOR UPDATE SKIP LOCKED` | The sweeps loop over tenants and `SET LOCAL app.tenant_id` per tenant |
| iot | SQS `command-requested`, SQS `device-ack` (IoT rule), SQS `device-event` | Publishes `/cmd` via the IoT data plane (QoS 1) and marks the Command sent; matches acknowledgements by command ID and accepts them only before `expires_at` (late acks recorded); writes device events and connection changes | Never retries a command after `expires_at` |
| telemetry | Kinesis stream (IoT rule) | Shard leases and checkpoints in `platform.stream_checkpoints`; batches of up to 500 records written with `pgx.CopyFrom` into `monitoring.measurements`; quality evaluation; alert policy evaluation → AlertOpened | Raw copies also go to S3 Parquet through a second IoT rule action (Firehose) |
| rollup | timer | Every 5 minutes upserts `measurements_15m` and, hourly, `measurements_1h` for closed buckets (`INSERT … ON CONFLICT DO UPDATE`) | Re-runnable for late data within 35 days |
| automation | EventBridge Scheduler ticks, TelemetryReceived, weather polls | Evaluates schedules and conditions, writes `control.automation_runs` (fired / skipped with reason), creates Commands through the Control module's command service | Same Command policy as the UI path |
| notification | SQS `notification-requested` | Applies preferences, consents and the recipient's scope version, renders en / ms templates, sends via SES or the WhatsApp provider, records `notify.deliveries`. As built (IR418): every 5 s per tenant it takes the e-mail notifications without a sent delivery with `FOR UPDATE SKIP LOCKED` (a new account's set-up e-mail is the identity provider's, IR461; the invitation queue went with IR479), renders English texts in code, sends over SMTP (`MAIL_SMTP_ADDR`, Mailpit locally), suppresses e-mails to ended memberships and retries a failure up to five attempts, 1, 2, 4 and 8 minutes apart (IR425); the SQS input, SES adapter, ms templates and WhatsApp are planned | Provider errors are retried with backoff; permanent failures surface on the alert (`delivery_failures`) |
| importexport | SQS `importexport-jobs` | CSV unit import preview / commit / undo (FR-A18), monthly energy report PDF (FR-C16), MRV and audit exports, attachment scan results | Reads and writes S3 through VPC endpoints |

As built (2026-10-14): the `scheduler`, `notification` and `outbox` roles run (see their rows). The `iot`, `telemetry`, `rollup`, `automation` and `importexport` roles start and idle in the compose stack: locally the IoT bridge (`cmd/iotbridge`) carries commands, acknowledgements and readings between the broker and the API over HTTP, schedule automations run in the scheduler (IR54), unit imports run inside the API operations, and the 15-minute and hourly rollups are not built yet.

The webhook receiver (`cmd/webhook`, planned: not built yet, compose profile `planned`, IR197) is a separate Echo service: `/stripe` verifies `Stripe-Signature` with a 5-minute tolerance, stores the event in `billing.stripe_events` (primary key = Stripe event ID, so replays are no-ops), retrieves the PaymentIntent, and then runs the billing confirmation in one transaction that emits PaymentConfirmed. It answers 2xx only after the event is stored.

## 8. Security in the code

- Secrets (database credentials via IAM authentication tokens or Secrets Manager, Stripe restricted key, provider keys) are loaded at start-up and refreshed on rotation; they are never logged (the slog handler masks keys named `*secret*`, `*token*`, `*password*`, `authorization`, e-mail and phone values). As built (IR369), `internal/platform/logging` builds every process's logger. It masks the value of a key naming a secret, cookie or authorization, and e-mail addresses, international phone numbers and connection-URL passwords in messages, strings and errors.
- The database role used by the services (`ac_app` / `ac_worker`) has no `BYPASSRLS` and no DDL rights; migrations run with a separate owner role.
- All SQL is parameterised (pgx placeholders); dynamic list filters are built from an allowlist of column names.
- Request bodies are size-limited and strictly decoded; outbound HTTP clients have timeouts and only allowlisted hosts (enforced again by the Network Firewall).
- `govulncheck` and image scanning run in CI; dependency updates weekly.

## 9. Testing strategy

| Level | Tool | Scope |
|---|---|---|
| Unit tests | `go test` | Pure helpers (campaign waves, payout months, reminder stages, thresholds), validation rules that REST cannot reach, authorization predicates, the REST binding — table-driven, no I/O |
| Integration tests | a local PostgreSQL 18 test database (IR441), reset per run (`make testdb`) | The operations with their SQL, RLS isolation (a second tenant sees nothing), constraints (overlap, one pending command, append-only audit), in one process and with the split services. Authorization by target: every operation a client, contractor or technician may call, tried with another party's records and a stale version (`TestForeignTargets`, IR366) |
| API tests | `httptest` + Echo | Status mapping, idempotency replay, version conflicts, HQ-network rule, owner-only rules; every SQL statement of a module runs in at least one test, checked on the coverage profile after `make cover` (IR311) |
| Contract tests | generated from the catalog | Every operation registered once, mode and authorization match the catalog, inputs reject unknown fields; each operation's input type takes exactly the fields of its contract input, and its result type answers exactly its contract result (`TestInputsMatchContract`, IR391, and `TestResultsMatchContract`, IR392: against `service/web/shared/lib/contractInputs.gen.json` and `contractResults.gen.json`, which the web's `contractInputs.test.ts` derives from the contract with the TypeScript checker and keeps current); each operation's input rules answer VALIDATION before authorization (`TestRequiredInputs`, IR307) |
| Acceptance tests | `test/acceptance` | AT-* cases from the requirement documents replayed against the API with fixture-contract.json seeds and a fixed clock (the same cases that drive the Phase 1A mock) |
| Integration | LocalStack, Stripe CLI, IoT device simulator | Outbox → SNS → SQS flows, Stripe webhooks, MQTT command round trip in staging |
| Load | k6 | Design capacity in backend architecture §13 |
| Web unit tests | Vitest (`service/web/shared`, `npm run test`; config as in the Next.js docs with the React plugin and jsdom) | The pure mappers of `@ac/web/lib` that the Server Components and client views share (device tiles and events, quality review evidence check, payout lines and PDF, command history, action messages), the BFF session (signed cookie, refresh, return paths) and the DAL (headers, errors, paging) (IR307); `npm run typecheck` includes the shared package and its tests (IR219) |

## 10. Build, run, and deploy

- `make gen` (the operation and contract generators; it also writes the web's routes.gen.ts and contracts.gen.ts, IR313), `make swagger` (Swagger 2.0 from the swag annotations of the handlers, served by the gateway at `/docs` and `/swagger.json`, IR220; as built, IR371: Swagger UI is one pinned release with integrity hashes, under its own Content-Security-Policy), `make test`, `make cover` (merged coverage of all packages), `make resetdb` (rebuild the local database from schema.sql and reseed), `make test` / `make test-cluster` (each on a freshly rebuilt test database, `ac_test` / `ac_test_cluster`), `make test-all` (both suites at once, about a minute and a quarter, IR225), `make up` (`docker compose --profile full up`), `make down`.
- Dockerfile: multi-stage `golang:1.26.10` builder (IR408, IR503; the module's toolchain) → `gcr.io/distroless/static-debian12:nonroot` runtime pinned by digest, read-only root filesystem, `healthcheck` subcommand, graceful SIGTERM drain ([container design §3](container-design.md#3-dockerfile-standards)).
- One image, several ECS services: `api`, `webhook`, and one service per worker role (independent autoscaling: API on CPU and request count, workers on SQS backlog and Kinesis iterator age).
- Deploy order: migrations (expand) → workers → API → BFF; contract steps of migrations ship one release later (database design §9).
- Health: `/healthz` (process), `/readyz` (database and required AWS clients reachable); ALB health checks use `/readyz`.

## 11. Open items

| ID | Open item |
|---|---|
| OPEN-GO-01 | Final choice of the TypeScript → Go contract generator (output reviewed for union types such as `UnitAction` and `Policy`) |
| OPEN-GO-02 | WhatsApp provider SDK (depends on OPEN-BE-04) |

Additional contracts: every IR in the [Re-review Correction Contracts](review-resolution-contracts.md) applies, the newest included. They take priority over older text on the same topic; follow IR72 for conflict precedence.
