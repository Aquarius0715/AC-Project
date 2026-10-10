---
document_id: DD-BACKEND
version: 0.30.0
status: draft
owner: design-agent
consumers: [implementation-agent, test-agent, review-agent, operations]
scope: production-target-logical
---

# Backend Architecture (production target, AWS)

## 1. Purpose, scope, and status

This document designs the production backend that will replace the Phase 1A mock adapter (NFR-06, [common design §4 and §8](common.md#4-frontend-data-service-boundaries)). It started as a logical, cloud-agnostic design (IR116, DEC-67); §3a maps every component to **AWS**, the hosting provider chosen on 2026-10-07 (IR117, DEC-68). Everything here is `PROPOSED` unless marked `OPEN`.

- Phase 1A scope is unchanged: the clickable demo still runs on the shared mock and makes no real payment, notification, IoT, or trading connection (FR-X05).
- The screens, operations, permissions, and state machines already defined in these documents are the contract the backend implements. The [operation catalog](operation-catalog.csv) (200 operations), [service contracts](service-contracts.ts), [write version catalog](write-version-catalog.csv), and the state transitions in [common design §5](common.md#5-state-transitions-and-consistency) are the source of truth; this document must not redefine them.
- Network zones, ports, and traffic rules are in the [network architecture](network-architecture.md).
- Diagrams: Figma file VOeKPrid46kOf24ktEfe8r, page “System Architecture”, boards 03 (backend), 04 (network), 06 (Go implementation and database), 07 (containers) and 08 (business-domain services and cross-domain contracts as built, IR197).

Decisions applied (DEC-67, DEC-68, DEC-69):

1. The frontend stack is **Next.js (App Router)**, as implemented in the four apps `service/web/{customer,partner,technician,admin}` with the shared package `service/web/shared` (IR178). In production it also acts as the **BFF** (backend for frontend): it serves the four role apps, holds the user session, and relays operations to the Core API. The browser never calls the Core API directly.
2. The backend is a **modular monolith** (one Core API deployable with strict module boundaries) plus **separately deployed workers** for IoT, scheduling, notifications, and exports, so that device traffic and background work scale and fail independently of user requests.
3. Hosting is **AWS** (DEC-68). Primary region Asia Pacific (Malaysia) `ap-southeast-5`; disaster recovery and any service not yet offered in ap-southeast-5 run in Asia Pacific (Singapore) `ap-southeast-1`. Check service availability per region at setup; keep personal data in ap-southeast-5 whenever the service exists there (privacy, §12).
4. Payments use **Stripe** (hosted Stripe Checkout, MYR, cards and FPX online banking as enabled on the account; DEC-68).
5. The HQ admin app is reachable **only from the company network** (office egress addresses and the company VPN); see §6 and the [network architecture §3](network-architecture.md#3-public-endpoints).
6. The AC manufacturer interface and the cellular SIM provider remain `OPEN`; retention periods (§7), capacity (§13), and customer office firewall requirements (network architecture §5a) are set by this document.
7. The backend is written in **Go** with the **Echo** framework (DEC-69); see the [backend Go design](backend-go-design.md), the [database design](database-design.md), and [db/schema.sql](db/schema.sql).

## 2. System context

| Actor or system | Interaction with the platform | Channel |
|---|---|---|
| Client (owner / member) | Monitors and controls own ACs, automations, maintenance requests, payments, users (owner) | Browser → Web/BFF |
| HQ user | Operates customers, devices, alerts, jobs, billing, restrictions, analytics, audit | Browser → Web/BFF |
| Contractor | Accepts offers, assigns own technicians, reviews reports, payouts | Browser → Web/BFF |
| Technician | Works assigned jobs on site, diagnostics, device maintenance | Mobile browser → Web/BFF |
| AC unit with IoT module | Sends telemetry, heartbeats, events, command acknowledgements; receives commands and firmware | MQTT over TLS → IoT gateway |
| Identity provider | Authenticates people (password, reset, two-step verification) | OIDC |
| Stripe | Hosted Stripe Checkout (cards, FPX) and signed result webhooks | HTTPS redirect + webhook |
| E-mail and WhatsApp providers | Deliver notifications, payment guidance, invites, reports | HTTPS API (egress) |
| Weather and location sources | Automation triggers (weather), client location with consent (FR-C05) | HTTPS API / browser |
| Speech service | Optional voice recognition for the assistant (FR-X02) | HTTPS API (egress) |
| Carbon registry / market | Future offset purchase and retirement (FR-A15, FR-C13 remain demo) | HTTPS API (egress), OPEN |

## 3. Runtime components

| Component | Responsibility | Runs as | Scales by | State |
|---|---|---|---|---|
| Edge (CDN, WAF, DDoS protection) | TLS termination for public names, static asset caching, request filtering, rate limits | Managed edge service | Provider | None |
| Web / BFF (Next.js) | Serves the four role apps and shared routes; OIDC login flow; server-side session; relays `/bff/ops/<operation>` (the browser's operation call with its JSON input) to the operation's REST route of the Core API with the user context (IR223); pushes change notifications to browsers (SSE) | Stateless containers behind the load balancer | Requests | Session in cache |
| Core API (modular monolith) | Executes the 200 operations: validation, authorization, state transitions, versions, audit, outbox events | Stateless containers in the app zone | Requests, CPU | Database |
| IoT gateway service | Device registry and credentials, MQTT bridge, command dispatch and acknowledgement matching, heartbeat and connection state, firmware operations | Containers next to the MQTT broker | Connected devices, messages | Database, broker |
| Telemetry processor | Validates and stores measurements (quality valid / missing / stale / suspect), evaluates alert policies, feeds automation conditions; a separate rollup role writes the 15-minute and hourly aggregates | Stream consumers | Messages per second | Time-series store |
| Automation engine | Evaluates schedules and event conditions; arbitrates capability → restriction → HQ policy → customer rule (DEC-09); creates Commands through the same Command policy | Workers | Rules due | Database |
| Scheduler | Time-based business transitions (§10) with exactly-once execution per due item | Several instances claiming items with database leases | Due items | Database |
| Notification worker | Renders en / ms templates, applies preferences and consents, delivers in-app, e-mail, WhatsApp; records delivery state | Queue consumers | Notifications | Database |
| Import / export worker | CSV unit import (FR-A18), monthly energy report (FR-C16), MRV and audit exports, attachment scanning | Queue consumers | Jobs | Object storage |
| Webhook receiver | Receives payment and provider callbacks, verifies signatures, converts them to internal events | Small stateless service | Requests | Outbox |
| Outbox relay | Publishes committed domain events from the database outbox to the message broker | Workers | Event rate | Database, broker |
| Relational database | System of record for all business entities, versions, outbox, audit | Managed, primary + standby, read replica | Storage, IOPS | Persistent |
| Telemetry store | Measurements, heartbeats, aggregates for energy and MRV (Aurora partitioned tables, hot 35 days; raw history as S3 Parquet, §3a) | Managed | Data volume | Persistent |
| Object storage | Photos, attachments, signatures, CSV files, generated reports | Managed, private | Data volume | Persistent |
| Cache | Sessions, idempotency keys, rate-limit counters, short-lived read caches | Managed, replicated | Memory | Ephemeral |
| Message broker | Domain events and work queues (at-least-once) | Managed | Throughput | Persistent queues |
| MQTT broker | Device connections and topics | Managed IoT messaging service | Connections | Retained state, sessions |

## 3a. AWS service mapping

| Component | AWS service (PROPOSED) | Notes |
|---|---|---|
| DNS | Amazon Route 53 | Health-checked failover records for app, admin, hooks, files, mqtt |
| CDN, WAF, DDoS | Amazon CloudFront, AWS WAF, AWS Shield Standard | WAF IP set restricts `admin.<domain>` to the company network |
| Public TLS certificates | AWS Certificate Manager (us-east-1 for CloudFront, ap-southeast-5 for the ALB) | Automatic renewal; device certificates are issued by AWS IoT Core / a private CA instead (network architecture §7) |
| Load balancer | Application Load Balancer (two AZs) | Reachable only from CloudFront (managed prefix list + secret header) |
| Web / BFF, Core API, workers | Amazon ECS on AWS Fargate | One service per component, autoscaling on CPU and queue depth; images in Amazon ECR |
| Identity provider | Amazon Cognito user pools | OIDC, TOTP MFA, password reset; one pool per environment |
| Relational database | Amazon Aurora PostgreSQL (Multi-AZ) | RLS by tenant, PITR 35 days, cross-region snapshot copy to ap-southeast-1 |
| Time-series data | Aurora PostgreSQL partitioned tables (hot 35 days) + Amazon S3 Parquet via Amazon Data Firehose, queried with Amazon Athena | Moves to a dedicated time-series store if the fleet outgrows the §13 design capacity |
| Object storage | Amazon S3 (versioning, SSE-KMS, Block Public Access) | Pre-signed URLs; GuardDuty Malware Protection for S3 scans uploads |
| Cache | Amazon ElastiCache (Valkey / Redis OSS compatible) | Sessions, idempotency keys, rate limits |
| Domain events and queues | Amazon SNS FIFO → Amazon SQS FIFO (message group = aggregate ID), dead-letter queues | Outbox relay publishes; consumers idempotent |
| MQTT and device management | AWS IoT Core (X.509 certificates, IoT policies, custom domain), IoT rules → Amazon Kinesis Data Streams, AWS IoT Jobs for firmware | Configurable custom endpoint `mqtt.<domain>` |
| Scheduler triggers | Amazon EventBridge Scheduler → ECS task / SQS | Due items still claimed with database leases (§10) |
| E-mail | Amazon SES | SPF, DKIM, DMARC; bounce and complaint events via SNS |
| Speech (optional) | Amazon Transcribe | Only if en / ms languages meet the assistant's needs |
| Secrets and keys | AWS Secrets Manager, AWS KMS | Automatic rotation; separate keys per environment |
| Observability | Amazon CloudWatch, AWS X-Ray / AWS Distro for OpenTelemetry | Logs 90 days hot, archived to S3 |
| Security and audit of the platform | AWS CloudTrail, Amazon GuardDuty, AWS Security Hub, AWS Config | Organization-wide, delegated security account |
| Accounts | AWS Organizations: management, security / log archive, shared network, dev, staging, prod | Service control policies limit regions to ap-southeast-5 and ap-southeast-1 |
| Delivery | GitHub Actions (OIDC to AWS) or AWS CodePipeline, AWS CDK / Terraform | Signed images; no long-lived access keys |

## 4. Core API modules

Each module owns its tables and is reached by other modules only through its public interface or domain events; no module reads another module's tables. The grouping follows the operation prefixes in the operation catalog.

| Module | Operation groups | Owns (main entities) | Main events published |
|---|---|---|---|
| Identity & access | session, demoSession (replaced by real sign-in), members (list, save), organizations, twoFactor, preferences, consents, clientUsers, auth | User, Membership, Permission grants, QualificationGrant, Preferences, Consent, ClientUser invite | MembershipChanged, ConsentChanged |
| Assets | customers, properties, spaces, locations, units | Organization, Customer, Property, Space, ACUnit, import batches | UnitChanged, LocationChanged |
| Devices | capabilities, devices, firmwareCampaigns | Capability, Device, Sensor, Calibration, Firmware operation and campaign | DeviceConnectionChanged, DeviceEventRaised, FirmwareOperationChanged |
| Control | commands, diagnosticRuns, automations, voice | Command, DiagnosticRun, Automation, Trigger log | CommandRequested, CommandSettled |
| Monitoring & alerts | telemetry, alerts, policies, ventilation, energy (read side of measurements) | Alert, AlertPolicy, DefaultRuleSetting, Ventilation log | AlertOpened, AlertResolved |
| Maintenance | jobs, plans, reports, attachments, parts, members (capacity, eligible, setUnavailability), certificates, contractors, rateCards, sla, filterCare | MaintenanceJob, Offer, Assignment, SlotProposal, Plan, WorkReport, Attachment, Certificate, Contractor, RateCard | JobStatusChanged, OfferIssued, ReportSubmitted |
| Billing | contracts, invoices, payments, inquiries, payouts | Contract, Invoice, PaymentAttempt, Receipt, Inquiry, PayoutStatement | InvoiceIssued, PaymentConfirmed, PayoutChanged |
| Restrictions | restrictions | Restriction, exception, release intent | RestrictionScheduled, RestrictionApplied, RestrictionReleaseRequested |
| Energy & carbon | energy, baselines, mrv, factors, offsets | Baseline, MRV report version, Emission factor, Offset record | MrvReportChanged |
| Notifications | notifications | Notification, delivery attempt, template | NotificationRequested |
| Audit | audit, writes | Audit record (append-only), write receipt | AuditRecorded |
| Read models | admin.summary, summaries | No tables; composed from the other modules' query interfaces | — |
| Demo (non-production) | demo, admin.summary test hooks | Demo clock, triggers | Not deployed to production |

The [operation persistence map](operation-persistence-map.csv) lists, for each of the 200 operations, the owning module, the tables it reads and writes in its own schema, the other modules it calls through their interfaces, and the write side effects (audit row, idempotency record, outbox event); it is generated by `docs/tools/gen_operation_persistence_map.py` from the operation catalog and `db/schema.sql`.

Cross-module rules that the modules must keep:

- One Command policy for every path (UI, voice, automation, restriction release): capabilities, restrictions and exceptions, online state, permission, and job requirements are checked in the Control module only (common design §5).
- Restriction release after payment (IR35) is an event chain: PaymentConfirmed → Restrictions sets release_requested → Control sends release Commands → the restriction is released only after the device observation confirms it.
- Maintenance job, offer, assignment, and slot rules follow IR113; follow-up classification follows IR114.

## 5. API contract between Web/BFF and Core API

| Topic | Contract |
|---|---|
| Transport | HTTPS with JSON on the internal network, REST routes per operation (IR222): the `rest_routes` column of the [operation catalog](operation-catalog.csv) gives each operation its routes — reads `GET /v1/<resource>[/{id}]` with the input in the path and the query string, creates `POST /v1/<resource>`, updates `PUT /v1/<resource>/{id}`, deletes `DELETE /v1/<resource>/{id}`, business commands `POST /v1/<resource>/{id}/<verb>` (for example `POST /v1/jobs/{jobId}/assign`); path parameters are named after the input fields they fill. Operation names, inputs, and results stay exactly those in the operation catalog and service contracts — the operation name is the ID in authorization, audit and `meta.operation` — so the frontend adapter maps one call to one operation and builds its route from the catalog. The web apps' data access layer and BFF relay build each request from the catalog's routes (`routes.gen.ts`, IR223). |
| Caller context | The BFF sends a short-lived signed service token plus the user context (userId, membershipId, tenantId, scopeVersion, sessionId). The Core API re-reads the Membership and never trusts permissions sent by the caller. |
| Results | Success returns `ServiceResult<T>`; failure returns a DomainError body with code and fields. HTTP status mapping: VALIDATION 422, UNAUTHENTICATED 401, FORBIDDEN 403, NOT_FOUND 404, CONFLICT 409, OFFLINE 409, TIMEOUT 504, RATE_LIMITED 429, UNAVAILABLE 503 (retryable) — all nine `ErrorCode` values ([backend Go design §6](backend-go-design.md#6-errors)). Out-of-scope reads return NOT_FOUND, never partial data (D01). |
| Writes and duplicates | Every write carries an `Idempotency-Key` (the frontend's duplicate-prevention key) kept for 24 hours; a repeat returns the original receipt (`writes.getResult`). |
| Versions | `WriteOptions.expectedVersion` travels in the `X-Expected-Version` header (non-negative integer; 0 names a row that does not exist yet, IR120); `WriteOptions.idempotencyKey` in `Idempotency-Key`. The Core API applies the write version catalog branch that matches the input (`all`, `<field> present`, `<field> omitted`, `event=a\|b`): a `required` branch without the header, an `omit` branch with it, or a non-integer value is VALIDATION before the handler runs; a mismatch with the stored version returns CONFLICT and changes nothing. |
| Lists | Cursor pagination (`cursor`, `limit`), stable sort with ID tiebreak (`sort=field:direction`), the catalogued filters as query parameters by name (lists repeat the parameter or separate the values with commas; an empty value is the empty list). |
| Correlation | Every request carries a correlation ID that is written to logs, traces, events, and audit records (`AuditView.correlationId`). |
| Change notification | The BFF keeps one Server-Sent Events stream per signed-in tab. Core events are filtered by tenant and scope and sent as resource keys; the browser invalidates the matching queries (replaces the mock event bus, IR71). |
| Files | Uploads and downloads use short-lived pre-signed object storage URLs issued by the Core API after authorization; files are scanned before they become visible. |
| Versioning of the API | Additive changes only within `/v1`; breaking changes need a new operation name or `/v2`. |

## 6. Authentication, sessions, and authorization

| Topic | Design |
|---|---|
| Sign-in | OIDC authorization code flow with PKCE against Amazon Cognito, performed by the BFF. Tokens stay on the server; the browser holds only an httpOnly, Secure, SameSite=Lax session cookie. CSRF protection on all BFF write routes. The Core API records the access token's `auth_time` as the user's last sign-in (IR268). |
| Session lifetime | Idle lifetime 30 minutes with a warning 120 seconds before expiry and explicit extend (same behaviour as IR55); absolute lifetime 12 hours (PROPOSED). Sign-out revokes the server session and IdP refresh token. |
| Two-step verification | TOTP in Amazon Cognito (FR-X08 becomes real). Required for every HQ user (PROPOSED). |
| HQ network restriction | HQ users (role admin) sign in and work only through `admin.<domain>`, which AWS WAF allows only from the company network (office egress addresses and the company VPN). The BFF issues an admin session only on that host and marks it `channel=hq-network`; the Core API rejects admin-role operations without that mark (FORBIDDEN, shared Page unavailable). `/admin` routes on `app.<domain>` return the shared Page unavailable view. Contractors, technicians, and clients are not restricted. |
| Password reset | Handled by the identity provider; the response never reveals whether an account exists (FR-X01). |
| Role and membership switch | One person may hold several Memberships; switching re-issues the session context, increments viewEpoch, and clears client caches (as in the demo). |
| Authorization | Evaluated in the Core API for every operation using the `authorization` column of the operation catalog: Membership.permissions (38 values, IR107), role, tenant, scope (customer / property / unit / job assignment), validity period, qualifications, and client role (owner-only operations such as `policies.setDefaultRule`, IR115). |
| Tenant isolation | Every business row carries tenantId; database row-level security enforces it as a second line of defence behind module checks. |
| Devices | Each IoT module has its own X.509 certificate issued at provisioning; the MQTT broker allows it to publish and subscribe only to its own topics. Revocation on unbind or tamper. |
| Services | Workload identities for service-to-service calls; secrets from a managed secret store, rotated automatically; no secrets in images, logs, or URLs (NFR-03). |

## 7. Data architecture

| Store | Data | Retention (PROPOSED) | Backup and recovery |
|---|---|---|---|
| Relational database | All business entities and versions, outbox, idempotency receipts (24 h), audit | Contracts, invoices, payments, payouts: 7 years after the end of the financial year (Malaysian tax and company record keeping); other business data: contract life + 7 years; idempotency receipts 24 hours | Point-in-time recovery (35 days), RPO 15 minutes; standby in a second zone; daily snapshot copied to ap-southeast-1 |
| Audit (relational, append-only) | AuditView records with masked before / after | 7 years; monthly export to S3 Object Lock (compliance mode) | Same as database + immutable export |
| Time-series data | Raw measurements (1-minute heartbeat cadence), connection events | Raw: 35 days hot in Aurora, 13 months in S3 Parquet; 15-minute aggregates 3 years; hourly aggregates 7 years (energy and MRV evidence) | Aurora PITR for hot data; S3 versioning + replication |
| Object storage | Photos, attachments, signatures, CSV files, reports, MRV exports | Job evidence and signatures 7 years after job completion; generated reports 7 years; CSV imports 90 days; unconfirmed uploads 24 hours | Versioning, SSE-KMS, cross-region replication to ap-southeast-1 |
| Notifications and delivery logs | In-app notifications, delivery attempts | 2 years | With the database |
| Application logs and traces | Masked structured logs, traces | 90 days hot, 1 year archive | CloudWatch → S3 |
| Cache | Sessions, rate limits, idempotency fast path | Minutes to hours | Rebuilt on failure (users sign in again) |

Personal data of closed accounts (name, e-mail, phone) is anonymised 30 days after closure unless a record above still needs it; consent records are kept for the retention of the data they cover. Confirm the periods with legal counsel before go-live.

Modelling rules:

- Times are stored in UTC; display uses the user's time zone (default Asia/Kuala_Lumpur) and locale en / ms (NFR-08).
- Money is stored in minor units with an ISO currency code (MYR by default); totals are never converted between currencies (FR-A01).
- Measurements keep observedAt, receivedAt, origin, quality, and the raw unit; missing data is never stored as 0 (common design §7).
- Every write increments the entity version and writes one outbox event in the same transaction.

## 8. Domain events and asynchronous processing

- **Outbox pattern**: a module writes its state change and an outbox row in one database transaction; the outbox relay publishes the event. Consumers are idempotent (event ID), delivery is at-least-once, and ordering is guaranteed per aggregate key (for example unitId, jobId, invoiceId).
- **Data between the domain services** (IR186–IR190): a service never reads another domain's tables. It keeps reference copies fed by change-capture events (`platform.capture_row` → `RowChanged:<schema>.<table>`, `events.Replica`) when it needs the owner's rows, and asks the owner's internal queries (`/internal/v1/queries/<name>`, API composition) when it needs the owner's rules, as the read models do.
- **Main event flows**

| Event | Producer | Consumers |
|---|---|---|
| CommandRequested | Control | IoT gateway (dispatch), audit |
| CommandSettled (acknowledged / failed / expired / late) | IoT gateway → Control | Units read model, notifications, restrictions (release observation), SSE |
| TelemetryReceived | IoT gateway | Telemetry processor (store, quality, alerts, automation conditions) |
| AlertOpened / AlertResolved | Monitoring & alerts | Notifications, maintenance (job suggestion), SSE |
| JobStatusChanged / OfferIssued / ReportSubmitted | Maintenance | Notifications, SLA, payouts, SSE |
| InvoiceIssued / PaymentConfirmed | Billing | Notifications, restrictions (IR35 release), SSE |
| RestrictionApplied / RestrictionReleaseRequested | Restrictions | Control (restriction Commands), notifications, SSE |
| NotificationRequested | Any module | Notification worker |

- **Dead letters**: after bounded retries with backoff, events go to a dead-letter queue with an alert to operations; replays reuse the event ID.

## 9. IoT design

| Topic | Design |
|---|---|
| Device model | One IoT module per AC (or one gateway per site for several ACs) bound to an ACUnit; capabilities come from the Capability register (FR-A04, FR-X06). Vendor protocol to the indoor unit is local to the module (OPEN, depends on the AC manufacturer). |
| Provisioning | Register serial (HQ or technician, FR-T11) → claim with a one-time code or QR → issue the device certificate → bind to the unit with a reason (rebind keeps history). |
| Topics | `t/<tenantId>/d/<deviceId>/telemetry`, `/heartbeat`, `/event` (up); `/cmd` (down); `/cmd/ack` (up); `/fw` (down) and `/fw/status` (up). QoS 1; commands are not retained. |
| Telemetry | Batched measurements with sensorId, metric, value, unit, observedAt, sequence. Heartbeat every 60 seconds; a value older than 120 seconds is stale; AC offline alert after 15 minutes without heartbeat (default policy rule). |
| Commands | Control creates the Command (requested) with expiresAt → IoT gateway publishes (sent) → the device acknowledges with the observed state → acknowledged only if received before expiresAt; otherwise expired, and a late acknowledgement is recorded as a late event without changing the result (common design §5). Commands to offline devices are rejected before sending. |
| Restrictions | Restriction and release are sent as Commands with policy versions; the device reports the applied restriction (ObservedRestriction). Release is shown as released only after that observation (IR35). |
| Firmware | Campaigns stage devices in waves (FR-A20); each device operation moves queued → running → succeeded / failed with checksum verification and rollback to the previous version; control actions are rejected while an update runs. |
| Device security | Mutual TLS, per-device certificates, topic-level access policy, tamper and power-loss events raised as device events (FR-T12). |
| Offline behaviour | Modules buffer telemetry while offline and upload with original observedAt; commands are never queued on the device beyond expiresAt. |

## 10. Scheduled business transitions

| Scheduled item | Trigger | Rule source |
|---|---|---|
| Restriction execution | executeAfter reached (≥ 24 hours after scheduling) | FR-A09, IR05 |
| Grace and exception expiry | Exception until reached | FR-A10 |
| Periodic plan job generation | Plan cadence | FR-A06 (`plans.generateNext`) |
| Slot proposal reply deadline | replyBy reached | IR113 |
| Offer expiry and re-offer | Offer deadline | FR-A06, FR-P02 |
| Work window end warning and discard | scheduledEnd − 15 minutes / scheduledEnd | IR89 |
| Auto-confirm completed jobs | 7 days after completion without rating | FR-C17 (BR-C17) |
| Follow-up classification reminder | createdAt + 1 business day | IR114 |
| Invoice reminders | Due and overdue dates | FR-A08 |
| Certificate expiry warnings | Status `expiring` from expiry − 60 days, `expired` at expiry | FR-P09, FR-A21 |
| Filter cleaning reminders | Every 15 minutes of business time: an overdue unit once per cleaning cycle (customer settings) | FR-C18, IR239 |
| Automation schedules | Schedule start / end | FR-C04 |
| Monthly energy report | Month end, when enabled in preferences | FR-C16 |
| Firmware campaign waves | Campaign schedule | FR-A20 |

The scheduler claims due items with a database lease (`FOR UPDATE SKIP LOCKED`) so that each item runs once even with several instances; handlers are idempotent and audited with actorRoleAtTime=system.

## 11. External integrations

| Integration | Direction | Pattern | Security |
|---|---|---|---|
| Identity provider | BFF ↔ IdP | OIDC code + PKCE, back-channel logout | TLS, client secret or private-key JWT |
| Stripe | Browser → Stripe Checkout; Stripe → webhook receiver; Billing → Stripe API | `payments.simulate` becomes a Checkout Session (currency MYR, invoice ID in metadata, Stripe idempotency key = our write key). `checkout.session.completed` / `payment_intent.succeeded` / `payment_intent.payment_failed` webhooks are verified (Stripe-Signature, 5-minute tolerance), then the PaymentIntent is retrieved before PaymentConfirmed (IR35 release). Manual payments stay `payments.recordManual`. Contractor payouts stay outside Stripe until decided (Stripe Connect is an option) | Card data never touches the platform (PCI DSS SAQ A); restricted API keys in Secrets Manager; webhook endpoint `hooks.<domain>/stripe` |
| E-mail (Amazon SES) | Notification worker → SES | Templated send; bounce and complaint events via SNS | IAM role, SPF / DKIM / DMARC on the sending domain |
| WhatsApp provider | Notification worker → provider | Business messaging API with approved templates; status webhooks | Provider credentials, signed webhooks |
| Weather source | Automation engine → provider | Polling per service area | API key, egress allowlist |
| Speech service (optional) | BFF → provider | Short audio upload or browser speech API; intents resolved by `voice.resolveIntent` | Microphone consent, no audio stored by default |
| Carbon registry (future) | Energy & carbon → registry | OPEN | OPEN |

## 12. Cross-cutting requirements

- **Observability**: structured JSON logs without secrets or personal contact data (masked), metrics, and distributed traces keyed by correlation ID; dashboards for API latency, command success and expiry, device connectivity, queue lag, webhook failures.
- **Security baseline**: OWASP ASVS level 2 as the target for the web and API; dependency and container scanning in CI; least-privilege access to data stores; encryption in transit (TLS 1.2+) and at rest; periodic penetration tests before go-live.
- **Privacy**: Malaysian Personal Data Protection Act obligations (notice, consent, access and correction, retention); location consent and withdrawal (FR-C05) are stored with timestamps; personal data is stored in ap-southeast-5 (Malaysia); only services unavailable there and DR copies use ap-southeast-1, with a transfer notice in the privacy policy.
- **Auditing**: every write and every denied attempt produces an audit record with actor, role at the time, target, versions, result, reason, and correlation ID (FR-X07, FR-A16).
- **Internationalisation**: notification templates and generated reports exist in en and ms; times are rendered in the recipient's time zone.

## 13. Non-functional targets (PROPOSED)

| Quality | Target |
|---|---|
| Availability | Web/BFF and Core API 99.5% monthly; IoT ingestion 99.9% (device buffering covers short gaps) |
| API latency | Reads p95 ≤ 300 ms, writes p95 ≤ 600 ms inside the region, excluding device round trips |
| Command round trip | p95 ≤ 5 seconds from sent to acknowledged for online devices |
| Telemetry freshness | p95 ≤ 10 seconds from device send to screen update |
| Capacity | Launch: 3,000 units, 600 customers, 300 concurrent users. Design capacity (3 years): 20,000 units, 4,000 customers, 1,500 concurrent users at peak; 20,000 telemetry batches per minute (≈ 330 per second, about 120,000 measurements per minute), 50 commands per second peak, ≈ 30 GB of hot raw telemetry and ≈ 1.5 TB of S3 Parquet per year. Beyond 20,000 units, move hot telemetry to a dedicated time-series store |
| Recovery | RPO 15 minutes, RTO 4 hours for a region failure (manual failover to the DR region) |

## 14. Environments and delivery

| Environment | Data | Integrations |
|---|---|---|
| Local / demo | Fixture seed (fixture-contract.json) | Docker Compose: `demo` profile = Phase 1A mock; `full` profile = all services with local stand-ins for AWS ([container design](container-design.md)) |
| Development | Synthetic data | Provider sandboxes, device simulator |
| Staging | Production-like synthetic data | Provider sandboxes, a small fleet of test devices |
| Production | Real data | Live providers and devices |

- Everything runs in Docker (IR119): the same images run in Compose locally and in CI and on ECS Fargate in staging and production; infrastructure as code for every environment; immutable image digests promoted from dev to staging to production.
- CI runs type checks, lint, unit, contract, and end-to-end tests (NFR-07), database migration checks, and security scans; production deploys use rolling or blue-green releases with automatic rollback on health checks.
- Database migrations are backward compatible for one release (expand, migrate, contract).

## 15. Migration from the Phase 1A mock

1. Keep screens and the operation interface; add a Core API adapter in the frontend that calls `/bff/ops/<operation>` and maps DomainError codes unchanged.
2. Replace the mock Repository per module, starting with read-only operations (assets, units, telemetry), then writes with versions and idempotency.
3. Replace the mock event bus with the BFF SSE stream; keep the IR71 invalidation lists.
4. Replace simulators with provider sandboxes and the device simulator in development and staging; the scenario clock remains only in the demo environment.
5. Keep the demo environment with the mock adapter for sales and training; production never includes demo operations.

## 16. Open items

| ID | Open item | Needed for |
|---|---|---|
| OPEN-BE-01 | Decided 2026-10-07: AWS, ap-southeast-5 primary, ap-southeast-1 DR (DEC-68) | — |
| OPEN-BE-02 | AC manufacturer interface and IoT module hardware | IoT design, capabilities |
| OPEN-BE-03 | Decided 2026-10-07: Stripe Checkout (DEC-68); enabled methods to confirm in the Stripe account | — |
| OPEN-BE-04 | WhatsApp business provider and template approval | Notifications |
| OPEN-BE-05 | Decided 2026-10-07: retention in §7 (confirm with legal counsel before go-live) | — |
| OPEN-BE-06 | Decided 2026-10-07: capacity in §13 | — |
| OPEN-BE-07 | Contractor payout rail (bank transfer file or Stripe Connect) | Payouts (FR-A23) |

Additional contracts for current version 0.30.0: Read IR01–139 in the [Re-review Correction Contracts](review-resolution-contracts.md). They take priority over older text on the same topic; follow IR72 for conflict precedence.
